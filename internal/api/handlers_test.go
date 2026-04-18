package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

	if string(p) == "--frame\r\n" {
		w.boundaries++
		if w.boundaries >= w.targetFrames {
			return 0, errStopStream
		}
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

	headerDeadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(headerDeadline) {
		if recorder.Header().Get("Content-Type") != "" {
			break
		}
		select {
		case <-done:
			t.Fatal("stream ended before headers were written")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

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

	boundaryDeadline := time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(boundaryDeadline) {
		if strings.Contains(recorder.Body.String(), "--frame\r\n") {
			break
		}
		select {
		case <-done:
			t.Fatal("stream ended before first frame boundary")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	if !strings.Contains(recorder.Body.String(), "--frame\r\n") {
		t.Fatal("expected at least one frame boundary before cancellation")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("stream did not stop after context cancellation")
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

	req, _ := http.NewRequest("GET", "/stream.mjpg", nil)
	w := httptest.NewRecorder()

	// Run request in goroutine with timeout
	done := make(chan bool)
	go func() {
		router.ServeHTTP(w, req)
		done <- true
	}()

	// Let streaming run for a moment to accumulate frames
	time.Sleep(600 * time.Millisecond)

	// Verify response headers (safe to check anytime during streaming)
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
	req1, _ := http.NewRequest("GET", "/stream.mjpg", nil)
	w1 := httptest.NewRecorder()
	go router.ServeHTTP(w1, req1)
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

		go func() {
			req := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil)
			errCh <- fm.StreamFrame(w, req, cfg.MaxStreamConnections)
		}()

		time.Sleep(15 * time.Millisecond)
		fm.stopCapture()

		select {
		case err := <-errCh:
			if err == nil {
				t.Fatalf("expected stream to stop with an error")
			}
			if !strings.Contains(err.Error(), "stream stopped") {
				t.Fatalf("expected stream stopped error, got %v", err)
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
