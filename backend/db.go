package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var (
	db          *sql.DB
	dataDir     string
	projectsDir string
	gcm         cipher.AEAD
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func initDB() error {
	var err error
	dataDir, err = filepath.Abs(env("DATA_DIR", "data"))
	if err != nil {
		return err
	}
	if h := os.Getenv("HOME"); h != "" { // e.g. HOME=/data/home so Claude's login and logs persist on the volume
		os.MkdirAll(h, 0o755)
	}
	projectsDir = filepath.Join(dataDir, "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		return err
	}
	if db, err = sql.Open("sqlite", filepath.Join(dataDir, "app.db")+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"); err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, created_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS sessions (token_hash TEXT PRIMARY KEY, user_id INTEGER NOT NULL, expires_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS projects (id INTEGER PRIMARY KEY, name TEXT UNIQUE NOT NULL, repo_url TEXT NOT NULL, owner TEXT, repo TEXT, dir TEXT NOT NULL, created_at INTEGER NOT NULL);
`)
	if err != nil {
		return err
	}
	// AES-256-GCM key for secrets at rest, kept in the data dir.
	keyFile := filepath.Join(dataDir, "secret.key")
	key, err := os.ReadFile(keyFile)
	if err != nil {
		key = make([]byte, 32)
		rand.Read(key)
		if err := os.WriteFile(keyFile, key, 0o600); err != nil {
			return err
		}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err = cipher.NewGCM(block)
	return err
}

func encrypt(s string) string {
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(s), nil))
}

func decrypt(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) < gcm.NonceSize() {
		return "", errors.New("bad ciphertext")
	}
	p, err := gcm.Open(nil, b[:gcm.NonceSize()], b[gcm.NonceSize():], nil)
	return string(p), err
}

func setSetting(k, v string) {
	db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, v)
}
func deleteSetting(k string) { db.Exec(`DELETE FROM settings WHERE key=?`, k) }

func githubToken() string {
	var v string
	if db.QueryRow(`SELECT value FROM settings WHERE key='github_token'`).Scan(&v) != nil {
		return ""
	}
	t, _ := decrypt(v)
	return t
}
