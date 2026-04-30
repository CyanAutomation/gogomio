package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/go-chi/chi/v5"
)

type captureLoopCountingCamera struct {
	activeCaptures int64
	maxActive      int64
}

func (c *captureLoopCountingCamera) Start(_, _, _, _ int) error { return nil }
func (c *captureLoopCountingCamera) Stop() error                { return nil }
func (c *captureLoopCountingCamera) IsReady() bool              { return true }

func (c *captureLoopCountingCamera) CaptureFrame() ([]byte, error) {
	active := atomic.AddInt64(&c.activeCaptures, 1)
	defer atomic.AddInt64(&c.activeCaptures, -1)

	for {
		currentMax := atomic.LoadInt64(&c.maxActive)
		if active <= currentMax {
			break
		}
		if atomic.CompareAndSwapInt64(&c.maxActive, currentMax, active) {
			break
		}
	}

	time.Sleep(4 * time.Millisecond)
	return []byte{0xFF, 0xD8, 0xFF, 0xD9}, nil
}

type repeatedFrameCamera struct{}

func (c *repeatedFrameCamera) Start(_, _, _, _ int) error { return nil }
func (c *repeatedFrameCamera) Stop() error                { return nil }
func (c *repeatedFrameCamera) IsReady() bool              { return true }
func (c *repeatedFrameCamera) CaptureFrame() ([]byte, error) {
	time.Sleep(5 * time.Millisecond)
	return []byte{0xFF, 0xD8, 0xAA, 0xBB, 0xCC, 0xFF, 0xD9}, nil
}

type readinessCamera struct {
	ready bool
}

func (c *readinessCamera) Start(_, _, _, _ int) error { return nil }
func (c *readinessCamera) Stop() error                { return nil }
func (c *readinessCamera) IsReady() bool              { return c.ready }
func (c *readinessCamera) CaptureFrame() ([]byte, error) {
	return nil, nil
}

var errStopStream = errors.New("stop stream")

type countingStreamWriter struct {
	header       http.Header
	targetFrames int

	mu         sync.Mutex
	boundaries int
}

type frameProbeWriter struct {
	header     http.Header
	targetJPEG []byte
	buf        []byte
}

func newFrameProbeWriter(targetJPEG []byte) *frameProbeWriter {
	return &frameProbeWriter{
		header:     make(http.Header),
		targetJPEG: targetJPEG,
	}
}

func (w *frameProbeWriter) Header() http.Header {
	return w.header
}

func (w *frameProbeWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	if len(w.targetJPEG) > 0 && strings.Contains(string(w.buf), string(w.targetJPEG)) {
		return 0, errStopStream
	}
	return len(p), nil
}

func (w *frameProbeWriter) WriteHeader(_ int) {}
func (w *frameProbeWriter) Flush()            {}

func newCountingStreamWriter(targetFrames int) *countingStreamWriter {
	return &countingStreamWriter{
		header:       make(http.Header),
		targetFrames: targetFrames,
	}
}

func (w *countingStreamWriter) Header() http.Header {
	return w.header
}

func (w *countingStreamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Count how many frame boundaries are in the data
	w.boundaries += strings.Count(string(p), "--frame\r\n")
	if w.boundaries >= w.targetFrames {
		return 0, errStopStream
	}
	return len(p), nil
}

func (w *countingStreamWriter) WriteHeader(_ int) {}
func (w *countingStreamWriter) Flush()            {}

func (w *countingStreamWriter) BoundaryCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.boundaries
}

// setupTestServer creates a test Chi router with API handlers
func setupTestServer(t *testing.T) (*chi.Mux, *camera.MockCamera, *config.Config) {
	cfg := &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 2,
		Port:                 8000,
		BindHost:             "0.0.0.0",
		MockCamera:           true,
	}

	mockCam := camera.NewMockCamera()
	if err := mockCam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
		t.Fatalf("Failed to start mock camera: %v", err)
	}

	router := chi.NewRouter()
	frame := NewFrameManager(mockCam, cfg)
	RegisterHandlers(router, frame, cfg)

	return router, mockCam, cfg
}

// TestHealthEndpoint tests the /health endpoint
func TestHealthEndpoint(t *testing.T) {
	router, cam, _ := setupTestServer(t)
	defer func() { _ = cam.Stop() }()

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Errorf("failed to parse JSON response: %v", err)
	}

	if status, ok := result["status"]; !ok || status != "ok" {
		t.Errorf("expected status 'ok', got %v", status)
	}
}

// TestReadyEndpoint tests the /ready endpoint
func TestReadyEndpoint(t *testing.T) {
	router, cam, _ := setupTestServer(t)
	defer func() { _ = cam.Stop() }()

	req, _ := http.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should be 200 because camera is ready
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("expected content type application/json, got %s", contentType)
	}
}

func TestHandleReadyNotReady(t *testing.T) {
	fm := NewFrameManager(&readinessCamera{ready: false}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handleReady(w, req, fm)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("expected content type application/json, got %s", contentType)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}
	if result["status"] != "initializing" {
		t.Errorf("expected status initializing, got %q", result["status"])
	}
}

func TestHandleReadyReady(t *testing.T) {
	fm := NewFrameManager(&readinessCamera{ready: true}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handleReady(w, req, fm)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("expected content type application/json, got %s", contentType)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}
	if result["status"] != "ready" {
		t.Errorf("expected status ready, got %q", result["status"])
	}
}

