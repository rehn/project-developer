package main

import (
	"log"
	"net/http"
	"net/url"
	"strings"
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
	priv.HandleFunc("GET /api/github/repos", handleRepos)
	priv.HandleFunc("GET /api/projects", handleListProjects)
	priv.HandleFunc("POST /api/projects", handleAddProject)
	priv.HandleFunc("DELETE /api/projects/{id}", handleDeleteProject)
	priv.HandleFunc("POST /api/projects/{id}/pull", handlePull)
	priv.HandleFunc("GET /api/projects/{id}/status", handleStatus)
	priv.HandleFunc("GET /api/pulls", handlePulls)
	priv.HandleFunc("GET /api/usage", handleUsage)
	priv.HandleFunc("POST /api/claude/run", handleClaudeRun)

	root := http.NewServeMux()
	root.Handle("/api/auth/password", requireAuth(priv))
	root.Handle("/api/auth/", pub)
	root.Handle("/api/", requireAuth(priv))

	// Localhost-only by default: this app can run Claude with write access to your machine.
	addr := env("ADDR", "127.0.0.1:3001")
	log.Printf("project-developer API on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, csrf(root)))
}
