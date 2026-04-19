package camera

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"
)

// TestFrameBufferWrite tests basic frame writing and reading
func TestFrameBufferWrite(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	testFrame := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG SOI marker

	n, err := fb.Write(testFrame)
	if err != nil {
		t.Errorf("Write returned error: %v", err)
	}
	if n != len(testFrame) {
		t.Errorf("Write returned %d, want %d", n, len(testFrame))
	}

	if fb.snapshot.data == nil {
		t.Error("frame is nil after write")
	}

	if !bytes.Equal(fb.snapshot.data, testFrame) {
		t.Errorf("frame mismatch: got %v, want %v", fb.snapshot.data, testFrame)
	}
}

// TestFrameBufferConditionSignaling tests waiter notification on writes.
func TestFrameBufferConditionSignaling(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	testFrame := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	received := make(chan []byte, 1)

	go func() {
		frame, _ := fb.WaitFrame(2*time.Second, 0)
		received <- frame
	}()

	time.Sleep(100 * time.Millisecond)

	_, _ = fb.Write(testFrame)

	// Should receive frame within timeout
	select {
	case frame := <-received:
		if !bytes.Equal(frame, testFrame) {
			t.Errorf("frame mismatch: got %v, want %v", frame, testFrame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for frame")
	}
}

// TestFrameBufferFPSThrottling tests FPS throttling when target_fps > 0
func TestFrameBufferFPSThrottling(t *testing.T) {
	stats := NewStreamStats()
	targetFPS := 10
	fb := NewFrameBuffer(stats, targetFPS)

	frame1 := []byte{1}
	frame2 := []byte{2}
	frame3 := []byte{3}

	// Write first frame
	n1, _ := fb.Write(frame1)
	if n1 != 1 {
		t.Errorf("first write returned %d", n1)
	}

	// Get timestamp after first write
	lastTime := fb.lastFrameMonotonic

	// Write second frame immediately (should be throttled)
	n2, _ := fb.Write(frame2)
	if n2 != 1 {
		t.Errorf("second write returned %d", n2)
	}

	// Frame should NOT have changed if throttled
	if !bytes.Equal(fb.snapshot.data, frame1) {
		t.Errorf("frame should still be frame1 after throttled write, got %v", fb.snapshot.data)
	}

	// Wait for throttle interval
	time.Sleep(time.Duration(1000/targetFPS) * time.Millisecond)

	// Write third frame (should succeed)
	n3, _ := fb.Write(frame3)
	if n3 != 1 {
		t.Errorf("third write returned %d", n3)
	}

	// Frame should now be frame3
	if !bytes.Equal(fb.snapshot.data, frame3) {
		t.Errorf("frame should be frame3 after throttle interval, got %v", fb.snapshot.data)
	}

	// Timestamp should have advanced
	if fb.lastFrameMonotonic == lastTime {
		t.Error("lastFrameMonotonic did not advance")
	}
}

// TestFrameBufferConcurrentReads tests multiple concurrent readers
func TestFrameBufferConcurrentReads(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	testFrame := []byte{0xFF, 0xD8}
	numReaders := 5
	results := make([][]byte, numReaders)
	var wg sync.WaitGroup

	// Start readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			frame, _ := fb.WaitFrame(2*time.Second, 0)
			results[idx] = frame
		}(i)
	}

	// Give readers time to block
	time.Sleep(100 * time.Millisecond)

	// Write frame (should notify all readers)
	_, _ = fb.Write(testFrame)

	// Wait for all readers
	done := make(chan struct{})
	go func() {
		wg.Wait()
		done <- struct{}{}
	}()

	select {
	case <-done:
		// Verify all readers got the frame
		for i, frame := range results {
			if !bytes.Equal(frame, testFrame) {
				t.Errorf("reader %d got %v, want %v", i, frame, testFrame)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for readers")
	}
}

// TestFrameBufferMultipleWrites tests that latest frame is always available
func TestFrameBufferMultipleWrites(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	frames := [][]byte{
		{1},
		{2},
		{3},
		{4},
		{5},
	}

	for _, frame := range frames {
		_, _ = fb.Write(frame)
	}

	if !bytes.Equal(fb.snapshot.data, frames[len(frames)-1]) {
		t.Errorf("last frame is %v, want %v", fb.snapshot.data, frames[len(frames)-1])
	}
}

// TestFrameBufferWriteUpdatesStats tests that stats are updated on write
func TestFrameBufferWriteUpdatesStats(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	initialCount, _, _ := stats.Snapshot()
	if initialCount != 0 {
		t.Errorf("initial frame count is %d, want 0", initialCount)
	}

	_, _ = fb.Write([]byte{1})
	_, _ = fb.Write([]byte{2})
	_, _ = fb.Write([]byte{3})

	finalCount, _, _ := stats.Snapshot()
	if finalCount != 3 {
		t.Errorf("final frame count is %d, want 3", finalCount)
	}
}

// TestFrameBufferWriteWithNilStats ensures NewFrameBuffer initializes stats
// when nil is provided and writes behave normally.
func TestFrameBufferWriteWithNilStats(t *testing.T) {
	targetFPS := 0
	fb := NewFrameBuffer(nil, targetFPS)

	if fb.stats == nil {
		t.Fatal("stats is nil, want initialized StreamStats")
	}

	frame1 := []byte{0xAA}
	frame2 := []byte{0xBB}

	if _, err := fb.Write(frame1); err != nil {
		t.Fatalf("first Write returned error: %v", err)
	}
	if _, err := fb.Write(frame2); err != nil {
		t.Fatalf("second Write returned error: %v", err)
	}

	if gotSeq := fb.CurrentSequence(); gotSeq != 2 {
		t.Fatalf("sequence is %d, want 2", gotSeq)
	}

	gotFrame := fb.GetFrame()
	if !bytes.Equal(gotFrame, frame2) {
		t.Fatalf("latest frame is %v, want %v", gotFrame, frame2)
	}

	count, _, _ := fb.stats.Snapshot()
	if count != 2 {
		t.Fatalf("stats frame count is %d, want 2", count)
	}
}

// TestFrameBufferConcurrentWrites exercises concurrent writers and validates
// state remains consistent when Write is called from many goroutines.
func TestFrameBufferConcurrentWrites(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	const writers = 64

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(writers)

	for i := 0; i < writers; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			_, _ = fb.Write([]byte{byte(idx)})
		}(i)
	}

	close(start)
	wg.Wait()

	if gotSeq := fb.CurrentSequence(); gotSeq != writers {
		t.Fatalf("frame sequence is %d, want %d", gotSeq, writers)
	}

	if gotFrame := fb.GetFrame(); len(gotFrame) != 1 {
		t.Fatalf("last frame length is %d, want 1", len(gotFrame))
	}

	fb.mu.Lock()
	lastMonotonic := fb.lastFrameMonotonic
	fb.mu.Unlock()

	if lastMonotonic <= 0 {
		t.Fatalf("lastFrameMonotonic is %d, want > 0", lastMonotonic)
	}

	count, _, _ := stats.Snapshot()
	if count != writers {
		t.Fatalf("stats frame count is %d, want %d", count, writers)
	}
}