// TestConfigEndpoint tests the /api/config endpoint
func TestConfigEndpoint(t *testing.T) {
	router, cam, cfg := setupTestServer(t)
	defer func() { _ = cam.Stop() }()

	req, _ := http.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	fieldChecks := []struct {
		name  string
		check func(t *testing.T, payload map[string]interface{})
	}{
		{
			name: "resolution matches configured dimensions",
			check: func(t *testing.T, payload map[string]interface{}) {
				t.Helper()
				resRaw, ok := payload["resolution"]
				if !ok {
					t.Fatal("response missing resolution field")
				}
				resolution, ok := resRaw.([]interface{})
				if !ok {
					t.Fatalf("resolution has unexpected type %T", resRaw)
				}
				if len(resolution) != 2 {
					t.Fatalf("resolution has unexpected length %d", len(resolution))
				}
				width, ok := resolution[0].(float64)
				if !ok {
					t.Fatalf("resolution[0] has unexpected type %T", resolution[0])
				}
				height, ok := resolution[1].(float64)
				if !ok {
					t.Fatalf("resolution[1] has unexpected type %T", resolution[1])
				}
				if int(width) != cfg.Resolution[0] {
					t.Errorf("expected resolution width %d, got %v", cfg.Resolution[0], width)
				}
				if int(height) != cfg.Resolution[1] {
					t.Errorf("expected resolution height %d, got %v", cfg.Resolution[1], height)
				}
			},
		},
		{
			name: "max_stream_connections is present and matches configured value",
			check: func(t *testing.T, payload map[string]interface{}) {
				t.Helper()
				maxConnRaw, ok := payload["max_stream_connections"]
				if !ok {
					t.Fatal("response missing max_stream_connections field")
				}
				maxConn, ok := maxConnRaw.(float64)
				if !ok {
					t.Fatalf("max_stream_connections has unexpected type %T", maxConnRaw)
				}
				if int(maxConn) != cfg.MaxStreamConnections {
					t.Errorf("max_stream_connections mismatch: %v vs %d", maxConn, cfg.MaxStreamConnections)
				}
			},
		},
	}

	for _, tc := range fieldChecks {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, result)
		})
	}
}

func TestDeprecatedAPIStatusEndpointUsesRuntimeConfig(t *testing.T) {
	router, cam, cfg := setupTestServer(t)
	defer func() { _ = cam.Stop() }()

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	fpsConfiguredRaw, ok := result["fps_configured"]
	if !ok {
		t.Fatal("response missing fps_configured field")
	}
	fpsConfigured, ok := fpsConfiguredRaw.(float64)
	if !ok {
		t.Fatalf("fps_configured has unexpected type %T", fpsConfiguredRaw)
	}
	if int(fpsConfigured) != cfg.FPS {
		t.Errorf("fps_configured mismatch: got %d, want %d", int(fpsConfigured), cfg.FPS)
	}

	jpegQualityRaw, ok := result["jpeg_quality"]
	if !ok {
		t.Fatal("response missing jpeg_quality field")
	}
	jpegQuality, ok := jpegQualityRaw.(float64)
	if !ok {
		t.Fatalf("jpeg_quality has unexpected type %T", jpegQualityRaw)
	}
	if int(jpegQuality) != cfg.JPEGQuality {
		t.Errorf("jpeg_quality mismatch: got %d, want %d", int(jpegQuality), cfg.JPEGQuality)
	}

	maxConnRaw, ok := result["max_stream_connections"]
	if !ok {
		t.Fatal("response missing max_stream_connections field")
	}
	maxConn, ok := maxConnRaw.(float64)
	if !ok {
		t.Fatalf("max_stream_connections has unexpected type %T", maxConnRaw)
	}
	if int(maxConn) != cfg.MaxStreamConnections {
		t.Errorf("max_stream_connections mismatch: got %d, want %d", int(maxConn), cfg.MaxStreamConnections)
	}
}

// TestSnapshotEndpoint tests the /snapshot.jpg endpoint
func TestSnapshotEndpoint(t *testing.T) {
	router, cam, _ := setupTestServer(t)
	defer func() { _ = cam.Stop() }()

	// Pre-populate frame by making a stream request in background
	// This ensures capture loop is started and frame buffer has content
	go func() {
		req, _ := http.NewRequest("GET", "/stream.mjpg", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}()

	// Wait for frame to be captured and buffered
	time.Sleep(200 * time.Millisecond)

	// Now test snapshot endpoint
	req, _ := http.NewRequest("GET", "/snapshot.jpg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "image/jpeg" {
		t.Errorf("expected content type image/jpeg, got %s", contentType)
	}

	// Check body is not empty and has JPEG markers
	body := w.Body.Bytes()
	if len(body) == 0 {
		t.Error("response body is empty")
	}

	if len(body) > 1 && (body[0] != 0xFF || body[1] != 0xD8) {
		t.Errorf("response does not have JPEG SOI marker: %02x %02x", body[0], body[1])
	}
}

// TestStreamEndpoint tests the /stream.mjpg endpoint initialization
func TestStreamEndpoint(t *testing.T) {
	router, cam, _ := setupTestServer(t)
	defer func() { _ = cam.Stop() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, _ := http.NewRequest("GET", "/stream.mjpg", nil)
	req = req.WithContext(ctx)
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		router.ServeHTTP(recorder, req)
	}()

	// Wait a bit for the stream to start sending frames
	time.Sleep(500 * time.Millisecond)

	// Cancel the context to stop the handler
	cancel()

	// Wait for handler to exit before reading response
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("stream did not stop after context cancellation")
	}

	// Now that handler is done, it's safe to read the response body without races
	body := recorder.Body.String()
	if !strings.Contains(body, "--frame\r\n") {
		t.Fatal("expected at least one frame boundary in response")
	}

	// Validate headers
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	contentType := recorder.Header().Get("Content-Type")
	if contentType != "multipart/x-mixed-replace; boundary=frame" {
		t.Fatalf("expected content type %q, got %q", "multipart/x-mixed-replace; boundary=frame", contentType)
	}
	if !strings.Contains(contentType, "boundary=frame") {
		t.Fatalf("expected stream boundary in content type, got %q", contentType)
	}
}

// TestIndexEndpoint tests the / root endpoint serves the embedded UI.
func TestIndexEndpoint(t *testing.T) {
	router, cam, _ := setupTestServer(t)
	defer func() { _ = cam.Stop() }()

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("expected content type %q, got %q", "text/html; charset=utf-8", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<title>Motion In Ocean - Go Edition</title>") {
		t.Fatalf("expected response body to contain embedded UI title marker")
	}
}

