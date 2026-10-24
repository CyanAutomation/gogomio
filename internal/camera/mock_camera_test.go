package camera

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
	"sync"
	"testing"
	"time"
)

func encodeMockCameraTestJPEG(t *testing.T, fill color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.SetRGBA(x, y, fill)
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatalf("encode fixture JPEG: %v", err)
	}
	return buf.Bytes()
}

func TestMockCameraConfiguredFrameSequenceLoopsAndCopiesFrames(t *testing.T) {
	original := encodeMockCameraTestJPEG(t, color.RGBA{R: 220, G: 20, B: 20, A: 255})
	second := encodeMockCameraTestJPEG(t, color.RGBA{R: 20, G: 20, B: 220, A: 255})
	frames := [][]byte{append([]byte(nil), original...), append([]byte(nil), second...)}

	clock := newFakeClock(time.Unix(1700000000, 0))
	mc := NewMockCameraWithClock(clock.Now, clock.Sleep)
	mc.SetFrameSequence(frames)
	frames[0][0] ^= 0xff
	if err := mc.Start(8, 6, 24, 90); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() { _ = mc.Stop() })

	first, err := mc.CaptureFrame()
	if err != nil {
		t.Fatalf("CaptureFrame 0 failed: %v", err)
	}
	if !bytes.Equal(first, original) {
		t.Fatal("first capture does not match first configured frame")
	}
	first[0] ^= 0xff

	gotSecond, err := mc.CaptureFrame()
	if err != nil {
		t.Fatalf("CaptureFrame 1 failed: %v", err)
	}
	if !bytes.Equal(gotSecond, second) {
		t.Fatal("second capture does not match second configured frame")
	}

	gotLooped, err := mc.CaptureFrame()
	if err != nil {
		t.Fatalf("CaptureFrame 2 failed: %v", err)
	}
	if !bytes.Equal(gotLooped, original) {
		t.Fatal("frame sequence did not loop to its first frame")
	}
	if _, err := jpeg.Decode(bytes.NewReader(gotLooped)); err != nil {
		t.Fatalf("looped frame is not a valid JPEG: %v", err)
	}

	mc.SetFrameSequence(nil)
	generated, err := mc.CaptureFrame()
	if err != nil {
		t.Fatalf("CaptureFrame after clearing sequence failed: %v", err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(generated))
	if err != nil {
		t.Fatalf("generated frame after clearing sequence is not a valid JPEG: %v", err)
	}
	if config.Width != 8 || config.Height != 6 {
		t.Fatalf("generated frame dimensions = %dx%d, want 8x6", config.Width, config.Height)
	}
}

func TestMockCameraConfiguredCaptureFailuresAreIndexedPerCapture(t *testing.T) {
	failure := errors.New("simulated sensor read failure")
	firstFrame := encodeMockCameraTestJPEG(t, color.RGBA{R: 220, G: 20, B: 20, A: 255})
	secondFrame := encodeMockCameraTestJPEG(t, color.RGBA{R: 20, G: 220, B: 20, A: 255})
	thirdFrame := encodeMockCameraTestJPEG(t, color.RGBA{R: 20, G: 20, B: 220, A: 255})
	clock := newFakeClock(time.Unix(1700000000, 0))
	mc := NewMockCameraWithClock(clock.Now, clock.Sleep)
	mc.SetFrameSequence([][]byte{firstFrame, secondFrame, thirdFrame})
	mc.SetCaptureFailures(map[int64]error{1: failure, 3: failure})
	if err := mc.Start(32, 24, 20, 80); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() { _ = mc.Stop() })

	wantFrames := map[int64][]byte{
		0: firstFrame,
		2: thirdFrame,
		4: secondFrame,
	}
	for captureIndex := int64(0); captureIndex < 5; captureIndex++ {
		frame, err := mc.CaptureFrame()
		if captureIndex == 1 || captureIndex == 3 {
			if !errors.Is(err, failure) {
				t.Errorf("CaptureFrame %d error = %v, want wrapped simulated failure", captureIndex, err)
			}
			if frame != nil {
				t.Errorf("CaptureFrame %d returned a frame with an injected failure", captureIndex)
			}
			continue
		}
		if err != nil {
			t.Errorf("CaptureFrame %d failed unexpectedly: %v", captureIndex, err)
			continue
		}
		want := wantFrames[captureIndex]
		if !bytes.Equal(frame, want) {
			t.Errorf("CaptureFrame %d did not match its configured fixture", captureIndex)
		}
		if _, err := jpeg.Decode(bytes.NewReader(frame)); err != nil {
			t.Errorf("CaptureFrame %d returned invalid JPEG: %v", captureIndex, err)
		}
	}

	if got := mc.GetFrameCounter(); got != 5 {
		t.Errorf("frame counter = %d, want 5 capture attempts", got)
	}
	if got := clock.Sleeps(); len(got) != 5 {
		t.Errorf("pacing sleeps = %d, want one per capture attempt", len(got))
	}

	mc.SetCaptureFailures(nil)
	frame, err := mc.CaptureFrame()
	if err != nil {
		t.Errorf("CaptureFrame failed after clearing configured failures: %v", err)
	} else if !bytes.Equal(frame, thirdFrame) {
		t.Errorf("CaptureFrame after clearing failures did not continue the frame sequence")
	}
}

