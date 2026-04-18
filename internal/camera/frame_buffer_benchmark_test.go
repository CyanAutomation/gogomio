package camera

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// legacyFrameBuffer benchmarks the previous WaitFrame timeout strategy, which
// allocated a timer each wait-loop iteration.
type legacyFrameBuffer struct {
	mu       sync.Mutex
	frame    []byte
	frameSeq uint64
	notifyCh chan struct{}
}

func newLegacyFrameBuffer() *legacyFrameBuffer {
	return &legacyFrameBuffer{notifyCh: make(chan struct{})}
}

func (fb *legacyFrameBuffer) WaitFrameWithContext(ctx context.Context, timeout time.Duration, lastSeenSeq uint64) ([]byte, uint64) {
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

func benchmarkWaitFrameTimeoutNew(b *testing.B, readers int) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	benchmarkWaitFrameTimeoutConcurrent(b, readers, func(timeout time.Duration) {
		_, _ = fb.WaitFrameWithContext(context.Background(), timeout, 0)
	})
}

func benchmarkWaitFrameTimeoutLegacy(b *testing.B, readers int) {
	fb := newLegacyFrameBuffer()

	benchmarkWaitFrameTimeoutConcurrent(b, readers, func(timeout time.Duration) {
		_, _ = fb.WaitFrameWithContext(context.Background(), timeout, 0)
	})
}

func benchmarkWaitFrameTimeoutConcurrent(b *testing.B, readers int, waitFn func(timeout time.Duration)) {
	b.Helper()
	b.ReportAllocs()

	var opIdx int64 = -1

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for {
				if int(atomic.AddInt64(&opIdx, 1)) >= b.N {
					return
				}
				waitFn(250 * time.Microsecond)
			}
		}()
	}

	b.ResetTimer()
	close(start)
	wg.Wait()
}

func BenchmarkWaitFrameTimeout_New(b *testing.B) {
	for _, readers := range []int{1, 8, 32} {
		b.Run(fmt.Sprintf("readers_%d", readers), func(b *testing.B) {
			benchmarkWaitFrameTimeoutNew(b, readers)
		})
	}
}

func BenchmarkWaitFrameTimeout_Legacy(b *testing.B) {
	for _, readers := range []int{1, 8, 32} {
		b.Run(fmt.Sprintf("readers_%d", readers), func(b *testing.B) {
			benchmarkWaitFrameTimeoutLegacy(b, readers)
		})
	}
}
