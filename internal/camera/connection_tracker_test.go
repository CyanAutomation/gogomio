package camera

import (
	"sync"
	"testing"
)

// TestConnectionTrackerInitialization tests that tracker starts at zero
func TestConnectionTrackerInitialization(t *testing.T) {
	tracker := NewConnectionTracker()

	if tracker.Count() != 0 {
		t.Errorf("initial count is %d, want 0", tracker.Count())
	}
}

// TestConnectionTrackerIncrement tests incrementing connections
func TestConnectionTrackerIncrement(t *testing.T) {
	tracker := NewConnectionTracker()

	tracker.Increment()
	if tracker.Count() != 1 {
		t.Errorf("count is %d, want 1", tracker.Count())
	}

	tracker.Increment()
	if tracker.Count() != 2 {
		t.Errorf("count is %d, want 2", tracker.Count())
	}
}

// TestConnectionTrackerDecrement tests decrementing connections
func TestConnectionTrackerDecrement(t *testing.T) {
	tracker := NewConnectionTracker()

	tracker.Increment()
	tracker.Increment()
	tracker.Decrement()

	if tracker.Count() != 1 {
		t.Errorf("count is %d, want 1", tracker.Count())
	}

	tracker.Decrement()
	if tracker.Count() != 0 {
		t.Errorf("count is %d, want 0", tracker.Count())
	}
}

// TestConnectionTrackerDecrementBelowZeroClamps ensures extra decrements never make count negative.
func TestConnectionTrackerDecrementBelowZeroClamps(t *testing.T) {
	tracker := NewConnectionTracker()

	tracker.Decrement()
	tracker.Decrement()

	if tracker.Count() != 0 {
		t.Errorf("count is %d, want 0", tracker.Count())
	}

	tracker.Increment()
	tracker.Decrement()
	tracker.Decrement()

	if tracker.Count() != 0 {
		t.Errorf("count after extra decrement is %d, want 0", tracker.Count())
	}
}

// TestConnectionTrackerTryIncrementSuccess tests successful increment within limit
func TestConnectionTrackerTryIncrementSuccess(t *testing.T) {
	tracker := NewConnectionTracker()
	maxConnections := 5

	for i := 0; i < maxConnections; i++ {
		ok := tracker.TryIncrement(maxConnections)
		if !ok {
			t.Errorf("TryIncrement failed at connection %d (max=%d)", i, maxConnections)
		}
	}

	if tracker.Count() != maxConnections {
		t.Errorf("count is %d, want %d", tracker.Count(), maxConnections)
	}
}

// TestConnectionTrackerTryIncrementFailure tests that increment fails at limit
func TestConnectionTrackerTryIncrementFailure(t *testing.T) {
	tracker := NewConnectionTracker()
	maxConnections := 3

	// Fill to limit
	for i := 0; i < maxConnections; i++ {
		tracker.TryIncrement(maxConnections)
	}

	// Try to exceed limit
	ok := tracker.TryIncrement(maxConnections)
	if ok {
		t.Errorf("TryIncrement should fail at max connections, but succeeded")
	}

	if tracker.Count() != maxConnections {
		t.Errorf("count is %d, want %d", tracker.Count(), maxConnections)
	}
}

// TestConnectionTrackerTryIncrementAfterDecrement tests TryIncrement after making room
func TestConnectionTrackerTryIncrementAfterDecrement(t *testing.T) {
	tracker := NewConnectionTracker()
	maxConnections := 3

	// Fill to limit
	for i := 0; i < maxConnections; i++ {
		tracker.TryIncrement(maxConnections)
	}

	// Try to exceed (should fail)
	ok := tracker.TryIncrement(maxConnections)
	if ok {
		t.Errorf("TryIncrement should fail at max")
	}

	// Decrement to make room
	tracker.Decrement()

	// Now should succeed
	ok = tracker.TryIncrement(maxConnections)
	if !ok {
		t.Errorf("TryIncrement should succeed after decrement")
	}

	if tracker.Count() != maxConnections {
		t.Errorf("count is %d, want %d", tracker.Count(), maxConnections)
	}
}

// TestConnectionTrackerConcurrentOperations tests thread-safety
func TestConnectionTrackerConcurrentOperations(t *testing.T) {
	tracker := NewConnectionTracker()
	maxConnections := 50
	numGoroutines := 100

	var wg sync.WaitGroup

	// Launch concurrent incrementers
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Try to increment; some will succeed, some will fail
			tracker.TryIncrement(maxConnections)
		}()
	}

	wg.Wait()

	finalCount := tracker.Count()

	// Count should not exceed max
	if finalCount > maxConnections {
		t.Errorf("count is %d, exceeds max of %d", finalCount, maxConnections)
	}
}

// TestConnectionTrackerConcurrentIncrementDecrement tests increment/decrement under load
func TestConnectionTrackerConcurrentIncrementDecrement(t *testing.T) {
	tracker := NewConnectionTracker()
	maxConnections := 10
	operationsPerGoroutine := 100

	var wg sync.WaitGroup

	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < operationsPerGoroutine; i++ {
				ok := tracker.TryIncrement(maxConnections)
				if ok {
					// If increment succeeded, eventually decrement
					tracker.Decrement()
				}
			}
		}()
	}

	wg.Wait()

	// Should end at 0 (all decremented)
	if tracker.Count() != 0 {
		t.Errorf("final count is %d, want 0", tracker.Count())
	}
}

// TestConnectionTrackerMaxZero tests behavior with max connection of 0
func TestConnectionTrackerMaxZero(t *testing.T) {
	tracker := NewConnectionTracker()

	// With max=0, all TryIncrement should fail
	ok := tracker.TryIncrement(0)
	if ok {
		t.Errorf("TryIncrement with max=0 should fail")
	}

	if tracker.Count() != 0 {
		t.Errorf("count is %d, want 0", tracker.Count())
	}
}

// TestConnectionTrackerLargeNumbers tests with large connection numbers
func TestConnectionTrackerLargeNumbers(t *testing.T) {
	tracker := NewConnectionTracker()
	maxConnections := 10000

	// Increment to nearly max
	for i := 0; i < maxConnections-1; i++ {
		tracker.TryIncrement(maxConnections)
	}

	// Should have room for one more
	ok := tracker.TryIncrement(maxConnections)
	if !ok {
		t.Errorf("TryIncrement should succeed for connection %d", maxConnections)
	}

	// Now should be full
	ok = tracker.TryIncrement(maxConnections)
	if ok {
		t.Errorf("TryIncrement should fail at max")
	}
}
