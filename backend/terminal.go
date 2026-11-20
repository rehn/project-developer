package main

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// Interactive Claude sessions: one PTY per project (plus one "login" session).
// Sessions outlive page reloads; reconnecting replays recent output.

const scrollback = 512 * 1024

type client struct {
	ws *websocket.Conn
	mu sync.Mutex
}

const (
	writeWait = 10 * time.Second // a client that can't take data this long is dropped
	pingEvery = 20 * time.Second // keeps idle proxies from closing the socket, and detects dead peers
	pongWait  = 60 * time.Second
)

// write sends to one browser. It never blocks for long: output is broadcast from the PTY reader,
// so a stalled connection must not be allowed to stall Claude itself. On failure the socket is
// closed, which ends that client's read loop and removes it from the session.
func (c *client) write(typ int, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	if err := c.ws.WriteMessage(typ, b); err != nil {
		c.ws.Close()
	}
}

type session struct {
	mu      sync.Mutex
	ptmx    *os.File
	cmd     *exec.Cmd
	buf     []byte
	clients map[*client]bool
	exited  bool
}

var (
	sessions   = map[string]*session{}
	sessionsMu sync.Mutex
	upgrader   = websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: allowedOrigin}
)

func (s *session) broadcast(typ int, b []byte) {
	s.mu.Lock()
	cs := make([]*client, 0, len(s.clients))
	for c := range s.clients {
		cs = append(cs, c)
	}
	s.mu.Unlock()
	for _, c := range cs {
		c.write(typ, b)
	}
}

func startPTY(key, dir string, args []string, cols, rows uint16) (*session, error) {
	bin, ok := findClaude()
	if strings.HasPrefix(key, "sh") { // plain shell in the project checkout (editor view)
		bin, ok = shellBin(), true
	}
	if !ok {
		return nil, &httpErr{500, "Claude Code CLI not found. Install it (https://claude.com/claude-code) or set CLAUDE_BIN."}
	}
	args, askTok := withMCP(key, args)
	bin, args, err := sandboxCmd(dir, bin, args, true)
	if err != nil {
		unregisterAsk(askTok)
		return nil, err
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	cmd.Env = append(cmd.Env, kubeEnv()...)
	if tok := githubToken(); tok != "" {
		// Lets `gh` and `git push` inside the session use the token saved in Settings.
		cmd.Env = append(cmd.Env, "GH_TOKEN="+tok, "GITHUB_TOKEN="+tok,
			"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=credential.https://github.com.helper",
			"GIT_CONFIG_VALUE_0=!gh auth git-credential")
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		unregisterAsk(askTok)
		return nil, err
	}
	s := &session{ptmx: ptmx, cmd: cmd, clients: map[*client]bool{}}
	go func() {
		chunk := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(chunk)
			if n > 0 {
				d := append([]byte(nil), chunk[:n]...)
				s.mu.Lock()
				s.buf = append(s.buf, d...)
				if len(s.buf) > scrollback {
					s.buf = s.buf[len(s.buf)-scrollback:]
				}
				s.mu.Unlock()
				s.broadcast(websocket.BinaryMessage, d)
			}
			if err != nil {
				break
			}
		}
		code := 0
		if err := cmd.Wait(); err != nil {
			code = -1
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
		}
		ptmx.Close()
		unregisterAsk(askTok)
		s.mu.Lock()
		s.exited = true
		s.mu.Unlock()
		msg, _ := json.Marshal(map[string]any{"type": "exit", "code": code})
		s.broadcast(websocket.TextMessage, msg)
		sessionsMu.Lock()
		if sessions[key] == s {
			delete(sessions, key)
		}
		sessionsMu.Unlock()
	}()
	return s, nil
}

func sessionTarget(r *http.Request) (key, dir string, args []string, err error) {
	p := r.URL.Query().Get("project")
	if p == "login" {
		home, _ := os.UserHomeDir()
		return "login", home, []string{"auth", "login"}, nil
	}
	if r.URL.Query().Get("mode") == "shell" {
		var proj project
		if proj, err = getProject(p); err != nil {
			return
		}
		return "sh" + p, proj.Dir, nil, nil
	}
	cont := r.URL.Query().Get("continue") == "1"
	if p == "general" {
		// Not tied to any project: runs in its own scratch directory on the data volume.
		dir = filepath.Join(dataDir, "general")
		if err = os.MkdirAll(dir, 0o755); err != nil {
			return
		}
		if cont {
			args = []string{"--continue"}
		}
		if m := getSetting(generalModelKey); m != "" {
			args = append([]string{"--model", m}, args...)
		}
		return "general", dir, args, nil
	}
	proj, err := getProject(p)
	if err != nil {
		return
	}
	if cont {
		args = []string{"--continue"}
	}
	if m := proj.Model; m != "" {
		args = append([]string{"--model", m}, args...)
	}
	return "p" + p, proj.Dir, args, nil
}

func handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	key, dir, args, err := sessionTarget(r)
	if err != nil {
		fail(w, err)
		return
	}
	u32 := func(k string, def uint16) uint16 {
		if n, err := strconv.Atoi(r.URL.Query().Get(k)); err == nil && n > 0 && n < 1000 {
			return uint16(n)
		}
		return def
	}
	cols, rows := u32("cols", 100), u32("rows", 30)

	sessionsMu.Lock()
	s := sessions[key]
	if s == nil {
		s, err = startPTY(key, dir, args, cols, rows)
		if err != nil {
			sessionsMu.Unlock()
			fail(w, err)
			return
		}
		sessions[key] = s
	}
	sessionsMu.Unlock()

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{ws: ws}
	s.mu.Lock()
	replay := append([]byte(nil), s.buf...)
	s.clients[c] = true
	s.mu.Unlock()
	pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
	if len(replay) > 0 {
		c.write(websocket.BinaryMessage, replay)
	}
	// Ping regularly; the browser answers automatically. No traffic for pongWait means the peer is gone.
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(pingEvery)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				c.write(websocket.PingMessage, nil)
			case <-done:
				return
			}
		}
	}()
	defer func() {
		close(done)
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		ws.Close()
	}()
	for {
		typ, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		ws.SetReadDeadline(time.Now().Add(pongWait))
		if typ == websocket.BinaryMessage {
			s.ptmx.Write(data) // keystrokes
		} else {
			var m struct {
				Type       string
				Cols, Rows uint16
			}
			if json.Unmarshal(data, &m) == nil && m.Type == "resize" && m.Cols > 0 && m.Rows > 0 {
				pty.Setsize(s.ptmx, &pty.Winsize{Cols: m.Cols, Rows: m.Rows})
			}
		}
	}
}

// DELETE /api/terminal?project=… ends the session (used for "Restart").
func handleTerminalKill(w http.ResponseWriter, r *http.Request) {
	key, _, _, err := sessionTarget(r)
	if err != nil {
		fail(w, err)
		return
	}
	sessionsMu.Lock()
	s := sessions[key]
	sessionsMu.Unlock()
	if s != nil {
		s.cmd.Process.Kill()
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// GET /api/terminal/sessions lists live sessions so the UI can show which projects are running.
func handleTerminalList(w http.ResponseWriter, r *http.Request) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	keys := []string{}
	for k := range sessions {
		keys = append(keys, k)
	}
	writeJSON(w, keys)
}
