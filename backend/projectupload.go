package main

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Files and folders uploaded from the browser into <project>/.uploaded, which is created on
// demand. Keeping them in one folder makes them easy to gitignore.

const (
	projectUploadDir = ".uploaded"
	maxProjectUpload = 500 << 20
)

// POST /api/projects/{id}/upload (multipart). Each part's field name is the URL-encoded path
// of the file relative to .uploaded/ (the browser's filename is reduced to its base name by
// Go's multipart reader, so it can't carry folder structure). Existing files are overwritten.
func handleProjectUpload(w http.ResponseWriter, r *http.Request) {
	p, err := getProject(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProjectUpload)
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, 400, "Expected a multipart upload")
		return
	}

	saved := []string{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if strings.Contains(err.Error(), "too large") {
				writeErr(w, 413, "Upload is larger than 500 MB")
				return
			}
			writeErr(w, 400, "Malformed upload")
			return
		}
		rel, err := url.PathUnescape(part.FormName())
		rel = strings.Trim(filepath.ToSlash(filepath.Clean("/"+rel)), "/")
		if err != nil || rel == "" || hasGitSegment(rel) {
			part.Close()
			writeErr(w, 400, "Invalid file path")
			return
		}
		dest, err := resolveIn(p, projectUploadDir+"/"+rel)
		if err != nil {
			part.Close()
			fail(w, err)
			return
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			part.Close()
			fail(w, err)
			return
		}
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			part.Close()
			fail(w, err)
			return
		}
		_, err = io.Copy(out, part)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(dest)
			if strings.Contains(err.Error(), "too large") {
				writeErr(w, 413, "Upload is larger than 500 MB")
				return
			}
			fail(w, err)
			return
		}
		saved = append(saved, rel)
	}
	if len(saved) == 0 {
		writeErr(w, 400, "No files in upload")
		return
	}
	writeJSON(w, map[string]any{"dir": projectUploadDir, "count": len(saved), "files": saved})
}