// TestMockCameraCaptureFrame verifies that captures are correctly sized JPEGs
// and that advancing the frame sequence changes the generated image.
func TestMockCameraCaptureFrame(t *testing.T) {
	const (
		width  = 64
		height = 48
		fps    = 20
	)
	frameInterval := time.Second / fps
	clock := newFakeClock(time.Unix(1700000000, 0))
	mc := NewMockCameraWithClock(clock.Now, clock.Sleep)

	err := mc.Start(width, height, fps, 90)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = mc.Stop() }()

	frames := make([][]byte, 2)
	decoded := make([]image.Image, len(frames))
	for i := range frames {
		frame, err := mc.CaptureFrame()
		if err != nil {
			t.Fatalf("CaptureFrame %d failed: %v", i, err)
		}
		frames[i] = frame

		decoded[i], err = jpeg.Decode(bytes.NewReader(frame))
		if err != nil {
			t.Fatalf("decode frame %d as JPEG: %v", i, err)
		}
		if bounds := decoded[i].Bounds(); bounds.Dx() != width || bounds.Dy() != height {
			t.Errorf("frame %d dimensions = %dx%d, want %dx%d", i, bounds.Dx(), bounds.Dy(), width, height)
		}
	}

	// Each synthetic frame advances its hue and counter decoration, making the
	// decoded image observably different from the preceding frame.
	if bytes.Equal(frames[0], frames[1]) {
		t.Fatal("successive frame JPEGs are identical")
	}
	center := image.Pt(width/2, height/2)
	if decoded[0].At(center.X, center.Y) == decoded[1].At(center.X, center.Y) {
		t.Fatal("successive frames did not change the center pixel")
	}

	sleeps := clock.Sleeps()
	if len(sleeps) != len(frames) {
		t.Fatalf("sleep calls = %d, want %d", len(sleeps), len(frames))
	}
	for i, slept := range sleeps {
		if slept != frameInterval {
			t.Errorf("sleep %d = %v, want frame interval %v", i, slept, frameInterval)
		}
	}
}

