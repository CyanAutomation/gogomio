---
name: http-streaming-patterns
description: Use when implementing or debugging MJPEG streaming, multipart HTTP responses, frame publishing, concurrent client handling, or connection lifecycle management. Ensures frames reach all clients, boundaries are correct, and the server gracefully handles client disconnections and backpressure.
---

# HTTP Streaming Patterns

Use this skill when the quality of the work depends on correct frame delivery to concurrent clients, proper multipart encoding, connection pooling, and backpressure handling in streaming scenarios.

Goal: ship streaming responses that frame all clients correctly, never drop unintended frames, handle client disconnects gracefully, and apply rate limiting fairly. Default toward: direct frame-buffer writes via io.Writer, immutable frame snapshots before publishing, early header setup, connection cleanup in deferred blocks.

## Working Model

MJPEG streaming in GoGoMio follows this flow:

```
Client connects via GET /stream.mjpg
    ↓
Handler registers client in ConnectionTracker
    ↓
Handler writes response headers (Content-Type, Cache-Control, boundary)
    ↓
Loop: Encoder reads frame from FrameBuffer, writes multipart boundary + frame
    ↓
Client disconnect OR error
    ↓
Handler deregisters client, closes response writer
```

Before implementing a streaming endpoint, answer three things:

- **What is the frame boundary?** (e.g., `--FRAME` marker, exact byte sequence)
- **When do headers get written?** (before first frame, at startup)
- **How do we detect client disconnect?** (flush error, context cancellation, response writer closed)

## Safe Defaults

### Handler Startup & Headers

- **Write response headers before any frame data**: Set `Content-Type: multipart/x-mixed-replace; boundary=...`, `Cache-Control: no-cache`, `Connection: close`, status 200 OK
- **Use ResponseWriter.Flush() after each boundary**: Ensures client sees boundary marker immediately; critical for frame detection
- **Register client in tracker before entering frame loop**: Increment connection count in deferred `defer deregister()`; this guards lazy camera start
- **Set response writer deadline if client is slow**: Use `conn.SetDeadline()` in underlying net.Conn to detect zombie clients

Example handler skeleton:

```go
func (fm *FrameManager) handleStream(w http.ResponseWriter, r *http.Request) {
    // 1. Register client (increments counter, may start camera)
    if err := fm.tracker.IncrementClients(); err != nil {
        http.Error(w, "too many clients", http.StatusServiceUnavailable)
        return
    }
    defer fm.tracker.DecrementClients()

    // 2. Write headers early
    w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=FRAME")
    w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
    w.Header().Set("Connection", "close")
    w.WriteHeader(http.StatusOK)

    // 3. Flush headers to client
    if flusher, ok := w.(http.Flusher); ok {
        flusher.Flush()
    }

    // 4. Frame loop with cancellation
    for {
        select {
        case <-r.Context().Done():
            return // client disconnect or context cancelled
        default:
        }

        frame, err := fm.buffer.GetFrame(r.Context())
        if err != nil {
            return // context expired
        }

        // Write boundary and frame
        if _, err := fmt.Fprintf(w, "--%s\r\n", "FRAME"); err != nil {
            return // write error = client disconnected
        }
        if _, err := fmt.Fprintf(w, "Content-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(frame.Data)); err != nil {
            return
        }
        if _, err := w.Write(frame.Data); err != nil {
            return
        }
        if _, err := w.Write([]byte("\r\n")); err != nil {
            return
        }

        // Flush to ensure client sees frame immediately
        if flusher, ok := w.(http.Flusher); ok {
            flusher.Flush()
        }
    }
}
```

### Frame Buffer Integration

- **FrameBuffer implements io.Writer**: Encoder (JPEG/H264) writes directly to buffer; buffer updates immutable snapshot atomically
- **Frame is immutable once published**: Client reads the same *Frame pointer for the duration of encoding; safe concurrent access
- **Use GetFrame(ctx) with context timeout**: Client wait time is bounded by context deadline; prevents indefinite blocks
- **Broadcast, don't buffer per-client**: One FrameBuffer serves all clients. No per-client buffer means no memory leaks on slow clients

### Rate Limiting & Backpressure

- **Per-IP limit (default: 100 requests / 10 seconds on /v1/ endpoints)**: Use middleware with `r.RemoteAddr` as key
- **Connection limit (e.g., max 10 concurrent streams)**: Enforce via `ConnectionTracker.IncrementClients()` returning error if limit exceeded
- **Slow client handling**: If client doesn't read frame data, the next `w.Write()` will block (TCP window full) or timeout. **Do not buffer per-client.**
- **Close slow connections**: If handler returns after context timeout or write error, the connection closes automatically

Example rate limiter middleware:

