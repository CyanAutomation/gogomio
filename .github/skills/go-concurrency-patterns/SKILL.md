---
name: go-concurrency-patterns
description: Use when writing or debugging concurrent Go code in GoGoMio: condition variables, atomic operations, graceful shutdown, avoiding send-on-closed-channel panics, or coordinating multiple goroutines. Ensures race-free shutdown, correct synchronization primitives, and clean resource cleanup.
---

# Go Concurrency Patterns

Use this skill when the quality of the work depends on correct synchronization, proper shutdown sequencing, or avoiding race conditions in concurrent code.

Goal: ship shutdown logic that never panics, synchronization patterns that prevent data races, and cleanup that is deterministic and complete. Default toward defensive patterns: early cleanup registration, atomic counters for hot paths, condition variables for fan-out, context propagation for cancellation.

## Working Model

Concurrency work in GoGoMio follows a shutdown-aware lifecycle:

1. **Startup**: Register cleanup in deferred block immediately after creation
2. **Running**: Goroutines monitor a done channel (via context.Done() or explicit close)
3. **Signal**: Receive shutdown signal (SIGTERM, HTTP server close, explicit Stop())
4. **Cleanup**: Signal all goroutines to exit; wait for completion with WaitGroup or explicit synchronization
5. **Verify**: All channels are closed, all sends are safe, no goroutines remain

Before any concurrent work, answer three things:

- **Who owns this channel?** (sender or receiver—never shared ownership)
- **When does cleanup start?** (immediately on error? after main loop? in deferred block?)
- **How many goroutines wait on this?** (affects sync.Cond vs simple channel choice)

## Safe Defaults

### Shutdown Orchestration

- **Defer cleanup registration early**: Register in deferred cleanup block immediately after starting a goroutine or spawning a resource
- **Use context.Context propagation**: Pass `ctx` or `ctx, cancel := context.WithCancel()` to all goroutine-spawning functions; cancel in cleanup
- **Close channels in reverse order of creation**: Last-created resources close first (LIFO). Receivers close last after senders have exited
- **Always sync.WaitGroup for goroutine completion**: Never assume a goroutine has exited; call `wg.Wait()` before returning from cleanup
- **Atomic operations for counters on hot paths**: Use `atomic.AddInt32()` for per-client connection count; mutexes only for state machines

### Synchronization Patterns

**For fan-out (one publisher, many subscribers):**
- Use `sync.Cond` with atomic frame swaps. See `FrameBuffer.GetFrame()` pattern: lock, read atomic value, unlock, wait for broadcast
- Never buffer per-subscriber; use immutable snapshots of shared state
- Broadcast with `cond.Broadcast()` after publishing (not `Signal()` for single client)

**For lazy resource lifecycle (start on first client, stop on last):**
- Maintain atomic counter of active clients (see `ConnectionTracker`)
- Increment before StartCapture(), decrement with deferred call after StopCapture()
- Wrap increment/decrement in mutex if accompanied by state check (e.g., "is camera running?")

**For worker pools or background tasks:**
- One done channel per goroutine or per logical group
- Use `select` with `ctx.Done()` to listen for cancellation
- Prefer `context.WithCancel()` over manual channels; it's composable

### Frame Publishing (FrameBuffer Pattern)

The FrameBuffer is the synchronization centerpiece for MJPEG streaming. Follow its design:

