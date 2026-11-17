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
		`<h1 class="brand-name">GoGoMio</h1>`,
		`id="page-description"`,
		`id="stream-placeholder-mio"`,
		`id="stream-companion"`,
		`id="camera-status-badge"`,
		`id="stream-status-badge"`,
		`id="diagnostics-btn"`,
		`<dialog id="diagnostics-modal"`,
		`<script src="/static/app.js?v=`,
	}

	for _, hook := range runtimeHooks {
		if !strings.Contains(body, hook) {
			t.Errorf("missing runtime hook %q in root HTML", hook)
		}
	}
	for _, landmark := range []struct {
		tag  string
		want int
	}{{tag: "<header", want: 1}, {tag: "<main", want: 1}, {tag: "<footer", want: 1}} {
		if got := strings.Count(body, landmark.tag); got != landmark.want {
			t.Errorf("%s landmark count: got %d, want %d", landmark.tag, got, landmark.want)
		}
	}
	if got := strings.Count(body, "<h1"); got != 1 {
		t.Errorf("h1 count: got %d, want 1", got)
	}

	if strings.Contains(body, "<script>") {
		t.Error("root HTML must load the generated UI script instead of containing inline JavaScript")
	}
}

// TestDashboardStatusAnnouncementsUseAccessibleLiveRegions guards WCAG status
// messages without requiring assistive technology to move focus.
// Contract: TC-WEB-05 (docs/testing/test-contracts.md).
func TestDashboardStatusAnnouncementsUseAccessibleLiveRegions(t *testing.T) {
	index, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	body := string(index)

	for _, marker := range []string{`id="snackbar-text"`, `class="stream-status"`, `id="camera-status-badge"`} {
		tag := openingTagContaining(t, body, marker)
		for _, attribute := range []string{`role="status"`, `aria-live="polite"`} {
			if !strings.Contains(tag, attribute) {
				t.Errorf("status message %s is missing %s: %s", marker, attribute, tag)
			}
		}
	}

	for _, marker := range []string{`id="snackbar-text"`, `id="camera-status-badge"`} {
		tag := openingTagContaining(t, body, marker)
		if !strings.Contains(tag, `aria-atomic="true"`) {
			t.Errorf("status message %s should announce its complete updated text: %s", marker, tag)
		}
	}
}

// TestDashboardForcedColorsUseSystemColors verifies that badges and primary
// controls remain distinguishable when the operating system forces a palette.
// Contract: TC-WEB-06 (docs/testing/test-contracts.md).
func TestDashboardForcedColorsUseSystemColors(t *testing.T) {
	index, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	css := strings.SplitN(string(index), "</style>", 2)[0]
	forcedColors := cssBlockBody(t, css, "@media (forced-colors: active)")

	var badge string
	for _, rule := range cssRuleBodiesForSelector(t, forcedColors, ".ui-badge") {
		if strings.Contains(rule, "background: Canvas;") && strings.Contains(rule, "color: CanvasText;") {
			badge = rule
			break
		}
	}
	if badge == "" {
		t.Fatal("forced-colors badge rule must use Canvas and CanvasText")
	}
	if got := cssPropertyValue(t, badge, "background"); got != "Canvas" {
		t.Errorf("forced-colors badge background = %q, want Canvas", got)
	}
	if got := cssPropertyValue(t, badge, "color"); got != "CanvasText" {
		t.Errorf("forced-colors badge text = %q, want CanvasText", got)
	}
	button := cssRuleBody(t, forcedColors, ".button--primary")
	if got := cssPropertyValue(t, button, "background"); got != "Highlight" {
		t.Errorf("forced-colors primary action background = %q, want Highlight", got)
	}
	if got := cssPropertyValue(t, button, "color"); got != "HighlightText" {
		t.Errorf("forced-colors primary action text = %q, want HighlightText", got)
	}
}

func openingTagContaining(t *testing.T, html, marker string) string {
	t.Helper()
	markerIndex := strings.Index(html, marker)
	if markerIndex < 0 {
		t.Fatalf("dashboard is missing %q", marker)
	}
	start := strings.LastIndex(html[:markerIndex], "<")
	if start < 0 {
		t.Fatalf("could not find an opening tag for %q", marker)
	}
	endOffset := strings.IndexByte(html[markerIndex:], '>')
	if endOffset < 0 {
		t.Fatalf("unterminated opening tag containing %q", marker)
	}
	return html[start : markerIndex+endOffset+1]
}

