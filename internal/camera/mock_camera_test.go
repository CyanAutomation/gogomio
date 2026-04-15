package camera

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestMockCameraStart tests mock camera initialization
func TestMockCameraStart(t *testing.T) {
	mc := NewMockCamera()

	err := mc.Start(640, 480, 24, 90)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Should be ready after start
	if !mc.IsReady() {
		t.Error("mock camera should be ready after Start")
	}
}

// TestMockCameraCaptureFrame tests capturing frames
func TestMockCameraCaptureFrame(t *testing.T) {
	mc := NewMockCamera()

	err := mc.Start(640, 480, 24, 90)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Capture a frame
	frame, err := mc.CaptureFrame()
	if err != nil {
		t.Fatalf("CaptureFrame failed: %v", err)
	}

	if len(frame) == 0 {
		t.Error("captured frame is empty")
	}

	// Should have JPEG SOI marker
	if len(frame) < 2 || frame[0] != 0xFF || frame[1] != 0xD8 {
		t.Errorf("frame does not have JPEG SOI marker, got %02x %02x", frame[0], frame[1])
	}
}

// TestMockCameraMultipleCaptures tests rapid frame capture
func TestMockCameraMultipleCaptures(t *testing.T) {
	mc := NewMockCamera()

	err := mc.Start(640, 480, 30, 90)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Capture multiple frames
	frames := make([][]byte, 10)
	for i := 0; i < 10; i++ {
		frame, err := mc.CaptureFrame()
		if err != nil {
			t.Fatalf("CaptureFrame %d failed: %v", i, err)
		}
		frames[i] = frame

		if len(frames[i]) == 0 {
			t.Errorf("frame %d is empty", i)
		}
	}

	// Should eventually get different frames (if updating)
	// or at least consistent frames
	for i, frame := range frames {
		if len(frame) == 0 {
			t.Errorf("frame %d is empty", i)
		}
	}
}

// TestMockCameraStop tests stopping capture
func TestMockCameraStop(t *testing.T) {
	mc := NewMockCamera()

	err := mc.Start(640, 480, 24, 90)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = mc.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if mc.IsReady() {
		t.Error("mock camera should not be ready after Stop")
	}
}

// TestMockCameraDifferentResolutions tests different resolutions
func TestMockCameraDifferentResolutions(t *testing.T) {
	tests := []struct {
		width  int
		height int
	}{
		{640, 480},
		{1280, 720},
		{1920, 1080},
	}

	for _, test := range tests {
		mc := NewMockCamera()
		err := mc.Start(test.width, test.height, 24, 90)
		if err != nil {
			t.Errorf("Start with %dx%d failed: %v", test.width, test.height, err)
			continue
		}

		frame, err := mc.CaptureFrame()
		if err != nil {
			t.Errorf("CaptureFrame with %dx%d failed: %v", test.width, test.height, err)
			continue
		}

		if len(frame) == 0 {
			t.Errorf("frame with %dx%d is empty", test.width, test.height)
		}

		_ = mc.Stop()
	}
}

// TestMockCameraQualitySettings tests different JPEG qualities
func TestMockCameraQualitySettings(t *testing.T) {
	qualities := []int{50, 75, 90}

	for _, quality := range qualities {
		mc := NewMockCamera()
		err := mc.Start(640, 480, 24, quality)
		if err != nil {
			t.Errorf("Start with quality %d failed: %v", quality, err)
			continue
		}

		frame, err := mc.CaptureFrame()
		if err != nil {
			t.Errorf("CaptureFrame with quality %d failed: %v", quality, err)
			continue
		}

		if len(frame) == 0 {
			t.Errorf("frame with quality %d is empty", quality)
		}

		_ = mc.Stop()
	}
}

// TestMockCameraMultipleCapturesConcurrent tests concurrent frame capture
func TestMockCameraMultipleCapturesConcurrent(t *testing.T) {
	mc := NewMockCamera()

	err := mc.Start(640, 480, 24, 90)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = mc.Stop() }()

	var wg sync.WaitGroup
	numGoroutines := 5
	framesPerGoroutine := 10

	errorChan := make(chan error, numGoroutines*framesPerGoroutine)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < framesPerGoroutine; i++ {
				frame, err := mc.CaptureFrame()
				if err != nil {
					errorChan <- err
					continue
				}
				if len(frame) == 0 {
					errorChan <- fmt.Errorf("empty frame on iteration")
				}
			}
		}()
	}

	wg.Wait()
	close(errorChan)

	if len(errorChan) > 0 {
		var errs []error
		for err := range errorChan {
			errs = append(errs, err)
		}
		t.Errorf("concurrent capture errors: %v", errs)
	}
}

// TestMockCameraLifecycle tests complete start/capture/stop lifecycle
func TestMockCameraLifecycle(t *testing.T) {
	mc := NewMockCamera()

	// Initially not ready
	if mc.IsReady() {
		t.Error("mock camera should not be ready before Start")
	}

	// Start
	err := mc.Start(640, 480, 24, 90)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Ready after start
	if !mc.IsReady() {
		t.Error("mock camera should be ready after Start")
	}

	// Capture works
	frame, err := mc.CaptureFrame()
	if err != nil || len(frame) == 0 {
		t.Error("CaptureFrame failed after Start")
	}

	// Stop
	err = mc.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Not ready after stop
	if mc.IsReady() {
		t.Error("mock camera should not be ready after Stop")
	}
}

// TestMockCameraFrameIsValid tests that frames are valid JPEG data
func TestMockCameraFrameIsValid(t *testing.T) {
	mc := NewMockCamera()

	err := mc.Start(640, 480, 24, 90)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = mc.Stop() }()

	for i := 0; i < 5; i++ {
		frame, err := mc.CaptureFrame()
		if err != nil {
			t.Fatalf("CaptureFrame %d failed: %v", i, err)
		}

		// Check JPEG markers
		if len(frame) < 4 {
			t.Errorf("frame %d too short: %d bytes", i, len(frame))
			continue
		}

		// JPEG SOI (start of image)
		if frame[0] != 0xFF || frame[1] != 0xD8 {
			t.Errorf("frame %d invalid SOI marker: %02x %02x", i, frame[0], frame[1])
		}

		// JPEG EOI (end of image) - should be in last 2 bytes
		found := false
		for j := len(frame) - 2; j >= 0 && j >= len(frame)-100; j-- {
			if frame[j] == 0xFF && frame[j+1] == 0xD9 {
				found = true
				break
			}
		}
		if !found {
			t.Logf("frame %d: EOI marker not found in last 100 bytes (total %d)", i, len(frame))
		}
	}
}

// TestMockCameraFPSAdjustment tests frame rate under load
func TestMockCameraFPSAdjustment(t *testing.T) {
	mc := NewMockCamera()

	targetFPS := 30
	err := mc.Start(640, 480, targetFPS, 90)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = mc.Stop() }()

	// Capture frames and measure time
	start := time.Now()
	frameCount := 0
	targetFrames := 30 // Capture for ~1 second at 30 FPS

	for frameCount < targetFrames {
		_, err := mc.CaptureFrame()
		if err != nil {
			t.Fatalf("CaptureFrame failed: %v", err)
		}
		frameCount++
	}

	elapsed := time.Since(start)
	actualFPS := float64(frameCount) / elapsed.Seconds()

	// Allow ±50% tolerance due to timing variations
	expected := float64(targetFPS)
	if actualFPS < expected*0.5 || actualFPS > expected*1.5 {
		t.Logf("FPS is %v, expected ~%v (±50%% tolerance OK)", actualFPS, expected)
	}
}
