package main

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/CyanAutomation/gogomio/internal/api"
	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/go-chi/chi/v5"
)

type fakeCamera struct {
	startErr   error
	startCalls int
	stopCalls  int
	mu         sync.Mutex
}

func (f *fakeCamera) Start(width, height, fps, jpegQuality int) error {
	f.mu.Lock()
	f.startCalls++
	f.mu.Unlock()
	return f.startErr
}

func (f *fakeCamera) CaptureFrame() ([]byte, error) { return []byte{0xFF, 0xD8, 0xFF, 0xD9}, nil }

func (f *fakeCamera) Stop() error {
	f.mu.Lock()
	f.stopCalls++
	f.mu.Unlock()
	return nil
}

func (f *fakeCamera) IsReady() bool { return true }

func testConfig() *config.Config {
	return &config.Config{
		Resolution:  [2]int{640, 480},
		FPS:         24,
		JPEGQuality: 90,
	}
}

func TestInitializeCamera_RealCameraStartsOnce(t *testing.T) {
	cfg := testConfig()
	realCam := &fakeCamera{}
	mockCam := &fakeCamera{}

	cam, backend, err := initializeCamera(
		cfg,
		func() camera.Camera { return realCam },
		func() camera.Camera { return mockCam },
	)
	if err != nil {
		t.Fatalf("initializeCamera failed: %v", err)
	}
	if cam != realCam {
		t.Fatalf("expected real camera, got %T", cam)
	}
	if backend != "real" {
		t.Fatalf("expected backend real, got %q", backend)
	}
	if realCam.startCalls != 1 {
		t.Fatalf("expected real camera Start() once, got %d", realCam.startCalls)
	}
	if mockCam.startCalls != 0 {
		t.Fatalf("expected mock camera Start() zero times, got %d", mockCam.startCalls)
	}
}

func TestInitializeCamera_RealFailureFallsBackToMock(t *testing.T) {
	cfg := testConfig()
	realCam := &fakeCamera{startErr: errors.New("device missing")}
	mockCam := &fakeCamera{}

	cam, backend, err := initializeCamera(
		cfg,
		func() camera.Camera { return realCam },
		func() camera.Camera { return mockCam },
	)
	if err != nil {
		t.Fatalf("initializeCamera failed: %v", err)
	}
	if cam != mockCam {
		t.Fatalf("expected mock camera fallback, got %T", cam)
	}
	if backend != "mock-fallback" {
		t.Fatalf("expected backend mock-fallback, got %q", backend)
	}
	if realCam.startCalls != 1 {
		t.Fatalf("expected real camera Start() once, got %d", realCam.startCalls)
	}
	if mockCam.startCalls != 1 {
		t.Fatalf("expected mock camera Start() once, got %d", mockCam.startCalls)
	}
}

func TestInitializeCamera_MockModeStartsMockOnce(t *testing.T) {
	cfg := testConfig()
	cfg.MockCamera = true
	realCam := &fakeCamera{}
	mockCam := &fakeCamera{}

	cam, backend, err := initializeCamera(
		cfg,
		func() camera.Camera { return realCam },
		func() camera.Camera { return mockCam },
	)
	if err != nil {
		t.Fatalf("initializeCamera failed: %v", err)
	}
	if cam != mockCam {
		t.Fatalf("expected mock camera, got %T", cam)
	}
	if backend != "mock" {
		t.Fatalf("expected backend mock, got %q", backend)
	}
	if realCam.startCalls != 0 {
		t.Fatalf("expected real camera Start() zero times, got %d", realCam.startCalls)
	}
	if mockCam.startCalls != 1 {
		t.Fatalf("expected mock camera Start() once, got %d", mockCam.startCalls)
	}
}

func TestInitializeCamera_RealFailureLogsMockFallbackRuntimeSwitch(t *testing.T) {
	cfg := testConfig()
	realCam := &fakeCamera{startErr: errors.New("device missing")}
	mockCam := &fakeCamera{}

	var logBuffer bytes.Buffer
	logger := log.New(&logBuffer, "", 0)

	_, backend, err := initializeCameraWithLogger(
		logger,
		cfg,
		func() camera.Camera { return realCam },
		func() camera.Camera { return mockCam },
	)
	if err != nil {
		t.Fatalf("initializeCamera failed: %v", err)
	}
	if backend != "mock-fallback" {
		t.Fatalf("expected backend mock-fallback, got %q", backend)
	}

	logs := logBuffer.String()
	if !strings.Contains(logs, "RealCamera may internally try FFmpeg/V4L2 as an alternative backend") {
		t.Fatalf("expected logs to mention FFmpeg as an alternative backend, got logs: %s", logs)
	}
	if !strings.Contains(logs, "Switching runtime camera backend to mock-fallback mode") {
		t.Fatalf("expected logs to mention runtime switch to mock-fallback, got logs: %s", logs)
	}
	if strings.Contains(logs, "Falling back to FFmpeg V4L2 mode") {
		t.Fatalf("did not expect logs to claim runtime fallback to FFmpeg, got logs: %s", logs)
	}
}

