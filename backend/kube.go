package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// A kubeconfig uploaded in Settings lives on the data volume and is exposed to every Claude session
// (and shell) through $KUBECONFIG, so `kubectl` works there. It is deliberately not ~/.kube/config,
// so running the app locally never touches your own kubeconfig.

const maxKubeconfig = 512 << 10

func kubeconfigPath() string { return filepath.Join(dataDir, "kube", "config") }

func kubeconfigSet() bool { return fileExists(kubeconfigPath()) }

// kubeEnv is appended to the environment of every session the app starts.
func kubeEnv() []string {
	if kubeconfigSet() {
		return []string{"KUBECONFIG=" + kubeconfigPath()}
	}
	return nil
}

// kubectl runs kubectl against the given kubeconfig with a short timeout.
func kubectl(path string, args ...string) (string, error) {
	bin, err := exec.LookPath("kubectl")
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, append([]string{"--kubeconfig", path}, args...)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New(strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// kubeInfo is merged into GET /api/settings.
func kubeInfo() map[string]any {
	info := map[string]any{"kubeconfigSet": kubeconfigSet(), "kubeContexts": []string{}, "kubeCurrent": ""}
	if !kubeconfigSet() {
		return info
	}
	if out, err := kubectl(kubeconfigPath(), "config", "get-contexts", "-o", "name"); err == nil && out != "" {
		info["kubeContexts"] = strings.Split(out, "\n")
	}
	if out, err := kubectl(kubeconfigPath(), "config", "current-context"); err == nil {
		info["kubeCurrent"] = out
	}
	return info
}

func handleSetKubeconfig(w http.ResponseWriter, r *http.Request) {
	var b struct{ Content string }
	if !readJSON(w, r, &b) {
		return
	}
	content := strings.TrimSpace(b.Content)
	if content == "" {
		os.Remove(kubeconfigPath())
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	if len(content) > maxKubeconfig {
		writeErr(w, 400, "That file is too large for a kubeconfig")
		return
	}
	if !strings.Contains(content, "clusters") {
		writeErr(w, 400, "That doesn't look like a kubeconfig (no clusters section)")
		return
	}
	dir := filepath.Dir(kubeconfigPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fail(w, err)
		return
	}
	tmp := kubeconfigPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(content+"\n"), 0o600); err != nil {
		fail(w, err)
		return
	}
	// Let kubectl parse it before it replaces the working one (skipped if kubectl isn't installed).
	if _, err := kubectl(tmp, "config", "get-contexts", "-o", "name"); err != nil {
		if _, missing := exec.LookPath("kubectl"); missing == nil {
			os.Remove(tmp)
			writeErr(w, 400, "kubectl couldn't read that kubeconfig: "+err.Error())
			return
		}
	}
	if err := os.Rename(tmp, kubeconfigPath()); err != nil {
		fail(w, err)
		return
	}
	info := kubeInfo()
	info["ok"] = true
	writeJSON(w, info)
}
