package camera

import (
	"flag"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func raceScaledDuration(base time.Duration) time.Duration {
	if isRaceMode() {
		return base * 3
	}
	return base
}

func isRaceMode() bool {
	raceFlag := flag.Lookup("test.race")
	return raceFlag != nil && raceFlag.Value.String() == "true"
}

// TestConnectionTrackerConcurrentIncrementDecrementPreservesCount checks that
// concurrent increments are all visible before any matching decrement begins.
// Contract: TC-CONN-01 (docs/testing/test-contracts.md).
func TestConnectionTrackerConcurrentIncrementDecrementPreservesCount(t *testing.T) {
	ct := NewConnectionTracker()
	const clients = 50
	start := make(chan struct{})
	decrement := make(chan struct{})
	var ready sync.WaitGroup
	var incremented sync.WaitGroup
	var wg sync.WaitGroup
	var decrementOnce sync.Once
	releaseDecrements := func() {
		decrementOnce.Do(func() { close(decrement) })
	}
	t.Cleanup(func() {
		releaseDecrements()
		wg.Wait()
	})
	ready.Add(clients)
	incremented.Add(clients)
	wg.Add(clients)
	for i := 0; i < clients; i++ {
		go func() {
			defer wg.Done()
			ready.Done()
			<-start
			ct.Increment()
			incremented.Done()
			<-decrement
			ct.Decrement()
		}()
	}

	ready.Wait()
	close(start)
	incremented.Wait()
	if got := ct.Count(); got != clients {
		t.Fatalf("count while all clients are connected = %d, want %d", got, clients)
	}

	releaseDecrements()
	wg.Wait()
	if got := ct.Count(); got != 0 {
		t.Fatalf("count after all clients disconnect = %d, want 0", got)
	}
}

// TestConnectionTrackerCountConsistency verifies Count() is always valid
func TestConnectionTrackerCountConsistency(t *testing.T) {
	ct := NewConnectionTracker()

	done := make(chan struct{})
	var wg sync.WaitGroup
	negativeCount := int32(0)

	// 20 concurrent threads incrementing and decrementing
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				ct.Increment()
				ct.Decrement()

				// Occasional extra decrements (should be safe)
				ct.Decrement()
				ct.Decrement()

				// Verify count is never negative
				count := ct.Count()
				if count < 0 {
					atomic.AddInt32(&negativeCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()
	close(done)

	if negativeCount > 0 {
		t.Errorf("connection count went negative %d times", negativeCount)
	}

	finalCount := ct.Count()
	if finalCount < 0 {
		t.Errorf("final connection count is negative: %d", finalCount)
	}
}
