---
name: go-concurrency-patterns
description: Use when changing or debugging GoGoMio goroutine lifecycles, shared frame state, channel ownership, counters, or shutdown races.
---

# Go Concurrency Patterns

Start from the concrete synchronization already used by the affected component. GoGoMio uses mutexes, atomics, notification channels, completion channels, and wait groups in different places; none is a universal default.

## Frame publication

[`FrameBuffer`](../../../internal/camera/frame_buffer.go) stores the latest frame and sequence under a mutex. Publishing closes the current notification channel and replaces it so all current waiters wake. Waiters must recheck the sequence under the mutex after waking.

- `Write` copies its input. `WriteImmutable` transfers ownership; the caller must never mutate the slice afterward.
- `WaitFrameWithContext` returns a newer frame sequence or stops on context cancellation or timeout. Returned frame bytes are shared and read-only.
- Do not replace this with a `sync.Cond` or atomic pointer without accounting for sequence checks, lost wakeups, timeout, cancellation, and the existing tests.

## Lifecycle and ownership

- Give each channel a clear owner. The sender-side owner, or a coordinator after all senders exit, closes it; receivers do not close it. Do not use a universal “close in reverse creation order” rule.
- Register cleanup immediately after acquiring a resource. Ensure cancellation actually interrupts the operation being waited on; a context alone does not unblock arbitrary I/O or `sync.Cond.Wait`.
- Use a `WaitGroup`, completion channel, or another explicit join mechanism when shutdown must know that work has ended. Choose based on the existing lifecycle; do not add a `WaitGroup` mechanically.
- Protect related state transitions with the same lock. A separate atomic counter does not make a check-and-act sequence atomic.
- A slow HTTP client blocks its own response write. Keep frame capture and per-client writes separate so one handler cannot stall publication for every stream.

## GoGoMio-specific synchronization

- [`ConnectionTracker`](../../../internal/camera/connection_tracker.go) protects its connection count with a mutex. It is not an atomic counter.
- [`FrameManager`](../../../internal/api/handlers.go) uses atomics for selected counters and a mutex/state machine for capture lifecycle transitions. Preserve those distinct responsibilities.
- `RealCamera` uses lifecycle states, generation IDs, cancellation, and completion channels to isolate start/stop/restart cycles. Review [`internal/camera/real_camera.go`](../../../internal/camera/real_camera.go) and its regression tests before changing the sequence.

## Verification

Use the race detector on the components involved:

```bash
go test ./internal/camera ./internal/api -run 'Test(FrameBuffer|ConnectionTracker|FrameManager|StreamFrame)' -race
go test -race ./internal/camera ./internal/api
```

When a change affects subprocess lifecycle, also run the camera tests described in [`camera-integration-patterns`](../camera-integration-patterns/SKILL.md). Prefer a focused regression test that exercises the race or shutdown ordering you changed.

## Related files

- [`internal/camera/frame_buffer.go`](../../../internal/camera/frame_buffer.go) — frame snapshots, notification, timeout, and cancellation
- [`internal/camera/connection_tracker.go`](../../../internal/camera/connection_tracker.go) — mutex-protected stream count
- [`internal/api/handlers.go`](../../../internal/api/handlers.go) — capture lifecycle and stream handlers
- [`internal/camera/frame_buffer_race_test.go`](../../../internal/camera/frame_buffer_race_test.go) — frame-buffer race regressions
- [`internal/camera/connection_tracker_race_test.go`](../../../internal/camera/connection_tracker_race_test.go) — connection-count race regressions
- [`docs/architecture/RACE_CONDITIONS_ANALYSIS.md`](../../../docs/architecture/RACE_CONDITIONS_ANALYSIS.md) — documented lifecycle history
