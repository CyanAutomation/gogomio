package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image/png"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// TestWebUIIncludesBootstrapScriptAndPublicAPIRoutes verifies stable,
// user-observable root-page requirements without pinning exact JS source text.
func TestWebUIIncludesBootstrapScriptAndPublicAPIRoutes(t *testing.T) {
	router := http.NewServeMux()
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
		`<script src="/static/aspect-ratio.js?v=`,
		`<script src="/static/diagnostics-dialog.js?v=`,
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

}

func TestDashboardResponsiveLayoutContracts(t *testing.T) {
	// Contract: TC-WEB-01 (docs/testing/test-contracts.md).
	index, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	css := strings.SplitN(string(index), "</style>", 2)[0]

	mobileStyles := cssBlockBody(t, css, "@media (max-width: 480px)")
	if !strings.Contains(mobileStyles, ".container {") || !strings.Contains(mobileStyles, "padding: var(--spacing-lg)") {
		t.Error("small-screen layout should reduce the dashboard container padding")
	}
	if !strings.Contains(mobileStyles, ".subtitle {") || !strings.Contains(mobileStyles, "font-size: var(--font-size-xs)") {
		t.Error("small-screen layout should reduce subtitle text size")
	}

	reducedMotionStyles := cssBlockBody(t, css, "@media (prefers-reduced-motion: reduce)")
	for _, property := range []string{"animation-duration: 0.01ms", "transition-duration: 0.01ms"} {
		if !strings.Contains(reducedMotionStyles, property) {
			t.Errorf("reduced-motion support is missing %q", property)
		}
	}

	secondaryButton := cssRuleBody(t, css, ".button--secondary")
	if strings.Contains(secondaryButton, "flex:") {
		t.Error("button variant must not determine its layout size")
	}
	streamActions := cssRuleBody(t, css, ".stream-controls > .button")
	if !strings.Contains(streamActions, "flex: 1") {
		t.Error("only the stream action row should grow its paired buttons")
	}

	streamViewer := cssRuleBody(t, css, ".stream-viewer")
	if !strings.Contains(streamViewer, "aspect-ratio: 4 / 3") {
		t.Error("stream viewer should retain a fluid 4:3 aspect ratio")
	}
	if strings.Contains(streamViewer, "min-height: 360px") {
		t.Error("stream viewer must not force a wider layout on small screens")
	}
}

func cssBlockBody(t *testing.T, css, selector string) string {
	t.Helper()
	marker := selector + " {"
	start := strings.Index(css, marker)
	if start < 0 {
		t.Fatalf("missing CSS block %q", selector)
	}

	openBrace := start + strings.IndexByte(css[start:], '{')
	depth := 0
	for i := openBrace; i < len(css); i++ {
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return css[openBrace+1 : i]
			}
		}
	}
	t.Fatalf("unterminated CSS block %q", selector)
	return ""
}

func cssRuleBody(t *testing.T, css, selector string) string {
	t.Helper()
	marker := selector + " {"
	start := strings.Index(css, marker)
	if start < 0 {
		t.Fatalf("missing CSS component rule %q", selector)
	}
	declarations := css[start+len(marker):]
	end := strings.IndexByte(declarations, '}')
	if end < 0 {
		t.Fatalf("unterminated CSS component rule %q", selector)
	}
	return declarations[:end]
}

// TestWebUINotFoundPath tests that non-root paths return 404
func TestWebUINotFoundPath(t *testing.T) {
	router := http.NewServeMux()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/invalid-path", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status code: got %d, want 404", w.Code)
	}
}

// TestWebUIRevalidation verifies that the root document is always revalidated.
func TestWebUIRevalidation(t *testing.T) {
	router := http.NewServeMux()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code: got %d, want 200", w.Code)
	}

	cacheControl := w.Header().Get("Cache-Control")
	if cacheControl != "no-cache" {
		t.Errorf("Cache-Control: got %q, want %q", cacheControl, "no-cache")
	}

	data, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	wantETag := etag(data)
	if got := w.Header().Get("ETag"); got != wantETag {
		t.Errorf("ETag: got %q, want %q", got, wantETag)
	}

	conditionalReq, _ := http.NewRequest("GET", "/", nil)
	conditionalReq.Header.Set("If-None-Match", wantETag)
	conditionalResponse := httptest.NewRecorder()
	router.ServeHTTP(conditionalResponse, conditionalReq)
	if conditionalResponse.Code != http.StatusNotModified {
		t.Errorf("conditional status code: got %d, want 304", conditionalResponse.Code)
	}
	if conditionalResponse.Body.Len() != 0 {
		t.Errorf("conditional response body: got %d bytes, want none", conditionalResponse.Body.Len())
	}
}

