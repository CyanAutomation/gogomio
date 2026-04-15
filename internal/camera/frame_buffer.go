package camera

import (
	"io"
	"sync"
	"time"
)

// FrameBuffer is a thread-safe circular buffer for JPEG frames.
// It implements io.Writer interface for use with Picamera2-style encoders.
type FrameBuffer struct {
	frame                 []byte
	condition             *sync.Cond
	stats                 *StreamStats
	lastFrameMonotonic    int64
	targetFrameIntervalNS int64
}

// NewFrameBuffer creates a new FrameBuffer.
// targetFPS <= 0 means no throttling.
func NewFrameBuffer(stats *StreamStats, targetFPS int) *FrameBuffer {
	fb := &FrameBuffer{
		condition: sync.NewCond(&sync.Mutex{}),
		stats:     stats,
	}
	if targetFPS > 0 {
		fb.targetFrameIntervalNS = 1e9 / int64(targetFPS)
	}
	return fb
}

// Write writes a frame to the buffer and signals waiting readers.
// Implements io.Writer interface.
func (fb *FrameBuffer) Write(buf []byte) (int, error) {
	size := len(buf)

	// Check if we should throttle based on target FPS
	now := time.Now().UnixNano()
	if fb.targetFrameIntervalNS > 0 && fb.lastFrameMonotonic > 0 {
		elapsed := now - fb.lastFrameMonotonic
		if elapsed < fb.targetFrameIntervalNS {
			// Too soon, skip this frame
			return size, nil
		}
	}

	fb.condition.L.Lock()
	defer fb.condition.L.Unlock()

	// Store frame and update timestamp
	fb.frame = make([]byte, len(buf))
	copy(fb.frame, buf)
	now = time.Now().UnixNano()
	fb.lastFrameMonotonic = now
	fb.stats.RecordFrame(now)

	// Signal all waiting readers
	fb.condition.Broadcast()

	return size, nil
}

// GetFrame returns a copy of the current frame (for snapshot endpoints).
func (fb *FrameBuffer) GetFrame() []byte {
	fb.condition.L.Lock()
	defer fb.condition.L.Unlock()

	if fb.frame == nil {
		return nil
	}

	// Return a copy to prevent external modifications
	frameCopy := make([]byte, len(fb.frame))
	copy(frameCopy, fb.frame)
	return frameCopy
}

// WaitFrame waits for a new frame to become available within the timeout duration.
// Uses condition variable for efficient waiting with short polling intervals.
// Returns nil if timeout is exceeded. This is used for efficient streaming.
func (fb *FrameBuffer) WaitFrame(timeout time.Duration) []byte {
	deadline := time.Now().Add(timeout)
	pollInterval := 50 * time.Millisecond // Increased from 5ms to 50ms (still very responsive)

	for {
		fb.condition.L.Lock()
		frame := fb.frame
		fb.condition.L.Unlock()

		// If frame available, return it
		if frame != nil {
			frameCopy := make([]byte, len(frame))
			copy(frameCopy, frame)
			return frameCopy
		}

		// Check if deadline exceeded
		if time.Now().After(deadline) {
			return nil
		}

		// Calculate remaining time
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil
		}

		// Use shorter sleep interval to maintain responsiveness
		if remaining < pollInterval {
			pollInterval = remaining
		}

		// Sleep briefly before next poll
		time.Sleep(pollInterval)
	}
}

// Ensure FrameBuffer implements io.Writer
var _ io.Writer = (*FrameBuffer)(nil)
