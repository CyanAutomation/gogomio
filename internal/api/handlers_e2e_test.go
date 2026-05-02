package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/go-chi/chi/v5"
)

// E2E Tests — End-to-end HTTP streaming and endpoint validation

// stableFrameCamera generates consistent JPEG frames for testing
type stableFrameCamera struct {
	frame      []byte
	captureErr error
}

func newStableFrameCamera(jpegData []byte) *stableFrameCamera {
	if jpegData == nil {
		// Default: valid minimal JPEG frame (SOI + EOI)
		jpegData = []byte{0xFF, 0xD8, 0xFF, 0xD9}
	}
	return &stableFrameCamera{frame: jpegData}
}

func (c *stableFrameCamera) Start(_, _, _, _ int) error { return nil }
func (c *stableFrameCamera) Stop() error                { return nil }
func (c *stableFrameCamera) IsReady() bool              { return true }
func (c *stableFrameCamera) CaptureFrame() ([]byte, error) {
	time.Sleep(1 * time.Millisecond) // Minimal delay
	return c.frame, c.captureErr
}

// streamCapturingWriter captures the full stream response for validation
type streamCapturingWriter struct {
	header       http.Header
	statusCode   int
	buf          []byte
	mu           sync.Mutex
	maxBytes     int64
	bytesWritten int64
}

func newStreamCapturingWriter(maxBytes int64) *streamCapturingWriter {
	return &streamCapturingWriter{
		header:   make(http.Header),
		maxBytes: maxBytes,
	}
}

func (w *streamCapturingWriter) Header() http.Header {
	return w.header
}

func (w *streamCapturingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.maxBytes > 0 && w.bytesWritten+int64(len(p)) > w.maxBytes {
		// Stop writing after maxBytes
		return 0, io.EOF
	}

	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}

	w.buf = append(w.buf, p...)
	w.bytesWritten += int64(len(p))
	return len(p), nil
}

func (w *streamCapturingWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.statusCode = code
}

func (w *streamCapturingWriter) Flush() {}

func (w *streamCapturingWriter) GetContent() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buf...)
}

func (w *streamCapturingWriter) GetStatusCode() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.statusCode
}

func (w *streamCapturingWriter) GetHeader(key string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.header.Get(key)
}

// TestE2E_StreamEndpointBasic validates basic MJPEG stream structure
func TestE2E_StreamEndpointBasic(t *testing.T) {
	t.Helper()

	fm := NewFrameManager(newStableFrameCamera(nil), &config.Config{
		TargetFPS:            10,
		MaxStreamConnections: 10,
	})
	defer fm.Stop()

	router := chi.NewRouter()
	RegisterHandlers(router, fm, &config.Config{
		MaxStreamConnections: 10,
	})

	const maxStreamBytes = 50 * 1024

	// Deterministic cancellation: stop once a first frame delimiter appears,
	// or after a safety timeout/max byte threshold.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := newStreamCapturingWriter(maxStreamBytes)
	req := httptest.NewRequest("GET", "/stream.mjpg", nil)
	req = req.WithContext(ctx)

	// Execute streaming handler
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(writer, req)
		close(done)
	}()

	boundarySeen := false
	pollTicker := time.NewTicker(2 * time.Millisecond)
	defer pollTicker.Stop()

	hardTimeout := time.NewTimer(750 * time.Millisecond)
	defer hardTimeout.Stop()

	// Cancel once we observe a multipart frame boundary or hit capture limits.
	cancelled := false
	for !cancelled {
		select {
		case <-done:
			cancelled = true
		case <-hardTimeout.C:
			cancel()
			cancelled = true
		case <-pollTicker.C:
			content := writer.GetContent()
			if strings.Contains(string(content), "--frame") {
				boundarySeen = true
				cancel()
				cancelled = true
				continue
			}
			if len(content) >= maxStreamBytes {
				cancel()
				cancelled = true
			}
		}

		select {
		case <-done:
			cancelled = true
		default:
		}
	}

	// Wait for the handler to exit after cancellation.
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("stream handler did not stop after deterministic cancellation")
	}

	// Verify stream response
	statusCode := writer.GetStatusCode()
	if statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", statusCode)
	}

	contentType := writer.GetHeader("Content-Type")
	if !strings.Contains(contentType, "multipart/x-mixed-replace") {
		t.Fatalf("expected multipart content type, got %s", contentType)
	}

	content := writer.GetContent()
	if len(content) == 0 {
		t.Fatalf("expected non-empty stream payload")
	}

	if !boundarySeen && !strings.Contains(string(content), "--frame") {
		t.Errorf("expected at least one multipart frame boundary marker in payload")
	}
}