// TestMockCameraDifferentResolutions verifies each requested output size is
// present in the generated JPEG, not just that capture returned some bytes.
// Contract: TC-CAMERA-02 (docs/testing/test-contracts.md).
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
		t.Run(fmt.Sprintf("%dx%d", test.width, test.height), func(t *testing.T) {
			clock := newFakeClock(time.Unix(1700000000, 0))
			mc := NewMockCameraWithClock(clock.Now, clock.Sleep)
			if err := mc.Start(test.width, test.height, 24, 90); err != nil {
				t.Fatalf("Start failed: %v", err)
			}
			t.Cleanup(func() { _ = mc.Stop() })

			frame, err := mc.CaptureFrame()
			if err != nil {
				t.Fatalf("CaptureFrame failed: %v", err)
			}
			if len(frame) == 0 {
				t.Fatal("CaptureFrame returned an empty JPEG")
			}

			config, err := jpeg.DecodeConfig(bytes.NewReader(frame))
			if err != nil {
				t.Fatalf("decode JPEG dimensions: %v", err)
			}
			if config.Width != test.width || config.Height != test.height {
				t.Fatalf("JPEG dimensions = %dx%d, want %dx%d", config.Width, config.Height, test.width, test.height)
			}
		})
	}
}

// TestMockCameraQualitySettings verifies higher JPEG quality produces a larger
// encoding for the same generated scene, and that each result remains decodable.
// Contract: TC-CAMERA-02 (docs/testing/test-contracts.md).
func TestMockCameraQualitySettings(t *testing.T) {
	qualities := []int{50, 75, 90}
	encodedSizes := make([]int, len(qualities))

	for i, quality := range qualities {
		t.Run(fmt.Sprintf("quality_%d", quality), func(t *testing.T) {
			clock := newFakeClock(time.Unix(1700000000, 0))
			mc := NewMockCameraWithClock(clock.Now, clock.Sleep)
			if err := mc.Start(640, 480, 24, quality); err != nil {
				t.Fatalf("Start failed: %v", err)
			}
			t.Cleanup(func() { _ = mc.Stop() })

			frame, err := mc.CaptureFrame()
			if err != nil {
				t.Fatalf("CaptureFrame failed: %v", err)
			}
			if _, err := jpeg.Decode(bytes.NewReader(frame)); err != nil {
				t.Fatalf("generated frame is not a decodable JPEG: %v", err)
			}
			encodedSizes[i] = len(frame)
		})
	}

	for i := 1; i < len(qualities); i++ {
		if encodedSizes[i] <= encodedSizes[i-1] {
			t.Errorf("quality %d JPEG size = %d bytes, want more than quality %d size %d bytes", qualities[i], encodedSizes[i], qualities[i-1], encodedSizes[i-1])
		}
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
		if _, err := jpeg.Decode(bytes.NewReader(frame)); err != nil {
			t.Fatalf("concurrent capture returned an invalid JPEG: %v", err)
		}
		h := fnv.New64a()
		if _, err := h.Write(frame); err != nil {
			t.Fatalf("failed to hash frame: %v", err)
		}
		seen[h.Sum64()] = struct{}{}
	}

	if len(seen) != totalCalls {
		t.Fatalf("expected %d unique frames, got %d", totalCalls, len(seen))
	}

	finalCounter := mc.GetFrameCounter()
	lastFrameTime := mc.GetLastFrameTime()

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
	const (
		width  = 64
		height = 48
	)
	clock := newFakeClock(time.Unix(1700000000, 0))
	mc := NewMockCameraWithClock(clock.Now, clock.Sleep)

	// Initially not ready
	if mc.IsReady() {
		t.Error("mock camera should not be ready before Start")
	}

	// Start
	err := mc.Start(width, height, 24, 90)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Ready after start
	if !mc.IsReady() {
		t.Error("mock camera should be ready after Start")
	}

	// Capture works
	frame, err := mc.CaptureFrame()
	if err != nil {
		t.Fatalf("CaptureFrame failed after Start: %v", err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("decode captured frame as JPEG: %v", err)
	}
	if bounds := decoded.Bounds(); bounds.Dx() != width || bounds.Dy() != height {
		t.Errorf("captured frame dimensions = %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), width, height)
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

	// Captures after Stop return no frame and report that the camera is not ready.
	frame, err = mc.CaptureFrame()
	if err == nil {
		t.Fatal("CaptureFrame after Stop succeeded, want camera not ready error")
	}
	if frame != nil {
		t.Errorf("CaptureFrame after Stop returned frame data, want nil")
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
