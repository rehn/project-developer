package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const cookieName = "pd_session"
const sessionTTL = 14 * 24 * time.Hour

type ctxKey struct{}

func sha(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func userFromRequest(r *http.Request) (id int64, name string, ok bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return
	}
	err = db.QueryRow(`SELECT u.id,u.username FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>?`,
		sha(c.Value), time.Now().Unix()).Scan(&id, &name)
	return id, name, err == nil
}

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _, ok := userFromRequest(r)
		if !ok {
			writeErr(w, 401, "Not authenticated")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

func startSession(w http.ResponseWriter, userID int64) {
	b := make([]byte, 32)
	rand.Read(b)
	tok := hex.EncodeToString(b)
	db.Exec(`INSERT INTO sessions VALUES(?,?,?)`, sha(tok), userID, time.Now().Add(sessionTTL).Unix())
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: env("COOKIE_SECURE", "") == "true", MaxAge: int(sessionTTL.Seconds()),
	})
}

// login throttle: 10 failures / 15 min / IP
var (
	failMu sync.Mutex
	fails  = map[string][2]int64{} // ip -> {count, resetUnix}
)

func throttled(ip string) bool {
	failMu.Lock()
	defer failMu.Unlock()
	f := fails[ip]
	return f[1] > time.Now().Unix() && f[0] >= 10
}
func recordFail(ip string) {
	failMu.Lock()
	defer failMu.Unlock()
	f := fails[ip]
	if f[1] < time.Now().Unix() {
		f = [2]int64{0, time.Now().Add(15 * time.Minute).Unix()}
	}
	f[0]++
	fails[ip] = f
}

func userCount() (n int) { db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); return }

func handleMe(w http.ResponseWriter, r *http.Request) {
	_, name, ok := userFromRequest(r)
	var user any
	if ok {
		user = map[string]any{"username": name}
	}
	writeJSON(w, map[string]any{"user": user, "needsSetup": userCount() == 0})
}

type creds struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// First-run only: creates the initial account.
func handleSetup(w http.ResponseWriter, r *http.Request) {
	if userCount() > 0 {
		writeErr(w, 403, "Already set up")
		return
	}
	var c creds
	if !readJSON(w, r, &c) {
		return
	}
	if len(c.Username) < 2 || len(c.Password) < 8 {
		writeErr(w, 400, "Username ≥2 chars and password ≥8 chars required")
		return
	}
	h, _ := bcrypt.GenerateFromPassword([]byte(c.Password), bcrypt.DefaultCost)
	res, err := db.Exec(`INSERT INTO users(username,password_hash,created_at) VALUES(?,?,?)`, c.Username, string(h), time.Now().Unix())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	startSession(w, id)
	writeJSON(w, map[string]bool{"ok": true})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if throttled(ip) {
		writeErr(w, 429, "Too many attempts, try later")
		return
	}
	var c creds
	if !readJSON(w, r, &c) {
		return
	}
	var id int64
	var hash string
	err := db.QueryRow(`SELECT id,password_hash FROM users WHERE username=?`, c.Username).Scan(&id, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(c.Password)) != nil {
		recordFail(ip)
		writeErr(w, 401, "Invalid username or password")
		return
	}
	startSession(w, id)
	writeJSON(w, map[string]bool{"ok": true})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		db.Exec(`DELETE FROM sessions WHERE token_hash=?`, sha(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1})
	writeJSON(w, map[string]bool{"ok": true})
}

func handlePassword(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(ctxKey{}).(int64)
	var b struct{ Current, Next string }
	if !readJSON(w, r, &b) {
		return
	}
	var hash string
	db.QueryRow(`SELECT password_hash FROM users WHERE id=?`, uid).Scan(&hash)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(b.Current)) != nil {
		writeErr(w, 401, "Wrong current password")
		return
	}
	if len(b.Next) < 8 {
		writeErr(w, 400, "New password ≥8 chars")
		return
	}
	h, _ := bcrypt.GenerateFromPassword([]byte(b.Next), bcrypt.DefaultCost)
	db.Exec(`UPDATE users SET password_hash=? WHERE id=?`, string(h), uid)
	writeJSON(w, map[string]bool{"ok": true})
}
