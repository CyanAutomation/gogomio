package camera

import (
	"sync/atomic"
	"time"
)

// ConnectionTracker tracks the number of active connections with a maximum limit.
// Thread-safe using atomic operations.
type ConnectionTracker struct {
	count int64
}

// NewConnectionTracker creates a new connection tracker starting at 0.
func NewConnectionTracker() *ConnectionTracker {
	return &ConnectionTracker{
		count: 0,
	}
}

// Increment increments the connection count without limit checking.
func (ct *ConnectionTracker) Increment() {
	atomic.AddInt64(&ct.count, 1)
}

// Decrement decrements the connection count.
// Safe to call even if count is 0 (will go negative, but caller should track state).
func (ct *ConnectionTracker) Decrement() {
	atomic.AddInt64(&ct.count, -1)
}

// TryIncrement attempts to increment the count if below maxConnections.
// Returns true if successful, false if at or above limit.
// Uses exponential backoff to reduce CPU spinning under high contention.
func (ct *ConnectionTracker) TryIncrement(maxConnections int) bool {
	backoff := time.Microsecond
	maxBackoff := 100 * time.Microsecond

	for attempts := 0; attempts < 10; attempts++ {
		current := atomic.LoadInt64(&ct.count)
		if current >= int64(maxConnections) {
			return false
		}

		// Try to atomically increment
		if atomic.CompareAndSwapInt64(&ct.count, current, current+1) {
			return true
		}

		// Backoff before retry
		if attempts > 0 {
			time.Sleep(backoff)
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	// Failed after retries, count likely at limit
	return false
}

// Count returns the current connection count.
func (ct *ConnectionTracker) Count() int {
	return int(atomic.LoadInt64(&ct.count))
}