// TestCORSHeaders tests CORS headers are present
func TestCORSHeaders(t *testing.T) {
	router, cam, _ := setupTestServer(t)
	defer func() { _ = cam.Stop() }()

	t.Run("preflight request includes expected CORS headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/config", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", http.MethodGet)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected status %d, got %d", http.StatusNoContent, w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("expected Access-Control-Allow-Origin %q, got %q", "*", got)
		}
		if got := w.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, OPTIONS" {
			t.Errorf("expected Access-Control-Allow-Methods %q, got %q", "GET, POST, PUT, OPTIONS", got)
		}
		if got := w.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type" {
			t.Errorf("expected Access-Control-Allow-Headers %q, got %q", "Content-Type", got)
		}
	})

	t.Run("normal request includes expected CORS headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("expected Access-Control-Allow-Origin %q, got %q", "*", got)
		}
	})

	t.Run("preflight with disallowed method is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/config", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", http.MethodDelete)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})
}

// TestMJPEGStreamingEndpoint tests the /stream.mjpg endpoint with frame transmission
func TestMJPEGStreamingEndpoint(t *testing.T) {
	router, cam, _ := setupTestServer(t)
	defer func() { _ = cam.Stop() }()

	// Wait for mock camera to generate frames with lazy capture
	time.Sleep(800 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	req, _ := http.NewRequest("GET", "/stream.mjpg", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	// Run request in goroutine - will exit when context times out
	done := make(chan bool)
	go func() {
		router.ServeHTTP(w, req)
		done <- true
	}()

	// Wait for handler to finish (context timeout or error)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream handler did not exit")
	}

	// Now that handler is done, it's safe to read response without races

	// Verify response headers
	if ct := w.Header().Get("Content-Type"); ct != "multipart/x-mixed-replace; boundary=frame" {
		t.Errorf("Content-Type: got %q, want multipart/x-mixed-replace", ct)
	}

	// Read response body - httptest.ResponseRecorder buffers everything
	responseBody := w.Body.String()

	// Verify MJPEG boundary markers are present
	if len(responseBody) == 0 {
		t.Fatal("no response body from stream")
	}

	if !contains(responseBody, "--frame") {
		t.Error("MJPEG boundary marker --frame not found in response")
	}

	if !contains(responseBody, "Content-Type: image/jpeg") {
		t.Error("JPEG Content-Type header not found in response")
	}

	if !contains(responseBody, "Content-Length:") {
		t.Error("Content-Length header not found in response")
	}

	// Verify status code is 200 (streaming started)
	if w.Code != http.StatusOK {
		t.Errorf("status code: got %d, want 200", w.Code)
	}
}

// TestStreamingConnectionLimit tests that max stream connections are enforced
func TestStreamingConnectionLimit(t *testing.T) {
	cfg := &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 1, // Limit to 1 connection
		Port:                 8000,
		BindHost:             "0.0.0.0",
		MockCamera:           true,
	}

	mockCam := camera.NewMockCamera()
	if err := mockCam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
		t.Fatalf("Failed to start mock camera: %v", err)
	}
	defer func() { _ = mockCam.Stop() }()

	router := chi.NewRouter()
	frame := NewFrameManager(mockCam, cfg)
	RegisterHandlers(router, frame, cfg)

	// Wait for frames to be available
	time.Sleep(600 * time.Millisecond)

	// First request should succeed
	ctx1, cancel1 := context.WithCancel(context.Background())
	req1, _ := http.NewRequestWithContext(ctx1, "GET", "/stream.mjpg", nil)
	w1 := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		router.ServeHTTP(w1, req1)
	}()
	time.Sleep(100 * time.Millisecond)

	// Second request should be rejected (conn limit)
	req2, _ := http.NewRequest("GET", "/stream.mjpg", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second connection status: got %d, want %d", w2.Code, http.StatusTooManyRequests)
	}

	if !contains(w2.Body.String(), "Max stream connections") {
		t.Error("error message not found in response")
	}

	cancel1()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("first stream handler did not exit after cancel")
	}
}

func TestRateLimitMiddlewareTreatsDifferentPortsAsSameClient(t *testing.T) {
	limiter := NewRateLimiter(1, 10*time.Second)
	middleware := rateLimitMiddleware(limiter, nil)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req1.RemoteAddr = "198.51.100.10:1234"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request status: got %d, want %d", w1.Code, http.StatusOK)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req2.RemoteAddr = "198.51.100.10:5678"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status: got %d, want %d", w2.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitMiddlewareRejectsSpoofedForwardedIPFromUntrustedPeer(t *testing.T) {
	limiter := NewRateLimiter(1, 10*time.Second)
	middleware := rateLimitMiddleware(limiter, nil)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req1.RemoteAddr = "10.10.0.1:1111"
	req1.Header.Set("X-Forwarded-For", " 203.0.113.42 , 172.16.0.10, 172.16.0.11 ")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request status: got %d, want %d", w1.Code, http.StatusOK)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req2.RemoteAddr = "10.10.0.2:2222"
	req2.Header.Set("X-Forwarded-For", "203.0.113.42, 172.16.0.20, 172.16.0.30")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second request status: got %d, want %d", w2.Code, http.StatusOK)
	}
}

func TestRateLimitMiddlewareAcceptsForwardedIPFromTrustedProxy(t *testing.T) {
	limiter := NewRateLimiter(1, 10*time.Second)
	middleware := rateLimitMiddleware(limiter, []string{"10.10.0.0/16"})
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req1.RemoteAddr = "10.10.0.1:1111"
	req1.Header.Set("X-Forwarded-For", " 203.0.113.42 , 172.16.0.10, 172.16.0.11 ")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request status: got %d, want %d", w1.Code, http.StatusOK)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req2.RemoteAddr = "10.10.0.2:2222"
	req2.Header.Set("X-Forwarded-For", "203.0.113.42, 172.16.0.20, 172.16.0.30")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status: got %d, want %d", w2.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimiterEvictsStaleEntries(t *testing.T) {
	window := 20 * time.Millisecond
	limiter := NewRateLimiter(2, window)

	const staleIP = "198.51.100.77"
	if !limiter.Allow(staleIP) {
		t.Fatalf("expected first request to be allowed")
	}

	time.Sleep(4 * window)

	if !limiter.Allow("203.0.113.10") {
		t.Fatalf("expected trigger request to be allowed")
	}

	for i := 0; i < 8; i++ {
		limiter.Allow("203.0.113.11")
	}

	limiter.mu.Lock()
	_, exists := limiter.requests[staleIP]
	limiter.mu.Unlock()
	if exists {
		t.Fatalf("expected stale IP entry %q to be evicted", staleIP)
	}
}

