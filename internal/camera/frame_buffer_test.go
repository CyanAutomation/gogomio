package camera

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"
)

// Contract: TC-FRAME-02 (docs/testing/test-contracts.md).
func TestFrameBufferWritePublishesLatestFrame(t *testing.T) {
	fb := NewFrameBuffer(NewStreamStats(), 0)
	frames := [][]byte{
		{0xFF, 0xD8, 0xFF, 0xE0}, // JPEG SOI marker
		{0x01, 0x02},
		{0x03, 0x04, 0x05},
	}

	for i, frame := range frames {
		n, err := fb.Write(frame)
		if err != nil {
			t.Fatalf("Write(%d) returned error: %v", i, err)
		}
		if n != len(frame) {
			t.Fatalf("Write(%d) returned %d bytes, want %d", i, n, len(frame))
		}
	}

	if got := fb.GetFrame(); !bytes.Equal(got, frames[len(frames)-1]) {
		t.Fatalf("GetFrame() = %v, want latest frame %v", got, frames[len(frames)-1])
	}
	if got, want := fb.CurrentSequence(), uint64(len(frames)); got != want {
		t.Fatalf("CurrentSequence() = %d, want %d", got, want)
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

func TestFrameBufferThrottleDoesNotWakeWaitersWithoutANewFrame(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 10)
	now := time.Unix(1700000000, 0)
	fb.nowFn = func() time.Time { return now }
	_, _ = fb.WriteImmutable([]byte{1})
	oldNotify := fb.notifyCh
	now = now.Add(time.Millisecond)
	_, _ = fb.WriteImmutable([]byte{2})
	select {
	case <-oldNotify:
		t.Fatal("throttled write woke waiters despite not publishing a frame")
	default:
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

// Contract: TC-FRAME-02 (docs/testing/test-contracts.md).
func TestFrameBufferWithoutStatsCollectorTracksPublishedFrames(t *testing.T) {
	fb := NewFrameBuffer(nil, 0)
	if fb.stats == nil {
		t.Fatal("NewFrameBuffer(nil, 0) did not create a stats collector")
	}

	for _, frame := range [][]byte{{0xAA, 0x01}, {0xBB, 0x02, 0x03}} {
		if _, err := fb.Write(frame); err != nil {
			t.Fatalf("Write returned error: %v", err)
		}
	}

	got, _, _ := fb.stats.Snapshot()
	if got != 2 {
		t.Fatalf("fallback stats recorded %d frames, want 2", got)
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

// TestFrameBufferWaitFrameReturnsPublishedData verifies repeated waits return
// the current published frame and sequence.
func TestFrameBufferWaitFrameReturnsPublishedData(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)
	frame := []byte{7, 8, 9}
	_, _ = fb.Write(frame)

	first, firstSeq := fb.WaitFrame(0, 0)
	second, secondSeq := fb.WaitFrame(0, 0)

	if firstSeq == 0 || secondSeq != firstSeq {
		t.Fatalf("published sequence changed without a write: first=%d second=%d", firstSeq, secondSeq)
	}
	if !bytes.Equal(first, frame) || !bytes.Equal(second, frame) {
		t.Fatalf("WaitFrame returned first=%v second=%v, want published frame %v", first, second, frame)
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

// Contract: TC-FRAME-03 (docs/testing/test-contracts.md).
func TestFrameBufferWriteImmutablePublishesWithoutCopying(t *testing.T) {
	fb := NewFrameBuffer(NewStreamStats(), 0)
	frame := []byte{4, 5, 6}

	n, err := fb.WriteImmutable(frame)
	if err != nil {
		t.Fatalf("WriteImmutable returned error: %v", err)
	}
	if n != len(frame) {
		t.Fatalf("WriteImmutable returned %d bytes, want %d", n, len(frame))
	}

	got, seq := fb.WaitFrame(0, 0)
	if !bytes.Equal(got, frame) {
		t.Fatalf("WaitFrame() = %v, want %v", got, frame)
	}
	if seq != 1 {
		t.Fatalf("WaitFrame() sequence = %d, want 1", seq)
	}
	if len(got) == 0 || &got[0] != &frame[0] {
		t.Fatal("WaitFrame() did not return the supplied frame storage")
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
