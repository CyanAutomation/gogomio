package camera

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"sync"
	"time"
)

// MockCamera is a testing implementation that generates synthetic JPEG frames.
type MockCamera struct {
	mu              sync.RWMutex
	ready           bool
	width           int
	height          int
	fps             int
	jpegQuality     int
	lastFrameTime   time.Time
	frameCounter    int64
}

// NewMockCamera creates a new mock camera.
func NewMockCamera() *MockCamera {
	return &MockCamera{}
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
	mc.lastFrameTime = time.Now()
	mc.frameCounter = 0

	return nil
}

// CaptureFrame captures and returns a JPEG frame.
func (mc *MockCamera) CaptureFrame() ([]byte, error) {
	mc.mu.RLock()
	if !mc.ready {
		mc.mu.RUnlock()
		return nil, fmt.Errorf("camera not ready")
	}

	width := mc.width
	height := mc.height
	quality := mc.jpegQuality
	frameNum := mc.frameCounter

	// Throttle to target FPS
	lastFrameTime := mc.lastFrameTime
	mc.mu.RUnlock()

	// Rate limit to fps
	frameInterval := time.Duration(float64(time.Second) / float64(mc.fps))
	if elapsed := time.Since(lastFrameTime); elapsed < frameInterval {
		time.Sleep(frameInterval - elapsed)
	}

	// Update frame counter and time
	mc.mu.Lock()
	mc.frameCounter++
	mc.lastFrameTime = time.Now()
	mc.mu.Unlock()

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
func (mc *MockCamera) generateTestFrame(width, height, quality int, frameNum int64) []byte {
	// Create an image with a color gradient
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with gradient pattern (changes with frame number)
	hue := (frameNum * 10) % 360
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Create a hue-based color pattern
			r := uint8((x * 255) / width)
			g := uint8((y * 255) / height)
			b := uint8(((x + y) * 255) / (width + height))

			// Rotate based on frame number
			rShift := uint8(hue % 256)
			r = uint8((uint16(r) + uint16(rShift)) % 256)

			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	// Add frame counter text (simple pixel pattern for now)
	// For a real implementation, would use golang.org/x/image/font
	drawFrameCounter(img, int(frameNum))

	// Encode to JPEG
	buf := &bytes.Buffer{}
	err := jpeg.Encode(buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		// Fallback to lower quality if encoding fails
		buf.Reset()
		jpeg.Encode(buf, img, &jpeg.Options{Quality: 75})
	}

	return buf.Bytes()
}

// drawFrameCounter draws a simple frame counter on the image.
// Uses simple patterns rather than text rendering.
func drawFrameCounter(img *image.RGBA, frameNum int) {
	// Draw a border
	bounds := img.Bounds()
	borderColor := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	// Top border
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		for i := 0; i < 2; i++ {
			img.SetRGBA(x, bounds.Min.Y+i, borderColor)
		}
	}

	// Bottom border
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		for i := 0; i < 2; i++ {
			img.SetRGBA(x, bounds.Max.Y-1-i, borderColor)
		}
	}

	// Left border
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for i := 0; i < 2; i++ {
			img.SetRGBA(bounds.Min.X+i, y, borderColor)
		}
	}

	// Right border
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for i := 0; i < 2; i++ {
			img.SetRGBA(bounds.Max.X-1-i, y, borderColor)
		}
	}

	// Draw frame counter pattern in top-left (simple grid pattern)
	x := bounds.Min.X + 10
	y := bounds.Min.Y + 10
	size := 5

	// Draw counter as a pattern (0-9 for last digit)
	digit := frameNum % 10
	for i := 0; i < digit; i++ {
		for dx := 0; dx < size; dx++ {
			for dy := 0; dy < size; dy++ {
				if x+i*size+dx < bounds.Max.X && y+dy < bounds.Max.Y {
					img.SetRGBA(x+i*size+dx, y+dy, borderColor)
				}
			}
		}
	}
}

// Ensure MockCamera implements Camera interface
var _ Camera = (*MockCamera)(nil)
