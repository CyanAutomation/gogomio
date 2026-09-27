// Package web provides the embedded web UI for GoGoMio.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
)

//go:embed *.html *.js
var webFS embed.FS

//go:embed mio
var mioFS embed.FS

const immutableCacheControl = "public, max-age=31536000, immutable"

type successfulFileCacheWriter struct {
	http.ResponseWriter
	cacheSuccessfulResponse bool
	wroteHeader             bool
}

func (w *successfulFileCacheWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if statusCode == http.StatusOK && w.cacheSuccessfulResponse {
		w.Header().Set("Cache-Control", immutableCacheControl)
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *successfulFileCacheWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

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
	mioHandler := http.StripPrefix("/static/mio/", http.FileServer(http.FS(mioSubFS)))
	r.Handle("/static/mio/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Resolve the name the same way as net/http's file server: clean it from
		// a synthetic root so that .. elements cannot escape mioSubFS.
		name := strings.TrimPrefix(req.URL.Path, "/static/mio/")
		name = strings.TrimPrefix(path.Clean("/"+name), "/")
		info, statErr := fs.Stat(mioSubFS, name)
		cacheSuccessfulResponse := statErr == nil && !info.IsDir()

		mioHandler.ServeHTTP(&successfulFileCacheWriter{
			ResponseWriter:          w,
			cacheSuccessfulResponse: cacheSuccessfulResponse,
		}, req)
	}))
}
