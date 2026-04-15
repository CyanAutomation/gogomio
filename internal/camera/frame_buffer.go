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
	frameSeq              uint64
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
	fb.frameSeq++
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

// CurrentSequence returns the latest published frame sequence.
func (fb *FrameBuffer) CurrentSequence() uint64 {
	fb.condition.L.Lock()
	defer fb.condition.L.Unlock()
	return fb.frameSeq
}

// WaitFrame waits for a frame newer than lastSeenSeq within timeout.
// Returns (nil, lastSeenSeq) if timeout is exceeded.
func (fb *FrameBuffer) WaitFrame(lastSeenSeq uint64, timeout time.Duration) ([]byte, uint64) {
	fb.condition.L.Lock()
	defer fb.condition.L.Unlock()

	if fb.frameSeq > lastSeenSeq && fb.frame != nil {
		frameCopy := make([]byte, len(fb.frame))
		copy(frameCopy, fb.frame)
		return frameCopy, fb.frameSeq
	}

	if timeout <= 0 {
		return nil, lastSeenSeq
	}

	timedOut := false
	timer := time.AfterFunc(timeout, func() {
		fb.condition.L.Lock()
		timedOut = true
		fb.condition.Broadcast()
		fb.condition.L.Unlock()
	})
	defer timer.Stop()

	for fb.frameSeq <= lastSeenSeq && !timedOut {
		fb.condition.Wait()
	}

	if fb.frameSeq <= lastSeenSeq || fb.frame == nil {
		return nil, lastSeenSeq
	}

	frameCopy := make([]byte, len(fb.frame))
	copy(frameCopy, fb.frame)
	return frameCopy, fb.frameSeq
}

// Ensure FrameBuffer implements io.Writer
var _ io.Writer = (*FrameBuffer)(nil)
