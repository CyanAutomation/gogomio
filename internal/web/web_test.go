package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestWebUIServingRoot tests that the web UI is properly served at root
func TestWebUIServingRoot(t *testing.T) {
	router := chi.NewRouter()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check status code
	if w.Code != http.StatusOK {
		t.Errorf("status code: got %d, want 200", w.Code)
	}

	// Check content type
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type: got %q, want text/html", ct)
	}

	// Check response body contains expected elements
	body := w.Body.String()
	expectedElements := []string{
		"<!DOCTYPE html>",
		"Motion In Ocean",
		"Live Stream",
		"Settings & Stats",
		"stream-img",
		"brightness",
		"contrast",
		"saturation",
	}

	for _, element := range expectedElements {
		if !strings.Contains(body, element) {
			t.Errorf("Expected element %q not found in response body", element)
		}
	}
}

// TestWebUIContentLength tests that Content-Length header is reasonable
func TestWebUIContentLength(t *testing.T) {
	router := chi.NewRouter()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	// HTML file should be at least 5KB (account for CSS and JS)
	if len(body) < 5000 {
		t.Errorf("Response body too small: got %d bytes, want > 5000", len(body))
	}
}

// TestWebUIContainsJavaScript tests that JavaScript is included
func TestWebUIContainsJavaScript(t *testing.T) {
	router := chi.NewRouter()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	jsElements := []string{
		"StreamController",
		"fetch",
		"api/settings",
		"api/config",
		"stream.mjpg",
	}

	for _, elem := range jsElements {
		if !strings.Contains(body, elem) {
			t.Errorf("Expected JavaScript element %q not found", elem)
		}
	}
}

// TestWebUIContainsStyling tests that CSS styling is included
func TestWebUIContainsStyling(t *testing.T) {
	router := chi.NewRouter()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	cssElements := []string{
		"background:",
		"display:",
		"flex",
		"grid",
		"border-radius:",
	}

	for _, elem := range cssElements {
		if !strings.Contains(body, elem) {
			t.Errorf("Expected CSS element %q not found", elem)
		}
	}
}

// TestWebUINotFoundPath tests that non-root paths return 404
func TestWebUINotFoundPath(t *testing.T) {
	router := chi.NewRouter()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/invalid-path", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status code: got %d, want 404", w.Code)
	}
}

// TestWebUICacheHeaders tests that caching headers are present
func TestWebUICacheHeaders(t *testing.T) {
	router := chi.NewRouter()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	cacheControl := w.Header().Get("Cache-Control")
	if cacheControl == "" {
		t.Error("Cache-Control header missing")
	}

	if !strings.Contains(cacheControl, "max-age") {
		t.Errorf("Cache-Control missing max-age: got %q", cacheControl)
	}
}
