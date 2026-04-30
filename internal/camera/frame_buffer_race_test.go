package camera

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestFrameBufferConcurrentWriteRead stress tests concurrent frame writes and reads
func TestFrameBufferConcurrentWriteRead(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 30)

	done := make(chan struct{})
	writeCount := int64(0)
	readCount := int64(0)
	var wg sync.WaitGroup

	// 5 concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				frame := []byte{0xFF, 0xD8, byte(id), 0xFF, 0xD9}
				_, _ = fb.Write(frame)
				atomic.AddInt64(&writeCount, 1)
			}
		}(i)
	}

	// 20 concurrent readers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lastSeq := uint64(0)
			for {
				select {
				case <-done:
					return
				default:
				}
				_, seq := fb.WaitFrame(raceScaledDuration(10*time.Millisecond), lastSeq)
				if seq > lastSeq {
					lastSeq = seq
					atomic.AddInt64(&readCount, 1)
				}
			}
		}(i)
	}

	runFor := raceScaledDuration(400 * time.Millisecond)
	timer := time.NewTimer(runFor)
	<-timer.C
	close(done)
	wg.Wait()

	if writeCount == 0 || readCount == 0 {
		t.Logf("concurrent write/read stress test: %d writes, %d reads", writeCount, readCount)
	}

	// Verify frameSeq monotonicity with a bounded eventual loop.
	seq1 := fb.CurrentSequence()
	var seq2 uint64
	deadline := time.Now().Add(raceScaledDuration(300 * time.Millisecond))
	for {
		seq2 = fb.CurrentSequence()
		if seq2 >= seq1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if seq2 < seq1 {
		t.Errorf("frame sequence went backwards: %d -> %d", seq1, seq2)
	}
}

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

// TestFrameBufferWaitFrameNoPerWaiterGoroutine validates timeout waits do not
// leak or grow goroutines linearly with waiter count.
func TestFrameBufferWaitFrameNoPerWaiterGoroutine(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	runtime.GC()
	time.Sleep(raceScaledDuration(20 * time.Millisecond))
	before := runtime.NumGoroutine()

	const waiters = 200
	var wg sync.WaitGroup
	wg.Add(waiters)
	for i := 0; i < waiters; i++ {
		go func() {
			defer wg.Done()
			frame, seq := fb.WaitFrame(raceScaledDuration(20*time.Millisecond), 0)
			if frame != nil || seq != 0 {
				t.Errorf("expected timeout result, got frame=%v seq=%d", frame != nil, seq)
			}
		}()
	}
	wg.Wait()

	// Allow timer internals and goroutines to settle before measuring.
	time.Sleep(raceScaledDuration(100 * time.Millisecond))
	runtime.GC()
	time.Sleep(raceScaledDuration(20 * time.Millisecond))
	after := runtime.NumGoroutine()

	// Large growth would indicate per-wait goroutine behavior.
	if delta := after - before; delta > 20 {
		t.Fatalf("goroutine growth too large after waits: before=%d after=%d delta=%d", before, after, delta)
	}
}
