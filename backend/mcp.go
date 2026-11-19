package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Project-to-project questions. Every Claude session started here gets a tiny MCP server
// ("projects") with two tools: list_projects and ask_project. ask_project runs a one-shot,
// read-only `claude -p` inside the other project's checkout and returns the answer, so e.g. the
// agent consuming an API can ask the agent building it. Each session may ask a limited number of
// questions (ASK_LIMIT, default 10) so two agents can't talk each other into a loop; restarting
// the session resets the count. The answering run has no MCP config, so it can't ask further.

const askTimeout = 5 * time.Minute

func askLimit() int {
	if n, err := strconv.Atoi(env("ASK_LIMIT", "10")); err == nil && n > 0 {
		return n
	}
	return 10
}

type askSess struct {
	mu         sync.Mutex
	callerKey  string // session key: "p<id>" or "general"
	callerName string
	asked      int
	limit      int
	tasks      int // task requests made this session
}

var (
	askSessions = map[string]*askSess{} // bearer token -> session
	askMu       sync.Mutex
	askBusy     = map[int64]*sync.Mutex{} // one question at a time per target project
)

func registerAsk(key string) string {
	name := "General"
	if strings.HasPrefix(key, "p") {
		if p, err := getProject(key[1:]); err == nil {
			name = p.Name
		}
	}
	b := make([]byte, 24)
	rand.Read(b)
	tok := hex.EncodeToString(b)
	askMu.Lock()
	askSessions[tok] = &askSess{callerKey: key, callerName: name, limit: askLimit()}
	askMu.Unlock()
	return tok
}

func unregisterAsk(tok string) {
	if tok == "" {
		return
	}
	askMu.Lock()
	delete(askSessions, tok)
	askMu.Unlock()
}

// withMCP prepends the flags that give a session access to the project-questions tools.
func withMCP(key string, args []string) ([]string, string) {
	if key == "login" {
		return args, ""
	}
	tok := registerAsk(key)
	_, port, err := net.SplitHostPort(env("ADDR", "127.0.0.1:3001"))
	if err != nil {
		port = "3001"
	}
	cfg, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"projects": map[string]any{
		"type":    "http",
		"url":     "http://127.0.0.1:" + port + "/mcp",
		"headers": map[string]string{"Authorization": "Bearer " + tok},
	}}})
	// Pre-approve the server's tools so Claude doesn't prompt on every question.
	// "=" forms: both flags are variadic and would otherwise swallow a following positional prompt.
	pre := []string{"--mcp-config=" + string(cfg), "--allowedTools=mcp__projects"}
	return append(pre, args...), tok
}

type rpcReq struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// handleMCP speaks the minimal MCP "streamable HTTP" transport: one JSON-RPC message per POST,
// answered with a single JSON response. It is authenticated by the per-session bearer token.
func handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	askMu.Lock()
	s := askSessions[tok]
	askMu.Unlock()
	if tok == "" || s == nil {
		writeErr(w, http.StatusUnauthorized, "Unknown session")
		return
	}
	var req rpcReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad JSON-RPC message")
		return
	}
	if len(req.ID) == 0 || string(req.ID) == "null" { // notification: nothing to answer
		w.WriteHeader(http.StatusAccepted)
		return
	}
	reply := func(result any) {
		writeJSON(w, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2025-03-26"
		}
		reply(map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "project-developer", "version": "1"},
		})
	case "ping":
		reply(map[string]any{})
	case "tools/list":
		reply(map[string]any{"tools": []any{
			map[string]any{
				"name":        "list_projects",
				"description": "List the other projects in Project Developer that you can ask questions.",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			},
			map[string]any{
				"name": "request_task",
				"description": fmt.Sprintf("Ask the user to have another project's agent do some work (e.g. add an endpoint your project needs). It does NOT run anything: the request is queued, and only if the user approves it is it typed into that project's own session. You get no result back, so describe the task completely and self-contained, then carry on and tell the user you requested it. At most %d requests per session; use ask_project for questions.", taskLimit()),
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project": map[string]any{"type": "string", "description": "Project name (see list_projects)"},
						"task":    map[string]any{"type": "string", "description": "What the other agent should do, with all needed context and acceptance criteria"},
					},
					"required": []string{"project", "task"},
				},
			},
			map[string]any{
				"name": "ask_project",
				"description": fmt.Sprintf("Ask the Claude agent of another project a question (e.g. how its API behaves, what a field means, whether an endpoint exists). "+
					"It answers read-only from that project's code and docs and cannot change anything. Be specific and self-contained. "+
					"You may ask at most %d questions per session, so batch related questions and prefer reading shared files yourself when you can.", s.limit),
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project":  map[string]any{"type": "string", "description": "Project name (see list_projects)"},
						"question": map[string]any{"type": "string", "description": "The question, with enough context to answer without this conversation"},
					},
					"required": []string{"project", "question"},
				},
			},
		}})
	case "tools/call":
		var p struct {
			Name      string `json:"name"`
			Arguments struct {
				Project  string `json:"project"`
				Question string `json:"question"`
				Task     string `json:"task"`
			} `json:"arguments"`
		}
		if json.Unmarshal(req.Params, &p) != nil {
			writeJSON(w, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32602, "message": "Invalid params"}})
			return
		}
		var text string
		var err error
		switch p.Name {
		case "list_projects":
			text = listOtherProjects(s)
		case "ask_project":
			text, err = askProject(r.Context(), s, p.Arguments.Project, p.Arguments.Question)
		case "request_task":
			text, err = requestTask(s, p.Arguments.Project, p.Arguments.Task)
		default:
			err = errors.New("Unknown tool " + p.Name)
		}
		if err != nil {
			text = err.Error()
		}
		reply(map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "isError": err != nil})
	default:
		writeJSON(w, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "Method not found"}})
	}
}

