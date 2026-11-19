package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// Live plan limits (5-hour / weekly %) as reported by Anthropic for the account
// logged in to Claude Code. This uses an undocumented endpoint, so failures are
// reported to the UI instead of breaking the page. The OAuth token stays on the
// server and is never refreshed here (refreshing could invalidate the CLI's login).

type window struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resetsAt"`
}

type extra struct {
	Enabled       bool    `json:"enabled"`
	MonthlyLimit  float64 `json:"monthlyLimit"`
	Used          float64 `json:"used"`
	Utilization   float64 `json:"utilization"`
	Currency      string  `json:"currency"`
	DecimalPlaces int     `json:"decimalPlaces"`
}

var (
	limitsMu    sync.Mutex
	limitsAt    time.Time
	limitsCache map[string]any
	limitsErr   string    // last upstream failure, cleared on success
	limitsRetry time.Time // no upstream calls before this time
)

func readClaudeOAuth() (token string, expiresAt time.Time, err error) {
	var raw []byte
	if runtime.GOOS == "darwin" {
		raw, err = exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output()
	}
	if len(raw) == 0 { // Linux / fallback: credentials file
		base := os.Getenv("CLAUDE_CONFIG_DIR")
		if base == "" {
			h, _ := os.UserHomeDir()
			base = filepath.Join(h, ".claude")
		}
		raw, err = os.ReadFile(filepath.Join(base, ".credentials.json"))
	}
	if err != nil || len(raw) == 0 {
		return "", time.Time{}, errors.New("Claude Code login not found — run `claude` and sign in")
	}
	var c struct {
		OAuth struct {
			AccessToken string `json:"accessToken"`
			ExpiresAt   int64  `json:"expiresAt"`
		} `json:"claudeAiOauth"`
	}
	if json.Unmarshal(raw, &c) != nil || c.OAuth.AccessToken == "" {
		return "", time.Time{}, errors.New("Claude Code login not in OAuth form (API-key login has no plan limits)")
	}
	return c.OAuth.AccessToken, time.UnixMilli(c.OAuth.ExpiresAt), nil
}

func fetchLimits() (map[string]any, error) {
	tok, exp, err := readClaudeOAuth()
	if err != nil {
		return nil, err
	}
	if time.Now().After(exp) {
		return nil, errors.New("Claude login token expired — run any `claude` command to refresh it")
	}
	req, _ := http.NewRequest("GET", "https://api.anthropic.com/api/oauth/usage", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("User-Agent", "project-developer")
	res, err := ghClient.Do(req)
	if err != nil {
		return nil, errors.New("Could not reach Anthropic: " + err.Error())
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		// Honour Retry-After; a 429 never retries sooner than 5 minutes (the endpoint is strict and
		// hammering it only prolongs the block).
		var wait time.Duration
		if n, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && n > 0 {
			wait = time.Duration(n) * time.Second
		}
		if res.StatusCode == http.StatusTooManyRequests && wait < 5*time.Minute {
			wait = 5 * time.Minute
		}
		if wait > 30*time.Minute {
			wait = 30 * time.Minute
		}
		return nil, &limitsError{"Anthropic usage endpoint returned " + res.Status, wait}
	}
	var shadow struct {
		FiveHour       *window2 `json:"five_hour"`
		SevenDay       *window2 `json:"seven_day"`
		SevenDayOpus   *window2 `json:"seven_day_opus"`
		SevenDaySonnet *window2 `json:"seven_day_sonnet"`
		Extra          *struct {
			Enabled       bool    `json:"is_enabled"`
			MonthlyLimit  float64 `json:"monthly_limit"`
			Used          float64 `json:"used_credits"`
			Utilization   float64 `json:"utilization"`
			Currency      string  `json:"currency"`
			DecimalPlaces int     `json:"decimal_places"`
		} `json:"extra_usage"`
	}
	if err := json.NewDecoder(res.Body).Decode(&shadow); err != nil {
		return nil, errors.New("Unexpected response from Anthropic usage endpoint")
	}
	conv := func(w *window2) *window {
		if w == nil {
			return nil
		}
		return &window{w.Utilization, w.ResetsAt}
	}
	out := map[string]any{
		"fiveHour": conv(shadow.FiveHour), "sevenDay": conv(shadow.SevenDay),
		"sevenDayOpus": conv(shadow.SevenDayOpus), "sevenDaySonnet": conv(shadow.SevenDaySonnet),
	}
	if e := shadow.Extra; e != nil && e.Enabled {
		out["extraUsage"] = extra{e.Enabled, e.MonthlyLimit, e.Used, e.Utilization, e.Currency, e.DecimalPlaces}
	}
	return out, nil
}

type window2 struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

// limitsError is a failed upstream call that also says how long to leave the endpoint alone.
type limitsError struct {
	msg  string
	wait time.Duration
}

func (e *limitsError) Error() string { return e.msg }

const (
	limitsTTL        = 2 * time.Minute // fresh data is reused this long, however many tabs poll
	limitsErrBackoff = time.Minute     // default pause after a failure
)

func handleLimits(w http.ResponseWriter, r *http.Request) {
	limitsMu.Lock()
	defer limitsMu.Unlock()
	now := time.Now()
	// Failures are remembered too: while backing off, the upstream isn't called at all.
	if now.After(limitsRetry) && (limitsCache == nil || now.Sub(limitsAt) > limitsTTL) {
		data, err := fetchLimits()
		if err != nil {
			wait := limitsErrBackoff
			var le *limitsError
			if errors.As(err, &le) && le.wait > 0 {
				wait = le.wait
			}
			limitsErr, limitsRetry = err.Error(), now.Add(wait)
		} else {
			limitsCache, limitsAt, limitsErr, limitsRetry = data, now, "", time.Time{}
		}
	}
	if limitsErr == "" {
		writeJSON(w, limitsCache)
		return
	}
	if limitsCache == nil {
		writeJSON(w, map[string]any{"error": limitsErr}) // 200: the page shows this inline
		return
	}
	// Keep showing the last known values, flagged as stale.
	out := map[string]any{"stale": limitsErr, "staleSince": limitsAt.UTC().Format(time.RFC3339)}
	for k, v := range limitsCache {
		out[k] = v
	}
	writeJSON(w, out)
}