// TestFrameBufferWaitFrameSuccess tests WaitFrame returns frame when available
func TestFrameBufferWaitFrameSuccess(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)
	testFrame := []byte{1, 2, 3, 4, 5}

	done := make(chan struct {
		frame []byte
		seq   uint64
	})
	go func() {
		frame, seq := fb.WaitFrame(2*time.Second, 0)
		done <- struct {
			frame []byte
			seq   uint64
		}{frame: frame, seq: seq}
	}()

	// Give goroutine time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Write frame
	_, _ = fb.Write(testFrame)

	select {
	case result := <-done:
		if !bytes.Equal(result.frame, testFrame) {
			t.Errorf("WaitFrame got %v, want %v", result.frame, testFrame)
		}
		if result.seq == 0 {
			t.Error("WaitFrame returned zero sequence after write")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for frame")
	}
}

// TestFrameBufferWaitFrameIgnoresUnchangedFrame ensures no duplicate immediate return
// when waiting with the current sequence value.
func TestFrameBufferWaitFrameIgnoresUnchangedFrame(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	_, _ = fb.Write([]byte{9, 9, 9})
	currentSeq := fb.CurrentSequence()

	start := time.Now()
	frame, seq := fb.WaitFrame(120*time.Millisecond, currentSeq)
	elapsed := time.Since(start)

	if frame != nil {
		t.Fatalf("WaitFrame returned frame for unchanged sequence: %v", frame)
	}
	if seq != currentSeq {
		t.Fatalf("WaitFrame returned seq %d, want %d on timeout", seq, currentSeq)
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("WaitFrame returned too quickly for unchanged sequence: elapsed=%v", elapsed)
	}
}

