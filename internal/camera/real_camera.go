package camera

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// RealCamera captures frames from a Raspberry Pi CSI camera via V4L2 device.
// Falls back to mock camera if device is unavailable.
type RealCamera struct {
	width        int
	height       int
	fps          int
	jpegQuality  int
	devicePath   string
	isReady      atomic.Bool
	isStopping   atomic.Bool
	captureMutex sync.Mutex
	lastCapture  time.Time

	// Process management
	proc *exec.Cmd
}

// NewRealCamera creates a new camera instance for Raspberry Pi CSI camera.
func NewRealCamera() *RealCamera {
	rc := &RealCamera{
		width:       640,
		height:      480,
		fps:         24,
		jpegQuality: 80,
		devicePath:  "/dev/video0",
	}
	return rc
}

// Start initializes the camera and begins capture preparation.
// Returns error if camera device not found, but doesn't fail - falls back to mock in main.
func (rc *RealCamera) Start(width, height, fps, jpegQuality int) error {
	rc.captureMutex.Lock()
	defer rc.captureMutex.Unlock()

	rc.width = width
	rc.height = height
	rc.fps = fps
	if jpegQuality < 1 || jpegQuality > 100 {
		jpegQuality = 80
	}
	rc.jpegQuality = jpegQuality

	// Check if device exists
	if _, err := os.Stat(rc.devicePath); err != nil {
		return fmt.Errorf("camera device not found at %s: %w", rc.devicePath, err)
	}

	// Verify ffmpeg is available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}

	rc.isReady.Store(true)
	rc.lastCapture = time.Now()

	return nil
}

// CaptureFrame captures a single JPEG frame from the camera device.
// Uses ffmpeg to read from V4L2 device and encode to JPEG.
func (rc *RealCamera) CaptureFrame() ([]byte, error) {
	if !rc.isReady.Load() {
		return nil, fmt.Errorf("camera not ready")
	}

	if rc.isStopping.Load() {
		return nil, fmt.Errorf("camera stopped")
	}

	rc.captureMutex.Lock()
	defer rc.captureMutex.Unlock()

	// FPS throttling: ensure minimum time between frames
	frameInterval := time.Duration(1e9/int64(rc.fps)) * time.Nanosecond
	since := time.Since(rc.lastCapture)
	if since < frameInterval {
		time.Sleep(frameInterval - since)
	}
	rc.lastCapture = time.Now()

	// Capture frame using ffmpeg from V4L2 device
	frameData, err := rc.captureViaFFmpeg()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg capture failed: %w", err)
	}

	return frameData, nil
}

// captureViaFFmpeg reads a frame from V4L2 device using ffmpeg
func (rc *RealCamera) captureViaFFmpeg() ([]byte, error) {
	// ffmpeg command to read from V4L2 device and output single JPEG
	cmd := exec.Command(
		"ffmpeg",
		"-f", "video4linux2",                          // Input format
		"-input_format", "mjpeg",                      // Request MJPEG if available
		"-i", rc.devicePath,                           // Input device
		"-frames:v", "1",                              // Capture 1 frame
		"-c:v", "mjpeg",                               // Encode to JPEG
		"-q:v", fmt.Sprintf("%d", rc.jpegQuality),   // JPEG quality (2-31, inverse)
		"-f", "image2",                                // Output format
		"-",                                           // Output to stdout
	)

	// Capture output
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("ffmpeg error: %w (stderr: %s)", err, stderr.String())
		}
	case <-time.After(2 * time.Second):
		cmd.Process.Kill()
		return nil, fmt.Errorf("ffmpeg capture timeout")
	}

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("no frame data captured")
	}

	return stdout.Bytes(), nil
}

// Stop stops the camera capture.
func (rc *RealCamera) Stop() error {
	rc.isStopping.Store(true)
	rc.isReady.Store(false)

	rc.captureMutex.Lock()
	defer rc.captureMutex.Unlock()

	if rc.proc != nil && rc.proc.Process != nil {
		_ = rc.proc.Process.Kill()
	}

	return nil
}

// IsReady returns true if camera is initialized and ready to capture.
func (rc *RealCamera) IsReady() bool {
	return rc.isReady.Load() && !rc.isStopping.Load()
}

// encodeFrameToJPEG converts an image.Image to JPEG bytes.
// Used internally if we get raw image frames instead of pre-encoded JPEG.
func encodeFrameToJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
