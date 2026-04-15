package web

import (
	"embed"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed *.html
var webFS embed.FS

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
}
