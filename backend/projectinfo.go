package main

import (
	"errors"
	"net/http"
	"strings"
	"sync"
)

// handleProjectInfo powers the project side panel: local checkout state plus GitHub repo,
// latest commit, open PRs and latest release. Each part reports its own error so one
// failing call doesn't blank the whole panel.
func handleProjectInfo(w http.ResponseWriter, r *http.Request) {
	p, err := getProject(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	ctx := r.Context()
	errs := map[string]string{}
	var mu sync.Mutex
	setErr := func(k string, e error) { mu.Lock(); errs[k] = e.Error(); mu.Unlock() }

	var wg sync.WaitGroup
	run := func(f func()) { wg.Add(1); go func() { defer wg.Done(); f() }() }

	// ---- local checkout ----
	local := map[string]any{}
	run(func() {
		hash, e1 := runGit(ctx, p.Dir, "rev-parse", "HEAD")
		branch, e2 := runGit(ctx, p.Dir, "rev-parse", "--abbrev-ref", "HEAD")
		status, e3 := runGit(ctx, p.Dir, "status", "--porcelain")
		if e := errors.Join(e1, e2, e3); e != nil {
			setErr("local", e)
			return
		}
		changes := 0
		for _, l := range strings.Split(status, "\n") {
			if l != "" {
				changes++
			}
		}
		local["hash"], local["branch"], local["changes"] = strings.TrimSpace(hash), strings.TrimSpace(branch), changes
	})

	// ---- GitHub ----
	var repo, commit, release map[string]any
	pulls := []map[string]any{}
	if p.Owner != "" {
		base := "/repos/" + p.Owner + "/" + p.Repo
		run(func() {
			var x struct {
				FullName      string  `json:"full_name"`
				HTMLURL       string  `json:"html_url"`
				Description   *string `json:"description"`
				DefaultBranch string  `json:"default_branch"`
				Private       bool    `json:"private"`
			}
			if e := gh(base, &x); e != nil {
				setErr("repo", e)
				return
			}
			repo = map[string]any{"fullName": x.FullName, "url": x.HTMLURL, "description": x.Description,
				"defaultBranch": x.DefaultBranch, "private": x.Private}
		})
		run(func() {
			var x []struct {
				SHA     string `json:"sha"`
				HTMLURL string `json:"html_url"`
				Commit  struct {
					Message string
					Author  struct{ Name, Date string }
				}
			}
			if e := gh(base+"/commits?per_page=1", &x); e != nil {
				setErr("commit", e)
				return
			}
			if len(x) > 0 {
				commit = map[string]any{"hash": x[0].SHA, "url": x[0].HTMLURL,
					"message": strings.SplitN(x[0].Commit.Message, "\n", 2)[0],
					"author":  x[0].Commit.Author.Name, "date": x[0].Commit.Author.Date}
			}
		})
		run(func() {
			var x []struct {
				Number    int    `json:"number"`
				Title     string `json:"title"`
				HTMLURL   string `json:"html_url"`
				Draft     bool   `json:"draft"`
				UpdatedAt string `json:"updated_at"`
				User      struct{ Login string }
				Head      struct{ Ref string }
			}
			if e := gh(base+"/pulls?state=open&per_page=30", &x); e != nil {
				setErr("pulls", e)
				return
			}
			for _, y := range x {
				pulls = append(pulls, map[string]any{"number": y.Number, "title": y.Title, "url": y.HTMLURL,
					"draft": y.Draft, "author": y.User.Login, "head": y.Head.Ref, "updatedAt": y.UpdatedAt})
			}
		})
		run(func() {
			// Most recent release including pre-releases (the /latest endpoint skips them).
			var x []struct {
				TagName     string `json:"tag_name"`
				Name        string `json:"name"`
				HTMLURL     string `json:"html_url"`
				Draft       bool   `json:"draft"`
				Prerelease  bool   `json:"prerelease"`
				PublishedAt string `json:"published_at"`
			}
			if e := gh(base+"/releases?per_page=10", &x); e != nil {
				setErr("release", e)
				return
			}
			for _, y := range x {
				if !y.Draft {
					release = map[string]any{"tag": y.TagName, "name": y.Name, "url": y.HTMLURL,
						"prerelease": y.Prerelease, "publishedAt": y.PublishedAt}
					break
				}
			}
		})
	}
	wg.Wait()

	writeJSON(w, map[string]any{
		"project": map[string]any{"id": p.ID, "name": p.Name, "repoUrl": p.RepoURL, "isGithub": p.Owner != ""},
		"local":   local, "repo": repo, "commit": commit, "pulls": pulls, "release": release, "errors": errs,
	})
}
