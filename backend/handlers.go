package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
func fail(w http.ResponseWriter, err error) {
	var he *httpErr
	if errors.As(err, &he) {
		writeErr(w, he.status, he.msg)
		return
	}
	writeErr(w, 500, err.Error())
}
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeErr(w, 400, "Invalid JSON")
		return false
	}
	return true
}
func clientIP(r *http.Request) string {
	// Behind a gateway/reverse proxy every request shares the proxy's address; opt in to trust X-Forwarded-For.
	if env("TRUST_PROXY", "") == "true" {
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			return strings.TrimSpace(strings.Split(xf, ",")[0])
		}
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

// ---- settings ----

func handleGetSettings(w http.ResponseWriter, r *http.Request) {
	t := githubToken()
	hint := ""
	if len(t) >= 4 {
		hint = "…" + t[len(t)-4:]
	}
	writeJSON(w, map[string]any{"githubTokenSet": t != "", "githubTokenHint": hint, "model": claudeModel()})
}

func handleSetToken(w http.ResponseWriter, r *http.Request) {
	var b struct{ Token string }
	if !readJSON(w, r, &b) {
		return
	}
	tok := strings.TrimSpace(b.Token)
	if tok == "" {
		deleteSetting("github_token")
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	var u struct{ Login string }
	if err := ghRequest("GET", "/user", tok, &u); err != nil { // verify before saving
		writeErr(w, 400, "GitHub rejected that token")
		return
	}
	setSetting("github_token", encrypt(tok))
	writeJSON(w, map[string]any{"ok": true, "login": u.Login})
}

func handleRepos(w http.ResponseWriter, r *http.Request) {
	var repos []struct {
		FullName    string  `json:"full_name"`
		CloneURL    string  `json:"clone_url"`
		Private     bool    `json:"private"`
		Description *string `json:"description"`
	}
	if err := gh("/user/repos?per_page=100&sort=pushed&affiliation=owner,collaborator,organization_member", &repos); err != nil {
		fail(w, err)
		return
	}
	out := []map[string]any{}
	for _, x := range repos {
		out = append(out, map[string]any{"fullName": x.FullName, "cloneUrl": x.CloneURL, "private": x.Private, "description": x.Description})
	}
	writeJSON(w, out)
}

// ---- projects ----

type project struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	RepoURL   string `json:"repoUrl"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Dir       string `json:"-"`
	CreatedAt int64  `json:"createdAt"`
}

func scanProject(s interface{ Scan(...any) error }) (p project, err error) {
	var o, rp *string
	err = s.Scan(&p.ID, &p.Name, &p.RepoURL, &o, &rp, &p.Dir, &p.CreatedAt)
	if o != nil {
		p.Owner = *o
	}
	if rp != nil {
		p.Repo = *rp
	}
	return
}

const projCols = `id,name,repo_url,owner,repo,dir,created_at`

func getProject(id string) (project, error) {
	p, err := scanProject(db.QueryRow(`SELECT `+projCols+` FROM projects WHERE id=?`, id))
	if err != nil {
		return p, &httpErr{404, "Project not found"}
	}
	return p, nil
}

func allProjects() []project {
	rows, err := db.Query(`SELECT ` + projCols + ` FROM projects ORDER BY name`)
	out := []project{}
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		if p, err := scanProject(rows); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func handleListProjects(w http.ResponseWriter, r *http.Request) { writeJSON(w, allProjects()) }

var (
	nameRe    = regexp.MustCompile(`^[\w.-]+$`)
	cloneLock sync.Mutex
)

func handleAddProject(w http.ResponseWriter, r *http.Request) {
	var b struct{ RepoURL, Name string }
	if !readJSON(w, r, &b) {
		return
	}
	url := strings.TrimSpace(b.RepoURL)
	if !strings.HasPrefix(url, "https://") || strings.ContainsAny(url, " \t\n") {
		writeErr(w, 400, "Use an https:// git URL")
		return
	}
	owner, repo, isGH := parseRepoURL(url)
	name := strings.TrimSpace(b.Name)
	if name == "" {
		name = repo
	}
	if name == "" {
		name = strings.TrimSuffix(url[strings.LastIndex(url, "/")+1:], ".git")
	}
	if !nameRe.MatchString(name) || name == "." || name == ".." {
		writeErr(w, 400, "Invalid project name")
		return
	}
	cloneLock.Lock()
	defer cloneLock.Unlock()
	dir := filepath.Join(projectsDir, name)
	if _, err := os.Stat(dir); err == nil {
		writeErr(w, 409, `A project named "`+name+`" already exists`)
		return
	}
	ctx, cancel := contextTimeout(r, 10*time.Minute)
	defer cancel()
	if _, err := runGit(ctx, "", "clone", "--", url, dir); err != nil {
		os.RemoveAll(dir)
		fail(w, err)
		return
	}
	var o, rp any
	if isGH {
		o, rp = owner, repo
	}
	res, err := db.Exec(`INSERT INTO projects(name,repo_url,owner,repo,dir,created_at) VALUES(?,?,?,?,?,?)`, name, url, o, rp, dir, time.Now().Unix())
	if err != nil {
		fail(w, err)
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, map[string]any{"id": id, "name": name})
}

func handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	p, err := getProject(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	db.Exec(`DELETE FROM projects WHERE id=?`, p.ID)
	if filepath.Dir(p.Dir) == projectsDir { // only remove checkouts we manage
		os.RemoveAll(p.Dir)
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func handlePull(w http.ResponseWriter, r *http.Request) {
	p, err := getProject(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	ctx, cancel := contextTimeout(r, 2*time.Minute)
	defer cancel()
	out, err := runGit(ctx, p.Dir, "pull", "--ff-only")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]string{"output": strings.TrimSpace(out)})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	p, err := getProject(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	ctx := r.Context()
	branch, e1 := runGit(ctx, p.Dir, "rev-parse", "--abbrev-ref", "HEAD")
	status, e2 := runGit(ctx, p.Dir, "status", "--porcelain")
	log, e3 := runGit(ctx, p.Dir, "log", "-n", "8", "--pretty=format:%h\t%s\t%an\t%ar")
	if err := errors.Join(e1, e2, e3); err != nil {
		fail(w, err)
		return
	}
	changes := []string{}
	for _, l := range strings.Split(status, "\n") {
		if l != "" {
			changes = append(changes, l)
		}
	}
	commits := []map[string]string{}
	for _, l := range strings.Split(log, "\n") {
		if f := strings.Split(l, "\t"); len(f) == 4 {
			commits = append(commits, map[string]string{"hash": f[0], "subject": f[1], "author": f[2], "when": f[3]})
		}
	}
	res := map[string]any{"branch": strings.TrimSpace(branch), "changes": changes, "commits": commits}

	// Check GitHub for new commits without touching the working tree. Fetches are
	// throttled per project so frequent status polling doesn't hammer the remote.
	if fetchDue(p.ID) {
		fctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if _, err := runGit(fctx, p.Dir, "fetch", "--quiet", "--prune"); err != nil {
			res["fetchError"] = err.Error()
		}
		cancel()
	}
	if counts, err := runGit(ctx, p.Dir, "rev-list", "--left-right", "--count", "HEAD...@{upstream}"); err == nil {
		if f := strings.Fields(counts); len(f) == 2 {
			res["ahead"], _ = strconv.Atoi(f[0])
			res["behind"], _ = strconv.Atoi(f[1])
		}
	}
	writeJSON(w, res)
}

const fetchInterval = 60 * time.Second

var (
	fetchMu   sync.Mutex
	lastFetch = map[int64]time.Time{}
)

// fetchDue reports whether the project's remote should be fetched now, and if so records the attempt.
func fetchDue(id int64) bool {
	fetchMu.Lock()
	defer fetchMu.Unlock()
	if time.Since(lastFetch[id]) < fetchInterval {
		return false
	}
	lastFetch[id] = time.Now()
	return true
}

func contextTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

// PUT /api/settings/model {model} sets the model new Claude sessions start with; "" means Claude Code's default.
// Accepts an alias (opus, sonnet, haiku, …) or a full model ID.
var modelRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:\[\]-]{0,79}$`)

func handleSetModel(w http.ResponseWriter, r *http.Request) {
	var b struct{ Model string }
	if !readJSON(w, r, &b) {
		return
	}
	m := strings.TrimSpace(b.Model)
	if m == "" {
		deleteSetting("claude_model")
	} else if !modelRe.MatchString(m) {
		writeErr(w, 400, "That doesn't look like a model name")
		return
	} else {
		setSetting("claude_model", m)
	}
	writeJSON(w, map[string]any{"ok": true, "model": m})
}
