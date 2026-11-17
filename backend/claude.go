package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	running   = map[int64]bool{}
	runningMu sync.Mutex
)

func contextTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

// handleClaudeRun runs the `claude` CLI headlessly inside the project checkout and
// streams its stream-json output to the browser as NDJSON. Closing the request
// (Stop button) cancels the context, which kills the process.
func handleClaudeRun(w http.ResponseWriter, r *http.Request) {
	var b struct {
		ProjectID int64  `json:"projectId"`
		Prompt    string `json:"prompt"`
		Mode      string `json:"mode"`
		SessionID string `json:"sessionId"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	p, err := getProject(strconv.FormatInt(b.ProjectID, 10))
	if err != nil {
		fail(w, err)
		return
	}
	if strings.TrimSpace(b.Prompt) == "" {
		writeErr(w, 400, "Empty prompt")
		return
	}
	mode := map[string]string{"plan": "plan", "acceptEdits": "acceptEdits", "bypassPermissions": "bypassPermissions"}[b.Mode]
	if mode == "" {
		mode = "acceptEdits"
	}
	runningMu.Lock()
	if running[p.ID] {
		runningMu.Unlock()
		writeErr(w, 409, "A run is already in progress for this project")
		return
	}
	running[p.ID] = true
	runningMu.Unlock()
	defer func() { runningMu.Lock(); delete(running, p.ID); runningMu.Unlock() }()

	args := []string{"-p", "--output-format", "stream-json", "--verbose", "--permission-mode", mode}
	if sid := strings.TrimSpace(b.SessionID); sid != "" && !strings.HasPrefix(sid, "-") {
		args = append(args, "--resume", sid)
	}
	cmd := exec.CommandContext(r.Context(), env("CLAUDE_BIN", "claude"), args...)
	cmd.Dir = p.Dir
	cmd.Stdin = strings.NewReader(b.Prompt) // prompt via stdin: no argv length/flag-injection issues
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fail(w, err)
		return
	}
	if err := cmd.Start(); err != nil {
		writeErr(w, 500, "Could not start claude CLI (set CLAUDE_BIN?): "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	fl, _ := w.(http.Flusher)
	emit := func(line []byte) {
		w.Write(bytes.TrimRight(line, "\n"))
		w.Write([]byte("\n"))
		if fl != nil {
			fl.Flush()
		}
	}
	rd := bufio.NewReaderSize(stdout, 1<<20)
	for {
		line, err := rd.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			emit(line)
		}
		if err != nil {
			if err != io.EOF {
				break
			}
			break
		}
	}
	if err := cmd.Wait(); err != nil && r.Context().Err() == nil {
		msg, _ := json.Marshal(map[string]string{"type": "app_error", "error": "claude exited: " + err.Error() + " " + strings.TrimSpace(stderr.String())})
		emit(msg)
	}
}
