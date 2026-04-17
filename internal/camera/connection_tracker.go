package camera

import (
	"sync"
)

// ConnectionTracker tracks the number of active connections with a maximum limit.
// Thread-safe using atomic operations for reads and mutex for writes.
type ConnectionTracker struct {
	count   int64
	countMu sync.Mutex
}

// NewConnectionTracker creates a new connection tracker starting at 0.
func NewConnectionTracker() *ConnectionTracker {
	return &ConnectionTracker{
		count: 0,
	}
}

// Increment increments the connection count without limit checking.
func (ct *ConnectionTracker) Increment() {
	ct.countMu.Lock()
	ct.count++
	ct.countMu.Unlock()
}

// Decrement decrements the connection count.
// Safe to call even if count is 0 (count is clamped at 0).
func (ct *ConnectionTracker) Decrement() {
	ct.countMu.Lock()
	if ct.count > 0 {
		ct.count--
	}
	ct.countMu.Unlock()
}

// TryIncrement attempts to increment the count if below maxConnections.
// Returns true if successful, false if at or above limit.
func (ct *ConnectionTracker) TryIncrement(maxConnections int) bool {
	ct.countMu.Lock()
	if ct.count >= int64(maxConnections) {
		ct.countMu.Unlock()
		return false
	}

	// Increment the count
	ct.count++
	ct.countMu.Unlock()
	return true
}

// Count returns the current connection count.
func (ct *ConnectionTracker) Count() int {
	ct.countMu.Lock()
	defer ct.countMu.Unlock()
	return int(ct.count)
}
