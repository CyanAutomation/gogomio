package camera

import (
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
				_, seq := fb.WaitFrame(10*time.Millisecond, lastSeq)
				if seq > lastSeq {
					lastSeq = seq
					atomic.AddInt64(&readCount, 1)
				}
			}
		}(i)
	}

	// Run for 1 second
	time.Sleep(1 * time.Second)
	close(done)
	wg.Wait()

	if writeCount == 0 || readCount == 0 {
		t.Logf("concurrent write/read stress test: %d writes, %d reads", writeCount, readCount)
	}

	// Verify frameSeq monotonicity
	seq1 := fb.CurrentSequence()
	time.Sleep(100 * time.Millisecond)
	seq2 := fb.CurrentSequence()
	if seq2 < seq1 {
		t.Errorf("frame sequence went backwards: %d -> %d", seq1, seq2)
	}
}

// TestFrameBufferWaitTimeoutRaceFree verifies WaitFrame timeout doesn't race
func TestFrameBufferWaitTimeoutRaceFree(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0) // No throttling

	done := make(chan struct{})
	var wg sync.WaitGroup
	successCount := 0
	timeoutCount := 0
	mu := &sync.Mutex{}

	// Write frames continuously
	wg.Add(1)
	go func() {
		defer wg.Done()
		frame := []byte{0xFF, 0xD8, 0x00, 0xFF, 0xD9}
		for {
			select {
			case <-done:
				return
			default:
				_, _ = fb.Write(frame)
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()

	// Many readers waiting with timeout
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				frame, _ := fb.WaitFrame(20*time.Millisecond, 0)
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

	wg.Wait()
	close(done)

	mu.Lock()
	total := successCount + timeoutCount
	mu.Unlock()

	if total != 30*50 {
		t.Errorf("expected 1500 waits, got %d", total)
	}

	t.Logf("WaitFrame race test: %d successful waits, %d timeouts", successCount, timeoutCount)
}
