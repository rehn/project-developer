package main

import (
	"errors"
	"net/http"
	"sort"
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
	merged := []map[string]any{}
	runs := []map[string]any{}
	tags := []map[string]any{}
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
			// The three most recently merged PRs. The API can't filter on merged, so fetch the
			// latest closed ones (newest activity first) and keep those that were merged.
			var x []struct {
				Number   int    `json:"number"`
				Title    string `json:"title"`
				HTMLURL  string `json:"html_url"`
				MergedAt string `json:"merged_at"`
				User     struct{ Login string }
				Head     struct{ Ref string }
			}
			if e := gh(base+"/pulls?state=closed&sort=updated&direction=desc&per_page=30", &x); e != nil {
				setErr("merged", e)
				return
			}
			sort.SliceStable(x, func(i, j int) bool { return x[i].MergedAt > x[j].MergedAt })
			for _, y := range x {
				if y.MergedAt == "" || len(merged) == 3 {
					continue
				}
				merged = append(merged, map[string]any{"number": y.Number, "title": y.Title, "url": y.HTMLURL,
					"author": y.User.Login, "head": y.Head.Ref, "mergedAt": y.MergedAt})
			}
		})
		run(func() {
			// GitHub Actions runs that haven't finished yet (queued, waiting, in_progress, …).
			// One call for the newest runs, filtered here, instead of one call per status.
			var x struct {
				WorkflowRuns []struct {
					ID           int64  `json:"id"`
					Name         string `json:"name"`
					DisplayTitle string `json:"display_title"`
					HTMLURL      string `json:"html_url"`
					Status       string `json:"status"`
					Event        string `json:"event"`
					HeadBranch   string `json:"head_branch"`
					RunStartedAt string `json:"run_started_at"`
					Actor        struct{ Login string }
				} `json:"workflow_runs"`
			}
			if e := gh(base+"/actions/runs?per_page=30", &x); e != nil {
				setErr("runs", e)
				return
			}
			for _, y := range x.WorkflowRuns {
				if y.Status != "completed" {
					runs = append(runs, map[string]any{"id": y.ID, "name": y.Name, "title": y.DisplayTitle,
						"url": y.HTMLURL, "status": y.Status, "event": y.Event, "branch": y.HeadBranch,
						"startedAt": y.RunStartedAt, "actor": y.Actor.Login})
				}
			}
		})
		run(func() {
			// GitHub lists tags by name, not version or date, so sort by semver here.
			var x []struct {
				Name   string
				Commit struct{ SHA string }
			}
			if e := gh(base+"/tags?per_page=100", &x); e != nil {
				setErr("tags", e)
				return
			}
			sort.SliceStable(x, func(i, j int) bool { return versionLess(x[j].Name, x[i].Name) })
			for i, y := range x {
				if i == 8 {
					break
				}
				tags = append(tags, map[string]any{"name": y.Name, "sha": y.Commit.SHA,
					"url": "https://github.com/" + p.Owner + "/" + p.Repo + "/releases/tag/" + y.Name})
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
		"local":   local, "repo": repo, "commit": commit, "pulls": pulls, "merged": merged, "runs": runs, "tags": tags, "release": release, "errors": errs,
	})
}
