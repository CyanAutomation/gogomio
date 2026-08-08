package camera

import (
	"math"
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

// TestStreamStatsSnapshotConsistency is a regression test for the requirement that
// Snapshot return frame count, timestamp, and FPS from one atomic published state,
// even when RecordFrame is concurrently publishing the next state.
func TestStreamStatsSnapshotConsistency(t *testing.T) {
	stats := NewStreamStats()

	const transitions int64 = 256
	const frameInterval = int64(time.Second)
	const baseTime = int64(1_000_000_000_000)

	assertPublishedState := func(frameCount int64, lastTime *int64, fps float64) {
		t.Helper()
		if frameCount == 0 {
			if lastTime != nil {
				t.Errorf("snapshot with no frames has last frame time %d, want nil", *lastTime)
			}
			if fps != 0 {
				t.Errorf("snapshot with no frames has FPS %v, want 0", fps)
			}
			return
		}

		wantTime := baseTime + (frameCount-1)*frameInterval
		if lastTime == nil {
			t.Errorf("snapshot with %d frames has nil last frame time", frameCount)
		} else if *lastTime != wantTime {
			t.Errorf("torn snapshot: count %d has last frame time %d, want %d", frameCount, *lastTime, wantTime)
		}

		wantFPS := 0.0
		if frameCount > 1 {
			wantFPS = 1
		}
		if fps != wantFPS {
			t.Errorf("torn snapshot: count %d has FPS %v, want %v", frameCount, fps, wantFPS)
		}
	}

	requested := make(chan int64)
	attempting := make(chan int64)
	published := make(chan int64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for timestamp := range requested {
			// This rendezvous releases the reader and writer at the boundary of
			// RecordFrame, so the following Snapshot races with that publication.
			attempting <- timestamp
			stats.RecordFrame(timestamp)
			published <- timestamp
		}
	}()

	assertPublishedState(stats.Snapshot())
	for count := int64(1); count <= transitions; count++ {
		timestamp := baseTime + (count-1)*frameInterval
		requested <- timestamp
		if got := <-attempting; got != timestamp {
			t.Fatalf("writer attempting timestamp %d, want %d", got, timestamp)
		}

		// Depending on lock acquisition, this must be exactly the state before
		// or after the transition; cross-field mixing identifies a torn read.
		frameCount, lastTime, fps := stats.Snapshot()
		if frameCount != count-1 && frameCount != count {
			t.Errorf("snapshot frame count is %d during transition %d, want %d or %d", frameCount, count, count-1, count)
		}
		assertPublishedState(frameCount, lastTime, fps)

		if got := <-published; got != timestamp {
			t.Fatalf("writer published timestamp %d, want %d", got, timestamp)
		}
		frameCount, lastTime, fps = stats.Snapshot()
		if frameCount != count {
			t.Errorf("frame count after transition is %d, want %d", frameCount, count)
		}
		assertPublishedState(frameCount, lastTime, fps)
	}

	close(requested)
	<-done
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