```go
var (
    mu         sync.Mutex
    ipCounts   map[string]int
    ipLastSeen map[string]time.Time
    limit      = 100
    window     = 10 * time.Second
)

func rateLimitMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ip := strings.Split(r.RemoteAddr, ":")[0]
        
        mu.Lock()
        now := time.Now()
        if last, ok := ipLastSeen[ip]; !ok || now.Sub(last) > window {
            ipCounts[ip] = 0
        }
        ipCounts[ip]++
        ipLastSeen[ip] = now
        count := ipCounts[ip]
        mu.Unlock()

        if count > limit {
            http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

## MJPEG Encoding Rules

### Boundary Markers

- **Use consistent, unique boundary**: `--FRAME` or `--BOUNDARY_GOGOMIO` (must not appear in JPEG data)
- **Every frame starts with boundary**: `--BOUNDARY_GOGOMIO\r\n`
- **Every frame ends with CRLF**: Final frame data is followed by `\r\n` before next boundary
- **Multipart header before frame**: `Content-Type: image/jpeg\r\nContent-Length: <size>\r\n\r\n`

Full frame structure:
```
--FRAME\r\n
Content-Type: image/jpeg\r\n
Content-Length: 12345\r\n
\r\n
[12345 bytes of JPEG data]\r\n
--FRAME\r\n
[next frame...]
```

### Client-Side Parsing

Clients (browsers, ffmpeg, OpenCV) parse MJPEG by:

1. Looking for boundary marker (`--FRAME\r\n`)
2. Reading headers until blank line (`\r\n\r\n`)
3. Reading `Content-Length` bytes as frame
4. Expecting `\r\n` before next boundary

**If boundary is malformed**, client may skip frames or hang waiting for next boundary. Always validate with:

```bash
curl -s http://localhost:8000/stream.mjpg | xxd | head -100
# Should show clear boundary markers and Content-Type/Content-Length headers
```

## Connection Management

### Detecting Client Disconnect

Clients disconnect when:

1. **Browser tab closes** → TCP FIN received; next `w.Write()` returns error
2. **Network drops** → TCP timeout (OS-dependent, often 15+ minutes)
3. **Client closes connection** → Explicit close; immediate error on write

**Detect via:**
- `io.Copy(w, src)` returns error
- `w.Write()` returns non-nil error (write error, not EOF)
- `r.Context().Done()` is signaled (browser cancel, graceful shutdown)
- `Flush()` returns error

**Always check write errors:**

```go
if _, err := w.Write(frameData); err != nil {
    // Client disconnected; return cleanly
    log.Printf("Client disconnect: %v", err)
    return
}
```

### Graceful Shutdown

When server shuts down:

1. Stop accepting new connections (HTTP server closes listener)
2. Existing handlers finish current frame write or context timeout
3. `defer fm.tracker.DecrementClients()` cleans up connection count
4. Last client disconnect triggers `StopCapture()` (camera stops)
5. Server waits for all handlers to return (via http.Server.Shutdown with timeout)

**Implement in server shutdown:**

```go
// In main.go or graceful shutdown handler
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := server.Shutdown(ctx); err != nil {
    log.Fatalf("Shutdown error: %v", err)
}
// Handlers have finished; camera is stopped
```

## Common Pitfalls

### Missing Boundary Markers

**Symptom:** Client decoder shows black/corrupted frames; ffmpeg skips frames.

**Root cause:** Boundary marker is missing, malformed, or doesn't match the `Content-Type` header.

**Fix:**
- Use exact same boundary in `Content-Type: multipart/x-mixed-replace; boundary=FRAME` and in frame prefix `--FRAME\r\n`
- No whitespace variations: `--FRAME` is not `-- FRAME` or `--FRAME ` (with trailing space)
- Test with `curl http://localhost:8000/stream.mjpg | hexdump -C` to verify byte-for-byte correctness

**Example (wrong):**
```go
w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=BOUNDARY")
fmt.Fprintf(w, "--FRAME\r\n") // doesn't match declared boundary
```

**Example (correct):**
```go
boundary := "FRAME"
w.Header().Set("Content-Type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", boundary))
fmt.Fprintf(w, "--%s\r\n", boundary) // matches exactly
```

### Forgetting to Flush After Boundary

**Symptom:** Client receives data in chunks; frames arrive buffered, not in real-time.

**Root cause:** `ResponseWriter` buffers data; without `Flush()`, boundary marker stays in buffer.

**Fix:**
- Call `w.(http.Flusher).Flush()` after writing boundary marker and frame data
- Check if ResponseWriter implements Flusher before calling (may be buffered)

```go
if _, err := w.Write(frameData); err != nil {
    return
}
if flusher, ok := w.(http.Flusher); ok {
    flusher.Flush() // push data to client immediately
}
```

