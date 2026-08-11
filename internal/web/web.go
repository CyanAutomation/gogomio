// Package web provides the embedded web UI for GoGoMio.
package web

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed *.html *.js
var webFS embed.FS

//go:embed mio
var mioFS embed.FS

// RegisterStaticFiles registers static file routes with the router.
func RegisterStaticFiles(r *chi.Mux) {
	// Serve index.html for root path
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")

		data, err := webFS.ReadFile("index.html")
		if err != nil {
			log.Printf("Error reading index.html: %v", err)
			http.Error(w, "Failed to load UI", http.StatusInternalServerError)
			return
		}
		if _, err := w.Write(data); err != nil {
			// Client likely disconnected
			_ = err
		}
	})

	r.HandleFunc("/static/aspect-ratio.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		data, err := webFS.ReadFile("aspect-ratio.js")
		if err != nil {
			http.Error(w, "Failed to load UI script", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(data)
	})

	// Serve MIO mascot images at /static/mio/
	mioSubFS, err := fs.Sub(mioFS, "mio")
	if err != nil {
		log.Printf("Error creating mio sub-filesystem: %v", err)
		return
	}
	mioHandler := http.FileServer(http.FS(mioSubFS))
	r.Handle("/static/mio/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.StripPrefix("/static/mio/", mioHandler).ServeHTTP(w, req)
	}))
}
