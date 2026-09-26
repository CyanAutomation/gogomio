// Package web provides the embedded web UI for GoGoMio.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

//go:embed *.html *.js
var webFS embed.FS

//go:embed mio
var mioFS embed.FS

const immutableCacheControl = "public, max-age=31536000, immutable"

func etag(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func etagMatches(header, tag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == tag || strings.TrimPrefix(candidate, "W/") == tag {
			return true
		}
	}
	return false
}

// RegisterStaticFiles registers static file routes with the router.
func RegisterStaticFiles(r *http.ServeMux) {
	// Serve index.html for root path
	r.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		data, err := webFS.ReadFile("index.html")
		if err != nil {
			log.Printf("Error reading index.html: %v", err)
			http.Error(w, "Failed to load UI", http.StatusInternalServerError)
			return
		}
		tag := etag(data)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", tag)
		if etagMatches(r.Header.Get("If-None-Match"), tag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if _, err := w.Write(data); err != nil {
			// Client likely disconnected
			_ = err
		}
	})

	r.HandleFunc("/static/aspect-ratio.js", func(w http.ResponseWriter, r *http.Request) {
		data, err := webFS.ReadFile("aspect-ratio.js")
		if err != nil {
			http.Error(w, "Failed to load UI script", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", immutableCacheControl)
		_, _ = w.Write(data)
	})

	// Serve MIO mascot images at /static/mio/
	mioSubFS, err := fs.Sub(mioFS, "mio")
	if err != nil {
		log.Printf("Error creating mio sub-filesystem: %v", err)
		return
	}
	r.Handle("/static/mio/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		name := strings.TrimPrefix(req.URL.Path, "/static/mio/")
		data, err := fs.ReadFile(mioSubFS, name)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(name)))
		w.Header().Set("Cache-Control", immutableCacheControl)
		_, _ = w.Write(data)
	}))
}
