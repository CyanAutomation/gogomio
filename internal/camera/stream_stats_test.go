package camera

import (
	"math"
	"testing"
	"time"
)

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

func TestFrameCountSince(t *testing.T) {
	const baseTime int64 = 1_000_000_000

	tests := []struct {
		name       string
		timestamps []int64
		since      int64
		want       int64
	}{
		{
			name:       "counts only recorded frames at or after the threshold",
			timestamps: []int64{baseTime - 3, baseTime - 2, baseTime - 1, baseTime, baseTime + 1},
			since:      baseTime - 1,
			want:       3,
		},
		{
			name:       "includes recorded frames equal to the threshold",
			timestamps: []int64{baseTime - 2, baseTime - 1, baseTime},
			since:      baseTime - 1,
			want:       2,
		},
		{
			name:       "counts repeated timestamps as distinct recorded frames",
			timestamps: []int64{baseTime - 1, baseTime, baseTime, baseTime},
			since:      baseTime,
			want:       3,
		},
		{
			name:  "returns zero when no frames have been recorded",
			since: baseTime,
			want:  0,
		},
		{
			name: "counts only frames retained after ring-buffer wraparound",
			timestamps: func() []int64 {
				timestamps := make([]int64, 35)
				for i := range timestamps {
					timestamps[i] = baseTime + int64(i)
				}
				return timestamps
			}(),
			since: baseTime,
			want:  30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := NewStreamStats()
			for _, timestamp := range tt.timestamps {
				stats.RecordFrame(timestamp)
			}

			if got := stats.FrameCountSince(tt.since); got != tt.want {
				t.Errorf("FrameCountSince(%d) = %d, want %d", tt.since, got, tt.want)
			}
		})
	}
}
