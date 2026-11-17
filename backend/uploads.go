package main

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Images pasted or dropped into the web terminal. They are stored outside the project
// checkouts (so they never end up in a commit) and their path is typed into the Claude
// prompt, where Claude Code reads them like any local image file.

const (
	maxUpload   = 20 << 20
	uploadTTL   = 7 * 24 * time.Hour
	uploadsName = "uploads"
)

var imageExt = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+1<<20)
	f, _, err := r.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "too large") {
			writeErr(w, 413, "Image is larger than 20 MB")
			return
		}
		writeErr(w, 400, "Expected an image in form field \"file\"")
		return
	}
	defer f.Close()

	// Trust the bytes, not the client's filename or Content-Type.
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	ext, ok := imageExt[http.DetectContentType(head[:n])]
	if !ok {
		writeErr(w, 415, "Only PNG, JPEG, GIF and WebP images are supported")
		return
	}

	dir := filepath.Join(dataDir, uploadsName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(w, err)
		return
	}
	pruneUploads(dir)

	rnd := make([]byte, 4)
	rand.Read(rnd)
	path := filepath.Join(dir, time.Now().Format("20060102-150405")+"-"+hex.EncodeToString(rnd)+ext)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		fail(w, err)
		return
	}
	_, err = io.Copy(out, io.MultiReader(strings.NewReader(string(head[:n])), f))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		fail(w, err)
		return
	}
	writeJSON(w, map[string]string{"path": path})
}

// pruneUploads removes uploads older than uploadTTL; they only matter for the prompt they were pasted into.
func pruneUploads(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-uploadTTL)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() && info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
