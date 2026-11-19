package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Task requests: one agent asks (via the request_task MCP tool) for another project's agent to do
// some work. Nothing runs until the user approves it in the UI; approving types the task into that
// project's own Claude session (starting one if needed), so it runs with that session's normal
// permission prompts. Requests are persisted so they survive a restart.

const maxPendingPerProject = 5

func taskLimit() int {
	if n, err := strconv.Atoi(env("TASK_LIMIT", "3")); err == nil && n > 0 {
		return n
	}
	return 3
}

type task struct {
	ID        int64  `json:"id"`
	From      string `json:"from"`
	ToProject int64  `json:"toProject"`
	To        string `json:"to"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"createdAt"`
}

const taskCols = `t.id, t.from_name, t.to_project, p.name, t.body, t.created_at`

func scanTask(s interface{ Scan(...any) error }) (t task, err error) {
	err = s.Scan(&t.ID, &t.From, &t.ToProject, &t.To, &t.Body, &t.CreatedAt)
	return
}

func pendingTasks() []task {
	out := []task{}
	rows, err := db.Query(`SELECT ` + taskCols + ` FROM tasks t JOIN projects p ON p.id = t.to_project WHERE t.status = 'pending' ORDER BY t.id`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		if t, err := scanTask(rows); err == nil {
			out = append(out, t)
		}
	}
	return out
}

func createTask(from string, to project, body string) (int64, error) {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE to_project = ? AND status = 'pending'`, to.ID).Scan(&n)
	if n >= maxPendingPerProject {
		return 0, fmt.Errorf("%s already has %d task requests waiting for the user's approval. Don't queue more; tell the user instead.", to.Name, n)
	}
	res, err := db.Exec(`INSERT INTO tasks (from_name, to_project, body, created_at) VALUES (?, ?, ?, ?)`, from, to.ID, body, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// requestTask is the request_task MCP tool.
func requestTask(s *askSess, name, body string) (string, error) {
	name, body = strings.TrimSpace(name), strings.TrimSpace(body)
	if name == "" || body == "" {
		return "", errors.New("Both project and task are required.")
	}
	if len(body) > 4000 {
		return "", errors.New("The task is too long (max 4000 characters). Put the details in a file in your repo and point to it.")
	}
	target := findProject(name)
	if target == nil {
		return "", fmt.Errorf("No project named %q. Use list_projects to see the options.", name)
	}
	if "p"+strconv.FormatInt(target.ID, 10) == s.callerKey {
		return "", errors.New("That is your own project — do the work yourself.")
	}
	s.mu.Lock()
	if s.tasks >= taskLimit() {
		s.mu.Unlock()
		return "", fmt.Errorf("Task request limit reached (%d per session). Tell the user what you still need from %s instead.", taskLimit(), target.Name)
	}
	s.tasks++
	s.mu.Unlock()
	id, err := createTask(s.callerName, *target, body)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Task #%d queued for %s. It only runs if the user approves it in the app, and you won't get a result back. Carry on, and tell the user you requested it.", id, target.Name), nil
}

func handleListTasks(w http.ResponseWriter, r *http.Request) { writeJSON(w, pendingTasks()) }

func handleRejectTask(w http.ResponseWriter, r *http.Request) {
	res, err := db.Exec(`UPDATE tasks SET status = 'rejected' WHERE id = ? AND status = 'pending'`, r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 404, "Task not found (already handled?)")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func handleApproveTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := scanTask(db.QueryRow(`SELECT `+taskCols+` FROM tasks t JOIN projects p ON p.id = t.to_project WHERE t.id = ? AND t.status = 'pending'`, id))
	if err != nil {
		writeErr(w, 404, "Task not found (already handled?)")
		return
	}
	// Claim it first so a double click can't deliver it twice; put it back if delivery fails.
	res, err := db.Exec(`UPDATE tasks SET status = 'approved' WHERE id = ? AND status = 'pending'`, id)
	if err != nil {
		fail(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 409, "Task was already handled")
		return
	}
	proj, err := getProject(strconv.FormatInt(t.ToProject, 10))
	if err == nil {
		var started bool
		started, err = deliverToSession(proj, fmt.Sprintf("Task requested by the %s project's agent, approved by the user:\n\n%s", t.From, t.Body))
		if err == nil {
			writeJSON(w, map[string]any{"ok": true, "started": started})
			return
		}
	}
	db.Exec(`UPDATE tasks SET status = 'pending' WHERE id = ?`, id)
	fail(w, err)
}

// deliverToSession gives msg to the project's Claude session: typed in as a paste if one is running,
// otherwise a new session is started with it as the first prompt.
func deliverToSession(p project, msg string) (started bool, err error) {
	key := "p" + strconv.FormatInt(p.ID, 10)
	sessionsMu.Lock()
	s := sessions[key]
	if s == nil {
		args := []string{}
		if p.Model != "" {
			args = append(args, "--model", p.Model)
		}
		args = append(args, msg) // positional prompt: an interactive session that starts with it
		s, err = startPTY(key, p.Dir, args, 100, 30)
		if err != nil {
			sessionsMu.Unlock()
			return false, err
		}
		sessions[key] = s
		sessionsMu.Unlock()
		return true, nil
	}
	sessionsMu.Unlock()
	// Bracketed paste keeps newlines from submitting early; Enter follows once it has been taken in.
	if _, err = s.ptmx.Write([]byte("\x1b[200~" + msg + "\x1b[201~")); err != nil {
		return false, err
	}
	time.Sleep(300 * time.Millisecond)
	_, err = s.ptmx.Write([]byte("\r"))
	return false, err
}