func TestRateLimiterManyUniqueIPsKeepsBehaviorAndCleansUp(t *testing.T) {
	window := 25 * time.Millisecond
	limiter := NewRateLimiter(2, window)

	for i := 0; i < 500; i++ {
		ip := "198.51.100." + strconv.Itoa(i)
		if !limiter.Allow(ip) {
			t.Fatalf("expected first request for %s to be allowed", ip)
		}
	}

	if !limiter.Allow("198.51.100.1") {
		t.Fatalf("expected second request for repeated ip to be allowed")
	}
	if limiter.Allow("198.51.100.1") {
		t.Fatalf("expected third request for repeated ip to be rate limited")
	}

	limiter.mu.Lock()
	startSize := len(limiter.requests)
	limiter.mu.Unlock()
	if startSize < 500 {
		t.Fatalf("expected at least 500 entries, got %d", startSize)
	}

	time.Sleep(4 * window)

	for i := 0; i < 24; i++ {
		ip := "203.0.113." + strconv.Itoa(i)
		if !limiter.Allow(ip) {
			t.Fatalf("expected cleanup trigger request for %s to be allowed", ip)
		}
	}

	limiter.mu.Lock()
	sizeAfterCleanup := len(limiter.requests)
	limiter.mu.Unlock()
	if sizeAfterCleanup >= startSize {
		t.Fatalf("expected stale entries to shrink map size, before=%d after=%d", startSize, sizeAfterCleanup)
	}

	if !limiter.Allow("198.51.100.1") {
		t.Fatalf("expected old key to be treated as new after eviction")
	}
}

func TestNewRateLimiterClampsNonPositiveMaxReqSec(t *testing.T) {
	limiter := NewRateLimiter(0, 10*time.Second)

	const ip = "203.0.113.200"
	for i := 0; i < rateLimiterDefaultMaxReqSec; i++ {
		if !limiter.Allow(ip) {
			t.Fatalf("expected request %d to be allowed with clamped max", i+1)
		}
	}

	if limiter.Allow(ip) {
		t.Fatalf("expected request above clamped max (%d) to be denied", rateLimiterDefaultMaxReqSec)
	}
}

func TestNewRateLimiterClampsNonPositiveWindow(t *testing.T) {
	limiter := NewRateLimiter(1, 0)

	const ip = "203.0.113.201"
	if !limiter.Allow(ip) {
		t.Fatalf("expected first request to be allowed")
	}
	if limiter.Allow(ip) {
		t.Fatalf("expected second immediate request to be denied within default window")
	}

	time.Sleep(rateLimiterDefaultWindow + 20*time.Millisecond)
	if !limiter.Allow(ip) {
		t.Fatalf("expected request to be allowed after default window elapsed")
	}
}

func TestFrameManagerCaptureLoopSingleGoroutineWithConcurrentStarts(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30}
	cam := &captureLoopCountingCamera{}
	fm := newFrameManager(cam, cfg, 40*time.Millisecond)
	t.Cleanup(fm.Stop)

	fm.startCapture()

	startDone := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			fm.startCapture()
			startDone <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-startDone
	}

	time.Sleep(100 * time.Millisecond)
	fm.stopCapture()

	if max := atomic.LoadInt64(&cam.maxActive); max > 1 {
		t.Fatalf("expected at most one active capture loop, saw %d", max)
	}
}

func TestFrameManagerCaptureLoopSingleGoroutineAcrossRapidClientFlaps(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30}
	cam := &captureLoopCountingCamera{}
	fm := newFrameManager(cam, cfg, 40*time.Millisecond)
	t.Cleanup(fm.Stop)

	for i := 0; i < 15; i++ {
		fm.IncrementClients()
		time.Sleep(6 * time.Millisecond)
		fm.DecrementClients()
	}

	if max := atomic.LoadInt64(&cam.maxActive); max > 1 {
		t.Fatalf("expected at most one active capture loop during rapid client flaps, saw %d", max)
	}
}

func TestFrameManagerStreamAndCaptureLifecycleRaceFree(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30, MaxStreamConnections: 2}
	cam := &captureLoopCountingCamera{}
	fm := NewFrameManager(cam, cfg)
	t.Cleanup(fm.Stop)

	for i := 0; i < 20; i++ {
		w := httptest.NewRecorder()
		errCh := make(chan error, 1)

		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			req := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil).WithContext(ctx)
			errCh <- fm.StreamFrame(w, req, cfg.MaxStreamConnections)
		}()

		time.Sleep(15 * time.Millisecond)
		fm.stopCapture()
		cancel()

		select {
		case err := <-errCh:
			if err == nil {
				t.Fatalf("expected stream to stop with an error")
			}
			if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "stream stopped") {
				t.Fatalf("expected context canceled or stream stopped error, got %v", err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("stream did not stop after capture shutdown on iteration %d", i)
		}
	}
}

func TestStreamFrameEmitsRepeatedIdenticalFramesBySequence(t *testing.T) {
	cfg := &config.Config{FPS: 120, TargetFPS: 120, MaxStreamConnections: 2}
	cam := &repeatedFrameCamera{}
	fm := NewFrameManager(cam, cfg)
	t.Cleanup(fm.Stop)

	writer := newCountingStreamWriter(3)
	req := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil)
	err := fm.StreamFrame(writer, req, cfg.MaxStreamConnections)
	if !errors.Is(err, errStopStream) {
		t.Fatalf("expected stream stop error, got %v", err)
	}

	if got := writer.BoundaryCount(); got < 3 {
		t.Fatalf("expected at least 3 frames written for repeated frame bytes, got %d", got)
	}
}

