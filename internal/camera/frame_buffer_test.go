package camera

import (
	"bytes"
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

	if fb.frame == nil {
		t.Error("frame is nil after write")
	}

	if !bytes.Equal(fb.frame, testFrame) {
		t.Errorf("frame mismatch: got %v, want %v", fb.frame, testFrame)
	}
}

// TestFrameBufferConditionSignaling tests condition variable notification
func TestFrameBufferConditionSignaling(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	testFrame := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	received := make(chan []byte)

	// Reader goroutine
	go func() {
		fb.condition.L.Lock()
		defer fb.condition.L.Unlock()
		fb.condition.Wait()
		received <- fb.frame
	}()

	// Give reader time to block on Wait()
	time.Sleep(100 * time.Millisecond)

	// Writer goroutine
	go func() {
		fb.Write(testFrame)
	}()

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
	if !bytes.Equal(fb.frame, frame1) {
		t.Errorf("frame should still be frame1 after throttled write, got %v", fb.frame)
	}

	// Wait for throttle interval
	time.Sleep(time.Duration(1000/targetFPS) * time.Millisecond)

	// Write third frame (should succeed)
	n3, _ := fb.Write(frame3)
	if n3 != 1 {
		t.Errorf("third write returned %d", n3)
	}

	// Frame should now be frame3
	if !bytes.Equal(fb.frame, frame3) {
		t.Errorf("frame should be frame3 after throttle interval, got %v", fb.frame)
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
			fb.condition.L.Lock()
			defer fb.condition.L.Unlock()
			fb.condition.Wait()
			results[idx] = fb.frame
		}(i)
	}

	// Give readers time to block
	time.Sleep(100 * time.Millisecond)

	// Write frame (should notify all readers)
	fb.Write(testFrame)

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
		fb.Write(frame)
	}

	if !bytes.Equal(fb.frame, frames[len(frames)-1]) {
		t.Errorf("last frame is %v, want %v", fb.frame, frames[len(frames)-1])
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

	fb.Write([]byte{1})
	fb.Write([]byte{2})
	fb.Write([]byte{3})

	finalCount, _, _ := stats.Snapshot()
	if finalCount != 3 {
		t.Errorf("final frame count is %d, want 3", finalCount)
	}
}

// TestFrameBufferWaitFrameSuccess tests WaitFrame returns frame when available
func TestFrameBufferWaitFrameSuccess(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)
	testFrame := []byte{1, 2, 3, 4, 5}

	done := make(chan []byte)
	go func() {
		frame := fb.WaitFrame(2 * time.Second)
		done <- frame
	}()

	// Give goroutine time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Write frame
	fb.Write(testFrame)

	select {
	case frame := <-done:
		if !bytes.Equal(frame, testFrame) {
			t.Errorf("WaitFrame got %v, want %v", frame, testFrame)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for frame")
	}
}

// TestFrameBufferWaitFrameTimeout tests WaitFrame returns nil on timeout
func TestFrameBufferWaitFrameTimeout(t *testing.T) {
	stats := NewStreamStats()
	fb := NewFrameBuffer(stats, 0)

	done := make(chan []byte)
	go func() {
		frame := fb.WaitFrame(100 * time.Millisecond)
		done <- frame
	}()

	select {
	case frame := <-done:
		if frame != nil {
			t.Errorf("WaitFrame got %v on timeout, want nil", frame)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout in test")
	}
}

