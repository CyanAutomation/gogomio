package camera

import (
	"sync/atomic"
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
func (ct *ConnectionTracker) TryIncrement(maxConnections int) bool {
	for {
		current := atomic.LoadInt64(&ct.count)
		if current >= int64(maxConnections) {
			return false
		}

		// Try to atomically increment
		if atomic.CompareAndSwapInt64(&ct.count, current, current+1) {
			return true
		}
		// Retry if CompareAndSwap failed (another goroutine changed count)
	}
}

// Count returns the current connection count.
func (ct *ConnectionTracker) Count() int {
	return int(atomic.LoadInt64(&ct.count))
}