func TestStreamFrameDedupeIsPerClientNotGlobal(t *testing.T) {
	cfg := &config.Config{FPS: 120, TargetFPS: 120, MaxStreamConnections: 2}
	cam := &repeatedFrameCamera{}
	fm := NewFrameManager(cam, cfg)
	t.Cleanup(fm.Stop)

	writerA := newCountingStreamWriter(2)
	writerB := newCountingStreamWriter(2)
	errCh := make(chan error, 2)

	go func() {
		reqA := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil)
		errCh <- fm.StreamFrame(writerA, reqA, cfg.MaxStreamConnections)
	}()
	go func() {
		reqB := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil)
		errCh <- fm.StreamFrame(writerB, reqB, cfg.MaxStreamConnections)
	}()

	for i := 0; i < 2; i++ {
		err := <-errCh
		if !errors.Is(err, errStopStream) {
			t.Fatalf("expected stream stop error, got %v", err)
		}
	}

	if got := writerA.BoundaryCount(); got < 2 {
		t.Fatalf("client A expected at least 2 frames, got %d", got)
	}
	if got := writerB.BoundaryCount(); got < 2 {
		t.Fatalf("client B expected at least 2 frames, got %d", got)
	}
}

func TestFrameManagerDecrementClientsClampsAtZeroAndTracksImbalance(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30}
	cam := &captureLoopCountingCamera{}
	fm := NewFrameManager(cam, cfg)
	t.Cleanup(fm.Stop)

	fm.DecrementClients()
	fm.DecrementClients()

	if count := atomic.LoadInt64(&fm.clientCount); count != 0 {
		t.Fatalf("client count = %d, want 0", count)
	}

	if imbalance := atomic.LoadInt64(&fm.clientImbalance); imbalance != 2 {
		t.Fatalf("client imbalance count = %d, want 2", imbalance)
	}

	fm.captureMu.Lock()
	started := fm.captureStarted
	fm.captureMu.Unlock()
	if started {
		t.Fatalf("capture should not be running after extra decrements from zero")
	}
}

func TestFrameManagerClientLifecycleWithExtraDecrementRemainsStable(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30}
	cam := &captureLoopCountingCamera{}
	fm := newFrameManager(cam, cfg, 40*time.Millisecond)
	t.Cleanup(fm.Stop)

	fm.IncrementClients()
	waitForCaptureState(t, fm, true)

	fm.DecrementClients()
	waitForCaptureState(t, fm, false)

	// Extra decrement should clamp and record imbalance, but not break future transitions.
	fm.DecrementClients()

	if count := atomic.LoadInt64(&fm.clientCount); count != 0 {
		t.Fatalf("client count after extra decrement = %d, want 0", count)
	}
	if imbalance := atomic.LoadInt64(&fm.clientImbalance); imbalance != 1 {
		t.Fatalf("client imbalance count = %d, want 1", imbalance)
	}

	fm.IncrementClients()
	waitForCaptureState(t, fm, true)

	fm.DecrementClients()
	waitForCaptureState(t, fm, false)
}

func TestFrameManagerGetFrameBurstyAccessKeepsCaptureWarm(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30}
	cam := &captureLoopCountingCamera{}
	fm := newFrameManager(cam, cfg, 80*time.Millisecond)
	t.Cleanup(fm.Stop)

	for i := 0; i < 8; i++ {
		frame := fm.GetFrame()
		if len(frame) == 0 {
			t.Fatalf("expected non-empty frame on iteration %d", i)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if starts := atomic.LoadInt64(&fm.captureStarts); starts != 1 {
		t.Fatalf("expected one capture loop start during bursty snapshots, got %d", starts)
	}

	waitForCaptureState(t, fm, true)
	time.Sleep(120 * time.Millisecond)
	waitForCaptureState(t, fm, false)
}

func TestFrameManagerGetFrameReturnsOwnedCopyForSnapshotAndStream(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30, MaxStreamConnections: 1}
	cam := &repeatedFrameCamera{}
	fm := newFrameManager(cam, cfg, 80*time.Millisecond)
	t.Cleanup(fm.Stop)

	first := fm.GetFrame()
	if len(first) == 0 {
		t.Fatal("expected initial snapshot frame")
	}

	original := append([]byte(nil), first...)
	first[2] = 0x11 // mutate caller-owned bytes

	second := fm.GetFrame()
	if len(second) != len(original) {
		t.Fatalf("snapshot length changed after mutation: got %d want %d", len(second), len(original))
	}
	if second[2] != original[2] {
		t.Fatalf("snapshot was affected by caller mutation: got byte 0x%X want 0x%X", second[2], original[2])
	}

	writer := newFrameProbeWriter(original)
	req := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil)
	err := fm.StreamFrame(writer, req, cfg.MaxStreamConnections)
	if !errors.Is(err, errStopStream) {
		t.Fatalf("expected stream to stop after probing frame, got %v", err)
	}
}

func TestScheduleStopCaptureFallbackWhenCleanupQueueSaturated(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30}
	cam := &captureLoopCountingCamera{}
	fm := newFrameManager(cam, cfg, 20*time.Millisecond)
	t.Cleanup(fm.Stop)

	fm.startCapture()
	waitForCaptureState(t, fm, true)
	fm.captureMu.Lock()
	expectedDone := fm.doneChan
	fm.captureMu.Unlock()

	// Keep cleanup loop occupied with one long request, then saturate queue buffer.
	busyReq := cleanupRequest{
		delay:  5 * time.Second,
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
	fm.cleanupCh <- busyReq
	time.Sleep(10 * time.Millisecond)
	for len(fm.cleanupCh) < cap(fm.cleanupCh) {
		fm.cleanupCh <- cleanupRequest{
			delay:  5 * time.Second,
			stopCh: make(chan struct{}),
			done:   make(chan struct{}),
		}
	}

	if count := atomic.LoadInt64(&fm.clientCount); count != 0 {
		t.Fatalf("client count before scheduling stop = %d, want 0", count)
	}
	fm.scheduleStopCapture()

	waitForCaptureState(t, fm, false)

	// Re-running the fallback path against the already-stopped capture should not panic.
	done := make(chan struct{})
	fm.fallbackWG.Add(1)
	go func() {
		defer close(done)
		fm.delayedStopFallback(cleanupRequest{
			delay:  0,
			stopCh: make(chan struct{}),
			done:   expectedDone,
		})
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("fallback idle stop did not return promptly")
	}
}

