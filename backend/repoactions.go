package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// ---- version ordering for tags ----

var versionRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?$`)

// versionLess orders semver-looking tags numerically (a pre-release sorts below its release);
// anything else sorts below all versions, by name.
func versionLess(a, b string) bool {
	ma, mb := versionRe.FindStringSubmatch(a), versionRe.FindStringSubmatch(b)
	if ma == nil || mb == nil {
		if (ma == nil) != (mb == nil) {
			return ma == nil
		}
		return a < b
	}
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(ma[i])
		y, _ := strconv.Atoi(mb[i])
		if x != y {
			return x < y
		}
	}
	if ma[4] == "" || mb[4] == "" {
		return ma[4] != "" && mb[4] == ""
	}
	return ma[4] < mb[4]
}

func githubProject(w http.ResponseWriter, r *http.Request) (project, bool) {
	p, err := getProject(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return p, false
	}
	if p.Owner == "" {
		writeErr(w, 400, "Not a GitHub repository")
		return p, false
	}
	return p, true
}

// ---- merge a pull request ----

func handleMergePull(w http.ResponseWriter, r *http.Request) {
	p, ok := githubProject(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || n <= 0 {
		writeErr(w, 400, "Invalid pull request number")
		return
	}
	var in struct {
		Method string `json:"method"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Method == "" {
		in.Method = "merge"
	}
	if in.Method != "merge" && in.Method != "squash" && in.Method != "rebase" {
		writeErr(w, 400, "method must be merge, squash or rebase")
		return
	}
	var out struct {
		Merged  bool   `json:"merged"`
		Message string `json:"message"`
		SHA     string `json:"sha"`
	}
	path := "/repos/" + p.Owner + "/" + p.Repo + "/pulls/" + strconv.Itoa(n) + "/merge"
	if err := ghSend("PUT", path, map[string]any{"merge_method": in.Method}, &out); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, out)
}

// ---- create a tag on the default branch ----

// Conservative subset of git's ref-name rules.
var tagRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

func handleCreateTag(w http.ResponseWriter, r *http.Request) {
	p, ok := githubProject(w, r)
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !tagRe.MatchString(in.Name) || strings.Contains(in.Name, "..") || strings.HasSuffix(in.Name, ".lock") {
		writeErr(w, 400, "Invalid tag name (use e.g. v1.2.3)")
		return
	}
	base := "/repos/" + p.Owner + "/" + p.Repo
	// Resolve the default branch's head now, so the tag lands on the latest main.
	var head struct {
		SHA string `json:"sha"`
	}
	if err := ghSend("GET", base+"/commits/HEAD", nil, &head); err != nil {
		fail(w, err)
		return
	}
	// A lightweight tag created with the user's token triggers tag-push workflows (e.g. releases).
	if err := ghSend("POST", base+"/git/refs", map[string]any{"ref": "refs/tags/" + in.Name, "sha": head.SHA}, nil); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]any{"name": in.Name, "sha": head.SHA,
		"url": "https://github.com/" + p.Owner + "/" + p.Repo + "/releases/tag/" + url.PathEscape(in.Name)})
}
