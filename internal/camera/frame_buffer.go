package camera

import (
	"context"
	"io"
	"sync"
	"time"
)

// FrameBuffer is a thread-safe circular buffer for JPEG frames.
// It implements io.Writer interface for use with Picamera2-style encoders.
type FrameBuffer struct {
	mu                    sync.Mutex
	frame                 []byte
	frameSeq              uint64
	notifyCh              chan struct{}
	stats                 *StreamStats
	lastFrameMonotonic    int64
	targetFrameIntervalNS int64
}

// NewFrameBuffer creates a new FrameBuffer.
// targetFPS <= 0 means no throttling.
func NewFrameBuffer(stats *StreamStats, targetFPS int) *FrameBuffer {
	fb := &FrameBuffer{
		notifyCh: make(chan struct{}),
		stats:    stats,
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

	fb.mu.Lock()
	defer fb.mu.Unlock()

	// Check if we should throttle based on target FPS.
	now := time.Now().UnixNano()
	if fb.targetFrameIntervalNS > 0 && fb.lastFrameMonotonic > 0 {
		elapsed := now - fb.lastFrameMonotonic
		if elapsed < fb.targetFrameIntervalNS {
			// Too soon, skip this frame.
			return size, nil
		}
	}

	// Store frame and update timestamp
	fb.frame = make([]byte, len(buf))
	copy(fb.frame, buf)
	fb.frameSeq++
	fb.lastFrameMonotonic = now
	fb.stats.RecordFrame(now)

	// Signal all waiting readers with a fresh publish channel.
	close(fb.notifyCh)
	fb.notifyCh = make(chan struct{})

	return size, nil
}

// GetFrame returns a copy of the current frame (for snapshot endpoints).
func (fb *FrameBuffer) GetFrame() []byte {
	fb.mu.Lock()
	defer fb.mu.Unlock()

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
	fb.mu.Lock()
	defer fb.mu.Unlock()
	return fb.frameSeq
}

// WaitFrame waits for a frame newer than lastSeenSeq within timeout.
// Returns (nil, lastSeenSeq) if timeout is exceeded.
func (fb *FrameBuffer) WaitFrame(timeout time.Duration, lastSeenSeq uint64) ([]byte, uint64) {
	return fb.WaitFrameWithContext(context.Background(), timeout, lastSeenSeq)
}

// WaitFrameWithContext waits for a frame newer than lastSeenSeq within timeout.
// Returns (nil, lastSeenSeq) when context is canceled or timeout is exceeded.
func (fb *FrameBuffer) WaitFrameWithContext(ctx context.Context, timeout time.Duration, lastSeenSeq uint64) ([]byte, uint64) {
	fb.mu.Lock()

	if fb.frameSeq > lastSeenSeq && fb.frame != nil {
		frameCopy := make([]byte, len(fb.frame))
		copy(frameCopy, fb.frame)
		seq := fb.frameSeq
		fb.mu.Unlock()
		return frameCopy, seq
	}

	if timeout <= 0 {
		fb.mu.Unlock()
		return nil, lastSeenSeq
	}

	deadline := time.Now().Add(timeout)

	for fb.frameSeq <= lastSeenSeq {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			fb.mu.Unlock()
			return nil, lastSeenSeq
		}

		notifyCh := fb.notifyCh
		fb.mu.Unlock()

		timer := time.NewTimer(remaining)
		timedOut := false
		select {
		case <-notifyCh:
		case <-ctx.Done():
		case <-timer.C:
			timedOut = true
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		fb.mu.Lock()
		if ctx.Err() != nil {
			fb.mu.Unlock()
			return nil, lastSeenSeq
		}
		if timedOut && fb.frameSeq <= lastSeenSeq {
			fb.mu.Unlock()
			return nil, lastSeenSeq
		}
	}

	if fb.frameSeq <= lastSeenSeq || fb.frame == nil {
		fb.mu.Unlock()
		return nil, lastSeenSeq
	}

	frameCopy := make([]byte, len(fb.frame))
	copy(frameCopy, fb.frame)
	seq := fb.frameSeq
	fb.mu.Unlock()
	return frameCopy, seq
}

// Ensure FrameBuffer implements io.Writer
var _ io.Writer = (*FrameBuffer)(nil)