func TestStreamFrameReturnsOnRequestContextCancelAndDecrementsCounters(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30, MaxStreamConnections: 2}
	cam := &captureLoopCountingCamera{}
	fm := newFrameManager(cam, cfg, 10*time.Millisecond)
	t.Cleanup(fm.Stop)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil).WithContext(ctx)
	writer := httptest.NewRecorder()

	errCh := make(chan error, 1)
	go func() {
		errCh <- fm.StreamFrame(writer, req, cfg.MaxStreamConnections)
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled error, got %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("stream did not return promptly after context cancellation")
	}

	if count := atomic.LoadInt64(&fm.clientCount); count != 0 {
		t.Fatalf("clientCount = %d, want 0", count)
	}
	if count := fm.connTracker.Count(); count != 0 {
		t.Fatalf("connection tracker count = %d, want 0", count)
	}
}

func TestStreamFrameReturnsPromptlyOnFrameManagerStop(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30, MaxStreamConnections: 2}
	cam := &captureLoopCountingCamera{}
	fm := newFrameManager(cam, cfg, 10*time.Millisecond)
	t.Cleanup(fm.Stop)

	req := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil)
	writer := httptest.NewRecorder()

	errCh := make(chan error, 1)
	go func() {
		errCh <- fm.StreamFrame(writer, req, cfg.MaxStreamConnections)
	}()

	time.Sleep(25 * time.Millisecond)
	fm.Stop()

	select {
	case err := <-errCh:
		if err == nil || err.Error() != "stream stopped" {
			t.Fatalf("expected stream stopped error, got %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("stream did not return promptly after FrameManager.Stop")
	}
}

func TestFrameManagerStopRaceWithDisconnectDecrementDoesNotPanic(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30, MaxStreamConnections: 2}
	cam := &captureLoopCountingCamera{}
	fm := newFrameManager(cam, cfg, 10*time.Millisecond)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic while stopping during disconnect race: %v", r)
		}
	}()
	t.Cleanup(fm.Stop)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil).WithContext(ctx)
	writer := httptest.NewRecorder()

	streamErr := make(chan error, 1)
	go func() {
		streamErr <- fm.StreamFrame(writer, req, cfg.MaxStreamConnections)
	}()

	time.Sleep(25 * time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		fm.Stop()
	}()
	go func() {
		defer wg.Done()
		cancel()
	}()
	wg.Wait()

	select {
	case err := <-streamErr:
		if err != nil && !errors.Is(err, context.Canceled) && err.Error() != "stream stopped" {
			t.Fatalf("expected context canceled or stream stopped error, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("stream did not return after stop/disconnect race")
	}
}

func TestFrameManagerConcurrentStopAndDecrementClientsDoesNotPanic(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30}

	for i := 0; i < 200; i++ {
		cam := &captureLoopCountingCamera{}
		fm := newFrameManager(cam, cfg, 10*time.Millisecond)

		fm.startCapture()
		waitForCaptureState(t, fm, true)
		atomic.StoreInt64(&fm.clientCount, 1)

		var wg sync.WaitGroup
		panicCh := make(chan any, 2)
		wg.Add(2)

		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
			}()
			fm.DecrementClients()
		}()

		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
			}()
			fm.Stop()
		}()

		wg.Wait()
		select {
		case p := <-panicCh:
			t.Fatalf("iteration %d: unexpected panic from Stop/DecrementClients race: %v", i, p)
		default:
		}

		// Ensure resources are cleaned up if Stop() happened to lose the race in the goroutine.
		fm.Stop()
	}
}

func TestFrameManagerConcurrentIncrementAndStopCancelTransitionsDoNotPanic(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30}

	for i := 0; i < 200; i++ {
		cam := &captureLoopCountingCamera{}
		fm := newFrameManager(cam, cfg, 10*time.Millisecond)

		panicCh := make(chan any, 4)
		var wg sync.WaitGroup
		wg.Add(3)

		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
			}()
			for j := 0; j < 20; j++ {
				fm.IncrementClients()
				time.Sleep(time.Microsecond)
			}
		}()

		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
			}()
			for j := 0; j < 20; j++ {
				fm.DecrementClients()
				time.Sleep(time.Microsecond)
			}
		}()

		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
			}()
			time.Sleep(2 * time.Millisecond)
			fm.Stop()
		}()

		wg.Wait()
		select {
		case p := <-panicCh:
			t.Fatalf("iteration %d: unexpected panic from Increment/Decrement/Stop race: %v", i, p)
		default:
		}

		// Ensure cleanup if Stop() lost the race in goroutine scheduling.
		fm.Stop()
	}
}

