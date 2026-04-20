package camera

import (
	"fmt"
	"hash/fnv"
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

// TestMockCameraCaptureFrameConcurrentSequencing validates that concurrent callers
// receive unique frame numbers and are paced by FPS timing.
func TestMockCameraCaptureFrameConcurrentSequencing(t *testing.T) {
	type fakeClock struct {
		mu    sync.Mutex
		now   time.Time
		slept time.Duration
	}

	clock := &fakeClock{now: time.Unix(0, 0)}
	nowFn := func() time.Time {
		clock.mu.Lock()
		defer clock.mu.Unlock()
		return clock.now
	}
	sleepFn := func(d time.Duration) {
		clock.mu.Lock()
		clock.slept += d
		clock.now = clock.now.Add(d)
		clock.mu.Unlock()
	}

	mc := NewMockCameraWithClock(nowFn, sleepFn)
	const (
		width      = 64
		height     = 48
		fps        = 20
		quality    = 80
		callers    = 8
		perCaller  = 4
		totalCalls = callers * perCaller
	)

	if err := mc.Start(width, height, fps, quality); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = mc.Stop() }()

	var wg sync.WaitGroup
	frames := make(chan []byte, totalCalls)
	errs := make(chan error, totalCalls)

	for c := 0; c < callers; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perCaller; i++ {
				frame, err := mc.CaptureFrame()
				if err != nil {
					errs <- err
					continue
				}
				frames <- frame
			}
		}()
	}

	wg.Wait()
	close(frames)
	close(errs)

	for err := range errs {
		t.Fatalf("CaptureFrame failed under concurrency: %v", err)
	}

	seen := make(map[uint64]struct{}, totalCalls)
	for frame := range frames {
		h := fnv.New64a()
		if _, err := h.Write(frame); err != nil {
			t.Fatalf("failed to hash frame: %v", err)
		}
		seen[h.Sum64()] = struct{}{}
	}

	if len(seen) != totalCalls {
		t.Fatalf("expected %d unique frames, got %d", totalCalls, len(seen))
	}

	mc.mu.RLock()
	finalCounter := mc.frameCounter
	lastFrameTime := mc.lastFrameTime
	mc.mu.RUnlock()

	if finalCounter != int64(totalCalls) {
		t.Fatalf("expected frameCounter=%d, got %d", totalCalls, finalCounter)
	}

	frameInterval := time.Second / time.Duration(fps)
	expectedLastFrameTime := time.Unix(0, 0).Add(time.Duration(totalCalls) * frameInterval)
	if !lastFrameTime.Equal(expectedLastFrameTime) {
		t.Fatalf("expected lastFrameTime=%v, got %v", expectedLastFrameTime, lastFrameTime)
	}

	clock.mu.Lock()
	totalSlept := clock.slept
	clock.mu.Unlock()
	if totalSlept < time.Duration(totalCalls)*frameInterval {
		t.Fatalf("expected total sleep >= %v, got %v", time.Duration(totalCalls)*frameInterval, totalSlept)
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

type fakeClock struct {
	mu      sync.Mutex
	current time.Time
	sleeps  []time.Duration
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{current: start}
}

func (fc *fakeClock) Now() time.Time {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.current
}

func (fc *fakeClock) Sleep(d time.Duration) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.sleeps = append(fc.sleeps, d)
	fc.current = fc.current.Add(d)
}

func (fc *fakeClock) Sleeps() []time.Duration {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	out := make([]time.Duration, len(fc.sleeps))
	copy(out, fc.sleeps)
	return out
}

// TestMockCameraFPSAdjustment tests deterministic frame pacing logic.
func TestMockCameraFPSAdjustment(t *testing.T) {
	const targetFPS = 30
	frameInterval := time.Second / targetFPS
	start := time.Unix(1700000000, 0) // deterministic baseline
	clock := newFakeClock(start)

	mc := NewMockCameraWithClock(clock.Now, clock.Sleep)
	err := mc.Start(640, 480, targetFPS, 90)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = mc.Stop() }()

	const captures = 5
	for i := 0; i < captures; i++ {
		_, err := mc.CaptureFrame()
		if err != nil {
			t.Fatalf("CaptureFrame failed: %v", err)
		}
	}

	sleeps := clock.Sleeps()
	if len(sleeps) != captures {
		t.Fatalf("expected %d sleep calls, got %d", captures, len(sleeps))
	}

	// With a controllable clock, each capture should pace by exactly one frame interval.
	for i, slept := range sleeps {
		if slept != frameInterval {
			t.Fatalf("sleep %d = %v, expected %v", i, slept, frameInterval)
		}
	}
}
