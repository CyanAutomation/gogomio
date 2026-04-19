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
		"stream-spinner-image",
		"stream-idle-image",
		"brightness",
		"contrast",
		"saturation",
		"/static/mio/mio_pose_idle.png",
		"/static/mio/mio_pose_concerned.png",
		"/static/mio/mio_pose_sleeping.png",
	}

	for _, element := range expectedElements {
		if !strings.Contains(body, element) {
			t.Errorf("Expected element %q not found in response body", element)
		}
	}
}

// TestWebUISemanticStructureAndFunctionalReferences tests stable semantic landmarks
// and user-visible functional selectors/routes.
func TestWebUISemanticStructureAndFunctionalReferences(t *testing.T) {
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

	// Stable top-level landmarks should remain present even if formatting is minified.
	requiredLandmarks := []string{"<header", "<footer"}
	for _, landmark := range requiredLandmarks {
		if !strings.Contains(body, landmark) {
			t.Errorf("Expected landmark %q not found in response body", landmark)
		}
	}

	// Validate user-facing functionality hooks via IDs and API/stream routes.
	requiredReferences := []string{
		`id="stream-img"`,
		`id="stream-spinner-image"`,
		`id="stream-idle-image"`,
		`id="start-stream"`,
		`id="stop-stream"`,
		`id="save-settings"`,
		`id="reset-settings"`,
		`id="diagnostics-btn"`,
		`id="brightness"`,
		`id="contrast"`,
		`id="saturation"`,
		"/stream.mjpg",
		"/api/settings",
		"/api/config",
		"/api/diagnostics",
	}

	for _, ref := range requiredReferences {
		if !strings.Contains(body, ref) {
			t.Errorf("Expected functional reference %q not found in response body", ref)
		}
	}
}

// TestWebUIIncludesBootstrapScriptAndPublicAPIRoutes ensures the page ships a
// stable script entrypoint and references the public routes used by the UI.
func TestWebUIIncludesBootstrapScriptAndPublicAPIRoutes(t *testing.T) {
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
	requiredReferences := []string{
		"<script>",
		"</script>",
		`document.addEventListener("DOMContentLoaded", () => {`,
		"/stream.mjpg",
		"/api/settings",
		"/api/config",
		"/api/diagnostics",
	}

	for _, ref := range requiredReferences {
		if !strings.Contains(body, ref) {
			t.Errorf("Expected script/bootstrap reference %q not found", ref)
		}
	}
}

// TestWebUIContainsStyling tests that required UI sections/components render
// and that stylesheet linkage is present for the inline style contract.
func TestWebUIContainsStyling(t *testing.T) {
	router := chi.NewRouter()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	requiredUIElements := []string{
		"<header",
		"Live Stream",
		"Settings & Stats",
		`id="stream-img"`,
		`id="stream-container"`,
		`id="start-stream"`,
		`id="stop-stream"`,
		`id="save-settings"`,
		`id="reset-settings"`,
		`id="diagnostics-btn"`,
		`id="brightness"`,
		`id="contrast"`,
		`id="saturation"`,
		"/static/mio/mio_pose_idle.png",
		"/static/mio/mio_pose_concerned.png",
		"/static/mio/mio_pose_sleeping.png",
	}

	for _, elem := range requiredUIElements {
		if !strings.Contains(body, elem) {
			t.Errorf("Expected UI element %q not found", elem)
		}
	}

	// Keep a minimal stylesheet linkage check without asserting styling details.
	if !strings.Contains(body, "<style>") {
		t.Error("Expected inline stylesheet linkage not found")
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