func TestDashboardResponsiveLayoutContracts(t *testing.T) {
	// Contract: TC-WEB-01 (docs/testing/test-contracts.md).
	index, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	css := strings.SplitN(string(index), "</style>", 2)[0]

	mobileStyles := cssBlockBody(t, css, "@media (max-width: 640px)")
	desktopContainer := cssRuleBody(t, css, ".container")
	mobileContainer := cssRuleBody(t, mobileStyles, ".container")
	desktopPadding := cssLengthPixels(t, css, cssPropertyValue(t, desktopContainer, "padding"))
	mobilePadding := cssLengthPixels(t, css, cssPropertyValue(t, mobileContainer, "padding"))
	if mobilePadding >= desktopPadding {
		t.Errorf("small-screen container padding = %gpx, want less than desktop %gpx", mobilePadding, desktopPadding)
	}
	desktopTitle := cssRuleBody(t, css, ".brand-name")
	mobileTitle := cssRuleBody(t, mobileStyles, ".brand-name")
	desktopFontSize := cssLengthPixels(t, css, cssPropertyValue(t, desktopTitle, "font-size"))
	mobileFontSize := cssLengthPixels(t, css, cssPropertyValue(t, mobileTitle, "font-size"))
	if mobileFontSize >= desktopFontSize {
		t.Errorf("small-screen page title font size = %gpx, want less than desktop %gpx", mobileFontSize, desktopFontSize)
	}
	header := cssRuleBody(t, css, ".site-header")
	headerShrink, hasHeaderShrink := cssProperty(header, "flex-shrink")
	headerFlex, hasHeaderFlex := cssProperty(header, "flex")
	if !((hasHeaderShrink && headerShrink == "0") || (hasHeaderFlex && (headerFlex == "none" || strings.HasPrefix(headerFlex, "0 0 ")))) {
		t.Errorf("page header must not shrink below its content; flex-shrink=%q flex=%q", headerShrink, headerFlex)
	}
	sectionTitle := cssRuleBody(t, css, ".section-title")
	whiteSpace, hasWhiteSpace := cssProperty(sectionTitle, "white-space")
	textWrap, hasTextWrap := cssProperty(sectionTitle, "text-wrap")
	if !((hasWhiteSpace && whiteSpace == "nowrap") || (hasTextWrap && textWrap == "nowrap")) {
		t.Errorf("section title must stay together on one line; white-space=%q text-wrap=%q", whiteSpace, textWrap)
	}
	sectionHeading := cssRuleBody(t, css, ".section-heading")
	if got := cssPropertyValue(t, sectionHeading, "display"); got != "flex" {
		t.Errorf("section heading layout = %q, want a flexible title/badge row", got)
	}
	mobileHeading := cssRuleBody(t, mobileStyles, ".section-heading")
	if got := cssPropertyValue(t, mobileHeading, "flex-wrap"); got != "wrap" {
		t.Errorf("small-screen section heading wrapping = %q, want wrap to avoid overflow", got)
	}
	desktopLayout := cssBlockBody(t, css, "@media (min-width: 1024px)")
	gridLayout := cssRuleBody(t, desktopLayout, ".main-grid")
	if !strings.Contains(cssPropertyValue(t, gridLayout, "grid-template-columns"), "minmax(280px, 1fr)") {
		t.Error("two-column layout must leave the status panel at least 280px wide")
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
	if value, found := cssProperty(declarations, property); found {
		return value
	}
	t.Fatalf("missing CSS property %q", property)
	return ""
}

func cssProperty(declarations, property string) (string, bool) {
	for _, declaration := range strings.Split(declarations, ";") {
		name, value, found := strings.Cut(declaration, ":")
		if found && strings.TrimSpace(name) == property {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "!important")), true
		}
	}
	return "", false
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

