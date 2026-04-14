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

// TestStreamStatsThreadSafety tests concurrent RecordFrame calls
func TestStreamStatsThreadSafety(t *testing.T) {
	stats := NewStreamStats()

	baseTime := time.Now().UnixNano()
	numGoroutines := 10
	framesPerGoroutine := 100

	var wg sync.WaitGroup

	// Launch concurrent writers
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < framesPerGoroutine; i++ {
				ts := baseTime + int64(g*10000000+i*1000000) // Spread timestamps
				stats.RecordFrame(ts)
			}
		}(g)
	}

	// Concurrent readers
	for r := 0; r < 5; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				stats.Snapshot()
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	frameCount, _, _ := stats.Snapshot()
	expectedCount := int64(numGoroutines * framesPerGoroutine)

	if frameCount != expectedCount {
		t.Errorf("frame count is %d, want %d", frameCount, expectedCount)
	}
}

// TestStreamStatsSnapshotConsistency tests that Snapshot is atomic
func TestStreamStatsSnapshotConsistency(t *testing.T) {
	stats := NewStreamStats()

	baseTime := time.Now().UnixNano()

	// Record several frames at specific intervals
	timestamps := []int64{
		baseTime,
		baseTime + 1000000, // 1ms
		baseTime + 2000000, // 2ms
		baseTime + 3000000, // 3ms
	}

	for _, ts := range timestamps {
		stats.RecordFrame(ts)
	}

	frameCount1, lastTime1, fps1 := stats.Snapshot()
	frameCount2, lastTime2, fps2 := stats.Snapshot()

	if frameCount1 != frameCount2 || *lastTime1 != *lastTime2 || fps1 != fps2 {
		t.Errorf("consecutive snapshots differ: (%d,%d,%v) vs (%d,%d,%v)",
			frameCount1, frameCount2, fps1, lastTime1, lastTime2, fps2)
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
