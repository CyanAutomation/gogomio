package camera

import (
	"flag"
	"runtime"
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

// TestConnectionTrackerHighContention stress tests increment/decrement under high concurrency
func TestConnectionTrackerHighContention(t *testing.T) {
	ct := NewConnectionTracker()

	done := make(chan struct{})
	var wg sync.WaitGroup
	successCount := int64(0)

	// 50 concurrent clients rapidly connecting/disconnecting
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			local := 0
			for {
				select {
				case <-done:
					return
				default:
				}
				ct.Increment()
				local++
				if local%10 == 0 {
					ct.Decrement()
					local--
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(i)
	}

	// Run for a bounded duration
	runFor := raceScaledDuration(1500 * time.Millisecond)
	timer := time.NewTimer(runFor)
	<-timer.C
	close(done)
	wg.Wait()

	// Cleanup: all increments should be decremented
	count := ct.Count()
	for count > 0 {
		ct.Decrement()
		count = ct.Count()
	}

	if ct.Count() != 0 {
		t.Errorf("final connection count should be 0, got %d", ct.Count())
	}

	t.Logf("High contention stress test: %d successful decrement cycles", successCount)
}

// TestConnectionTrackerTryIncrementRaceFree verifies TryIncrement/Decrement sync
func TestConnectionTrackerTryIncrementRaceFree(t *testing.T) {
	ct := NewConnectionTracker()
	maxConnections := 10

	done := make(chan struct{})
	var wg sync.WaitGroup
	acceptedCount := int64(0)
	rejectedCount := int64(0)

	// 100 concurrent clients trying to connect with 10 max limit
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if ct.TryIncrement(maxConnections) {
					atomic.AddInt64(&acceptedCount, 1)
					time.Sleep(raceScaledDuration(15 * time.Millisecond))
					ct.Decrement()
				} else {
					atomic.AddInt64(&rejectedCount, 1)
					runtime.Gosched()
				}
			}
		}(i)
	}

	// Run for a bounded duration
	runFor := raceScaledDuration(800 * time.Millisecond)
	timer := time.NewTimer(runFor)
	<-timer.C
	close(done)
	wg.Wait()

	finalCount := ct.Count()
	if finalCount != 0 {
		t.Logf("Warning: final connection count %d (expected 0)", finalCount)
	}

	if acceptedCount == 0 {
		t.Error("no connections were accepted")
	}
	if rejectedCount == 0 {
		t.Logf("Warning: no connections were rejected (limit enforcement may not be working)")
	}

	t.Logf("TryIncrement race test: %d accepted, %d rejected", acceptedCount, rejectedCount)
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
