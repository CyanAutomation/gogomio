package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/go-chi/chi/v5"
)

// E2E Tests — End-to-end HTTP streaming and endpoint validation

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

	const numClients = 2
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	guard, stopGuard := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopGuard()

	writers := make([]*streamCapturingWriter, numClients)
	done := make([]chan struct{}, numClients)
	for i := range writers {
		writers[i] = newStreamCapturingWriter(50 * 1024)
		done[i] = make(chan struct{})
		req := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil).WithContext(ctx)
		go func(clientID int) {
			router.ServeHTTP(writers[clientID], req)
			close(done[clientID])
		}(i)
	}

	for clientID, writer := range writers {
		select {
		case frame := <-writer.FirstFrame():
			if writer.GetStatusCode() != http.StatusOK {
				t.Fatalf("client-%d: expected status 200, got %d", clientID, writer.GetStatusCode())
			}
			if !strings.Contains(writer.GetHeader("Content-Type"), "multipart/x-mixed-replace; boundary=frame") {
				t.Fatalf("client-%d: invalid content type %q", clientID, writer.GetHeader("Content-Type"))
			}
			if len(frame) < 4 || frame[0] != 0xff || frame[1] != 0xd8 || frame[len(frame)-2] != 0xff || frame[len(frame)-1] != 0xd9 {
				t.Fatalf("client-%d: multipart payload is not a complete JPEG: %x", clientID, frame)
			}
		case <-guard.Done():
			t.Fatalf("timed out waiting for client-%d to receive a complete multipart JPEG frame", clientID)
		}
	}

	if connections := fm.connTracker.Count(); connections != numClients {
		t.Fatalf("expected both clients to be concurrently registered, got %d", connections)
	}
	if clients := atomic.LoadInt64(&fm.clientCount); clients != numClients {
		t.Fatalf("expected both streaming clients to be active, got %d", clients)
	}

	cancel()
	for clientID, clientDone := range done {
		select {
		case <-clientDone:
		case <-guard.Done():
			t.Fatalf("timed out waiting for client-%d connection release", clientID)
		}
	}
	if connections := fm.connTracker.Count(); connections != 0 {
		t.Fatalf("expected connection count to return to zero, got %d", connections)
	}
	if clients := atomic.LoadInt64(&fm.clientCount); clients != 0 {
		t.Fatalf("expected streaming client count to return to zero, got %d", clients)
	}
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

	testConfig := &config.Config{
		TargetFPS:            20,
		MaxStreamConnections: 1,
	}
	fm := NewFrameManager(cam, testConfig)
	defer fm.Stop()

	router := chi.NewRouter()
	RegisterHandlers(router, fm, testConfig)

	// Simulate a disconnecting client using a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/stream.mjpg", nil)
	req = req.WithContext(ctx)

	writer := newStreamCapturingWriter(50 * 1024)
	connectionCountBefore := atomic.LoadInt64(&fm.clientCount)

	// Start streaming in background
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(writer, req)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	// This is the only timeout in the test: it guards both observable milestones
	// so a regression cannot deadlock the suite.
	deadlockGuard, stopDeadlockGuard := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopDeadlockGuard()
	admissionPoll := time.NewTicker(time.Millisecond)
	defer admissionPoll.Stop()
	admitted := false
	for !admitted {
		select {
		case <-admissionPoll.C:
			admitted = atomic.LoadInt64(&fm.clientCount) > connectionCountBefore
		case <-done:
			statusCode := writer.GetStatusCode()
			if statusCode == http.StatusTooManyRequests {
				t.Fatalf("stream request was not admitted: got status %d (check MaxStreamConnections)", statusCode)
			}
			t.Fatalf("stream handler exited before request admission with status %d", statusCode)
		case <-deadlockGuard.Done():
			t.Fatal("timed out waiting for stream request admission")
		}
	}
	if statusCode := writer.GetStatusCode(); statusCode == http.StatusTooManyRequests {
		t.Fatalf("stream request was not admitted: got status %d (check MaxStreamConnections)", statusCode)
	}

	select {
	case <-writer.FirstBoundary():
	case <-deadlockGuard.Done():
		t.Fatal("timed out waiting for the first complete MJPEG boundary")
	}

	// Simulate client disconnect by canceling context
	cancel()

	// Wait for handler to complete
	select {
	case <-done:
		t.Log("  ✓ Handler completed after client disconnect")
	case <-deadlockGuard.Done():
		t.Fatal("handler did not complete after disconnect (possible goroutine leak)")
	}

	if connectionCount := atomic.LoadInt64(&fm.clientCount); connectionCount != connectionCountBefore {
		t.Fatalf("stream connection count did not return to %d after disconnect: got %d", connectionCountBefore, connectionCount)
	}

	t.Logf("✓ Client disconnection handled cleanly: %d bytes streamed before disconnect", writer.GetBytesWritten())
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