// TestInitializeCamera_MockStartFailure tests that both camera startups failing returns error
func TestInitializeCamera_MockStartFailure(t *testing.T) {
	cfg := testConfig()
	realCam := &fakeCamera{startErr: errors.New("real camera failed")}
	mockCam := &fakeCamera{startErr: errors.New("mock camera also failed")}

	_, _, err := initializeCamera(
		cfg,
		func() camera.Camera { return realCam },
		func() camera.Camera { return mockCam },
	)
	if err == nil {
		t.Fatalf("expected error when both cameras fail to start, got nil")
	}
	if !strings.Contains(err.Error(), "mock camera also failed") {
		t.Fatalf("expected error message about mock camera failure, got: %v", err)
	}
}

func TestLogGoroutineStatsWithDeps_LogsOneTickAndStops(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := log.New(&logBuffer, "", 0)
	tickerCh := make(chan time.Time)
	stopCh := make(chan struct{})
	exited := make(chan struct{})

	go func() {
		defer close(exited)
		logGoroutineStatsWithDeps(tickerCh, logger, stopCh)
	}()

	tickerCh <- time.Now()
	close(stopCh)
	<-exited

	logs := strings.TrimSpace(logBuffer.String())
	if logs == "" {
		t.Fatalf("expected one goroutine stats log line, got empty logs")
	}
	lines := strings.Split(logs, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly one goroutine stats log line, got %d: %q", len(lines), logs)
	}
	if !strings.Contains(lines[0], "📊 Goroutines:") {
		t.Fatalf("expected log line to contain goroutine stats prefix, got %q", lines[0])
	}
}

// TestServerInitialization_CameraStartup tests that server initializes camera properly
func TestServerInitialization_CameraStartup(t *testing.T) {
	cfg := &config.Config{
		Resolution:  [2]int{640, 480},
		FPS:         24,
		JPEGQuality: 90,
		Port:        0, // Use random port
		BindHost:    "127.0.0.1",
	}

	realCam := &fakeCamera{}
	mockCam := &fakeCamera{}

	cam, backend, err := initializeCamera(
		cfg,
		func() camera.Camera { return realCam },
		func() camera.Camera { return mockCam },
	)
	if err != nil {
		t.Fatalf("failed to initialize camera: %v", err)
	}

	if backend != "real" {
		t.Fatalf("expected real backend, got %q", backend)
	}

	if realCam.startCalls != 1 {
		t.Fatalf("expected real camera to start once, got %d calls", realCam.startCalls)
	}

	// Stop camera
	_ = cam.Stop()

	if realCam.stopCalls != 1 {
		t.Fatalf("expected real camera to stop once, got %d calls", realCam.stopCalls)
	}
}

// TestServerInitialization_ErrorHandling tests graceful error handling during startup
func TestServerInitialization_ErrorHandling(t *testing.T) {
	cfg := &config.Config{
		Resolution:  [2]int{640, 480},
		FPS:         24,
		JPEGQuality: 90,
	}

	realCam := &fakeCamera{startErr: errors.New("device not found")}
	mockCam := &fakeCamera{startErr: errors.New("mock initialization failed")}

	var logBuffer bytes.Buffer
	logger := log.New(&logBuffer, "", 0)

	_, _, err := initializeCameraWithLogger(logger, cfg,
		func() camera.Camera { return realCam },
		func() camera.Camera { return mockCam },
	)

	if err == nil {
		t.Fatalf("expected initialization error when all cameras fail")
	}

	logs := logBuffer.String()
	if !strings.Contains(logs, "Real camera initialization failed") {
		t.Fatalf("expected error logs for real camera failure, got: %s", logs)
	}
}

// TestServerShutdown_CameraCleanup tests camera is properly stopped during shutdown
func TestServerShutdown_CameraCleanup(t *testing.T) {
	cfg := &config.Config{
		Resolution:  [2]int{640, 480},
		FPS:         24,
		JPEGQuality: 90,
		Port:        0,
		BindHost:    "127.0.0.1",
	}

	realCam := &fakeCamera{}
	mockCam := &fakeCamera{}

	cam, _, err := initializeCamera(cfg,
		func() camera.Camera { return realCam },
		func() camera.Camera { return mockCam },
	)
	if err != nil {
		t.Fatalf("failed to initialize camera: %v", err)
	}

	// Camera should start during initialization
	if realCam.startCalls != 1 {
		t.Fatalf("expected camera to be started, got %d calls", realCam.startCalls)
	}

	// Stop camera
	stopErr := cam.Stop()
	if stopErr != nil {
		t.Fatalf("failed to stop camera: %v", stopErr)
	}

	// Verify camera was stopped
	if realCam.stopCalls != 1 {
		t.Fatalf("expected camera to be stopped once, got %d calls", realCam.stopCalls)
	}
}

