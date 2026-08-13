package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/config"
)

type fakeCamera struct {
	startErr   error
	startCalls int
	stopCalls  int
	mu         sync.Mutex
}

type trackingListener struct {
	net.Listener
	camera                  *fakeCamera
	closeCalls              int
	closeAfterCameraStopped bool
	mu                      sync.Mutex
}

func (l *trackingListener) Close() error {
	l.camera.mu.Lock()
	cameraStopped := l.camera.stopCalls > 0
	l.camera.mu.Unlock()

	l.mu.Lock()
	l.closeCalls++
	l.closeAfterCameraStopped = cameraStopped
	l.mu.Unlock()
	return l.Listener.Close()
}

func (f *fakeCamera) Start(width, height, fps, jpegQuality int) error {
	f.mu.Lock()
	f.startCalls++
	f.mu.Unlock()
	return f.startErr
}

func (f *fakeCamera) CaptureFrame() ([]byte, error) { return []byte{0xFF, 0xD8, 0xFF, 0xD9}, nil }

func (f *fakeCamera) CaptureFrameWithContext(ctx context.Context) ([]byte, error) {
	return f.CaptureFrame()
}

func (f *fakeCamera) Stop() error {
	f.mu.Lock()
	f.stopCalls++
	f.mu.Unlock()
	return nil
}

func (f *fakeCamera) IsReady() bool { return true }

func testConfig() *config.Config {
	return &config.Config{
		Resolution:  [2]int{640, 640},
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
		Resolution:  [2]int{640, 640},
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
		Resolution:  [2]int{640, 640},
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
		Resolution:  [2]int{640, 640},
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

// TestConcurrentServerInitialization verifies that the production application
// initializer does not share lifecycle state between concurrent applications.
func TestConcurrentServerInitialization(t *testing.T) {
	const applicationCount = 3

	apps := make([]*application, applicationCount)
	cameras := make([]*fakeCamera, applicationCount)
	listeners := make([]*trackingListener, applicationCount)
	errs := make(chan error, applicationCount)
	start := make(chan struct{})
	var ready sync.WaitGroup
	var wg sync.WaitGroup
	ready.Add(applicationCount)

	for i := 0; i < applicationCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			ready.Done()
			<-start

			cam := &fakeCamera{}
			cameras[index] = cam
			app, _, err := initializeApplication(&config.Config{
				Resolution:  [2]int{640, 480},
				FPS:         24,
				TargetFPS:   24,
				JPEGQuality: 90,
				MockCamera:  true,
				BindHost:    "127.0.0.1",
				Port:        0,
			}, applicationDependencies{
				newRealCamera: func() camera.Camera { return &fakeCamera{} },
				newMockCamera: func() camera.Camera { return cam },
				listen: func(network, address string) (net.Listener, error) {
					listener, listenErr := net.Listen(network, address)
					if listenErr != nil {
						return nil, listenErr
					}
					tracked := &trackingListener{Listener: listener, camera: cam}
					listeners[index] = tracked
					return tracked, nil
				},
			})
			if err != nil {
				errs <- err
				return
			}
			apps[index] = app
		}(i)
	}

	ready.Wait()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent application initialization failed: %v", err)
	}

	for i, app := range apps {
		if app == nil {
			t.Fatalf("application %d was not initialized", i)
		}
		if app.camera != cameras[i] || app.listener != listeners[i] {
			t.Fatalf("application %d does not own its injected resources", i)
		}
		for j := 0; j < i; j++ {
			if app.camera == apps[j].camera || app.frameManager == apps[j].frameManager ||
				app.router == apps[j].router || app.listener == apps[j].listener {
				t.Fatalf("applications %d and %d share initialization state", i, j)
			}
		}
	}

	apps[0].cleanup()
	if cameras[0].stopCalls != 1 || listeners[0].closeCalls != 1 || !listeners[0].closeAfterCameraStopped {
		t.Fatal("first application cleanup did not close its camera and listener")
	}
	for i := 1; i < applicationCount; i++ {
		if cameras[i].stopCalls != 0 || listeners[i].closeCalls != 0 {
			t.Fatalf("cleaning application 0 affected application %d", i)
		}
		apps[i].cleanup()
		if cameras[i].stopCalls != 1 || listeners[i].closeCalls != 1 || !listeners[i].closeAfterCameraStopped {
			t.Fatalf("application %d did not run its independent cleanup", i)
		}
	}
}

// TestNewShutdownContext_SignalCancelsApplicationContext covers REQ-GRACEFUL-SHUTDOWN.
func TestNewShutdownContext_SignalCancelsApplicationContext(t *testing.T) {
	testSigChan := make(chan os.Signal, 1)
	appCtx, cancel := newShutdownContext(context.Background(), testSigChan)
	t.Cleanup(cancel)

	testSigChan <- syscall.SIGINT
	select {
	case <-appCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for application context cancellation")
	}

	if !errors.Is(appCtx.Err(), context.Canceled) {
		t.Fatalf("expected application context to be cancelled, got %v", appCtx.Err())
	}
}
