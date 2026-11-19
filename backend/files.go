package main

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// File access for the editor view, confined to the project's checkout.
// Symlinks are resolved, so a link pointing outside the checkout can't be used to escape it.

const maxEditFile = 2 << 20

func shellBin() string {
	if p, err := exec.LookPath("bash"); err == nil {
		return p
	}
	return "/bin/sh"
}

// resolveIn maps a project-relative path to a real path inside the checkout.
func resolveIn(p project, rel string) (string, error) {
	root, err := filepath.EvalSymlinks(p.Dir)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(root, filepath.Clean("/"+rel)) // Clean on a rooted path drops any ".."
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		// New file: its directory must exist inside the checkout (checked below via the parent).
		parent, perr := filepath.EvalSymlinks(filepath.Dir(abs))
		if perr != nil {
			if os.IsNotExist(perr) {
				parent = filepath.Dir(abs) // directories will be created; the lexical path is already inside root
			} else {
				return "", perr
			}
		}
		real = filepath.Join(parent, filepath.Base(abs))
	}
	if real != root && !strings.HasPrefix(real, root+string(filepath.Separator)) {
		return "", &httpErr{400, "Path is outside the project"}
	}
	return real, nil
}

func hasGitSegment(rel string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(filepath.Clean("/"+rel)), "/") {
		if seg == ".git" {
			return true
		}
	}
	return false
}

type fileEntry struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
}

// GET /api/projects/{id}/files?path=<dir>
func handleListFiles(w http.ResponseWriter, r *http.Request) {
	p, err := getProject(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	dir, err := resolveIn(p, r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		writeErr(w, 404, "Can't read that folder")
		return
	}
	out := []fileEntry{}
	for _, e := range ents {
		if e.Name() == ".git" {
			continue
		}
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			if st, err := os.Stat(filepath.Join(dir, e.Name())); err == nil {
				isDir = st.IsDir()
			}
		}
		out = append(out, fileEntry{e.Name(), isDir})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeJSON(w, out)
}

// GET /api/projects/{id}/file?path=<file>
func handleReadFile(w http.ResponseWriter, r *http.Request) {
	p, err := getProject(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	path, err := resolveIn(p, r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		writeErr(w, 404, "File not found")
		return
	}
	if st.Size() > maxEditFile {
		writeErr(w, 413, "File is too large to edit here (max 2 MB)")
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		fail(w, err)
		return
	}
	if !utf8.Valid(b) || strings.ContainsRune(string(b), 0) {
		writeErr(w, 415, "Binary file — can't be edited here")
		return
	}
	writeJSON(w, map[string]any{"content": string(b)})
}

// PUT /api/projects/{id}/file  {path, content}
func handleWriteFile(w http.ResponseWriter, r *http.Request) {
	p, err := getProject(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	var b struct{ Path, Content string }
	if !readJSON(w, r, &b) {
		return
	}
	if b.Path == "" {
		fail(w, &httpErr{400, "Path required"})
		return
	}
	if hasGitSegment(b.Path) {
		fail(w, &httpErr{400, "Editing inside .git is not allowed here"})
		return
	}
	path, err := resolveIn(p, b.Path)
	if err != nil {
		fail(w, err)
		return
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		if st.IsDir() {
			writeErr(w, 400, "That is a folder")
			return
		}
		mode = st.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fail(w, err)
		return
	}
	if err := os.WriteFile(path, []byte(b.Content), mode); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
