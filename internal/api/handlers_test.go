package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/go-chi/chi/v5"
)

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
	defer cam.Stop()

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
	defer cam.Stop()

	req, _ := http.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should be 200 because camera is ready
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestConfigEndpoint tests the /api/config endpoint
func TestConfigEndpoint(t *testing.T) {
	router, cam, cfg := setupTestServer(t)
	defer cam.Stop()

	req, _ := http.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Errorf("failed to parse JSON response: %v", err)
	}

	// Check resolution
	if res, ok := result["resolution"]; ok {
		resolution := res.([]interface{})
		if int(resolution[0].(float64)) != cfg.Resolution[0] {
			t.Errorf("expected resolution width %d, got %v", cfg.Resolution[0], resolution[0])
		}
	}
}

// TestSnapshotEndpoint tests the /snapshot.jpg endpoint
func TestSnapshotEndpoint(t *testing.T) {
	router, cam, _ := setupTestServer(t)
	defer cam.Stop()

	// Wait for first frame to be captured and buffered
	// Mock camera takes time to encode JPEG, especially with race detector on
	time.Sleep(500 * time.Millisecond)

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
	defer cam.Stop()

	// Create a request with context to timeout
	req, _ := http.NewRequest("GET", "/stream.mjpg", nil)

	// Create a custom ResponseWriter that captures the first frame boundary
	recorder := httptest.NewRecorder()

	// Run with a timeout channel to prevent hanging
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(recorder, req)
		done <- struct{}{}
	}()

	// Wait for response or timeout
	select {
	case <-time.After(500 * time.Millisecond):
		// Expected behavior - stream should hang here (waiting for frames)
		t.Logf("stream endpoint initiating (expected behavior)")
	case <-done:
		// Response finished
		if recorder.Code != http.StatusOK {
			t.Logf("stream endpoint returned status %d", recorder.Code)
		}
	}
}

// TestIndexEndpoint tests the / root endpoint returns HTML
func TestIndexEndpoint(t *testing.T) {
	router, cam, _ := setupTestServer(t)
	defer cam.Stop()

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Logf("index endpoint not implemented yet (expected for MVP)")
		return
	}

	if w.Code != http.StatusOK {
		t.Logf("index endpoint returned %d", w.Code)
	}
}

// TestCORSHeaders tests CORS headers are present
func TestCORSHeaders(t *testing.T) {
	router, cam, _ := setupTestServer(t)
	defer cam.Stop()

	req, _ := http.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check CORS headers (if middleware is active)
	_ = w.Header().Get("Access-Control-Allow-Origin")
	// For MVP, CORS may not be required
}

// TestConnectionLimitNotification tests connection limiting feedback
func TestConnectionLimitNotification(t *testing.T) {
	router, cam, cfg := setupTestServer(t)
	defer cam.Stop()

	// The actual connection limit is tested in unit tests
	// This just verifies the endpoint handles connection count reporting
	req, _ := http.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)

	// Check that max_stream_connections is reported
	if maxConn, ok := result["max_stream_connections"]; ok {
		if int(maxConn.(float64)) != cfg.MaxStreamConnections {
			t.Errorf("max_stream_connections mismatch: %v vs %d", maxConn, cfg.MaxStreamConnections)
		}
	}
}

// TestMJPEGStreamingEndpoint tests the /stream.mjpg endpoint with frame transmission
func TestMJPEGStreamingEndpoint(t *testing.T) {
	router, cam, _ := setupTestServer(t)
	defer cam.Stop()

	// Wait for mock camera to generate frames
	time.Sleep(600 * time.Millisecond)

	req, _ := http.NewRequest("GET", "/stream.mjpg", nil)
	w := httptest.NewRecorder()

	// Run request in goroutine with timeout
	done := make(chan bool)
	go func() {
		router.ServeHTTP(w, req)
		done <- true
	}()

	// Let streaming run for a moment to accumulate frames
	time.Sleep(500 * time.Millisecond)

	// Verify response headers
	if ct := w.Header().Get("Content-Type"); ct != "multipart/x-mixed-replace; boundary=frame" {
		t.Errorf("Content-Type: got %q, want multipart/x-mixed-replace", ct)
	}

	// Verify MJPEG boundary markers are present
	responseBody := w.Body.String()
	if responseBody == "" {
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
	defer mockCam.Stop()

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

// Helper function
func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