### Keeping Dead Connections Alive

**Symptom:** Concurrent clients count grows indefinitely; server memory leaks; old clients still showing on dashboard.

**Root cause:** Handler never exits because:
- Write error is not checked (zombie write loop)
- Context is never checked (infinite wait on FrameBuffer)
- Handler panic is swallowed (defer cleanup doesn't run)

**Fix:**
- Check `w.Write()` error; return if non-nil
- Multiplex FrameBuffer.GetFrame with `r.Context().Done()` in select
- Always defer deregister: `defer fm.tracker.DecrementClients()`

```go
for {
    select {
    case <-r.Context().Done():
        return // context cancelled (server shutdown)
    default:
    }

    frame, err := fm.buffer.GetFrame(r.Context())
    if err != nil {
        return // context deadline exceeded
    }

    if _, err := w.Write(frameData); err != nil {
        return // client disconnected
    }
}
```

### Buffer Bloat on Slow Clients

**Symptom:** Server memory grows when one client is slow; other clients are unaffected but server is OOM.

**Root cause:** Buffering frames per-client or keeping too many frames in buffer for one slow reader.

**Fix:**
- FrameBuffer stores only the latest frame (not a queue)
- Slow client's read blocks on `w.Write()`; TCP backpressure slows encoder
- If TCP send window fills, encoder blocks; no extra buffering occurs
- Never buffer per-client; broadcast pattern prevents memory leaks

**Correct pattern (FrameBuffer):**
```go
type FrameBuffer struct {
    latestFrame *Frame // atomic, one frame only
    cond        *sync.Cond
}
// All clients read from latestFrame; no per-client buffer
```

### Incorrect Content-Length or Frame Size

**Symptom:** Client reads partial frame data; next frame starts mid-image.

**Root cause:** `Content-Length` header doesn't match actual frame size; boundary marker appears in middle of JPEG.

**Fix:**
- Verify JPEG encoding produces exact size before writing `Content-Length`
- Use `len(jpegBuffer.Bytes())` as truth source
- Test with `curl` and hexdump to verify exact byte count

```go
jpegBuffer := &bytes.Buffer{}
encoder := jpeg.NewEncoder(jpegBuffer, config)
encoder.Encode(image)
frameData := jpegBuffer.Bytes()

// Write with exact size
fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(frameData))
w.Write(frameData)
```

## Streaming Endpoint Checklist

Before shipping a streaming endpoint:

- [ ] Headers written before first frame data
- [ ] Boundary marker is consistent with `Content-Type` declaration
- [ ] `Flush()` called after each frame (or frame + boundary)
- [ ] Write errors are checked and handler returns on error
- [ ] Client disconnect detected via context or write error
- [ ] `defer fm.tracker.DecrementClients()` or equivalent cleanup
- [ ] Rate limit middleware applied if `/v1/` endpoint
- [ ] Manual test: `curl http://localhost:8000/stream.mjpg | ffplay -` (should show live stream)
- [ ] Stress test: 10 concurrent clients for 1 minute; memory stable; all clients receive frames
- [ ] Shutdown test: kill server while 5 clients connected; verify no panic, all connections close

## Verification via Tests

GoGoMio includes streaming tests that validate multipart encoding and concurrent clients:

```bash
# Run streaming handler tests
go test -v ./internal/api -run TestHandleStream -race

# Benchmark frame throughput
go test -v ./internal/api -bench=BenchmarkFrameWrite -benchmem

# Stress test: concurrent clients
go test -v ./internal/camera -run TestFrameBufferConcurrent -race
```

## Litmus Checks

- **Does `curl http://localhost:8000/stream.mjpg` show raw MJPEG data with clear boundaries?** Run hexdump and verify boundary markers every few KB.
- **Do 10 concurrent clients all receive frames at the advertised FPS?** Monitor with `curl` in 10 terminals; all should print `Content-Length` at same rate.
- **Does killing the server close all client connections gracefully?** Check for connection errors, not panics.
- **Does a slow client (e.g., `curl | sleep 10`) block others?** Open fast client in parallel; fast client should not slow down.
- **Can a client reconnect immediately after disconnect?** Kill one client and reconnect; verify frame counter resumes (no frame drops for other clients).

## Related Files

- [internal/api/handlers.go](../../../internal/api/handlers.go) — `/stream.mjpg` handler implementation
- [internal/camera/frame_buffer.go](../../../internal/camera/frame_buffer.go) — Frame publishing and atomic snapshot
- [internal/camera/connection_tracker.go](../../../internal/camera/connection_tracker.go) — Client registration and connection limit
- [internal/api/handlers_test.go](../../../internal/api/handlers_test.go) — Streaming handler tests