func TestWebUIUsesContentVersionedAssetURLs(t *testing.T) {
	index, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}

	assets := []struct {
		url  string
		file string
		fs   fs.ReadFileFS
	}{
		{url: "/static/aspect-ratio.js", file: "aspect-ratio.js", fs: webFS},
		{url: "/static/diagnostics-dialog.js", file: "diagnostics-dialog.js", fs: webFS},
		{url: "/static/mio/mio_pose_idle.png", file: "mio/mio_pose_idle.png", fs: mioFS},
		{url: "/static/mio/mio_pose_sleeping.png", file: "mio/mio_pose_sleeping.png", fs: mioFS},
		{url: "/static/mio/mio_pose_curious.png", file: "mio/mio_pose_curious.png", fs: mioFS},
		{url: "/static/mio/mio_pose_happy.png", file: "mio/mio_pose_happy.png", fs: mioFS},
		{url: "/static/mio/mio_pose_concerned.png", file: "mio/mio_pose_concerned.png", fs: mioFS},
		{url: "/static/mio/mio_pose_angry.png", file: "mio/mio_pose_angry.png", fs: mioFS},
		{url: "/static/mio/mio_pose_looking.png", file: "mio/mio_pose_looking.png", fs: mioFS},
	}
	for _, asset := range assets {
		data, err := asset.fs.ReadFile(asset.file)
		if err != nil {
			t.Fatalf("read %s: %v", asset.file, err)
		}
		sum := sha256.Sum256(data)
		want := asset.url + "?v=" + hex.EncodeToString(sum[:])
		if !bytes.Contains(index, []byte(want)) {
			t.Errorf("index.html does not reference content-versioned URL %q", want)
		}
	}
}

func TestMioStaticAssetsAreServed(t *testing.T) {
	router := http.NewServeMux()
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

			if cacheControl := w.Header().Get("Cache-Control"); cacheControl != immutableCacheControl {
				t.Errorf("Cache-Control: got %q, want %q", cacheControl, immutableCacheControl)
			}
		})
	}
}

func TestUIScriptsAreImmutable(t *testing.T) {
	router := http.NewServeMux()
	RegisterStaticFiles(router)

	for _, asset := range []string{"aspect-ratio.js", "diagnostics-dialog.js"} {
		t.Run(asset, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/static/"+asset+"?v=test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status code: got %d, want 200", w.Code)
			}
			if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
				t.Errorf("Content-Type: got %q, want text/javascript", got)
			}
			if got := w.Header().Get("Cache-Control"); got != immutableCacheControl {
				t.Errorf("Cache-Control: got %q, want %q", got, immutableCacheControl)
			}
		})
	}
}

func TestLegacyMioStaticAssetsAreNotServed(t *testing.T) {
	router := http.NewServeMux()
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
		assertNotPubliclyCacheable(t, w.Header().Get("Cache-Control"))
	}
}

func TestMioStaticAssetRedirectIsNotPubliclyCacheable(t *testing.T) {
	router := http.NewServeMux()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/static/mio/mio_pose_idle.png/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("status code: got %d, want %d", w.Code, http.StatusMovedPermanently)
	}
	assertNotPubliclyCacheable(t, w.Header().Get("Cache-Control"))
}

func assertNotPubliclyCacheable(t *testing.T, cacheControl string) {
	t.Helper()
	public := false
	positiveMaxAge := false
	for _, directive := range strings.Split(strings.ToLower(cacheControl), ",") {
		directive = strings.TrimSpace(directive)
		if directive == "public" {
			public = true
			continue
		}
		value, found := strings.CutPrefix(directive, "max-age=")
		if !found {
			continue
		}
		maxAge, err := strconv.ParseInt(strings.Trim(value, `"`), 10, 64)
		if err == nil && maxAge > 0 {
			positiveMaxAge = true
		}
	}
	if public && positiveMaxAge {
		t.Errorf("Cache-Control %q marks a non-200 response public with a positive max-age", cacheControl)
	}
}