- **Atomic frame swap**: Store the latest frame as an atomic `*Frame` pointer; update with `atomic.StorePointer()`, read with `atomic.LoadPointer()`
- **Condition variable for broadcast**: Use `sync.Cond` to wake all waiting clients after frame is published
- **Lock only for signal/broadcast**: The lock protects the condition variable, not the frame itself (it's immutable)
- **Implement io.Writer**: This allows encoders to write directly into the buffer without intermediate buffering

Pattern:
```go
type FrameBuffer struct {
    mu        sync.Mutex
    cond      *sync.Cond
    latestFrame unsafe.Pointer // *Frame
}

func (fb *FrameBuffer) Publish(frame *Frame) {
    atomic.StorePointer(&fb.latestFrame, unsafe.Pointer(frame))
    fb.mu.Lock()
    fb.cond.Broadcast()
    fb.mu.Unlock()
}

func (fb *FrameBuffer) GetFrame(ctx context.Context) (*Frame, error) {
    fb.mu.Lock()
    defer fb.mu.Unlock()
    
    for {
        f := (*Frame)(atomic.LoadPointer(&fb.latestFrame))
        if f != nil {
            return f, nil
        }
        if err := ctx.Err(); err != nil {
            return nil, err
        }
        fb.cond.Wait() // reacquires mu after waking
    }
}
```

## Common Pitfalls

### Send on Closed Channel

**Symptom:** `panic: send on closed channel` during shutdown

**Root cause:** A goroutine sends to a channel while another goroutine closes it, or after it's already closed.

**Fix:**
- Only the **sender** closes the channel (or defer in cleanup if sender has exited)
- Ensure all senders have exited before closing
- Use `sync.WaitGroup` to synchronize sender exit: `wg.Wait()` before `close(ch)`

**Example (wrong):**
```go
go func() {
    defer wg.Done()
    ch <- data // may panic if ch is closed elsewhere
}()
close(ch) // closes while goroutine still sends
```

**Example (correct):**
```go
go func() {
    defer wg.Done()
    select {
    case ch <- data: // safe send
    case <-ctx.Done(): // or <-done; goroutine exits cleanly
        return
    }
}()
wg.Wait() // ensure sender exited
close(ch) // now safe to close
```

### Missing Cleanup on Error Paths

**Symptom:** Goroutines leak on error; resources not released.

**Root cause:** Cleanup code is conditional (only runs on happy path) or cleanup registration happens after goroutine spawn.

**Fix:**
- Register cleanup **before** spawning goroutine (in deferred block if using explicit cleanup)
- Wrap goroutine spawn in function that handles errors and ensures cleanup
- Always call `defer` immediately after resource acquisition

**Example (wrong):**
```go
if err := startCamera(); err != nil {
    return err // goroutine left running if startCamera fails
}
go func() {
    defer cameraCleanup()
    // ...
}()
```

**Example (correct):**
```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if err := startCamera(ctx); err != nil {
    return err // cancel() is deferred, cleanup is safe
}
go func() {
    defer cameraCleanup()
    <-ctx.Done()
}()
```

### Race Between Increment and Decrement During Shutdown

**Symptom:** Last client disconnect doesn't trigger StopCapture; or StopCapture runs while new clients are connecting.

**Root cause:** No synchronization between atomic counter check and the action it guards.

**Fix:**
- Pair atomic increment/decrement with a mutex-guarded state machine (e.g., `isRunning` flag)
- Atomically check counter and update state in one lock
- Use `sync.Once` for one-time setup (e.g., StartCapture should run only once)

**Example (wrong):**
```go
// Thread A: connect
atomic.AddInt32(&numClients, 1) // now 1
if atomic.LoadInt32(&numClients) == 1 {
    startCamera() // might run twice
}

// Thread B: disconnect (interleaved)
atomic.AddInt32(&numClients, -1) // now 0
if atomic.LoadInt32(&numClients) == 0 {
    stopCamera() // might run twice
}
```

**Example (correct):**
```go
var (
    numClients int32
    mu         sync.Mutex
    isRunning  bool
)

func addClient() error {
    mu.Lock()
    defer mu.Unlock()
    
    if atomic.AddInt32(&numClients, 1) == 1 && !isRunning {
        if err := startCamera(); err != nil {
            atomic.AddInt32(&numClients, -1)
            return err
        }
        isRunning = true
    }
    return nil
}

func removeClient() {
    mu.Lock()
    defer mu.Unlock()
    
    if atomic.AddInt32(&numClients, -1) == 0 && isRunning {
        stopCamera()
        isRunning = false
    }
}
```

### Goroutines Outliving Their Context

**Symptom:** Server shuts down cleanly but a stray goroutine wakes up minutes later and panics on closed channel.

**Root cause:** Goroutine is blocked on I/O (network read, file read, cond.Wait) and never sees cancellation signal; context propagation missed.

**Fix:**
- Use `context.WithTimeout()` or `context.WithCancel()` for all blocking operations
- Pass `ctx` to all functions that spawn goroutines
- Use `select` with `ctx.Done()` to multiplex blocking calls with cancellation

**Example (wrong):**
```go
go func() {
    data, _ := readFromNetwork() // blocks forever, never sees ctx.Done()
    ch <- data // panics if ch is closed during shutdown
}()
```

**Example (correct):**
```go
go func() {
    select {
    case data := <-networkChan: // if network provides context-aware channel
        ch <- data
    case <-ctx.Done():
        return // unblock gracefully
    }
}()
```

## Race Condition Checklist

Use this checklist when debugging crashes or suspected data races:

- [ ] **Shutdown panics?** Check if any goroutine sends to a closed channel. Verify all senders have exited before close.
- [ ] **Resource leaks on error?** Ensure cleanup is deferred before any non-deferred operation that can fail.
- [ ] **Stale reads?** If reading a counter or flag, is it protected by the same mutex that protects writes?
- [ ] **Double Start/Stop?** Are StartCapture/StopCapture guarded by state machine + mutex, not just atomic counter?
- [ ] **Goroutines blocking indefinitely?** Do all blocking operations (cond.Wait, readNetwork) multiplex with ctx.Done()?
- [ ] **WaitGroup sync?** Does code call wg.Wait() before assuming all goroutines have exited?

## Verification via Tests

GoGoMio includes race-detection tests. Run them to validate concurrency:

```bash
# Run all tests with race detector
go test ./... -v -race -cover

# Run concurrency-specific tests
go test -v ./internal/camera -run ".*[Rr]ace.*" -race
go test -v ./internal/camera -run "TestFrameBuffer" -race
go test -v ./internal/camera -run "TestConnectionTracker" -race
```

If a test passes without `-race` but fails with it, you've found a real race condition.

## Litmus Checks

- **Can you kill the server without panics?** Trigger SIGTERM and verify no "send on closed channel" panic. Check logs for goroutine leaks (`go-deadlock`, pprof).
- **Do all 10 concurrent MJPEG clients disconnect cleanly?** Open 10 /stream.mjpg clients, kill the server, verify all client connections close and no goroutines remain.
- **Does the last client disconnect stop the camera?** Monitor camera subprocess with `ps aux | grep libcamera-vid` before/after last client closes.
- **Does the frame buffer publish to all waiting clients?** Verify condition variable broadcast works: add debug log in `cond.Broadcast()`, open 5 clients, observe all 5 get new frames.
- **Are atomic operations used only for counters?** Grep for `atomic.*` outside `*Int32` and `*Pointer` operations; any complex state should use mutexes instead.

## Related Files

- [internal/camera/frame_buffer.go](../../../internal/camera/frame_buffer.go) — `sync.Cond` + atomic frame swap pattern
- [internal/camera/connection_tracker.go](../../../internal/camera/connection_tracker.go) — Atomic client count + state machine
- [docs/architecture/RACE_CONDITIONS_ANALYSIS.md](../../../docs/architecture/RACE_CONDITIONS_ANALYSIS.md) — Analysis of known race conditions and fixes
- [internal/camera/frame_buffer_race_test.go](../../../internal/camera/frame_buffer_race_test.go) — Race condition tests for FrameBuffer
