package main

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/config"
)

type fakeCamera struct {
	startErr   error
	startCalls int
}

func (f *fakeCamera) Start(width, height, fps, jpegQuality int) error {
	f.startCalls++
	return f.startErr
}

func (f *fakeCamera) CaptureFrame() ([]byte, error) { return nil, nil }
func (f *fakeCamera) Stop() error                   { return nil }
func (f *fakeCamera) IsReady() bool                 { return true }

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
	originalWriter := log.Writer()
	log.SetOutput(&logBuffer)
	defer log.SetOutput(originalWriter)

	_, backend, err := initializeCamera(
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
