package camera

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestFrameBufferWaitTimeoutRaceFree verifies WaitFrame timeout doesn't race
func TestFrameBufferWaitTimeoutRaceFree(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0) // No throttling

	done := make(chan struct{})
	var readersWG sync.WaitGroup
	var writerWG sync.WaitGroup
	successCount := 0
	timeoutCount := 0
	mu := &sync.Mutex{}

	// Write frames continuously
	writerWG.Add(1)
	go func() {
		defer writerWG.Done()
		frame := []byte{0xFF, 0xD8, 0x00, 0xFF, 0xD9}
		for {
			select {
			case <-done:
				return
			default:
				_, _ = fb.Write(frame)
				time.Sleep(raceScaledDuration(2 * time.Millisecond))
			}
		}
	}()

	// Many readers waiting with timeout
	for i := 0; i < 30; i++ {
		readersWG.Add(1)
		go func(id int) {
			defer readersWG.Done()
			for j := 0; j < 50; j++ {
				frame, _ := fb.WaitFrame(raceScaledDuration(20*time.Millisecond), 0)
				mu.Lock()
				if frame != nil {
					successCount++
				} else {
					timeoutCount++
				}
				mu.Unlock()
			}
		}(i)
	}

	readersWG.Wait()
	close(done)
	writerWG.Wait()

	mu.Lock()
	total := successCount + timeoutCount
	mu.Unlock()

	if total != 30*50 {
		t.Logf("timeout branch diagnostic: success=%d timeout=%d expected=%d", successCount, timeoutCount, 30*50)
		t.Errorf("expected 1500 waits, got %d", total)
	}

	t.Logf("WaitFrame race test: %d successful waits, %d timeouts", successCount, timeoutCount)
}

// TestFrameBufferWaitFrameNoPerWaiterGoroutine verifies that a fixed set of
// concurrent waits all return when their timeout expires or context is canceled.
func TestFrameBufferWaitFrameNoPerWaiterGoroutine(t *testing.T) {
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
