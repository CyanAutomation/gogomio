package camera

import (
	"context"
	"io"
	"sync"
	"time"
)

// frameSnapshot captures an immutable frame payload and its publish sequence.
//
// The []byte payload must never be mutated after publication.
type frameSnapshot struct {
	data []byte
	seq  uint64
}

// FrameBuffer is a thread-safe latest-frame store for JPEG frames.
// It implements io.Writer interface for use with Picamera2-style encoders.
type FrameBuffer struct {
	mu                    sync.Mutex
	snapshot              frameSnapshot
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

	// Clone once on publication to preserve immutability for all readers.
	frameData := make([]byte, size)
	copy(frameData, buf)

	fb.snapshot = frameSnapshot{
		data: frameData,
		seq:  fb.snapshot.seq + 1,
	}
	fb.lastFrameMonotonic = now
	fb.stats.RecordFrame(now)

	// Signal all waiting readers with a fresh publish channel.
	close(fb.notifyCh)
	fb.notifyCh = make(chan struct{})

	return size, nil
}

// GetFrame returns a defensive copy of the current frame (for snapshot endpoints).
func (fb *FrameBuffer) GetFrame() []byte {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if fb.snapshot.data == nil {
		return nil
	}

	frameCopy := make([]byte, len(fb.snapshot.data))
	copy(frameCopy, fb.snapshot.data)
	return frameCopy
}

// CurrentSequence returns the latest published frame sequence.
func (fb *FrameBuffer) CurrentSequence() uint64 {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	return fb.snapshot.seq
}

// WaitFrame waits for a frame newer than lastSeenSeq within timeout.
//
// Returned frame bytes are shared and MUST be treated as read-only.
// Returns (nil, lastSeenSeq) if timeout is exceeded.
func (fb *FrameBuffer) WaitFrame(timeout time.Duration, lastSeenSeq uint64) ([]byte, uint64) {
	return fb.WaitFrameWithContext(context.Background(), timeout, lastSeenSeq)
}

// WaitFrameWithContext waits for a frame newer than lastSeenSeq within timeout.
//
// Returned frame bytes are shared and MUST be treated as read-only.
// Returns (nil, lastSeenSeq) when context is canceled or timeout is exceeded.
func (fb *FrameBuffer) WaitFrameWithContext(ctx context.Context, timeout time.Duration, lastSeenSeq uint64) ([]byte, uint64) {
	if timeout <= 0 {
		fb.mu.Lock()
		defer fb.mu.Unlock()
		if fb.snapshot.seq > lastSeenSeq && fb.snapshot.data != nil {
			return fb.snapshot.data, fb.snapshot.seq
		}
		return nil, lastSeenSeq
	}

	if err := ctx.Err(); err != nil {
		return nil, lastSeenSeq
	}

	timer := time.NewTimer(timeout)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	for {
		fb.mu.Lock()
		if fb.snapshot.seq > lastSeenSeq && fb.snapshot.data != nil {
			frame := fb.snapshot.data
			seq := fb.snapshot.seq
			fb.mu.Unlock()
			return frame, seq
		}
		notifyCh := fb.notifyCh
		fb.mu.Unlock()

		select {
		case <-notifyCh:
			// A frame may have been published; re-check under lock.
		case <-ctx.Done():
			return nil, lastSeenSeq
		case <-timer.C:
			return nil, lastSeenSeq
		}
	}
}

// Ensure FrameBuffer implements io.Writer
var _ io.Writer = (*FrameBuffer)(nil)
