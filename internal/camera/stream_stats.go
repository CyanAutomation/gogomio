package camera

import (
	"container/ring"
	"sync"
)

// StreamStats tracks real-time streaming statistics: frame count, FPS, and timestamps.
// Thread-safe for concurrent reads and writes.
type StreamStats struct {
	mu                 sync.RWMutex
	frameCount         int64
	lastFrameTimestamp *int64 // Unix nanoseconds, nil if no frames yet
	frameTimestamps    *ring.Ring
	maxFramesInWindow  int
}

// NewStreamStats creates a new StreamStats tracker.
// Uses a rolling window of 30 frames for FPS calculation.
func NewStreamStats() *StreamStats {
	ringSize := 30
	return &StreamStats{
		frameTimestamps:   ring.New(ringSize),
		maxFramesInWindow: ringSize,
	}
}

// RecordFrame records a frame with its timestamp.
// timestamp should be in Unix nanoseconds (e.g., time.Now().UnixNano()).
func (s *StreamStats) RecordFrame(timestamp int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.frameCount++
	s.lastFrameTimestamp = &timestamp

	// Add to rolling window
	s.frameTimestamps.Value = timestamp
	s.frameTimestamps = s.frameTimestamps.Next()
}

// Snapshot returns a consistent view of frame count, last timestamp, and calculated FPS.
// Returns (frameCount, lastTimestampPtr, fps).
// lastTimestampPtr is nil if no frames have been recorded.
func (s *StreamStats) Snapshot() (int64, *int64, float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	frameCount := s.frameCount

	// Last timestamp (make a copy to avoid external mutations)
	var lastTimestamp *int64
	if s.lastFrameTimestamp != nil {
		copied := *s.lastFrameTimestamp
		lastTimestamp = &copied
	}

	// Calculate FPS from rolling window
	fps := s.calculateFPS()

	return frameCount, lastTimestamp, fps
}

// calculateFPS calculates FPS based on the rolling 30-frame window.
// Must be called under read lock.
func (s *StreamStats) calculateFPS() float64 {
	// Collect timestamps from the ring
	var timestamps []int64
	s.frameTimestamps.Do(func(v interface{}) {
		if v != nil {
			timestamps = append(timestamps, v.(int64))
		}
	})

	if len(timestamps) < 2 {
		return 0.0
	}

	// Find min and max timestamps to calculate span
	minTS := timestamps[0]
	maxTS := timestamps[0]
	for _, ts := range timestamps {
		if ts < minTS {
			minTS = ts
		}
		if ts > maxTS {
			maxTS = ts
		}
	}

	timeSpan := maxTS - minTS
	if timeSpan == 0 {
		return 0.0
	}

	// FPS = (number of frames - 1) / time span in seconds
	// -1 because FPS is intervals between frames, not frames themselves
	numFrames := float64(len(timestamps) - 1)
	timeSpanSeconds := float64(timeSpan) / 1e9
	return numFrames / timeSpanSeconds
}

// FrameCountSince returns the number of frames captured since timestamp.
// Useful for health checks.
func (s *StreamStats) FrameCountSince(sinceNS int64) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.lastFrameTimestamp == nil || *s.lastFrameTimestamp < sinceNS {
		return 0
	}

	return s.frameCount
}

// LastFrameAgeSeconds returns the age of the last frame in seconds.
// Returns -1 if no frames yet.
func (s *StreamStats) LastFrameAgeSeconds(nowNS int64) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.lastFrameTimestamp == nil {
		return -1
	}

	ageNS := nowNS - *s.lastFrameTimestamp
	return float64(ageNS) / 1e9
}
