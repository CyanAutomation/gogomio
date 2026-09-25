---
name: http-streaming-patterns
description: Use when changing GoGoMio MJPEG endpoints, multipart framing, stream limits, client cancellation, or frame delivery under backpressure.
---

# HTTP Streaming

Use the existing handler and frame-buffer contracts as the source of truth. The current stream implementation is [`FrameManager.StreamFrame`](../../../internal/api/handlers.go).

## Current stream contract

- Stream routes register through `StreamFrame`, which enforces `MaxStreamConnections`, ties the handler to the current stream session, and updates capture-client lifecycle counts.
- The handler sends `Content-Type: multipart/x-mixed-replace; boundary=frame`, disables intermediary buffering, and requires `http.Flusher` support.
- It waits for a frame sequence newer than that client's last sequence with `WaitFrameWithContext`. A timeout retries; request cancellation or manager shutdown ends the handler.
- Each part uses `--frame\r\n`, `Content-Type: image/jpeg`, the exact `Content-Length`, a blank line, the JPEG bytes, and trailing CRLF. `writeMultipartFrame` owns this framing.
- The handler checks write errors and flushes after each complete frame. `http.Flusher.Flush()` has no error return; detect failures through request cancellation and writes.
- The rate limiter is applied to all routes by `RegisterHandlers` and supports trusted proxy configuration. Reuse it instead of adding a second limiter or parsing `RemoteAddr` with `strings.Split`.

## Backpressure and shutdown

- A slow client's blocked write occupies that client's handler. Frame capture and other clients use separate work; do not add a per-client frame queue without measuring its memory and delivery tradeoffs.
- Stream handlers wait on the latest-frame sequence; slow clients may skip intermediate frames by design. They do not receive a durable frame history.
- The server intentionally has no write timeout because streams are long-lived. Do not add a server-wide write deadline without checking the stream behavior.
- Keep stream registration and deregistration balanced on every exit path. Preserve context cancellation, manager shutdown signaling, and connection-limit behavior.

## Verification

Run the streaming endpoint, framing, and connection-limit tests:

```bash
go test ./internal/api -run 'Test(StreamEndpoint|MJPEGStreamingEndpoint|StreamingConnectionLimit|StreamFrame|E2E_StreamEndpoint)' -race
go test ./internal/api -run '^$' -bench 'Benchmark(WriteMultipartFrame|StreamFixedFrames)' -benchmem
```

When changing frame bytes or multipart headers, update the focused handler tests and verify the declared boundary and part length against the written bytes. The benchmark command selects benchmark functions that exist in [`internal/api/handlers_benchmark_test.go`](../../../internal/api/handlers_benchmark_test.go).

## Related files

- [`internal/api/handlers.go`](../../../internal/api/handlers.go) — stream route registration, lifecycle, and multipart writer
- [`internal/camera/frame_buffer.go`](../../../internal/camera/frame_buffer.go) — latest-frame sequence and wait semantics
- [`internal/camera/connection_tracker.go`](../../../internal/camera/connection_tracker.go) — concurrent stream limit
- [`internal/api/handlers_test.go`](../../../internal/api/handlers_test.go) and [`internal/api/handlers_e2e_test.go`](../../../internal/api/handlers_e2e_test.go) — stream and cancellation tests
