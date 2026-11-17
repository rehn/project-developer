package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

// findClaude locates the Claude Code CLI: $CLAUDE_BIN, PATH, common install spots,
// then the native binary bundled with the VS Code extension.
func findClaude() (string, bool) {
	if b := os.Getenv("CLAUDE_BIN"); b != "" {
		if p, err := exec.LookPath(b); err == nil {
			return p, true
		}
		return b, false
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p, true
	}
	home, _ := os.UserHomeDir()
	for _, c := range []string{
		filepath.Join(home, ".local/bin/claude"), filepath.Join(home, ".claude/local/claude"),
		"/opt/homebrew/bin/claude", "/usr/local/bin/claude",
	} {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, true
		}
	}
	matches, _ := filepath.Glob(filepath.Join(home, ".vscode*/extensions/anthropic.claude-code-*/resources/native-binary/claude"))
	sort.Strings(matches)
	if len(matches) > 0 {
		return matches[len(matches)-1], true
	}
	return "claude", false
}

// handleClaudeAuth reports whether the CLI is installed and logged in (`claude auth status`).
func handleClaudeAuth(w http.ResponseWriter, r *http.Request) {
	bin, ok := findClaude()
	if !ok {
		writeJSON(w, map[string]any{"installed": false})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, bin, "auth", "status").Output() // exits non-zero when logged out
	var st map[string]any
	if json.Unmarshal(out, &st) != nil {
		st = map[string]any{"loggedIn": false}
	}
	writeJSON(w, map[string]any{
		"installed": true, "bin": bin, "loggedIn": st["loggedIn"] == true,
		"authMethod": st["authMethod"], "expiresAt": st["loginExpiresAt"],
	})
}
