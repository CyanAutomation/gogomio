package camera

import (
	"bytes"
	"context"
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
	startedAt     time.Time
	frameCounter  int64
	now           func() time.Time
	sleep         func(time.Duration)
	frameSequence [][]byte
	timedSequence [][]byte
	captureErrors map[int64]error
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

// SetFrameSequence configures JPEG frames to return in order, looping after the
// last frame. The camera copies the supplied bytes. Passing an empty sequence
// restores generated synthetic frames. The sequence index follows capture
// attempts since the most recent Start, including attempts that fail.
func (mc *MockCamera) SetFrameSequence(frames [][]byte) {
	sequence := make([][]byte, len(frames))
	for i, frame := range frames {
		sequence[i] = append([]byte(nil), frame...)
	}

	mc.mu.Lock()
	mc.frameSequence = sequence
	mc.timedSequence = nil
	mc.mu.Unlock()
}

// SetTimedFrameSequence configures a looping fixture sequence that advances
// according to elapsed playback time at the configured capture FPS. The camera
// copies the supplied bytes. Passing an empty sequence restores generated
// synthetic frames unless an attempt-indexed sequence is configured later.
func (mc *MockCamera) SetTimedFrameSequence(frames [][]byte) {
	sequence := make([][]byte, len(frames))
	for i, frame := range frames {
		sequence[i] = append([]byte(nil), frame...)
	}

	mc.mu.Lock()
	mc.timedSequence = sequence
	mc.frameSequence = nil
	mc.mu.Unlock()
}

// SetCaptureFailures configures errors for zero-based capture-attempt indexes
// since the most recent Start. Attempts without a configured error continue
// normally. Passing nil or an empty map clears all configured failures.
func (mc *MockCamera) SetCaptureFailures(failures map[int64]error) {
	captureErrors := make(map[int64]error, len(failures))
	for captureIndex, err := range failures {
		if err != nil {
			captureErrors[captureIndex] = err
		}
	}

	mc.mu.Lock()
	mc.captureErrors = captureErrors
	mc.mu.Unlock()
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
	now := mc.now()
	mc.startedAt = now
	mc.lastFrameTime = now
	mc.frameCounter = 0

	return nil
}

// CaptureFrame captures and returns a JPEG frame.
func (mc *MockCamera) CaptureFrame() ([]byte, error) {
	return mc.CaptureFrameWithContext(context.Background())
}

// CaptureFrameWithContext captures and returns a JPEG frame.
func (mc *MockCamera) CaptureFrameWithContext(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

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
	startedAt := mc.startedAt
	captureErr := mc.captureErrors[frameNum]
	var configuredFrame []byte
	hasConfiguredFrame := false
	now := mc.now()
	frameInterval := time.Duration(float64(time.Second) / float64(fps))
	if len(mc.timedSequence) > 0 {
		elapsed := now.Sub(startedAt)
		if elapsed < 0 {
			elapsed = 0
		}
		frameIndex := int((elapsed / frameInterval) % time.Duration(len(mc.timedSequence)))
		configuredFrame = mc.timedSequence[frameIndex]
		hasConfiguredFrame = true
	} else if len(mc.frameSequence) > 0 {
		frameIndex := int(frameNum % int64(len(mc.frameSequence)))
		configuredFrame = mc.frameSequence[frameIndex]
		hasConfiguredFrame = true
	}

	// Reserve frame number and update timing under one critical section.
	mc.frameCounter++
	sleepDuration := time.Duration(0)
	nextFrameTime := lastFrameTime.Add(frameInterval)
	if elapsed := now.Sub(lastFrameTime); elapsed < frameInterval {
		sleepDuration = frameInterval - elapsed
	} else {
		// Drop missed frame slots instead of trying to catch up after an idle
		// period or a slow JPEG encode. Real cameras deliver the latest frame.
		nextFrameTime = now
	}
	mc.lastFrameTime = nextFrameTime
	mc.mu.Unlock()

	// Sleep and JPEG generation happen outside the lock.
	if sleepDuration > 0 {
		mc.sleep(sleepDuration)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	if captureErr != nil {
		return nil, fmt.Errorf("mock camera capture %d failed: %w", frameNum, captureErr)
	}
	if hasConfiguredFrame {
		return append([]byte(nil), configuredFrame...), nil
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

// GetFrameCounter returns the number of frames reserved/captured so far.
func (mc *MockCamera) GetFrameCounter() int64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	return mc.frameCounter
}

// GetLastFrameTime returns the most recent scheduled frame time.
func (mc *MockCamera) GetLastFrameTime() time.Time {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	return mc.lastFrameTime
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