func cssRuleBodiesForSelector(t *testing.T, css, selector string) []string {
	t.Helper()
	var bodies []string
	for offset := 0; offset < len(css); {
		openOffset := strings.IndexByte(css[offset:], '{')
		closeOffset := strings.IndexByte(css[offset:], '}')
		if openOffset < 0 && strings.TrimSpace(css[offset:]) == "" {
			break
		}
		if openOffset < 0 || closeOffset < 0 || closeOffset < openOffset {
			t.Fatalf("malformed CSS while looking for selector %q", selector)
		}
		open := offset + openOffset
		close := offset + closeOffset
		for _, candidate := range strings.Split(css[offset:open], ",") {
			if strings.TrimSpace(candidate) == selector {
				bodies = append(bodies, css[open+1:close])
				break
			}
		}
		offset = close + 1
	}
	if len(bodies) == 0 {
		t.Fatalf("missing CSS rule for selector %q", selector)
	}
	return bodies
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

// TestWebUIUsesContentVersionedAssetURLs verifies that every required embedded
// asset reference carries the SHA-256 digest of the bytes served for that URL.
// Contract: TC-WEB-08 (docs/testing/test-contracts.md).
func TestWebUIUsesContentVersionedAssetURLs(t *testing.T) {
	index, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	app, err := webFS.ReadFile("generated/app.js")
	if err != nil {
		t.Fatal(err)
	}
	ui := append(append([]byte(nil), index...), app...)

	assets := []struct {
		name           string
		url            string
		file           string
		fs             fs.ReadFileFS
		wantReferences int
	}{
		{name: "application script", url: "/static/app.js", file: "generated/app.js", fs: webFS, wantReferences: 1},
		{name: "preloaded font", url: "/static/fonts/quicksand-latin-wght-normal.woff2", file: "fonts/quicksand-latin-wght-normal.woff2", fs: fontFS, wantReferences: 2},
		{name: "idle MIO pose", url: "/static/mio/mio_pose_idle.png", file: "mio/mio_pose_idle.png", fs: mioFS, wantReferences: 1},
		{name: "sleeping MIO pose", url: "/static/mio/mio_pose_sleeping.png", file: "mio/mio_pose_sleeping.png", fs: mioFS, wantReferences: 1},
		{name: "concerned MIO pose", url: "/static/mio/mio_pose_concerned.png", file: "mio/mio_pose_concerned.png", fs: mioFS, wantReferences: 1},
		{name: "happy MIO pose", url: "/static/mio/mio_pose_happy.png", file: "mio/mio_pose_happy.png", fs: mioFS, wantReferences: 1},
		{name: "worried MIO pose", url: "/static/mio/mio_pose_worried.png", file: "mio/mio_pose_worried.png", fs: mioFS, wantReferences: 1},
		{name: "curious MIO pose", url: "/static/mio/mio_pose_curious.png", file: "mio/mio_pose_curious.png", fs: mioFS, wantReferences: 1},
		{name: "angry MIO pose", url: "/static/mio/mio_pose_angry.png", file: "mio/mio_pose_angry.png", fs: mioFS, wantReferences: 1},
		{name: "looking MIO pose", url: "/static/mio/mio_pose_looking.png", file: "mio/mio_pose_looking.png", fs: mioFS, wantReferences: 1},
	}
	for _, asset := range assets {
		t.Run(asset.name, func(t *testing.T) {
			data, err := asset.fs.ReadFile(asset.file)
			if err != nil {
				t.Fatalf("read %s: %v", asset.file, err)
			}
			assertContentVersionedAssetReference(t, ui, asset.url, data, asset.wantReferences)
		})
	}
}

func assertContentVersionedAssetReference(t *testing.T, content []byte, assetURL string, asset []byte, wantReferences int) {
	t.Helper()
	digest := sha256.Sum256(asset)
	wantQuery := []byte("?v=" + hex.EncodeToString(digest[:]))
	urlBytes := []byte(assetURL)
	count := 0
	for offset := 0; offset < len(content); {
		relativeIndex := bytes.Index(content[offset:], urlBytes)
		if relativeIndex < 0 {
			break
		}
		start := offset + relativeIndex
		afterURL := content[start+len(urlBytes):]
		if !bytes.HasPrefix(afterURL, wantQuery) {
			t.Errorf("asset reference %q is missing its content digest %q", assetURL, wantQuery)
		} else {
			afterQuery := afterURL[len(wantQuery):]
			if len(afterQuery) == 0 || (afterQuery[0] != '\'' && afterQuery[0] != '"') {
				t.Errorf("asset reference %q has unexpected query parameters or trailing data", assetURL)
			}
		}
		count++
		offset = start + len(urlBytes)
	}
	if count != wantReferences {
		t.Errorf("asset %q references = %d, want %d", assetURL, count, wantReferences)
	}
}

func TestSelfHostedFontIsServedWithImmutableCaching(t *testing.T) {
	router := http.NewServeMux()
	RegisterStaticFiles(router)

	req, _ := http.NewRequest("GET", "/static/fonts/quicksand-latin-wght-normal.woff2?v=test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code: got %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "font/woff2" {
		t.Errorf("Content-Type: got %q, want font/woff2", got)
	}
	if got := w.Header().Get("Cache-Control"); got != immutableCacheControl {
		t.Errorf("Cache-Control: got %q, want %q", got, immutableCacheControl)
	}
	if w.Body.Len() == 0 {
		t.Fatal("font response body is empty")
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
