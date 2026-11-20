package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Per-project sandbox. Every Claude session, editor shell and project question runs inside
// bubblewrap with the whole filesystem read-only except the project's own checkout and HOME
// (Claude's login/history), so one project's agent can't modify another project's files.
// The app's database and encryption key are hidden from the sandbox altogether.
//
// SANDBOX=auto (default): sandbox if bwrap works here, otherwise run unsandboxed (local dev).
// SANDBOX=on: refuse to start a process when the sandbox is unavailable.
// SANDBOX=off: never sandbox.
//
// Needs `bubblewrap` and unprivileged user namespaces (in Kubernetes: seccompProfile Unconfined).

var (
	sandboxOnce sync.Once
	sandboxBin  string // path to bwrap, empty when unavailable
	sandboxErr  error
)

func sandboxProbe() {
	p, err := exec.LookPath("bwrap")
	if err != nil {
		sandboxErr = errors.New("bubblewrap (bwrap) is not installed")
		return
	}
	if out, err := exec.Command(p, "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "true").CombinedOutput(); err != nil {
		sandboxErr = errors.New("bubblewrap can't create a sandbox here (user namespaces blocked?): " + string(out))
		return
	}
	sandboxBin = p
}

// sandboxCmd wraps bin+args so the process can only write inside dir (when writeDir) and HOME.
// It returns the command unchanged when the sandbox is off or (in auto mode) unavailable.
func sandboxCmd(dir, bin string, args []string, writeDir bool) (string, []string, error) {
	mode := env("SANDBOX", "auto")
	if mode == "off" {
		return bin, args, nil
	}
	sandboxOnce.Do(sandboxProbe)
	if sandboxBin == "" {
		if mode == "on" {
			return "", nil, errors.New("Sandbox required (SANDBOX=on) but unavailable: " + sandboxErr.Error() + ". Fix it or set SANDBOX=auto.")
		}
		warnNoSandbox()
		return bin, args, nil
	}

	a := []string{"--die-with-parent", "--unshare-pid",
		"--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--tmpfs", "/tmp"}
	bound := map[string]bool{}
	bind := func(flag, p string) {
		if p == "" || bound[p] {
			return
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		if _, err := os.Stat(p); err != nil {
			return
		}
		bound[p] = true
		a = append(a, flag, p, p)
	}
	if writeDir {
		bind("--bind", dir)
	}
	bind("--bind", os.Getenv("HOME"))
	bind("--bind", os.Getenv("CLAUDE_CONFIG_DIR"))
	// Hide the app's secrets (DB holds the GitHub token, secret.key decrypts it).
	for _, f := range []string{"secret.key", "app.db", "app.db-wal", "app.db-shm"} {
		p := filepath.Join(dataDir, f)
		if _, err := os.Stat(p); err == nil {
			a = append(a, "--ro-bind", "/dev/null", p)
		}
	}
	a = append(a, "--chdir", dir, "--", bin)
	return sandboxBin, append(a, args...), nil
}

var warnOnce sync.Once

func warnNoSandbox() {
	warnOnce.Do(func() {
		log.Printf("WARNING: sandbox unavailable (%v); projects are NOT isolated from each other. Set SANDBOX=on to refuse instead.", sandboxErr)
	})
}
