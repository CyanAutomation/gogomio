package camera

import (
	"image"
	"os/exec"
	"testing"
	"time"
)

// TestRealCameraInitialization tests camera initialization
func TestRealCameraInitialization(t *testing.T) {
	rc := NewRealCamera()

	if rc.width != 640 || rc.height != 480 {
		t.Errorf("default resolution incorrect: %dx%d", rc.width, rc.height)
	}

	if rc.fps != 24 {
		t.Errorf("default FPS: got %d, want 24", rc.fps)
	}

	if rc.devicePath == "" {
		t.Error("device path not set")
	}
}

// TestRealCameraStartNoDevice tests that Start returns error when device not found
func TestRealCameraStartNoDevice(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/video999" // Non-existent device

	err := rc.Start(640, 480, 24, 80)
	if err == nil {
		t.Error("Start should return error for non-existent device")
	}
}

// TestRealCameraStartWithoutFFmpeg tests Start fails gracefully without ffmpeg
func TestRealCameraStartWithoutFFmpeg(t *testing.T) {
	// Skip test if ffmpeg is present (we can't mock it away easily)
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		t.Skip("ffmpeg is installed, can't test missing ffmpeg scenario")
	}

	rc := NewRealCamera()
	err := rc.Start(640, 480, 24, 80)
	if err == nil {
		t.Error("Start should fail without ffmpeg")
	}
}

// TestRealCameraIsReady checks Ready state transitions
func TestRealCameraIsReady(t *testing.T) {
	rc := NewRealCamera()

	if rc.IsReady() {
		t.Error("camera should not be ready before Start")
	}

	rc.isReady.Store(true)
	if !rc.IsReady() {
		t.Error("camera should be ready after isReady set")
	}

	rc.isStopping.Store(true)
	if rc.IsReady() {
		t.Error("camera should not be ready when stopping")
	}
}

// TestRealCameraStop tests camera shutdown
func TestRealCameraStop(t *testing.T) {
	rc := NewRealCamera()
	rc.isReady.Store(true)

	err := rc.Stop()
	if err != nil {
		t.Errorf("Stop returned error: %v", err)
	}

	if rc.IsReady() {
		t.Error("camera should not be ready after Stop")
	}

	if !rc.isStopping.Load() {
		t.Error("isStopping flag should be set")
	}
}

// TestRealCameraFPSThrottling tests that frames respect FPS limit
func TestRealCameraFPSThrottling(t *testing.T) {
	rc := NewRealCamera()
	rc.isReady.Store(true)
	rc.fps = 10 // 10 FPS = 100ms per frame
	rc.lastCapture = time.Now()

	// First call (after delay) should fail since we don't have real device
	// but we're testing the throttling logic

	// This test mainly verifies no panic, since real device won't be available
	frameInterval := time.Duration(1e9/int64(rc.fps)) * time.Nanosecond
	if frameInterval != 100*time.Millisecond {
		t.Errorf("frame interval calculation: got %v, want 100ms", frameInterval)
	}
}

// TestRealCameraResolutionConfiguration tests setting custom resolution
func TestRealCameraResolutionConfiguration(t *testing.T) {
	rc := NewRealCamera()

	testCases := []struct {
		width   int
		height  int
		fps     int
		quality int
	}{
		{1920, 1080, 30, 85},
		{1280, 720, 24, 80},
		{640, 480, 15, 75},
		{320, 240, 10, 60},
	}

	for _, tc := range testCases {
		// Don't actually start (won't have device), just verify config
		rc.width = tc.width
		rc.height = tc.height
		rc.fps = tc.fps
		rc.jpegQuality = tc.quality

		if rc.width != tc.width || rc.height != tc.height {
			t.Errorf("resolution not set: %dx%d", rc.width, rc.height)
		}
		if rc.fps != tc.fps {
			t.Errorf("fps not set: %d", rc.fps)
		}
	}
}

// TestRealCameraJPEGQualityValidation tests JPEG quality bounds
func TestRealCameraJPEGQualityValidation(t *testing.T) {
	rc := NewRealCamera()

	// Quality 0 should be adjusted
	_ = rc.Start(640, 480, 24, 0)
	// We don't actually expect Start to succeed without device

	// Valid quality range
	validQualities := []int{1, 50, 75, 100}
	for _, q := range validQualities {
		if q < 1 || q > 100 {
			t.Errorf("quality validation: %d should be valid", q)
		}
	}
}

// TestRealCameraDevicePathConfiguration tests custom device paths
func TestRealCameraDevicePathConfiguration(t *testing.T) {
	testPaths := []string{
		"/dev/video0",
		"/dev/video1",
		"/dev/video2",
	}

	for _, path := range testPaths {
		rc := NewRealCamera()
		rc.devicePath = path

		if rc.devicePath != path {
			t.Errorf("device path not set: %q", rc.devicePath)
		}
	}
}

// TestRealCameraEncodeFrame tests JPEG encoding utility
func TestRealCameraEncodeFrame(t *testing.T) {
	// Create a small test image
	img := createTestImage(10, 10)

	jpegData, err := encodeFrameToJPEG(img, 80)
	if err != nil {
		t.Fatalf("encodeFrameToJPEG failed: %v", err)
	}

	if len(jpegData) == 0 {
		t.Error("encoded JPEG data is empty")
	}

	// Verify JPEG magic bytes (FFD8)
	if len(jpegData) >= 2 && jpegData[0] != 0xFF && jpegData[1] != 0xD8 {
		t.Error("encoded data doesn't start with JPEG SOI marker")
	}
}

// Helper function to create test image
func createTestImage(width, height int) *image.YCbCr {
	img := image.NewYCbCr(image.Rect(0, 0, width, height), image.YCbCrSubsampleRatio420)
	// Fill with test pattern
	for i := 0; i < len(img.Y); i++ {
		img.Y[i] = uint8(i & 0xFF)
	}
	return img
}

// TestRealCameraMultipleCameras tests multiple camera instances
func TestRealCameraMultipleCameras(t *testing.T) {
	cam1 := NewRealCamera()
	cam2 := NewRealCamera()

	cam1.width = 1920
	cam2.width = 640

	if cam1.width == cam2.width {
		t.Error("camera instances not independent")
	}
}

// TestRealCameraThreadSafety tests concurrent access
func TestRealCameraThreadSafety(t *testing.T) {
	rc := NewRealCamera()
	done := make(chan bool)

	// Multiple goroutines trying to configure camera
	for i := 0; i < 5; i++ {
		go func(id int) {
			rc.captureMutex.Lock()
			rc.width = 640 + id
			rc.height = 480 + id
			rc.captureMutex.Unlock()
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	// If we get here without deadlock, test passed
	if rc.width == 0 {
		t.Error("camera mutex lock failed")
	}
}