func TestFrameManagerStopUnblocksBlockedCleanupSender(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30}
	cam := &captureLoopCountingCamera{}
	fm := newFrameManager(cam, cfg, 20*time.Millisecond)
	t.Cleanup(fm.Stop)

	// Keep cleanup loop occupied and saturate cleanup queue so an enqueue sender blocks.
	busyReq := cleanupRequest{
		delay:  5 * time.Second,
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
	fm.cleanupCh <- busyReq
	time.Sleep(10 * time.Millisecond)
	for len(fm.cleanupCh) < cap(fm.cleanupCh) {
		fm.cleanupCh <- cleanupRequest{
			delay:  5 * time.Second,
			stopCh: make(chan struct{}),
			done:   make(chan struct{}),
		}
	}

	resultCh := make(chan cleanupEnqueueResult, 1)
	go func() {
		resultCh <- fm.tryEnqueueCleanupRequest(cleanupRequest{
			delay:  5 * time.Second,
			stopCh: make(chan struct{}),
			done:   make(chan struct{}),
		}, time.After(2*time.Second))
	}()

	// Wait until sender is registered, proving Stop() must unblock a potentially blocked sender.
	deadline := time.Now().Add(300 * time.Millisecond)
	senderRegistered := false
	for time.Now().Before(deadline) {
		fm.cleanupSendMu.Lock()
		senders := fm.cleanupSenders
		fm.cleanupSendMu.Unlock()
		if senders > 0 {
			senderRegistered = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !senderRegistered {
		t.Fatal("cleanup sender never registered as active")
	}

	stopDone := make(chan struct{})
	go func() {
		fm.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Stop() did not return while cleanup sender was blocked")
	}

	select {
	case result := <-resultCh:
		if result != cleanupEnqueueSkipped {
			t.Fatalf("enqueue result = %v, want %v", result, cleanupEnqueueSkipped)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocked cleanup sender did not exit after Stop()")
	}
}

func TestFrameManagerStopReturnsDuringTimerCancelExpiryRaces(t *testing.T) {
	cfg := &config.Config{FPS: 30, TargetFPS: 30}
	cam := &captureLoopCountingCamera{}
	fm := newFrameManager(cam, cfg, 5*time.Millisecond)
	t.Cleanup(fm.Stop)

	for i := 0; i < 100; i++ {
		stopCh := make(chan struct{})
		req := cleanupRequest{
			delay:  1 * time.Millisecond,
			stopCh: stopCh,
			done:   make(chan struct{}),
		}

		fm.fallbackWG.Add(1)
		go fm.delayedStopFallback(req)

		// Race cancellation against timer expiry.
		time.Sleep(500 * time.Microsecond)
		close(stopCh)
	}

	stopDone := make(chan struct{})
	go func() {
		fm.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return during timer cancel/expiry races")
	}
}

func TestHandleSettingsGet_ReturnsSettingsEnvelope(t *testing.T) {
	fm := NewFrameManager(&readinessCamera{ready: true}, &config.Config{})
	if err := fm.settingsM.SetMany(map[string]interface{}{
		"brightness": 80,
		"contrast":   40,
	}); err != nil {
		t.Fatalf("failed to seed settings manager: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	w := httptest.NewRecorder()
	handleSettingsGet(w, req, fm)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	settingsRaw, ok := payload["settings"]
	if !ok {
		t.Fatalf("expected settings envelope key in response: %v", payload)
	}
	settingsMap, ok := settingsRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("expected settings to be object, got %T", settingsRaw)
	}

	if settingsMap["brightness"] != float64(80) {
		t.Errorf("expected brightness=80, got %v", settingsMap["brightness"])
	}
	if settingsMap["contrast"] != float64(40) {
		t.Errorf("expected contrast=40, got %v", settingsMap["contrast"])
	}
}

func waitForCaptureState(t *testing.T, fm *FrameManager, want bool) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		fm.captureMu.Lock()
		started := fm.captureStarted
		fm.captureMu.Unlock()

		if started == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	fm.captureMu.Lock()
	got := fm.captureStarted
	fm.captureMu.Unlock()
	t.Fatalf("captureStarted = %v, want %v", got, want)
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ====================== PHASE 3: Lifecycle Tests ======================

// TestFrameManager_CaptureLoop_StartAndStop tests capture loop start and stop
func TestFrameManager_CaptureLoop_StartAndStop(t *testing.T) {
	cam := camera.NewMockCamera()
	if err := cam.Start(640, 480, 24, 90); err != nil {
		t.Fatalf("failed to start camera: %v", err)
	}
	defer func() { _ = cam.Stop() }()

	fm := NewFrameManager(cam, &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 2,
	})
	defer fm.Stop()

	// Initially capture should not be started
	fm.captureMu.Lock()
	if fm.captureStarted {
		fm.captureMu.Unlock()
		t.Fatal("expected captureStarted to be false initially")
	}
	fm.captureMu.Unlock()

	// Start capture
	fm.startCapture()
	waitForCaptureState(t, fm, true)

	// Stop capture
	fm.stopCapture()
	waitForCaptureState(t, fm, false)
}

// TestFrameManager_CaptureLoop_RestartAfterStop tests restarting capture after stop
func TestFrameManager_CaptureLoop_RestartAfterStop(t *testing.T) {
	cam := camera.NewMockCamera()
	if err := cam.Start(640, 480, 24, 90); err != nil {
		t.Fatalf("failed to start camera: %v", err)
	}
	defer func() { _ = cam.Stop() }()

	fm := NewFrameManager(cam, &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 2,
	})
	defer fm.Stop()

	// Start, stop, then start again
	fm.startCapture()
	waitForCaptureState(t, fm, true)

	fm.stopCapture()
	waitForCaptureState(t, fm, false)

	// Restart
	fm.startCapture()
	waitForCaptureState(t, fm, true)

	fm.stopCapture()
	waitForCaptureState(t, fm, false)
}

// TestFrameManager_CaptureLoop_IdempotentStart tests that starting twice is idempotent
func TestFrameManager_CaptureLoop_IdempotentStart(t *testing.T) {
	cam := camera.NewMockCamera()
	if err := cam.Start(640, 480, 24, 90); err != nil {
		t.Fatalf("failed to start camera: %v", err)
	}
	defer func() { _ = cam.Stop() }()

	fm := NewFrameManager(cam, &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 2,
	})
	defer fm.Stop()

	// Get initial capture starts count
	fm.captureMu.Lock()
	initialStarts := fm.captureStarts
	fm.captureMu.Unlock()

	// Start capture twice
	fm.startCapture()
	waitForCaptureState(t, fm, true)

	fm.captureMu.Lock()
	startsAfterFirst := fm.captureStarts
	fm.captureMu.Unlock()

	fm.startCapture() // Should be idempotent
	waitForCaptureState(t, fm, true)

	fm.captureMu.Lock()
	startsAfterSecond := fm.captureStarts
	fm.captureMu.Unlock()

	// Only one actual start should have occurred
	if startsAfterSecond != startsAfterFirst {
		t.Fatalf("expected capture to start once, but got %d starts", startsAfterSecond-initialStarts)
	}
}

// TestFrameManager_StopCaptureIfIdle tests idle timeout behavior
func TestFrameManager_StopCaptureIfIdle(t *testing.T) {
	cam := camera.NewMockCamera()
	if err := cam.Start(640, 480, 24, 90); err != nil {
		t.Fatalf("failed to start camera: %v", err)
	}
	defer func() { _ = cam.Stop() }()

	fm := NewFrameManager(cam, &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 2,
	})
	defer fm.Stop()

	// Start capture
	fm.startCapture()
	waitForCaptureState(t, fm, true)

	// Get the current done channel
	fm.captureMu.Lock()
	done := fm.doneChan
	fm.captureMu.Unlock()

	// Should be able to stop when no clients are connected
	if !fm.stopCaptureIfIdle(done) {
		t.Fatal("expected stopCaptureIfIdle to succeed when no clients")
	}

	waitForCaptureState(t, fm, false)
}

