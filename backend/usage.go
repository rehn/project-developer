package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type tok struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheWrite int64 `json:"cacheWrite"`
	CacheRead  int64 `json:"cacheRead"`
	Messages   int64 `json:"messages"`
}

func (t *tok) add(u struct{ In, Out, CW, CR int64 }) {
	t.Input += u.In
	t.Output += u.Out
	t.CacheWrite += u.CW
	t.CacheRead += u.CR
	t.Messages++
}

var (
	usageMu    sync.Mutex
	usageAt    time.Time
	usageCache map[string]any
)

func claudeProjectsRoot() string {
	base := os.Getenv("CLAUDE_CONFIG_DIR")
	if base == "" {
		h, _ := os.UserHomeDir()
		base = filepath.Join(h, ".claude")
	}
	return filepath.Join(base, "projects")
}

// computeUsage aggregates token usage from Claude Code's local session logs.
func computeUsage() map[string]any {
	byDay, byModel, byProj := map[string]*tok{}, map[string]*tok{}, map[string]*tok{}
	var total, last5h, last7d tok
	seen := map[string]bool{}
	now := time.Now()
	get := func(m map[string]*tok, k string) *tok {
		if m[k] == nil {
			m[k] = &tok{}
		}
		return m[k]
	}
	root := claudeProjectsRoot()
	dirs, _ := os.ReadDir(root)
	for _, d := range dirs {
		files, _ := filepath.Glob(filepath.Join(root, d.Name(), "*.jsonl"))
		for _, f := range files {
			fh, err := os.Open(f)
			if err != nil {
				continue
			}
			rd := bufio.NewReaderSize(fh, 1<<20)
			for {
				line, err := rd.ReadBytes('\n')
				if len(line) > 0 && strings.Contains(string(line), `"usage"`) {
					var o struct {
						Type      string `json:"type"`
						Timestamp string `json:"timestamp"`
						RequestID string `json:"requestId"`
						Message   struct {
							ID    string `json:"id"`
							Model string `json:"model"`
							Usage *struct {
								In  int64 `json:"input_tokens"`
								Out int64 `json:"output_tokens"`
								CW  int64 `json:"cache_creation_input_tokens"`
								CR  int64 `json:"cache_read_input_tokens"`
							} `json:"usage"`
						} `json:"message"`
					}
					if json.Unmarshal(line, &o) == nil && o.Type == "assistant" && o.Message.Usage != nil &&
						o.Message.Model != "<synthetic>" {
						key := o.Message.ID + ":" + o.RequestID
						ts, perr := time.Parse(time.RFC3339, o.Timestamp)
						if perr == nil && (o.Message.ID == "" || !seen[key]) {
							seen[key] = true
							u := struct{ In, Out, CW, CR int64 }{o.Message.Usage.In, o.Message.Usage.Out, o.Message.Usage.CW, o.Message.Usage.CR}
							model := o.Message.Model
							if model == "" {
								model = "unknown"
							}
							get(byDay, ts.UTC().Format("2006-01-02")).add(u)
							get(byModel, model).add(u)
							get(byProj, d.Name()).add(u)
							total.add(u)
							if now.Sub(ts) < 5*time.Hour {
								last5h.add(u)
							}
							if now.Sub(ts) < 7*24*time.Hour {
								last7d.add(u)
							}
						}
					}
				}
				if err != nil {
					break
				}
			}
			fh.Close()
		}
	}
	type named struct {
		Key string
		tok
	}
	flat := func(m map[string]*tok, keyName string, sortBy func(a, b named) bool) []map[string]any {
		l := []named{}
		for k, v := range m {
			l = append(l, named{k, *v})
		}
		sort.Slice(l, func(i, j int) bool { return sortBy(l[i], l[j]) })
		out := []map[string]any{}
		for _, n := range l {
			out = append(out, map[string]any{keyName: n.Key, "input": n.Input, "output": n.Output,
				"cacheWrite": n.CacheWrite, "cacheRead": n.CacheRead, "messages": n.Messages})
		}
		return out
	}
	byOutput := func(a, b named) bool { return a.Output > b.Output }
	projs := flat(byProj, "project", byOutput)
	if len(projs) > 20 {
		projs = projs[:20]
	}
	return map[string]any{
		"total": total, "last5h": last5h, "last7d": last7d,
		"byDay":     flat(byDay, "date", func(a, b named) bool { return a.Key < b.Key }),
		"byModel":   flat(byModel, "model", byOutput),
		"byProject": projs,
	}
}

func handleUsage(w http.ResponseWriter, r *http.Request) {
	usageMu.Lock()
	defer usageMu.Unlock()
	if usageCache == nil || time.Since(usageAt) > 30*time.Second {
		usageCache, usageAt = computeUsage(), time.Now()
	}
	writeJSON(w, usageCache)
}
