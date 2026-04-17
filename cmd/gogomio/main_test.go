package main

import (
	"errors"
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
