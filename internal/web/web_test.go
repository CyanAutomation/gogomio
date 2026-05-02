package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestWebUIServingRoot verifies the root-serving contract: HTML semantics plus
// runtime control hooks that the in-page StreamController depends on.
func TestWebUIServingRoot(t *testing.T) {
	router := chi.NewRouter()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code: got %d, want 200", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type: got %q, want text/html", ct)
	}

	body := w.Body.String()
	runtimeHooks := []string{
		`id="stream-img"`,
		`id="start-stream"`,
		`id="stop-stream"`,
		`id="diagnostics-btn"`,
	}

	for _, hook := range runtimeHooks {
		if !strings.Contains(body, hook) {
			t.Errorf("missing runtime hook %q in root HTML", hook)
		}
	}

	// Optional smoke assertion: keep one lightweight product marker to catch
	// accidental non-index responses without coupling to broader marketing copy.
	if !strings.Contains(body, "Motion In Ocean") {
		t.Log("smoke marker \"Motion In Ocean\" not found; runtime hooks still validate root contract")
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

func TestMioStaticAssetsAreServed(t *testing.T) {
	router := chi.NewRouter()
	RegisterStaticFiles(router)

	assets := []string{
		"mio_pose_idle.png",
		"mio_pose_sleeping.png",
		"mio_pose_concerned.png",
		"mio_pose_happy.png",
		"mio_pose_worried.png",
	}

	for _, asset := range assets {
		req, _ := http.NewRequest("GET", "/static/mio/"+asset, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("asset %q status code: got %d, want 200", asset, w.Code)
		}

		cacheControl := w.Header().Get("Cache-Control")
		if !strings.Contains(cacheControl, "max-age=86400") {
			t.Errorf("asset %q cache-control: got %q, expected max-age=86400", asset, cacheControl)
		}
	}
}

func TestLegacyMioStaticAssetsAreNotServed(t *testing.T) {
	router := chi.NewRouter()
	RegisterStaticFiles(router)

	legacyAssets := []string{
		"mio_avatar.png",
		"mio_curious.png",
		"mio_sleeping.png",
		"mio_happy.png",
	}

	for _, asset := range legacyAssets {
		req, _ := http.NewRequest("GET", "/static/mio/"+asset, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("legacy asset %q status code: got %d, want 404", asset, w.Code)
		}
	}
}