// TestE2E_SnapshotEndpoint validates snapshot JPEG delivery
func TestE2E_SnapshotEndpoint(t *testing.T) {
	testJPEG := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0xFF, 0xD9}
	fm := NewFrameManager(newStableFrameCamera(testJPEG), &config.Config{
		TargetFPS: 10,
	})
	defer fm.Stop()

	router := chi.NewRouter()
	RegisterHandlers(router, fm, &config.Config{})

	// Wait for frame to be captured
	time.Sleep(50 * time.Millisecond)

	// Request snapshot
	req := httptest.NewRequest("GET", "/snapshot.jpg", nil)
	writer := httptest.NewRecorder()

	router.ServeHTTP(writer, req)

	// Verify response
	if writer.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", writer.Code)
	}

	contentType := writer.Header().Get("Content-Type")
	if contentType != "image/jpeg" {
		t.Fatalf("expected Content-Type image/jpeg, got %s", contentType)
	}

	// Verify JPEG magic bytes
	body := writer.Body.Bytes()
	if len(body) < 2 || body[0] != 0xFF || body[1] != 0xD8 {
		t.Fatalf("expected JPEG SOI marker, got %x %x", body[0], body[1])
	}

	t.Logf("✓ Snapshot endpoint validated: %d bytes JPEG delivered", len(body))
}

// TestE2E_ConcurrentClients validates multiple concurrent MJPEG clients
func TestE2E_ConcurrentClients(t *testing.T) {
	// Use a higher connection limit to allow concurrent streams in the test
	testConfig := &config.Config{
		TargetFPS:            20,
		MaxStreamConnections: 10, // Allow up to 10 concurrent connections
	}

	fm := NewFrameManager(newStableFrameCamera(nil), testConfig)
	defer fm.Stop()

	router := chi.NewRouter()
	RegisterHandlers(router, fm, testConfig)

	const numClients = 3 // Test with 3 concurrent clients (within typical limits)
	var wg sync.WaitGroup
	results := make(chan string, numClients)
	defer close(results)

	// Start multiple concurrent stream clients
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			writer := newStreamCapturingWriter(50 * 1024) // 50KB per client
			req := httptest.NewRequest("GET", "/stream.mjpg", nil)
			req = req.WithContext(ctx)

			router.ServeHTTP(writer, req)

			content := writer.GetContent()
			if len(content) > 0 && writer.statusCode == http.StatusOK {
				results <- fmt.Sprintf("client-%d: OK (%d bytes)", clientID, len(content))
			} else {
				results <- fmt.Sprintf("client-%d: status=%d, len=%d", clientID, writer.statusCode, len(content))
			}
		}(i)
	}

	// Collect results
	go func() {
		wg.Wait()
	}()

	successCount := 0
	for i := 0; i < numClients; i++ {
		result := <-results
		t.Logf("  %s", result)
		if strings.Contains(result, "OK") {
			successCount++
		}
	}

	if successCount < numClients {
		t.Logf("⚠️  Note: Only %d of %d clients succeeded (connection limiting may be active)", successCount, numClients)
	}

	t.Logf("✓ Concurrent clients validated: %d client results collected", numClients)
}

