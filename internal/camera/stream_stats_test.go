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

// TestStreamStatsFPSCalculation tests exact FPS calculation for regular frame cadences.
func TestStreamStatsFPSCalculation(t *testing.T) {
	const baseTime int64 = 1_700_000_000_000
	const highFrequencyFrames = 30
	const highFrequencyIntervalNS = int64(time.Second / 120)
	highFrequencySpanNS := int64(highFrequencyFrames-1) * highFrequencyIntervalNS
	highFrequencyFPS := float64(highFrequencyFrames-1) / (float64(highFrequencySpanNS) / 1e9)
	for _, tt := range []struct {
		name       string
		frames     int
		intervalNS int64
		wantFPS    float64
	}{
		{name: "one thousand frames per second", frames: 10, intervalNS: 1_000_000, wantFPS: 1000},
		{name: "high frequency frame cadence", frames: highFrequencyFrames, intervalNS: highFrequencyIntervalNS, wantFPS: highFrequencyFPS},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stats := NewStreamStats()
			for i := 0; i < tt.frames; i++ {
				stats.RecordFrame(baseTime + int64(i)*tt.intervalNS)
			}

			frameCount, lastTime, fps := stats.Snapshot()
			if frameCount != int64(tt.frames) {
				t.Fatalf("frame count is %d, want %d", frameCount, tt.frames)
			}
			if lastTime == nil {
				t.Fatal("last frame time is nil")
			}
			if math.Abs(fps-tt.wantFPS) > 1e-9 {
				t.Fatalf("FPS is %v, want %v ± 1e-9 from fixed frame timestamps", fps, tt.wantFPS)
			}
		})
	}
}

// TestStreamStatsWindowSliding verifies FPS uses the newest 30 frames, excluding
// older frames with a different cadence.
// Contract: TC-STATS-01 (docs/testing/test-contracts.md).
func TestStreamStatsWindowSliding(t *testing.T) {
	stats := NewStreamStats()
	const baseTime int64 = 1_000_000_000

	// These first 20 frames are spaced 100 ms apart. If they remain in the FPS
	// calculation, they substantially lower the result.
	for i := int64(0); i < 20; i++ {
		stats.RecordFrame(baseTime + i*100_000_000)
	}
	// The newest 30 frames span 29 ms, so their rolling-window FPS is exactly
	// 1000 regardless of the older cadence.
	for i := int64(0); i < 30; i++ {
		stats.RecordFrame(baseTime + 2_000_000_000 + i*1_000_000)
	}

	frameCount, _, fps := stats.Snapshot()

	if frameCount != 50 {
		t.Errorf("frame count is %d, want 50", frameCount)
	}

	if fps != 1000 {
		t.Errorf("FPS is %v, want 1000 from the newest 30 frames", fps)
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

// Contract: TC-STATS-01 (docs/testing/test-contracts.md).
func TestStreamStatsReportsZeroFPSForIdenticalTimestamps(t *testing.T) {
	stats := NewStreamStats()

	const ts int64 = 1_700_000_000_000
	for i := 0; i < 10; i++ {
		stats.RecordFrame(ts)
	}

	_, _, fps := stats.Snapshot()

	if fps != 0 {
		t.Fatalf("FPS for identical timestamps = %v, want 0", fps)
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
