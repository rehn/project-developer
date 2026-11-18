package main

import (
	"log"
	"net/http"
	"time"
)

// requestTimeout answers 503 (and logs it) when a plain GET takes longer than d, so a stuck
// handler (e.g. waiting on the single SQLite connection) shows up instead of hanging silently.
// The terminal WebSocket is long-lived and mutating requests (clone, merge, upload) may
// legitimately be slow, so only other GETs are covered.
func requestTimeout(d time.Duration, next http.Handler) http.Handler {
	guarded := http.TimeoutHandler(next, d, `{"error":"Request timed out"}`)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path == "/api/terminal" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		guarded.ServeHTTP(&timeoutLogger{ResponseWriter: w, r: r, start: start}, r)
	})
}

type timeoutLogger struct {
	http.ResponseWriter
	r     *http.Request
	start time.Time
}

func (t *timeoutLogger) WriteHeader(code int) {
	if code == http.StatusServiceUnavailable && time.Since(t.start) >= 100*time.Millisecond {
		log.Printf("request timed out: %s %s after %s", t.r.Method, t.r.URL.Path, time.Since(t.start).Round(time.Second))
	}
	t.ResponseWriter.WriteHeader(code)
}