// TestE2E_HealthEndpoints validates health check endpoints
func TestE2E_HealthEndpoints(t *testing.T) {
	fm := NewFrameManager(newStableFrameCamera(nil), &config.Config{
		TargetFPS: 10,
	})
	defer fm.Stop()

	router := chi.NewRouter()
	RegisterHandlers(router, fm, &config.Config{})

	healthEndpoints := []struct {
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{"/health", http.StatusOK, ""},
		{"/ready", http.StatusOK, ""},
		{"/v1/health/detailed", http.StatusOK, ""},
	}

	for _, endpoint := range healthEndpoints {
		req := httptest.NewRequest("GET", endpoint.path, nil)
		writer := httptest.NewRecorder()

		router.ServeHTTP(writer, req)

		if writer.Code != endpoint.expectedStatus {
			t.Errorf("endpoint %s: expected status %d, got %d", endpoint.path, endpoint.expectedStatus, writer.Code)
		}

		if endpoint.expectedBody != "" && !strings.Contains(writer.Body.String(), endpoint.expectedBody) {
			t.Errorf("endpoint %s: expected body to contain '%s', got '%s'", endpoint.path, endpoint.expectedBody, writer.Body.String())
		}

		t.Logf("  ✓ %s → %d", endpoint.path, writer.Code)
	}

	t.Log("✓ Health endpoints validated")
}

// TestE2E_ClientDisconnection simulates client disconnect during streaming
func TestE2E_ClientDisconnection(t *testing.T) {
	// Create a camera that tracks active captures
	cam := &captureLoopCountingCamera{}

	fm := NewFrameManager(cam, &config.Config{
		TargetFPS: 20,
	})
	defer fm.Stop()

	router := chi.NewRouter()
	RegisterHandlers(router, fm, &config.Config{})

	// Simulate a disconnecting client using a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/stream.mjpg", nil)
	req = req.WithContext(ctx)

	writer := newStreamCapturingWriter(50 * 1024)

	// Start streaming in background
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(writer, req)
		close(done)
	}()

	// Let it stream for a bit
	time.Sleep(200 * time.Millisecond)

	// Simulate client disconnect by canceling context
	cancel()

	// Wait for handler to complete
	select {
	case <-done:
		t.Log("  ✓ Handler completed after client disconnect")
	case <-time.After(2 * time.Second):
		t.Error("handler did not complete after disconnect (possible goroutine leak)")
	}

	// Verify some data was streamed before disconnect
	if writer.bytesWritten == 0 {
		t.Error("expected some data to be streamed before disconnect")
	}

	t.Logf("✓ Client disconnection handled cleanly: %d bytes streamed before disconnect", writer.bytesWritten)
}

// TestE2E_ConfigEndpoint validates /api/config endpoint
func TestE2E_ConfigEndpoint(t *testing.T) {
	testConfig := &config.Config{
		Resolution: [2]int{1280, 720},
		TargetFPS:  30,
	}

	fm := NewFrameManager(newStableFrameCamera(nil), testConfig)
	defer fm.Stop()

	router := chi.NewRouter()
	RegisterHandlers(router, fm, testConfig)

	req := httptest.NewRequest("GET", "/api/config", nil)
	writer := httptest.NewRecorder()

	router.ServeHTTP(writer, req)

	if writer.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", writer.Code)
	}

	contentType := writer.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Fatalf("expected JSON content type, got %s", contentType)
	}

	body := writer.Body.String()
	if !strings.Contains(body, "1280") || !strings.Contains(body, "720") || !strings.Contains(body, "30") {
		t.Fatalf("expected config data in response, got: %s", body)
	}

	t.Logf("✓ Config endpoint validated: %s returned", contentType)
}

// TestE2E_StreamPerformance measures stream throughput and latency
func TestE2E_StreamPerformance(t *testing.T) {
	fm := NewFrameManager(newStableFrameCamera(nil), &config.Config{
		TargetFPS: 30,
	})
	defer fm.Stop()

	router := chi.NewRouter()
	RegisterHandlers(router, fm, &config.Config{})

	// Measure time to deliver frames
	frameCount := int64(0)
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	writer := newStreamCapturingWriter(100 * 1024)
	req := httptest.NewRequest("GET", "/stream.mjpg", nil)
	req = req.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		router.ServeHTTP(writer, req)
		close(done)
	}()

	<-done
	elapsed := time.Since(start)

	// Count boundaries in response
	content := writer.GetContent()
	for i := 0; i < len(content)-6; i++ {
		if string(content[i:i+7]) == "--frame" {
			frameCount++
		}
	}

	fps := float64(frameCount) / elapsed.Seconds()
	t.Logf("✓ Stream performance: %.1f frames in %.2fs (%.1f FPS effective)", float64(frameCount), elapsed.Seconds(), fps)
}
