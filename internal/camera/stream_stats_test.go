package camera

import (
	"math"
	"sync"
	"testing"
	"time"
)

// TestStreamStatsInitialization tests that stats start at zero
func TestStreamStatsInitialization(t *testing.T) {
	stats := NewStreamStats()

	frameCount, lastTime, fps := stats.Snapshot()

	if frameCount != 0 {
		t.Errorf("initial frame count is %d, want 0", frameCount)
	}
	if lastTime != nil {
		t.Errorf("initial last frame time is %v, want nil", lastTime)
	}
	if fps != 0 {
		t.Errorf("initial FPS is %v, want 0", fps)
	}
}

// TestStreamStatsRecordFrame tests frame recording
func TestStreamStatsRecordFrame(t *testing.T) {
	stats := NewStreamStats()

	ts1 := time.Now().UnixNano()
	stats.RecordFrame(ts1)

	frameCount, lastTime, fps := stats.Snapshot()

	if frameCount != 1 {
		t.Errorf("frame count is %d, want 1", frameCount)
	}
	if lastTime == nil || *lastTime != ts1 {
		t.Errorf("last frame time mismatch")
	}
	if fps != 0 {
		t.Errorf("FPS should be 0 with single frame, got %v", fps)
	}
}

// TestStreamStatsFPSCalculation tests FPS calculation with multiple frames
func TestStreamStatsFPSCalculation(t *testing.T) {
	stats := NewStreamStats()

	// Record 10 frames at ~1ms intervals
	baseTime := time.Now().UnixNano()
	for i := 0; i < 10; i++ {
		ts := baseTime + int64(i*1000000) // 1ms apart (nanoseconds)
		stats.RecordFrame(ts)
	}

	frameCount, _, fps := stats.Snapshot()

	if frameCount != 10 {
		t.Errorf("frame count is %d, want 10", frameCount)
	}

	// With 10 frames over ~9ms, FPS should be roughly (9 frames / 9ms) = 1000 FPS
	// Allow some tolerance due to timing variations
	if fps < 500 || fps > 1500 {
		t.Errorf("FPS is %v, want ~1000", fps)
	}
}

// TestStreamStatsWindowSliding tests that FPS window is a sliding 30-frame window
func TestStreamStatsWindowSliding(t *testing.T) {
	stats := NewStreamStats()

	baseTime := time.Now().UnixNano()

	// Record 50 frames at 1ms intervals
	for i := 0; i < 50; i++ {
		ts := baseTime + int64(i*1000000) // 1ms apart
		stats.RecordFrame(ts)
	}

	frameCount, _, fps := stats.Snapshot()

	if frameCount != 50 {
		t.Errorf("frame count is %d, want 50", frameCount)
	}

	// FPS should be calculated from last 30 frames only (frames 20-49)
	// Span is ~29ms (29 frames over 29ms = 1000 FPS)
	if fps < 500 || fps > 1500 {
		t.Errorf("FPS is %v, want ~1000 (from rolling 30-frame window)", fps)
	}
}

// TestStreamStatsThreadSafety tests concurrent RecordFrame calls and consistent snapshots.
func TestStreamStatsThreadSafety(t *testing.T) {
	stats := NewStreamStats()

	baseTime := time.Now().UnixNano()
	numGoroutines := 10
	framesPerGoroutine := 100
	numReaders := 5
	expectedCount := int64(numGoroutines * framesPerGoroutine)

	type snapshot struct {
		frameCount int64
		lastTime   *int64
		fps        float64
	}

	start := make(chan struct{})
	writersDone := make(chan struct{})
	readerReady := make(chan struct{}, numReaders)
	snapshots := make(chan []snapshot, numReaders)

	var writerWG sync.WaitGroup

	// Launch concurrent writers
	for g := 0; g < numGoroutines; g++ {
		writerWG.Add(1)
		go func(id int) {
			defer writerWG.Done()
			<-start
			for i := 0; i < framesPerGoroutine; i++ {
				ts := baseTime + int64(id*framesPerGoroutine+i)*1_000_000
				stats.RecordFrame(ts)
			}
		}(g)
	}

	// Readers start with the writers and keep taking snapshots until every record
	// has completed. Each reader owns its result slice, so collecting the results
	// does not add synchronization around Snapshot itself.
	for r := 0; r < numReaders; r++ {
		go func() {
			readerReady <- struct{}{}
			<-start

			var results []snapshot
			for {
				frameCount, lastTime, fps := stats.Snapshot()
				results = append(results, snapshot{frameCount, lastTime, fps})

				select {
				case <-writersDone:
					snapshots <- results
					return
				default:
				}
			}
		}()
	}

	// Do not release the writers until all readers are waiting at the same start
	// barrier. writersDone is the corresponding completion barrier.
	for r := 0; r < numReaders; r++ {
		<-readerReady
	}
	close(start)
	writerWG.Wait()
	close(writersDone)

	for r := 0; r < numReaders; r++ {
		var previousCount int64
		for _, result := range <-snapshots {
			if result.frameCount < previousCount {
				t.Errorf("snapshot frame count decreased from %d to %d", previousCount, result.frameCount)
			}
			if result.frameCount > expectedCount {
				t.Errorf("snapshot frame count is %d, exceeds %d completed records", result.frameCount, expectedCount)
			}

			if result.frameCount == 0 {
				if result.lastTime != nil {
					t.Errorf("snapshot with no frames has last frame time %d, want nil", *result.lastTime)
				}
				if result.fps != 0 {
					t.Errorf("snapshot with no frames has FPS %v, want 0", result.fps)
				}
			} else {
				if result.lastTime == nil {
					t.Errorf("snapshot with %d frames has nil last frame time", result.frameCount)
				} else if *result.lastTime < baseTime || *result.lastTime >= baseTime+expectedCount*1_000_000 {
					t.Errorf("snapshot last frame time %d is outside the recorded range", *result.lastTime)
				}
				if result.frameCount == 1 && result.fps != 0 {
					t.Errorf("snapshot with one frame has FPS %v, want 0", result.fps)
				}
				if result.frameCount > 1 && (math.IsNaN(result.fps) || math.IsInf(result.fps, 0) || result.fps <= 0) {
					t.Errorf("snapshot with %d frames has invalid FPS %v", result.frameCount, result.fps)
				}
			}

			previousCount = result.frameCount
		}
	}

	frameCount, _, _ := stats.Snapshot()

	if frameCount != expectedCount {
		t.Errorf("frame count is %d, want %d", frameCount, expectedCount)
	}
}

