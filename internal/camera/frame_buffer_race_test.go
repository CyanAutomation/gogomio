package camera

import (
	"context"
	"testing"
	"time"
)

// TestFrameBufferWaitersReturnNoFrameAfterTimeoutOrCancellation verifies that
// timed-out and canceled waits return without inventing a frame or sequence.
// Contract: TC-FRAME-01 (docs/testing/test-contracts.md).
func TestFrameBufferWaitersReturnNoFrameAfterTimeoutOrCancellation(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	const waiters = 200
	entered := make(chan struct{}, waiters)
	type waitResult struct {
		id       int
		hasFrame bool
		seq      uint64
	}
	completed := make(chan waitResult, waiters)
	fb.waiterRegisteredHook = func() {
		entered <- struct{}{}
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < waiters; i++ {
		go func(id int) {
			ctx := context.Background()
			timeout := raceScaledDuration(20 * time.Millisecond)
			if id%2 != 0 {
				ctx = cancelCtx
				timeout = time.Hour
			}

			frame, seq := fb.WaitFrameWithContext(ctx, timeout, 0)
			completed <- waitResult{id: id, hasFrame: frame != nil, seq: seq}
		}(i)
	}

	deadline := time.NewTimer(raceScaledDuration(5 * time.Second))
	defer deadline.Stop()
	for i := 0; i < waiters; i++ {
		select {
		case <-entered:
		case <-deadline.C:
			t.Fatalf("only %d of %d waits entered WaitFrameWithContext", i, waiters)
		}
	}

	// Release the cancellable half. The other half return on their own timers.
	cancel()

	seen := make([]bool, waiters)
	for i := 0; i < waiters; i++ {
		select {
		case result := <-completed:
			if seen[result.id] {
				t.Fatalf("wait %d reported completion more than once", result.id)
			}
			seen[result.id] = true
			if result.hasFrame || result.seq != 0 {
				t.Errorf("wait %d returned an unexpected frame: frame=%v seq=%d", result.id, result.hasFrame, result.seq)
			}
		case <-deadline.C:
			t.Fatalf("only %d of %d waits completed after timeout or cancellation", i, waiters)
		}
	}
}
