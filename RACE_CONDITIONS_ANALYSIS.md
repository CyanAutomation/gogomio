# Race Conditions and Concurrency Analysis - GoGoMio

## Codebase Summary

**GoGoMio** is a Raspberry Pi CSI camera streaming server written in Go. It:
- Captures MJPEG frames from a camera via a long-lived subprocess
- Serves frames via HTTP endpoints (`/stream.mjpg`, `/snapshot.jpg`, `/api/*`)
- Implements lazy capture startup/shutdown based on client connections
- Provides health monitoring and diagnostics endpoints
- Uses goroutines for frame capture, HTTP streaming, and cleanup orchestration

**Key Components:**
- `cmd/gogomio/main.go` - Entry point, HTTP server setup, signal handling
- `internal/api/handlers.go` - HTTP handler management, `FrameManager` (complex concurrent state)
- `internal/camera/frame_buffer.go` - Thread-safe latest-frame buffer
- `internal/camera/stream_stats.go` - Frame statistics with RWMutex
- `internal/camera/connection_tracker.go` - Connection count tracking
- `internal/camera/real_camera.go` - Camera process management
- `internal/settings/settings.go` - Persistent settings storage

---

## Critical Race Conditions & Bugs

### 1. **CRITICAL: Send on Closed Channel in scheduleStopCapture()** ⚠️ CRASH BUG
**File:** [internal/api/handlers.go](internal/api/handlers.go#L224)  
**Lines:** L224, L312  
**Severity:** CRITICAL - Causes panic/crash

**Description:**
When the HTTP server shuts down, a race condition can occur between:
- HTTP handlers calling `DecrementClients()` → `scheduleStopCapture()` (still processing requests)
- Main goroutine calling `Stop()` which closes `fm.cleanupCh`

**Scenario:**
```
Thread 1 (signal handler):
  1. server.Close() called (closes existing connections)
  2. defer fm.Stop() starts execution
  3. close(fm.cleanupCh) at line 312 ← CHANNEL CLOSED

Thread 2 (HTTP handler - still running):
  1. Gets <-ctx.Done() (connection closed by server.Close())
  2. defer fm.DecrementClients()
  3. clientCount becomes 0
  4. scheduleStopCapture() called
  5. Line 224: case fm.cleanupCh <- req:
     ← PANIC: send on closed channel
```

**Fix Required:**
```go
// In scheduleStopCapture(), use select with timeout to handle closed channel:
select {
case fm.cleanupCh <- req:
    // Success
default:
    // Channel may be closed or full; handle gracefully
    log.Printf("Cannot schedule stop; cleanup channel may be shutting down")
    fm.fallbackWG.Add(1)
    go fm.delayedStopFallback(req)
}
```

OR add synchronization with Stop() to ensure scheduleStopCapture() doesn't execute during shutdown.

---

### 2. **CRITICAL: Double-Close of cleanupCh on Multiple Stop() Calls** ⚠️ CRASH BUG
**File:** [internal/api/handlers.go](internal/api/handlers.go#L308-L320)  
**Lines:** L312  
**Severity:** HIGH - Causes panic if Stop() called twice

**Description:**
If `Stop()` is called multiple times (via defer or explicit calls), the second call will panic on `close(fm.cleanupCh)`.

```go
// In Stop():
close(fm.cleanupCh)  // ← Not protected by sync.Once, can panic on second close
close(fm.cleanupStop) // ← Protected by sync.Once, OK
```

**Fix Required:**
```go
var closeOnce sync.Once

func (fm *FrameManager) Stop() {
    fm.stopCapture()
    
    closeOnce.Do(func() {
        close(fm.cleanupCh)
    })
    
    fm.cleanupStopOnce.Do(func() {
        close(fm.cleanupStop)
    })
    
    <-fm.cleanupDone
    fm.fallbackWG.Wait()
}
```

---

### 3. **Race Condition: stopChancel Reference-Capture Race** ⚠️ MODERATE
**File:** [internal/api/handlers.go](internal/api/handlers.go#L115-L126)  
**Lines:** L118-120, L165-175  
**Severity:** MODERATE - Can cause stale references

**Description:**
In `scheduleStopCapture()`, the `stopChancel` is captured inside the lock (good), but the reference is used later outside the lock in cleanup goroutines.

While this is somewhat mitigated by checking `fm.doneChan != expectedDone`, there's still a window where:
- `stopChancel` is read and captured
- A new client connects, closing and recreating `stopChancel`
- The old reference is still used by the cleanup goroutine

This doesn't cause a crash (channels are closed properly), but the logic becomes harder to reason about.

**Current Code (mostly safe, but fragile):**
```go
func (fm *FrameManager) scheduleStopCapture() {
    fm.captureMu.Lock()
    stopChancel := fm.stopChancel  // ← Captured under lock
    done := fm.doneChan
    fm.captureMu.Unlock()
    
    // Later, in a goroutine:
    // case <-stopChancel: // ← Might be a different channel now
}
```

**Fix Recommendation:**
Pass channel references more explicitly or use immutable references.

---

### 4. **Race: streamDone Becomes Stale in StreamFrame()** ⚠️ MODERATE
**File:** [internal/api/handlers.go](internal/api/handlers.go#L395-L420)  
**Lines:** L395-397  
**Severity:** MODERATE - Logic/liveness issue, not crash

**Description:**
In `StreamFrame()`, `streamDone` is captured early and reused in a loop:

```go
fm.captureMu.Lock()
streamDone := fm.doneChan    // ← Single capture
fm.captureMu.Unlock()

for {
    select {
    case <-streamDone:  // ← Stale reference if capture restarted
        return
    // ...
    }
}
```

If capture is restarted (new client connects while this one is streaming, then old client disconnects), `fm.doneChan` is reassigned. However, `streamDone` still points to the old channel. This could cause issues:
- If the old channel is closed, the stream exits prematurely
- If a new capture starts while streaming continues, the stream doesn't notice

**Fix Required:**
Re-capture `fm.doneChan` periodically or redesign the streaming logic:
```go
for {
    fm.captureMu.Lock()
    streamDone := fm.doneChan  // ← Recapture each iteration
    fm.captureMu.Unlock()
    
    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-streamDone:
        return fmt.Errorf("stream stopped")
    default:
    }
    // ... rest of loop
}
```

---

### 5. **Data Race: Unsynchronized Access to captureStarted** ⚠️ MODERATE
**File:** [internal/api/handlers.go](internal/api/handlers.go#L140-L278)  
**Lines:** L140, L152, L156, L238, L273  
**Severity:** MODERATE - Most accesses are locked, but pattern is fragile

**Description:**
The `captureStarted` field is accessed in multiple places, though most are protected by `captureMu`:
- ✅ `startCapture()` - locked
- ✅ `stopCapture()` - locked
- ✅ `stopCaptureIfIdle()` - locked
- ✅ `captureLoop` defer - locked

However, the logic depends on careful state management. The pattern in `captureLoop` defer is defensive:
```go
defer func() {
    fm.captureMu.Lock()
    if fm.doneChan == done {  // ← Defensive: only update if this is still the active one
        fm.captureStarted = false
    }
    fm.captureMu.Unlock()
}()
```

This is actually good defensive programming, but it makes the codebase fragile to changes. If the comparison is removed or changed, it becomes racy.

**Recommendation:**
Document this invariant clearly. Consider renaming to be more explicit about what the field tracks:
```go
activeCaptureDone chan struct{}  // Only valid if captureStarted is true
```

---

### 6. **Potential Issue: Unprotected Access to isReady in RealCamera** ⚠️ MODERATE
**File:** [internal/camera/real_camera.go](internal/camera/real_camera.go#L83-L90)  
**Lines:** L83, L90, L188-189  
**Severity:** MODERATE - Uses atomic, but pattern could be clearer

**Description:**
The `isReady` field uses `atomic.Bool` (good), but there's a short window in `CaptureFrame()`:
```go
func (rc *RealCamera) CaptureFrame() ([]byte, error) {
    if !rc.isReady.Load() {  // ← Check
        return nil, fmt.Errorf("camera not ready")
    }
    // ... [code that uses camera] ...
    rc.frameMutex.Lock()
    // ...
}
```

Between the `isReady.Load()` check and the actual frame access, camera could become not-ready. However, this is mitigated by:
- Camera startup runs to completion before `isReady.Store(true)`
- `Stop()` sets `isStopping.Store(true)` which causes `CaptureFrame()` to return early
- The `frameMutex` protects the actual frame data

This is relatively safe but could be clearer.

---

### 7. **Race Condition: Frame Publication Race in FrameBuffer** ⚠️ LOW
**File:** [internal/camera/frame_buffer.go](internal/camera/frame_buffer.go#L40-L70)  
**Lines:** L50-65  
**Severity:** LOW - By design, but could be clearer

**Description:**
In `WriteImmutable()`, if a frame is skipped due to FPS throttling:
```go
if elapsed < fb.targetFrameIntervalNS {
    // Too soon, skip this frame.
    return size, nil  // ← notifyCh is NOT closed/recreated
}
```

Waiters on the old `notifyCh` are not notified. They must wait for a timeout or cancellation. This is by design (throttling), but it could silently cause streams to stall.

**Current Behavior:** INTENTIONAL
- Frames published too fast are dropped
- Waiting streams see stale sequence and continue waiting
- Next frame (if published after interval) will notify all waiters

**Recommendation:**
Document this behavior in comments.

---

### 8. **Race Condition: StreamStats FPS Caching** ⚠️ LOW
**File:** [internal/camera/stream_stats.go](internal/camera/stream_stats.go#L30-L65)  
**Lines:** L38-49  
**Severity:** LOW - Race exists but unlikely and non-critical

**Description:**
In `Snapshot()`, the FPS cache is marked stale after recording a frame:
```go
func (s *StreamStats) RecordFrame(timestamp int64) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    s.frameCount++
    s.lastFrameTimestamp = &timestamp
    s.frameTimestamps.Value = timestamp
    s.frameTimestamps = s.frameTimestamps.Next()
    s.fpsStale = true  // ← Mark cache as stale
}
```

Multiple concurrent calls to `Snapshot()` from different HTTP handlers could cause:
- One reader calculates FPS and caches it
- Another reader sees stale=true and recalculates

This is not a crash or data corruption, just potential duplicate calculation. The `RWMutex` prevents the calculation itself from being racy.

**Current Code:** SAFE - RWMutex protects all access to shared fields

---

## Summary Table

| Issue | File | Lines | Severity | Type | Impact |
|-------|------|-------|----------|------|--------|
| Send on closed channel | handlers.go | 224, 312 | **CRITICAL** | Race | CRASH during shutdown |
| Double-close of cleanupCh | handlers.go | 312 | **CRITICAL** | Logic | CRASH on multiple Stop() |
| Stale stopChancel reference | handlers.go | 118-120, 165-175 | HIGH | Race | Timing/reference issues |
| Stale streamDone reference | handlers.go | 395-397 | HIGH | Race | Stream stops unexpectedly |
| Fragile captureStarted logic | handlers.go | 140-278 | MODERATE | Logic | Potential state corruption |
| RealCamera isReady TOCTOU | real_camera.go | 83-90, 188-189 | MODERATE | Race | Non-critical timing |
| FrameBuffer throttling silently drops notifies | frame_buffer.go | 50-65 | LOW | Design | Streams stall during throttling |
| StreamStats FPS recalculation | stream_stats.go | 38-49 | LOW | Race | Harmless duplicate work |

---

## Recommended Fixes (Priority Order)

### P0: CRITICAL - Must Fix
1. **scheduleStopCapture() closed channel panic:**
   - Use `sync.Once` to protect `close(fm.cleanupCh)` in both locations
   - OR: Add safe-send wrapper that handles closed channels

2. **Stop() double-close panic:**
   - Protect second `close(fm.cleanupCh)` with `sync.Once`

### P1: HIGH - Should Fix
3. **streamDone stale reference:**
   - Recapture `fm.doneChan` each loop iteration with lock
   
4. **stopChancel reference race:**
   - Pass complete closure references instead of field access
   - OR: Use more robust synchronization

### P2: MEDIUM - Nice to Have
5. **captureStarted fragile logic:**
   - Add clear documentation and invariant checking
   - Consider redesigning capture start/stop lifecycle

6. **Frame throttling silent drops:**
   - Document behavior or notify waiters on skip

---

## Testing Recommendations

1. **Run with Go race detector:**
   ```bash
   go test -race ./...
   go run -race cmd/gogomio/main.go
   ```

2. **Stress test shutdown:**
   - Send SIGINT while streaming from multiple clients
   - Verify no panics during cleanup

3. **Concurrent client churn:**
   - Connect/disconnect clients rapidly
   - Monitor for deadlocks and stale references

4. **Capture restart scenarios:**
   - Verify streams continue correctly when capture restarts
   - Test with multiple streams during restart
