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

// TestFrameBufferFPSThrottling tests FPS throttling when target_fps > 0
func TestFrameBufferFPSThrottling(t *testing.T) {
	stats := NewStreamStats()
	targetFPS := 10
	fb := NewFrameBuffer(stats, targetFPS)
	frameInterval := time.Second / time.Duration(targetFPS)

	now := time.Unix(1700000000, 0)
	nowPtr := &now
	fb.nowFn = func() time.Time {
		return *nowPtr
	}

	frame1 := []byte{1}
	frame2 := []byte{2}
	frame3 := []byte{3}

	// Write first frame
	n1, _ := fb.Write(frame1)
	if n1 != 1 {
		t.Errorf("first write returned %d", n1)
	}

	// Write the second frame just before the throttle boundary.
	now = now.Add(frameInterval - time.Nanosecond)
	n2, _ := fb.Write(frame2)
	if n2 != 1 {
		t.Errorf("second write returned %d", n2)
	}

	if got := fb.GetFrame(); !bytes.Equal(got, frame1) {
		t.Errorf("frame should still be frame1 before throttle boundary, got %v", got)
	}

	// Writes at the throttle boundary are published.
	now = now.Add(time.Nanosecond)
	n3, _ := fb.Write(frame3)
	if n3 != 1 {
		t.Errorf("third write returned %d", n3)
	}

	if got := fb.GetFrame(); !bytes.Equal(got, frame3) {
		t.Errorf("frame should be frame3 at throttle boundary, got %v", got)
	}
}

func TestFrameBufferFPSThrottlingWithClockJump(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 10)

	base := time.Unix(1700000000, 0)
	times := []time.Time{
		base,
		base.Add(20 * time.Millisecond),
		base.Add(-1 * time.Hour), // simulated wall-clock jump backwards
		base.Add(130 * time.Millisecond),
	}
	idx := 0
	fb.nowFn = func() time.Time {
		v := times[idx]
		if idx < len(times)-1 {
			idx++
		}
		return v
	}

	_, _ = fb.Write([]byte{1})
	_, _ = fb.Write([]byte{2})
	if got := fb.GetFrame(); !bytes.Equal(got, []byte{1}) {
		t.Fatalf("second frame should be throttled, got %v", got)
	}

	_, _ = fb.Write([]byte{3})
	if got := fb.GetFrame(); !bytes.Equal(got, []byte{1}) {
		t.Fatalf("clock-jump frame should be throttled, got %v", got)
	}

	_, _ = fb.Write([]byte{4})
	if got := fb.GetFrame(); !bytes.Equal(got, []byte{4}) {
		t.Fatalf("frame after interval should publish, got %v", got)
	}
}

// TestFrameBufferConcurrentReads tests multiple concurrent readers
func TestFrameBufferConcurrentReads(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	testFrame := []byte{0xFF, 0xD8}
	const numReaders = 5
	results := make([][]byte, numReaders)
	ready := make(chan struct{}, numReaders)
	fb.waiterRegisteredHook = func() {
		ready <- struct{}{}
	}

	// This is the test's only failure deadline. Reader readiness, publication,
	// and completion are otherwise coordinated entirely through notifications.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			frame, _ := fb.WaitFrameWithContext(ctx, time.Hour, 0)
			results[idx] = frame
		}(i)
	}

	for i := 0; i < numReaders; i++ {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal("deadline exceeded waiting for readers to register")
		}
	}

	_, _ = fb.Write(testFrame)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("deadline exceeded waiting for readers to receive frame")
	}

	for i, frame := range results {
		if !bytes.Equal(frame, testFrame) {
			t.Errorf("reader %d got %v, want %v", i, frame, testFrame)
		}
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

// TestFrameBufferWriteWithNilStats ensures writes and reads behave normally
// when NewFrameBuffer is asked to create its own stats collector.
func TestFrameBufferWriteWithNilStats(t *testing.T) {
	fb := NewFrameBuffer(nil, 0)
	frame1 := []byte{0xAA, 0x01}
	frame2 := []byte{0xBB, 0x02, 0x03}

	n, err := fb.Write(frame1)
	if err != nil {
		t.Fatalf("first Write returned error: %v", err)
	}
	if n != len(frame1) {
		t.Fatalf("first Write returned %d bytes, want %d", n, len(frame1))
	}
	if gotSeq := fb.CurrentSequence(); gotSeq != 1 {
		t.Fatalf("sequence after first Write is %d, want 1", gotSeq)
	}
	gotFrame, firstSeq := fb.WaitFrame(0, 0)
	if !bytes.Equal(gotFrame, frame1) || firstSeq != 1 {
		t.Fatalf("first WaitFrame returned frame %v at sequence %d, want %v at sequence 1", gotFrame, firstSeq, frame1)
	}

	n, err = fb.Write(frame2)
	if err != nil {
		t.Fatalf("second Write returned error: %v", err)
	}
	if n != len(frame2) {
		t.Fatalf("second Write returned %d bytes, want %d", n, len(frame2))
	}
	if gotSeq := fb.CurrentSequence(); gotSeq != firstSeq+1 {
		t.Fatalf("sequence after second Write is %d, want %d", gotSeq, firstSeq+1)
	}

	if latest := fb.GetFrame(); !bytes.Equal(latest, frame2) {
		t.Fatalf("latest frame is %v, want %v", latest, frame2)
	}
	gotFrame, secondSeq := fb.WaitFrame(0, firstSeq)
	if !bytes.Equal(gotFrame, frame2) || secondSeq != 2 {
		t.Fatalf("second WaitFrame returned frame %v at sequence %d, want %v at sequence 2", gotFrame, secondSeq, frame2)
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
	lastFrameTime := fb.lastFrameTime
	fb.mu.Unlock()

	if lastFrameTime.IsZero() {
		t.Fatal("lastFrameTime is zero, want non-zero")
	}

	count, _, _ := stats.Snapshot()
	if count != writers {
		t.Fatalf("stats frame count is %d, want %d", count, writers)
	}
}

// TestFrameBufferWaitFrameSuccess tests that a waiting reader is notified by a write.
func TestFrameBufferWaitFrameSuccess(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)
	initialFrame := []byte{1, 2, 3}
	testFrame := []byte{4, 5, 6}
	if _, err := fb.Write(initialFrame); err != nil {
		t.Fatalf("initial Write returned error: %v", err)
	}
	initialSeq := fb.CurrentSequence()

	waiting := make(chan struct{})
	var waitingOnce sync.Once
	fb.waiterRegisteredHook = func() { waitingOnce.Do(func() { close(waiting) }) }

	done := make(chan struct {
		frame []byte
		seq   uint64
	})
	go func() {
		frame, seq := fb.WaitFrame(time.Second, initialSeq)
		done <- struct {
			frame []byte
			seq   uint64
		}{frame: frame, seq: seq}
	}()

	writeDone := make(chan error, 1)
	go func() {
		<-waiting
		_, err := fb.Write(testFrame)
		writeDone <- err
	}()

	select {
	case result := <-done:
		if !bytes.Equal(result.frame, testFrame) {
			t.Errorf("WaitFrame got %v, want %v", result.frame, testFrame)
		}
		if wantSeq := initialSeq + 1; result.seq != wantSeq {
			t.Errorf("WaitFrame returned sequence %d, want %d", result.seq, wantSeq)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for frame")
	}

	if err := <-writeDone; err != nil {
		t.Fatalf("Write returned error: %v", err)
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
