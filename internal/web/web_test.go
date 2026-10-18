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

// TestDashboardRootServesInteractivePage verifies the browser entry page and
// its required controls are served to users.
func TestDashboardRootServesInteractivePage(t *testing.T) {
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
		`<script src="/static/app.js?v=`,
	}

	for _, hook := range runtimeHooks {
		if !strings.Contains(body, hook) {
			t.Errorf("missing runtime hook %q in root HTML", hook)
		}
	}

	if strings.Contains(body, "<script>") {
		t.Error("root HTML must load the generated UI script instead of containing inline JavaScript")
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
	desktopContainer := cssRuleBody(t, css, ".container")
	mobileContainer := cssRuleBody(t, mobileStyles, ".container")
	desktopPadding := cssLengthPixels(t, css, cssPropertyValue(t, desktopContainer, "padding"))
	mobilePadding := cssLengthPixels(t, css, cssPropertyValue(t, mobileContainer, "padding"))
	if mobilePadding >= desktopPadding {
		t.Errorf("small-screen container padding = %gpx, want less than desktop %gpx", mobilePadding, desktopPadding)
	}
	desktopSubtitle := cssRuleBody(t, css, ".subtitle")
	mobileSubtitle := cssRuleBody(t, mobileStyles, ".subtitle")
	desktopFontSize := cssLengthPixels(t, css, cssPropertyValue(t, desktopSubtitle, "font-size"))
	mobileFontSize := cssLengthPixels(t, css, cssPropertyValue(t, mobileSubtitle, "font-size"))
	if mobileFontSize >= desktopFontSize {
		t.Errorf("small-screen subtitle font size = %gpx, want less than desktop %gpx", mobileFontSize, desktopFontSize)
	}

	reducedMotionStyles := cssBlockBody(t, css, "@media (prefers-reduced-motion: reduce)")
	for _, property := range []string{"animation-duration", "transition-duration"} {
		value := cssPropertyValue(t, reducedMotionStyles, property)
		if duration := cssMilliseconds(t, value); duration < 0 || duration > 1 {
			t.Errorf("reduced-motion %s = %gms, want a duration from 0 to 1ms", property, duration)
		}
	}
}

func cssPropertyValue(t *testing.T, declarations, property string) string {
	t.Helper()
	for _, declaration := range strings.Split(declarations, ";") {
		name, value, found := strings.Cut(declaration, ":")
		if found && strings.TrimSpace(name) == property {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "!important"))
		}
	}
	t.Fatalf("missing CSS property %q", property)
	return ""
}

func cssLengthPixels(t *testing.T, css, value string) float64 {
	t.Helper()
	if strings.HasPrefix(value, "var(") && strings.HasSuffix(value, ")") {
		variable := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "var("), ")"))
		root := cssRuleBody(t, css, ":root")
		value = cssPropertyValue(t, root, variable)
	}
	pixels, err := strconv.ParseFloat(strings.TrimSuffix(value, "px"), 64)
	if err != nil || !strings.HasSuffix(value, "px") {
		t.Fatalf("CSS length %q is not a resolvable pixel value", value)
	}
	return pixels
}

func cssMilliseconds(t *testing.T, value string) float64 {
	t.Helper()
	milliseconds, err := strconv.ParseFloat(strings.TrimSuffix(value, "ms"), 64)
	if err != nil || !strings.HasSuffix(value, "ms") {
		t.Fatalf("CSS duration %q is not a resolvable millisecond value", value)
	}
	return milliseconds
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
	app, err := webFS.ReadFile("generated/app.js")
	if err != nil {
		t.Fatal(err)
	}
	ui := append(index, app...)

	assets := []struct {
		url  string
		file string
		fs   fs.ReadFileFS
	}{
		{url: "/static/app.js", file: "generated/app.js", fs: webFS},
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
		if !bytes.Contains(ui, []byte(want)) {
			t.Errorf("web UI does not reference content-versioned URL %q", want)
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

	for _, asset := range []string{"app.js"} {
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