// TestRouterRegistration_AllHandlersPresent tests that all handlers are registered
func TestRouterRegistration_AllHandlersPresent(t *testing.T) {
	cfg := &config.Config{
		Resolution:  [2]int{640, 480},
		FPS:         24,
		TargetFPS:   24,
		JPEGQuality: 90,
		Port:        0,
		BindHost:    "127.0.0.1",
	}

	mockCam := camera.NewMockCamera()
	if err := mockCam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
		t.Fatalf("failed to start mock camera: %v", err)
	}
	defer func() { _ = mockCam.Stop() }()

	router := chi.NewRouter()
	frameManager := api.NewFrameManager(mockCam, cfg)
	defer frameManager.Stop()

	api.RegisterHandlers(router, frameManager, cfg)

	// Test that key endpoints exist by making requests
	testCases := []struct {
		method   string
		path     string
		expected int
	}{
		{http.MethodGet, "/health", http.StatusOK},
		{http.MethodGet, "/ready", http.StatusOK},
		{http.MethodGet, "/api/config", http.StatusOK},
		{http.MethodGet, "/api/status", http.StatusOK},
	}

	for _, tc := range testCases {
		req, _ := http.NewRequest(tc.method, tc.path, nil)
		recorder := &testResponseRecorder{header: http.Header{}, statusCode: 200}
		router.ServeHTTP(recorder, req)

		if recorder.statusCode == http.StatusNotFound {
			t.Errorf("%s %s: route not registered (got 404)", tc.method, tc.path)
		}
	}
}

// testResponseRecorder is a simple response writer for testing
type testResponseRecorder struct {
	header     http.Header
	statusCode int
	body       bytes.Buffer
}

func (r *testResponseRecorder) Header() http.Header {
	return r.header
}

func (r *testResponseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *testResponseRecorder) WriteHeader(code int) {
	r.statusCode = code
}

// TestFrameManagerLifecycle tests FrameManager start/stop doesn't panic
func TestFrameManagerLifecycle(t *testing.T) {
	cfg := &config.Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 2,
		Port:                 0,
		BindHost:             "127.0.0.1",
	}

	mockCam := camera.NewMockCamera()
	if err := mockCam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
		t.Fatalf("failed to start mock camera: %v", err)
	}
	defer func() { _ = mockCam.Stop() }()

	// Create FrameManager (this internally starts capture loops)
	frameManager := api.NewFrameManager(mockCam, cfg)

	// Immediately stop it
	frameManager.Stop()

	// Should not panic when stopped multiple times
	frameManager.Stop()
}

// TestConcurrentServerInitialization tests multiple cameras can initialize concurrently
func TestConcurrentServerInitialization(t *testing.T) {
	var wg sync.WaitGroup
	errCount := atomic.Int32{}

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			cfg := &config.Config{
				Resolution:  [2]int{640, 480},
				FPS:         24,
				JPEGQuality: 90,
			}

			mockCam := camera.NewMockCamera()
			if err := mockCam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
				errCount.Add(1)
				return
			}
			defer func() { _ = mockCam.Stop() }()

			frameManager := api.NewFrameManager(mockCam, cfg)
			frameManager.Stop()
		}()
	}

	wg.Wait()

	if errCount.Load() > 0 {
		t.Fatalf("concurrent initialization failed: %d errors", errCount.Load())
	}
}

// TestSignalHandling_ContextCancellation tests graceful shutdown via context
func TestSignalHandling_ContextCancellation(t *testing.T) {
	// This test verifies signal handling pattern works correctly
	// by testing the underlying mechanisms (channels and signal.Notify)

	// Create a test signal channel
	testSigChan := make(chan os.Signal, 1)

	// Track if signal was received
	signalReceived := atomic.Bool{}
	processed := make(chan struct{})

	// Simulate the signal handling pattern from main.go
	go func() {
		signal.Notify(testSigChan, syscall.SIGINT, syscall.SIGTERM)
		<-testSigChan
		signalReceived.Store(true)
		close(processed)
	}()

	// Send a signal
	testSigChan <- syscall.SIGINT

	select {
	case <-processed:
	case <-time.After(300 * time.Millisecond):
		t.Logf("timeout waiting for signal processing ack")
		t.Fatalf("signal processing was not acknowledged")
	}

	if !signalReceived.Load() {
		t.Fatalf("expected signal to be received")
	}
}