// TestFrameManager_StopCaptureIfIdle_WithClient tests idle stop is prevented when client connected
func TestFrameManager_StopCaptureIfIdle_WithClient(t *testing.T) {
	cam := camera.NewMockCamera()
	if err := cam.Start(640, 480, 24, 90); err != nil {
		t.Fatalf("failed to start camera: %v", err)
	}
	defer func() { _ = cam.Stop() }()

	fm := NewFrameManager(cam, &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 2,
	})
	defer fm.Stop()

	// Start capture
	fm.startCapture()
	waitForCaptureState(t, fm, true)

	// Simulate a client connecting
	atomic.AddInt64(&fm.clientCount, 1)

	// Get the current done channel
	fm.captureMu.Lock()
	done := fm.doneChan
	fm.captureMu.Unlock()

	// Should NOT be able to stop when a client is connected
	if fm.stopCaptureIfIdle(done) {
		t.Fatal("expected stopCaptureIfIdle to fail when client connected")
	}

	// Verify capture is still running
	waitForCaptureState(t, fm, true)

	// Clean up client count
	atomic.AddInt64(&fm.clientCount, -1)

	// Now it should work
	if !fm.stopCaptureIfIdle(done) {
		t.Fatal("expected stopCaptureIfIdle to succeed after client disconnect")
	}
}

// TestFrameManager_ScheduleStopCapture tests scheduling delayed capture stop
func TestFrameManager_ScheduleStopCapture(t *testing.T) {
	cam := camera.NewMockCamera()
	if err := cam.Start(640, 480, 24, 90); err != nil {
		t.Fatalf("failed to start camera: %v", err)
	}
	defer func() { _ = cam.Stop() }()

	cfg := &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 2,
	}
	fm := NewFrameManager(cam, cfg)
	defer fm.Stop()

	// Start capture
	fm.startCapture()
	waitForCaptureState(t, fm, true)

	// Schedule stop (should stop after idle timeout ~3 seconds)
	fm.scheduleStopCapture()

	// Wait for capture to stop (with timeout longer than idle delay + buffer)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		fm.captureMu.Lock()
		started := fm.captureStarted
		fm.captureMu.Unlock()
		if !started {
			return // Success
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatal("expected capture to stop after scheduling stop")
}

// TestFrameManager_MultipleConcurrentStreams tests multiple simultaneous streams
func TestFrameManager_MultipleConcurrentStreams(t *testing.T) {
	router, cam, _ := setupTestServer(t)
	defer func() { _ = cam.Stop() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start 2 concurrent streams
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := httptest.NewRequest("GET", "/stream.mjpg", nil)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)
		}()
	}

	// Let streams run briefly
	time.Sleep(500 * time.Millisecond)

	// Cancel to stop streams
	cancel()

	// Wait for all streams to finish
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent streams did not complete in time")
	}
}

// TestFrameManager_CleanupLoop_GracefulShutdown tests cleanup loop exits cleanly
func TestFrameManager_CleanupLoop_GracefulShutdown(t *testing.T) {
	cam := camera.NewMockCamera()
	if err := cam.Start(640, 480, 24, 90); err != nil {
		t.Fatalf("failed to start camera: %v", err)
	}
	defer func() { _ = cam.Stop() }()

	fm := NewFrameManager(cam, &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 2,
	})

	// Start capture to activate cleanup loop
	fm.startCapture()
	waitForCaptureState(t, fm, true)

	// Stop frame manager (should cleanly stop cleanup loop)
	fm.Stop()

	// Verify cleanup loop exited
	select {
	case <-fm.cleanupDone:
		// Success - cleanup loop exited
	case <-time.After(1 * time.Second):
		t.Fatal("cleanup loop did not exit during shutdown")
	}
}

// TestFrameManager_FrameBuffer_ConcurrentAccess tests frame buffer concurrent reads
func TestFrameManager_FrameBuffer_ConcurrentAccess(t *testing.T) {
	cam := camera.NewMockCamera()
	if err := cam.Start(640, 480, 24, 90); err != nil {
		t.Fatalf("failed to start camera: %v", err)
	}
	defer func() { _ = cam.Stop() }()

	fm := NewFrameManager(cam, &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 2,
	})
	defer fm.Stop()

	// Start capture
	fm.startCapture()
	waitForCaptureState(t, fm, true)

	// Allow frames to be captured
	time.Sleep(200 * time.Millisecond)

	// Simulate concurrent reads from multiple clients
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Each goroutine tries to read frame 10 times
			for j := 0; j < 10; j++ {
				frame := fm.GetFrame()
				if len(frame) == 0 {
					t.Error("expected non-empty frame")
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
}

// TestFrameManager_Metrics_FrameCount tests frame counting during capture
func TestFrameManager_Metrics_FrameCount(t *testing.T) {
	cam := camera.NewMockCamera()
	if err := cam.Start(640, 480, 24, 90); err != nil {
		t.Fatalf("failed to start camera: %v", err)
	}
	defer func() { _ = cam.Stop() }()

	fm := NewFrameManager(cam, &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 2,
	})
	defer fm.Stop()

	// Start capture
	fm.startCapture()
	waitForCaptureState(t, fm, true)

	// Let some frames be captured
	time.Sleep(200 * time.Millisecond)

	// Verify that frames were captured by checking if frame buffer has data
	frame := fm.GetFrame()
	if len(frame) == 0 {
		t.Errorf("expected frames to be captured")
	}
}

// TestFrameManager_Stop_MultipleTimes tests Stop() is idempotent
func TestFrameManager_Stop_MultipleTimes(t *testing.T) {
	cam := camera.NewMockCamera()
	if err := cam.Start(640, 480, 24, 90); err != nil {
		t.Fatalf("failed to start camera: %v", err)
	}
	defer func() { _ = cam.Stop() }()

	fm := NewFrameManager(cam, &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 2,
	})

	// Start capture
	fm.startCapture()
	waitForCaptureState(t, fm, true)

	// Stop multiple times - should not panic
	fm.Stop()
	fm.Stop()
	fm.Stop()
}