func listOtherProjects(s *askSess) string {
	var b strings.Builder
	for _, p := range allProjects() {
		if "p"+strconv.FormatInt(p.ID, 10) == s.callerKey {
			continue
		}
		fmt.Fprintf(&b, "- %s (%s)\n", p.Name, p.RepoURL)
	}
	if b.Len() == 0 {
		return "There are no other projects."
	}
	return b.String()
}

func askProject(ctx context.Context, s *askSess, name, question string) (string, error) {
	name, question = strings.TrimSpace(name), strings.TrimSpace(question)
	if name == "" || question == "" {
		return "", errors.New("Both project and question are required.")
	}
	target := findProject(name)
	if target == nil {
		return "", fmt.Errorf("No project named %q. Use list_projects to see the options.", name)
	}
	if "p"+strconv.FormatInt(target.ID, 10) == s.callerKey {
		return "", errors.New("That is your own project — answer it yourself.")
	}

	s.mu.Lock()
	if s.asked >= s.limit {
		s.mu.Unlock()
		return "", fmt.Errorf("Question limit reached (%d per session). Don't ask more; tell the user what you still need to know. Restarting the session resets the limit.", s.limit)
	}
	s.asked++
	left := s.limit - s.asked
	s.mu.Unlock()

	askMu.Lock()
	lock := askBusy[target.ID]
	if lock == nil {
		lock = &sync.Mutex{}
		askBusy[target.ID] = lock
	}
	askMu.Unlock()
	lock.Lock()
	defer lock.Unlock()

	out, err := runAsk(ctx, *target, s.callerName, question)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s\n\n(Answer from %s's agent. %d question(s) left this session.)", out, target.Name, left), nil
}

// runAsk answers one question with a fresh, read-only Claude run inside the target's checkout.
func runAsk(parent context.Context, target project, callerName, question string) (string, error) {
	bin, ok := findClaude()
	if !ok {
		return "", errors.New("Claude Code CLI not found on the server.")
	}
	ctx, cancel := context.WithTimeout(parent, askTimeout)
	defer cancel()
	sys := "You are answering a question from the coding agent of another project (" + callerName + ") that works together with this one (" + target.Name + "). " +
		"Answer from the code and docs in this repository. You are read-only: never modify files or run commands. " +
		"Be concrete and concise (file paths, endpoints, request/response shapes). If something isn't implemented or you can't tell, say so instead of guessing."
	args := []string{"-p", "--append-system-prompt", sys,
		"--allowedTools=Read,Grep,Glob",
		"--disallowedTools=Bash,Edit,Write,NotebookEdit,WebFetch,WebSearch"}
	if target.Model != "" {
		args = append(args, "--model", target.Model)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = target.Dir
	cmd.Stdin = strings.NewReader("Question from the " + callerName + " project:\n\n" + question)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("%s's agent didn't answer within %s.", target.Name, askTimeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return "", fmt.Errorf("%s's agent failed: %v %s", target.Name, err, msg)
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("%s's agent returned no answer.", target.Name)
	}
	if len(out) > 30000 {
		out = out[:30000] + "\n…(truncated)"
	}
	return out, nil
}

func findProject(name string) *project {
	for _, p := range allProjects() {
		if strings.EqualFold(p.Name, name) || strconv.FormatInt(p.ID, 10) == name {
			p := p
			return &p
		}
	}
	return nil
}
