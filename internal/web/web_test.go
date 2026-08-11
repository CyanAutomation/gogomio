package web

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestWebUIIncludesBootstrapScriptAndPublicAPIRoutes verifies stable,
// user-observable root-page requirements without pinning exact JS source text.
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

	// Validate references to public API routes consumed by the UI.
	publicRoutes := []string{
		"/api/config",
		"/api/stream/stop",
		"/api/diagnostics",
	}
	for _, route := range publicRoutes {
		if !strings.Contains(body, route) {
			t.Errorf("missing public route reference %q in root HTML", route)
		}
	}

	// Keep bootstrap verification stable by asserting bootstrap-specific behavior
	// instead of matching entire script source text.
	if !strings.Contains(body, "new StreamController();") {
		t.Error("missing StreamController bootstrap initialization in root HTML")
	}
	if !strings.Contains(body, `id="diagnostics-btn" onclick="openDiagnosticsModal()"`) {
		t.Error("missing stable element-to-action linkage for diagnostics button")
	}
}

// TestStreamImageFillsContainerWithoutCropping protects the stream presentation
// from regressions that either leave unused image dimensions or crop camera frames.
func TestStreamImageFillsContainerWithoutCropping(t *testing.T) {
	page, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatalf("read embedded index.html: %v", err)
	}
	match := regexp.MustCompile(`(?s)#stream-img\s*\{([^}]*)\}`).FindSubmatch(page)
	if match == nil {
		t.Fatal("stream image CSS rule is missing")
	}
	styles := string(match[1])
	streamImageStyles := []string{
		"width: 100%;",
		"height: 100%;",
		"object-fit: contain;",
	}

	for _, style := range streamImageStyles {
		if !strings.Contains(styles, style) {
			t.Errorf("stream image CSS missing %q", style)
		}
	}

	if strings.Contains(styles, "object-fit: cover;") {
		t.Error("stream image CSS uses object-fit: cover, which crops frames")
	}
}

// TestStreamContainerUsesConfiguredResolution verifies the UI derives the
// container proportions from camera dimensions instead of assuming widescreen.
func TestStreamContainerUsesConfiguredResolution(t *testing.T) {
	pageBytes, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatalf("read embedded index.html: %v", err)
	}
	page := string(pageBytes)

	if strings.Contains(page, "aspect-ratio: 16 / 9;") {
		t.Error("stream container hard-codes a 16:9 aspect ratio")
	}
	if !strings.Contains(page, "style.aspectRatio") ||
		!strings.Contains(page, "`${config.resolution[0]} / ${config.resolution[1]}`") {
		t.Error("stream container does not derive its aspect ratio from the configured resolution")
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

// TestWebUICacheHeaders verifies the root page has the expected cache policy directives and TTL.
func TestWebUICacheHeaders(t *testing.T) {
	router := chi.NewRouter()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code: got %d, want 200", w.Code)
	}

	cacheControl := w.Header().Get("Cache-Control")
	if !strings.Contains(cacheControl, "max-age=3600") {
		t.Errorf("cache-control: got %q, want directive %q", cacheControl, "max-age=3600")
	}
	if !strings.Contains(cacheControl, "public") {
		t.Errorf("cache-control: got %q, missing directive %q", cacheControl, "public")
	}
	if strings.Contains(cacheControl, "no-cache") {
		t.Errorf("cache-control: got %q, should not include %q", cacheControl, "no-cache")
	}
}

func TestMioStaticAssetsAreServed(t *testing.T) {
	router := chi.NewRouter()
	RegisterStaticFiles(router)

	tests := []struct {
		name string
		path string
	}{
		{name: "idle", path: "/static/mio/mio_pose_idle.png"},
		{name: "sleeping", path: "/static/mio/mio_pose_sleeping.png"},
		{name: "concerned", path: "/static/mio/mio_pose_concerned.png"},
		{name: "happy", path: "/static/mio/mio_pose_happy.png"},
		{name: "worried", path: "/static/mio/mio_pose_worried.png"},
		{name: "curious", path: "/static/mio/mio_pose_curious.png"},
		{name: "angry", path: "/static/mio/mio_pose_angry.png"},
		{name: "looking", path: "/static/mio/mio_pose_looking.png"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status code: got %d, want 200", w.Code)
			}
			if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
				t.Errorf("Content-Type: got %q, want image/png", got)
			}

			body := w.Body.Bytes()
			if len(body) == 0 {
				t.Fatal("response body is empty")
			}
			if _, err := png.Decode(bytes.NewReader(body)); err != nil {
				t.Errorf("decode response as PNG: %v", err)
			}

			cacheControl := w.Header().Get("Cache-Control")
			if !strings.Contains(cacheControl, "public") || !strings.Contains(cacheControl, "max-age=86400") {
				t.Errorf("Cache-Control: got %q, want public and max-age=86400", cacheControl)
			}
			if strings.Contains(cacheControl, "no-cache") {
				t.Errorf("Cache-Control: got %q, should not include no-cache", cacheControl)
			}
		})
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
