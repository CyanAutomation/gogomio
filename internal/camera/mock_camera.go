package camera

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"sync"
	"time"
)

// MockCamera is a testing implementation that generates synthetic JPEG frames.
type MockCamera struct {
	mu            sync.RWMutex
	ready         bool
	width         int
	height        int
	fps           int
	jpegQuality   int
	lastFrameTime time.Time
	frameCounter  int64
	now           func() time.Time
	sleep         func(time.Duration)
}

// NewMockCamera creates a new mock camera.
func NewMockCamera() *MockCamera {
	return NewMockCameraWithClock(time.Now, time.Sleep)
}

// NewMockCameraWithClock creates a mock camera with injectable time functions.
// This is primarily intended for deterministic tests.
func NewMockCameraWithClock(now func() time.Time, sleep func(time.Duration)) *MockCamera {
	if now == nil {
		now = time.Now
	}
	if sleep == nil {
		sleep = time.Sleep
	}

	return &MockCamera{
		now:   now,
		sleep: sleep,
	}
}

// Start initializes the mock camera.
func (mc *MockCamera) Start(width, height, fps, jpegQuality int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("invalid resolution: %dx%d", width, height)
	}
	if fps <= 0 {
		return fmt.Errorf("invalid FPS: %d", fps)
	}
	if jpegQuality < 1 || jpegQuality > 100 {
		return fmt.Errorf("invalid JPEG quality: %d", jpegQuality)
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.width = width
	mc.height = height
	mc.fps = fps
	mc.jpegQuality = jpegQuality
	mc.ready = true
	mc.lastFrameTime = mc.now()
	mc.frameCounter = 0

	return nil
}

// CaptureFrame captures and returns a JPEG frame.
func (mc *MockCamera) CaptureFrame() ([]byte, error) {
	mc.mu.Lock()
	if !mc.ready {
		mc.mu.Unlock()
		return nil, fmt.Errorf("camera not ready")
	}

	width := mc.width
	height := mc.height
	quality := mc.jpegQuality
	frameNum := mc.frameCounter
	fps := mc.fps
	lastFrameTime := mc.lastFrameTime

	// Reserve frame number and update timing under one critical section.
	mc.frameCounter++
	frameInterval := time.Duration(float64(time.Second) / float64(fps))
	now := mc.now()
	sleepDuration := time.Duration(0)
	nextFrameTime := now
	if elapsed := now.Sub(lastFrameTime); elapsed < frameInterval {
		sleepDuration = frameInterval - elapsed
		nextFrameTime = lastFrameTime.Add(frameInterval)
	}
	mc.lastFrameTime = nextFrameTime
	mc.mu.Unlock()

	// Sleep and JPEG generation happen outside the lock.
	if sleepDuration > 0 {
		mc.sleep(sleepDuration)
	}

	// Generate a synthetic frame
	frame := mc.generateTestFrame(width, height, quality, frameNum)
	return frame, nil
}

// Stop stops the mock camera.
func (mc *MockCamera) Stop() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.ready = false
	mc.frameCounter = 0

	return nil
}

// IsReady returns true if the camera is ready.
func (mc *MockCamera) IsReady() bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	return mc.ready
}

// generateTestFrame creates a synthetic JPEG frame with color gradient and frame counter.
// Optimized for performance: Direct pixel buffer access instead of SetRGBA per pixel.
func (mc *MockCamera) generateTestFrame(width, height, quality int, frameNum int64) []byte {
	// Create an image with a color gradient
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Pre-compute hue rotation once per frame
	hue := uint8((frameNum * 10) % 360)

	// Direct pixel buffer access for efficiency
	// RGBA format: each pixel is 4 bytes (R, G, B, A)
	pix := img.Pix
	stride := img.Stride

	// Fast fill with gradient pattern - direct buffer manipulation
	for y := 0; y < height; y++ {
		offset := y * stride
		for x := 0; x < width; x++ {
			// Calculate base colors
			r := uint8((x * 255) / width)
			g := uint8((y * 255) / height)
			b := uint8(((x + y) * 255) / (width + height))

			// Apply hue rotation
			r = uint8((uint16(r) + uint16(hue)) % 256)

			// Write directly to pixel buffer (R, G, B, A at stride)
			idx := offset + x*4
			pix[idx] = r     // R
			pix[idx+1] = g   // G
			pix[idx+2] = b   // B
			pix[idx+3] = 255 // A (always opaque)
		}
	}

	// Add frame counter decoration with minimal overhead
	drawFrameCounterOptimized(img, int(frameNum))

	// Encode to JPEG
	buf := &bytes.Buffer{}
	err := jpeg.Encode(buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		// Fallback to lower quality if encoding fails
		buf.Reset()
		if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 75}); err != nil {
			// If both fail, return error (shouldn't happen normally)
			return nil
		}
	}

	return buf.Bytes()
}

// drawFrameCounterOptimized draws a simple border on the image using direct buffer access.
// Optimized version with minimal overhead.
func drawFrameCounterOptimized(img *image.RGBA, frameNum int) {
	// Draw a border efficiently using direct pixel buffer
	bounds := img.Bounds()
	pix := img.Pix
	stride := img.Stride
	maxX := bounds.Max.X
	maxY := bounds.Max.Y

	// Top and bottom borders (2 pixels each)
	for x := 0; x < maxX; x++ {
		// Top border
		for i := 0; i < 2 && i < maxY; i++ {
			idx := i*stride + x*4
			pix[idx] = 255
			pix[idx+1] = 255
			pix[idx+2] = 255
			pix[idx+3] = 255
		}
		// Bottom border
		for i := 0; i < 2 && maxY-1-i >= 0; i++ {
			idx := (maxY-1-i)*stride + x*4
			pix[idx] = 255
			pix[idx+1] = 255
			pix[idx+2] = 255
			pix[idx+3] = 255
		}
	}

	// Left and right borders (2 pixels each)
	for y := 0; y < maxY; y++ {
		// Left border
		for i := 0; i < 2 && i < maxX; i++ {
			idx := y*stride + i*4
			pix[idx] = 255
			pix[idx+1] = 255
			pix[idx+2] = 255
			pix[idx+3] = 255
		}
		// Right border
		for i := 0; i < 2 && maxX-1-i >= 0; i++ {
			idx := y*stride + (maxX-1-i)*4
			pix[idx] = 255
			pix[idx+1] = 255
			pix[idx+2] = 255
			pix[idx+3] = 255
		}
	}
}

// Ensure MockCamera implements Camera interface
var _ Camera = (*MockCamera)(nil)
