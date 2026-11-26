package main

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// allowedOrigins: browser origins permitted to make state-changing requests.
func allowedOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true // non-browser client
	}
	if u, err := url.Parse(o); err == nil && u.Host == r.Host {
		return true
	}
	for _, a := range strings.Split(env("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"), ",") {
		if strings.TrimSpace(a) == o {
			return true
		}
	}
	return false
}

func csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && !allowedOrigin(r) {
			writeErr(w, 403, "Bad origin")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	if err := initDB(); err != nil {
		log.Fatal(err)
	}
	pub := http.NewServeMux()
	pub.HandleFunc("GET /api/auth/me", handleMe)
	pub.HandleFunc("POST /api/auth/setup", handleSetup)
	pub.HandleFunc("POST /api/auth/login", handleLogin)
	pub.HandleFunc("POST /api/auth/logout", handleLogout)

	priv := http.NewServeMux()
	priv.HandleFunc("POST /api/auth/password", handlePassword)
	priv.HandleFunc("GET /api/settings", handleGetSettings)
	priv.HandleFunc("PUT /api/settings/github-token", handleSetToken)
	priv.HandleFunc("PUT /api/settings/kubeconfig", handleSetKubeconfig)
	priv.HandleFunc("GET /api/github/repos", handleRepos)
	priv.HandleFunc("GET /api/projects", handleListProjects)
	priv.HandleFunc("POST /api/projects", handleAddProject)
	priv.HandleFunc("DELETE /api/projects/{id}", handleDeleteProject)
	priv.HandleFunc("PUT /api/projects/{id}/model", handleSetModel)
	priv.HandleFunc("GET /api/general/model", handleGetGeneralModel)
	priv.HandleFunc("PUT /api/general/model", handleSetGeneralModel)
	priv.HandleFunc("POST /api/projects/{id}/pull", handlePull)
	priv.HandleFunc("GET /api/projects/{id}/status", handleStatus)
	priv.HandleFunc("GET /api/projects/{id}/info", handleProjectInfo)
	priv.HandleFunc("GET /api/projects/{id}/files", handleListFiles)
	priv.HandleFunc("GET /api/projects/{id}/file", handleReadFile)
	priv.HandleFunc("PUT /api/projects/{id}/file", handleWriteFile)
	priv.HandleFunc("POST /api/projects/{id}/upload", handleProjectUpload)
	priv.HandleFunc("POST /api/projects/{id}/pulls/{number}/merge", handleMergePull)
	priv.HandleFunc("POST /api/projects/{id}/tags", handleCreateTag)
	priv.HandleFunc("GET /api/usage/limits", handleLimits)
	priv.HandleFunc("GET /api/tasks", handleListTasks)
	priv.HandleFunc("POST /api/tasks/{id}/approve", handleApproveTask)
	priv.HandleFunc("POST /api/tasks/{id}/reject", handleRejectTask)
	priv.HandleFunc("GET /api/claude/auth", handleClaudeAuth)
	priv.HandleFunc("GET /api/terminal", handleTerminalWS)
	priv.HandleFunc("DELETE /api/terminal", handleTerminalKill)
	priv.HandleFunc("GET /api/terminal/sessions", handleTerminalList)
	priv.HandleFunc("POST /api/uploads", handleUpload)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			writeErr(w, 503, "db unavailable")
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	root.HandleFunc("/mcp", handleMCP) // project questions for Claude sessions; own bearer-token auth
	root.Handle("/api/auth/password", requireAuth(priv))
	root.Handle("/api/auth/", pub)
	root.Handle("/api/", requireAuth(priv))
	if dir := staticDir(); dir != "" {
		root.Handle("/", staticHandler(dir))
		log.Printf("serving UI from %s", dir)
	}

	// Localhost-only by default: this app can run Claude with write access to your machine.
	addr := env("ADDR", "127.0.0.1:3001")
	log.Printf("project-developer API on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, requestTimeout(30*time.Second, csrf(root))))
}

// staticDir returns the exported Next.js UI directory, if present (Docker/production).
func staticDir() string {
	d := env("STATIC_DIR", "../web/out")
	if st, err := os.Stat(filepath.Join(d, "index.html")); err == nil && !st.IsDir() {
		return d
	}
	return ""
}

func staticHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exported pages live at <route>/index.html; unknown routes fall back to the app shell.
		clean := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(clean); err != nil || (st.IsDir() && !fileExists(filepath.Join(clean, "index.html"))) {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/_next/static/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fs.ServeHTTP(w, r)
	})
}

func fileExists(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }
