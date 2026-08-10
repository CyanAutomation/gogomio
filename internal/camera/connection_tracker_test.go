package camera

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestConnectionTrackerIncrement tests the tracker lifecycle.
func TestConnectionTrackerIncrement(t *testing.T) {
	tracker := NewConnectionTracker()

	if tracker.Count() != 0 {
		t.Errorf("initial count is %d, want 0", tracker.Count())
	}

	tracker.Increment()
	if tracker.Count() != 1 {
		t.Errorf("count is %d, want 1", tracker.Count())
	}

	tracker.Increment()
	if tracker.Count() != 2 {
		t.Errorf("count is %d, want 2", tracker.Count())
	}

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

func TestConnectionTrackerTryIncrementLimits(t *testing.T) {
	tests := []struct {
		name string
		max  int
	}{
		{name: "zero", max: 0},
		{name: "one", max: 1},
		{name: "configured maximum", max: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewConnectionTracker()

			for admitted := 0; admitted < tt.max; admitted++ {
				if !tracker.TryIncrement(tt.max) {
					t.Fatalf("TryIncrement rejected connection %d below limit %d", admitted+1, tt.max)
				}
			}
			if tracker.TryIncrement(tt.max) {
				t.Errorf("TryIncrement admitted a connection at limit %d", tt.max)
			}
			if got := tracker.Count(); got != tt.max {
				t.Errorf("final count is %d, want %d", got, tt.max)
			}
		})
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

// TestConnectionTrackerConcurrentOperations verifies the connection-limit requirement:
// concurrent callers must never admit more than maxConnections clients.
func TestConnectionTrackerConcurrentOperations(t *testing.T) {
	tracker := NewConnectionTracker()
	maxConnections := 50
	numGoroutines := 100

	var ready sync.WaitGroup
	var workers sync.WaitGroup
	var succeeded atomic.Int64
	var failed atomic.Int64
	start := make(chan struct{})
	ready.Add(numGoroutines)
	workers.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer workers.Done()
			ready.Done()
			<-start

			if tracker.TryIncrement(maxConnections) {
				succeeded.Add(1)
			} else {
				failed.Add(1)
			}
		}()
	}

	ready.Wait()
	close(start)
	workers.Wait()

	if got := succeeded.Load(); got != int64(maxConnections) {
		t.Errorf("successful TryIncrement calls = %d, want %d", got, maxConnections)
	}
	if got, want := failed.Load(), int64(numGoroutines-maxConnections); got != want {
		t.Errorf("failed TryIncrement calls = %d, want %d", got, want)
	}
	if got := tracker.Count(); got != maxConnections {
		t.Errorf("count is %d, want %d", got, maxConnections)
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