// TestStreamStatsFPSWithZeroTimeSpan tests FPS when frames arrive simultaneously
func TestStreamStatsFPSWithZeroTimeSpan(t *testing.T) {
	stats := NewStreamStats()

	// Record 10 frames with identical timestamp
	ts := time.Now().UnixNano()
	for i := 0; i < 10; i++ {
		stats.RecordFrame(ts)
	}

	_, _, fps := stats.Snapshot()

	if !math.IsNaN(fps) && fps != 0 {
		t.Errorf("FPS with zero time span should be 0 or NaN, got %v", fps)
	}
}

// TestStreamStatsHighFrequency tests FPS calculation at high frequency (e.g., 120 FPS)
func TestStreamStatsHighFrequency(t *testing.T) {
	stats := NewStreamStats()

	baseTime := time.Now().UnixNano()
	targetFPS := 120.0
	frameIntervalNS := int64(1e9 / targetFPS) // nanoseconds per frame

	// Record 30 frames at 120 FPS
	for i := 0; i < 30; i++ {
		ts := baseTime + int64(i)*frameIntervalNS
		stats.RecordFrame(ts)
	}

	_, _, fps := stats.Snapshot()

	// Should be close to 120 FPS
	tolerance := targetFPS * 0.1 // 10% tolerance
	if fps < targetFPS-tolerance || fps > targetFPS+tolerance {
		t.Errorf("FPS is %v, want ~%v (target with ±10%% tolerance)", fps, targetFPS)
	}
}

// TestFrameCountSinceMixedTimestamps tests counting only timestamps at/after threshold.
func TestFrameCountSinceMixedTimestamps(t *testing.T) {
	stats := NewStreamStats()

	baseTime := time.Now().UnixNano()
	timestamps := []int64{
		baseTime - 3_000_000,
		baseTime - 2_000_000,
		baseTime - 1_000_000,
		baseTime,
		baseTime + 1_000_000,
	}
	for _, ts := range timestamps {
		stats.RecordFrame(ts)
	}

	count := stats.FrameCountSince(baseTime - 1_000_000)
	if count != 3 {
		t.Errorf("FrameCountSince returned %d, want 3", count)
	}
}

// TestFrameCountSinceBoundaryEquality tests that threshold equality is included.
func TestFrameCountSinceBoundaryEquality(t *testing.T) {
	stats := NewStreamStats()

	baseTime := time.Now().UnixNano()
	timestamps := []int64{
		baseTime - 2_000_000,
		baseTime - 1_000_000,
		baseTime,
	}
	for _, ts := range timestamps {
		stats.RecordFrame(ts)
	}

	count := stats.FrameCountSince(baseTime - 1_000_000)
	if count != 2 {
		t.Errorf("FrameCountSince returned %d, want 2", count)
	}
}

// TestFrameCountSinceNoDoubleCounting ensures each timestamp is counted once.
func TestFrameCountSinceNoDoubleCounting(t *testing.T) {
	stats := NewStreamStats()

	baseTime := time.Now().UnixNano()
	timestamps := []int64{
		baseTime - 2_000_000,
		baseTime - 1_000_000,
		baseTime,
		baseTime + 1_000_000,
	}
	for _, ts := range timestamps {
		stats.RecordFrame(ts)
	}

	count := stats.FrameCountSince(baseTime - 1_000_000)
	if count != 3 {
		t.Errorf("FrameCountSince returned %d, want 3", count)
	}
}

// TestFrameCountSinceEmptyRingReturnsZero ensures empty timestamps return zero.
func TestFrameCountSinceEmptyRingReturnsZero(t *testing.T) {
	stats := NewStreamStats()

	count := stats.FrameCountSince(time.Now().UnixNano() - 1_000_000)
	if count != 0 {
		t.Errorf("FrameCountSince returned %d, want 0", count)
	}
}
