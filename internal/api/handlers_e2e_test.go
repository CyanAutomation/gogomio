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
)

// E2E Tests — End-to-end HTTP streaming and endpoint validation

// TestE2E_StreamEndpointBasic validates basic MJPEG stream structure
// Contract: TC-HTTP-01 (docs/testing/test-contracts.md).
func TestE2E_StreamEndpointBasic(t *testing.T) {
	testConfig := &config.Config{
		TargetFPS:            10,
		MaxStreamConnections: 10,
	}
	fm := NewFrameManager(newStableFrameCamera(newTestJPEGFrame(t)), testConfig)
	defer fm.Stop()

	router := RegisterHandlers(http.NewServeMux(), fm, testConfig)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := newStreamCapturingWriter(0)
	req := httptest.NewRequest("GET", "/stream.mjpg", nil)
	req = req.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		router.ServeHTTP(writer, req)
		close(done)
	}()

	guard := time.NewTimer(2 * time.Second)
	defer guard.Stop()
	var frame []byte
	select {
	case frame = <-writer.FirstFrame():
	case <-guard.C:
		cancel()
		select {
		case <-done:
		case <-time.After(250 * time.Millisecond):
			t.Fatal("stream handler did not stop after cancellation")
		}
		t.Fatal("timed out waiting for a complete multipart JPEG frame")
	}
	cancel()
	select {
	case <-done:
	case <-guard.C:
		t.Fatal("stream handler did not stop after cancellation")
	}

	if got := writer.GetStatusCode(); got != http.StatusOK {
		t.Fatalf("expected status 200, got %d", got)
	}
	if got := writer.GetHeader("Content-Type"); !strings.Contains(got, "multipart/x-mixed-replace; boundary=frame") {
		t.Fatalf("unexpected multipart content type %q", got)
	}
	assertJPEGFrame(t, frame)
}

// TestE2E_SnapshotEndpointAfterStreaming validates snapshot JPEG delivery after
// an MJPEG client has started and stopped capture.
// Contract: TC-HTTP-01 (docs/testing/test-contracts.md).
func TestE2E_SnapshotEndpointAfterStreaming(t *testing.T) {
	testJPEG := newTestJPEGFrame(t)
	testConfig := &config.Config{TargetFPS: 10, MaxStreamConnections: 2}
	fm := NewFrameManager(newStableFrameCamera(testJPEG), testConfig)
	defer fm.Stop()

	router := RegisterHandlers(http.NewServeMux(), fm, testConfig)

	streamCtx, cancelStream := context.WithCancel(context.Background())
	streamWriter := newStreamCapturingWriter(0)
	streamDone := make(chan struct{})
	streamRequest := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil).WithContext(streamCtx)
	go func() {
		router.ServeHTTP(streamWriter, streamRequest)
		close(streamDone)
	}()

	select {
	case streamedFrame := <-streamWriter.FirstFrame():
		assertJPEGFrame(t, streamedFrame)
	case <-time.After(2 * time.Second):
		cancelStream()
		select {
		case <-streamDone:
		case <-time.After(250 * time.Millisecond):
			t.Fatal("stream handler did not stop after cancellation")
		}
		t.Fatal("timed out waiting for a complete multipart JPEG before snapshot")
	}
	cancelStream()
	select {
	case <-streamDone:
	case <-time.After(2 * time.Second):
		t.Fatal("stream handler did not stop after cancellation")
	}

	// Request a snapshot after the stream has published a frame.
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

	assertJPEGFrame(t, writer.Body.Bytes())

	t.Logf("✓ Snapshot endpoint validated: %d bytes JPEG delivered", writer.Body.Len())
}

// TestE2E_ConcurrentClients validates multiple concurrent MJPEG clients
func TestE2E_ConcurrentClients(t *testing.T) {
	// Use a higher connection limit to allow concurrent streams in the test
	testConfig := &config.Config{
		TargetFPS:            20,
		MaxStreamConnections: 10, // Allow up to 10 concurrent connections
	}

	fm := NewFrameManager(newStableFrameCamera(newTestJPEGFrame(t)), testConfig)
	defer fm.Stop()

	router := RegisterHandlers(http.NewServeMux(), fm, testConfig)

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
			assertJPEGFrame(t, frame)
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

	router := RegisterHandlers(http.NewServeMux(), fm, &config.Config{})

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

	router := RegisterHandlers(http.NewServeMux(), fm, testConfig)

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

	router := RegisterHandlers(http.NewServeMux(), fm, testConfig)

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