// TestFrameBufferWaitFrameTimeout tests WaitFrame returns nil on timeout
func TestFrameBufferWaitFrameTimeout(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	done := make(chan struct {
		frame []byte
		seq   uint64
	})
	go func() {
		frame, seq := fb.WaitFrame(100*time.Millisecond, 0)
		done <- struct {
			frame []byte
			seq   uint64
		}{frame: frame, seq: seq}
	}()

	select {
	case result := <-done:
		if result.frame != nil {
			t.Errorf("WaitFrame got %v on timeout, want nil", result.frame)
		}
		if result.seq != 0 {
			t.Errorf("WaitFrame returned seq %d on initial timeout, want 0", result.seq)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout in test")
	}
}

// TestFrameBufferWaitFrameWithContextCancel returns quickly on context cancellation.
func TestFrameBufferWaitFrameWithContextCancel(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	frame, seq := fb.WaitFrameWithContext(ctx, 2*time.Second, 0)
	elapsed := time.Since(start)

	if frame != nil {
		t.Fatalf("WaitFrameWithContext got frame %v, want nil on canceled context", frame)
	}
	if seq != 0 {
		t.Fatalf("WaitFrameWithContext returned seq %d, want 0 on canceled context", seq)
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("WaitFrameWithContext canceled too slowly: elapsed=%v", elapsed)
	}
}

// TestFrameBufferWaitFrameReturnsSharedReadOnlyData validates WaitFrame reuses
// the published immutable snapshot to avoid per-read allocations.
func TestFrameBufferWaitFrameReturnsSharedReadOnlyData(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)
	frame := []byte{7, 8, 9}
	_, _ = fb.Write(frame)

	first, firstSeq := fb.WaitFrame(0, 0)
	second, secondSeq := fb.WaitFrame(0, 0)

	if firstSeq == 0 || secondSeq == 0 {
		t.Fatalf("expected non-zero sequence, got first=%d second=%d", firstSeq, secondSeq)
	}
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("expected non-empty frames")
	}
	if &first[0] != &second[0] {
		t.Fatalf("expected shared underlying frame storage, got %p and %p", &first[0], &second[0])
	}
}

// TestFrameBufferGetFrameReturnsCopy ensures snapshot reads remain safe against
// caller-side mutation.
func TestFrameBufferGetFrameReturnsCopy(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)
	_, _ = fb.Write([]byte{1, 2, 3})

	snap := fb.GetFrame()
	snap[0] = 9

	current, _ := fb.WaitFrame(0, 0)
	if current[0] != 1 {
		t.Fatalf("snapshot mutation leaked into buffer: got %d, want 1", current[0])
	}
}

// TestFrameBufferWriteImmutableAdoptsStorage ensures immutable writes do not
// clone and readers observe the adopted shared bytes.
func TestFrameBufferWriteImmutableAdoptsStorage(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)
	frame := []byte{4, 5, 6}

	_, _ = fb.WriteImmutable(frame)
	current, _ := fb.WaitFrame(0, 0)

	if &current[0] != &frame[0] {
		t.Fatalf("expected adopted frame storage, got %p and %p", &current[0], &frame[0])
	}
}

// TestFrameBufferWriteCopiesInput ensures standard Write remains defensive for
// mutable callers.
func TestFrameBufferWriteCopiesInput(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)
	frame := []byte{1, 2, 3}

	_, _ = fb.Write(frame)
	frame[0] = 9

	current, _ := fb.WaitFrame(0, 0)
	if current[0] != 1 {
		t.Fatalf("Write should clone caller bytes: got %d, want 1", current[0])
	}
}
