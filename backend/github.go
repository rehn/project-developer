package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var repoRe = regexp.MustCompile(`(?i)github\.com[/:]([^/\s]+)/([^/\s]+?)(?:\.git)?/?$`)

func parseRepoURL(u string) (owner, repo string, ok bool) {
	m := repoRe.FindStringSubmatch(u)
	if m == nil {
		return
	}
	return m[1], m[2], true
}

type httpErr struct {
	status int
	msg    string
}

func (e *httpErr) Error() string { return e.msg }

var ghClient = &http.Client{Timeout: 20 * time.Second}

func ghRequest(method, path, token string, out any) error {
	return ghDo(method, path, token, nil, out)
}

// ghDo sends an optional JSON body. GitHub error bodies are reduced to their "message".
func ghDo(method, path, token string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, env("GITHUB_API_URL", "https://api.github.com")+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "project-developer")
	res, err := ghClient.Do(req)
	if err != nil {
		return &httpErr{502, "GitHub unreachable: " + err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4000))
		st := res.StatusCode
		if st == 401 {
			st = 400
		}
		var m struct{ Message string }
		if json.Unmarshal(b, &m) == nil && m.Message != "" {
			return &httpErr{st, fmt.Sprintf("GitHub: %d %s", res.StatusCode, m.Message)}
		}
		if len(b) > 200 {
			b = b[:200]
		}
		return &httpErr{st, fmt.Sprintf("GitHub: %d %s", res.StatusCode, b)}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func gh(path string, out any) error {
	return ghSend("GET", path, nil, out)
}

// ghSend calls the GitHub API with the stored token.
func ghSend(method, path string, body, out any) error {
	t := githubToken()
	if t == "" {
		return &httpErr{400, "No GitHub token set. Add one in Settings."}
	}
	return ghDo(method, path, t, body, out)
}

// runGit passes the token via GIT_CONFIG_* env (scoped to github.com), so it never
// appears in argv, the remote URL, or .git/config.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	tok := githubToken()
	if tok != "" {
		b64 := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + tok))
		cmd.Env = append(cmd.Env, "GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
			"GIT_CONFIG_VALUE_0=Authorization: Basic "+b64)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if len(msg) > 400 {
			msg = msg[:400]
		}
		return "", &httpErr{400, "git failed: " + msg}
	}
	return out.String(), nil
}
