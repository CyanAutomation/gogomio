---
name: camera-integration-patterns
description: Use when implementing camera backends, handling subprocess lifecycle, error recovery, format negotiation, or coordinating real/mock camera swapping. Ensures camera processes start cleanly, errors are diagnosed quickly, and mock cameras work as fallback without hardware.
---

# Camera Integration Patterns

Use this skill when the quality of the work depends on correct subprocess lifecycle management, robust error handling, graceful fallback to mock cameras, and health monitoring of camera backends.

Goal: ship camera integrations that start reliably, fail with clear errors, recover gracefully, and can be tested without hardware. Default toward: interface abstraction for real/mock swapping, early subprocess error detection, deferred cleanup registration, context propagation for cancellation, health checks before accepting client connections.

## Working Model

Camera integration in GoGoMio follows this lifecycle:

```
FrameManager.Start()
    ↓
Attempt NewRealCamera (libcamera-vid or ffmpeg subprocess)
    ↓
Subprocess spawns; stdout pipe opened
    ↓
Encoder reads from pipe, publishes JPEG to FrameBuffer
    ↓
Health monitor checks frame freshness, subprocess liveness
    ↓
On error OR if MIO_MOCK=1:
    → Kill subprocess (SIGTERM, wait)
    → Fall back to NewMockCamera (synthetic JPEG generator)
    ↓
Encoder resumes; clients see no interruption
    ↓
On Stop() or server shutdown:
    → Close stdout pipe
    → Send SIGTERM to subprocess
    → Wait for process exit (with timeout)
    → Cleanup
```

Before implementing a camera backend, answer three things:

- **When does the subprocess start?** (on FrameManager.Start() or on first client connect?)
- **What subprocess error indicates unrecoverable failure?** (syntax error in codec args vs. camera not found)
- **How long to wait for subprocess to exit after SIGTERM?** (2s? 5s?)

## Safe Defaults

### Camera Interface

All cameras implement the `Camera` interface:

```go
type Camera interface {
    Start(ctx context.Context) error
    Stop() error
    GetFrameRate() float64
    GetResolution() (width, height int)
    // io.Reader is inherited; Read returns JPEG frames
    io.Reader
}
```

**Interface benefits:**
- Real and mock cameras are swappable
- Tests run without hardware
- Fallback is transparent to callers

**Always implement both:**

```go
// RealCamera: spawns libcamera-vid or ffmpeg subprocess
type RealCamera struct {
    cmd    *exec.Cmd
    stdout io.ReadCloser
    // ...
}

// MockCamera: generates synthetic JPEG frames
type MockCamera struct {
    fps    float64
    ticker *time.Ticker
    // ...
}
```

### Subprocess Lifecycle

- **Defer cleanup immediately after `exec.Command` creation**: Register `defer cmd.Process.Kill()` before Start()
- **Check early stderr for startup errors**: Read subprocess stderr in a goroutine; if error appears within 1 second of Start(), return immediately
- **Use SIGTERM (graceful) then SIGKILL (force) on Stop**: Send SIGTERM, wait with timeout; if not dead after timeout, send SIGKILL
- **Wrap cmd.Wait() to detect error exit codes**: Exit code != 0 indicates subprocess error; log and trigger fallback

Example Real Camera startup:

```go
func (rc *RealCamera) Start(ctx context.Context) error {
    // 1. Create command
    cmd := exec.CommandContext(ctx, "libcamera-vid",
        "-t", "0", "--codec", "mjpeg",
        "-o", "-", // stdout
    )
    
    // 2. Setup pipes
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return fmt.Errorf("stdout pipe: %w", err)
    }
    
    stderr, err := cmd.StderrPipe()
    if err != nil {
        return fmt.Errorf("stderr pipe: %w", err)
    }

    // 3. Start subprocess
    if err := cmd.Start(); err != nil {
        return fmt.Errorf("start subprocess: %w", err)
    }
    
    // 4. Defer cleanup
    defer func() {
        if err != nil {
            cmd.Process.Kill()
        }
    }()

    // 5. Watch stderr for early errors (connection refused, camera not found)
    stderrDone := make(chan error, 1)
    go func() {
        scanner := bufio.NewScanner(stderr)
        for scanner.Scan() {
            line := scanner.Text()
            log.Printf("[libcamera stderr] %s", line)
            if strings.Contains(line, "error") || strings.Contains(line, "failed") {
                stderrDone <- fmt.Errorf("subprocess error: %s", line)
                return
            }
        }
        stderrDone <- scanner.Err()
    }()

    // 6. Wait for first frame or error (with timeout)
    select {
    case err := <-stderrDone:
        if err != nil && err != io.EOF {
            return err
        }
    case <-time.After(3 * time.Second):
        // Assume subprocess is healthy if no error in 3 seconds
    case <-ctx.Done():
        return ctx.Err()
    }

    // 7. Store subprocess handle for later cleanup
    rc.cmd = cmd
    rc.stdout = stdout
    return nil
}
```

### Error Detection & Fallback

**Detect subprocess errors:**
- **Broken pipe on Read**: `Read()` from stdout returns EOF early (subprocess crashed)
- **High frame latency**: If `FrameBuffer` hasn't received new frame in 10 seconds, subprocess is stuck
- **Exit code != 0**: `cmd.Wait()` returns error with exit code

**Trigger fallback:**

```go
func (fm *FrameManager) detectCameraError(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    lastFrameTime := time.Now()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            currentFrame := fm.buffer.GetLatestFrame()
            if currentFrame != nil && currentFrame.Timestamp == lastFrameTime {
                // No new frame in 10 seconds; camera is stuck
                log.Error("Camera health check failed; falling back to mock")
                fm.recoverToMockCamera()
                return
            }
            if currentFrame != nil {
                lastFrameTime = currentFrame.Timestamp
            }
        }
    }
}
```

### Graceful Fallback to Mock Camera

When RealCamera fails, switch to MockCamera without dropping connections:

```go
func (fm *FrameManager) recoverToMockCamera() error {
    // 1. Stop real camera (if running)
    if fm.camera != nil {
        fm.camera.Stop()
    }

    // 2. Create and start mock camera
    mockCam := NewMockCamera(640, 480, 30.0)
    if err := mockCam.Start(context.Background()); err != nil {
        return fmt.Errorf("mock camera failed: %w", err)
    }

    // 3. Update encoder input
    fm.camera = mockCam
    log.Warn("Switched to mock camera; real camera unavailable")

    // 4. Encoder loop resumes reading from mock; clients see no interruption
    return nil
}
```

**Key points:**
- Clients don't need to reconnect
- FrameBuffer continues publishing frames (source changed, but API unchanged)
- Encoder loop reads from `fm.camera` (interface), so swapping is transparent

### Health Monitoring

Run a background goroutine that validates camera health:

```go
func (fm *FrameManager) MonitorHealth(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if !fm.isHealthy() {
                log.Error("Camera unhealthy; initiating recovery")
                fm.recoverToMockCamera()
                return
            }
        }
    }
}

func (fm *FrameManager) isHealthy() bool {
    // Frame freshness check
    latestFrame := fm.buffer.GetLatestFrame()
    if latestFrame == nil {
        return false // No frames received
    }
    if time.Since(latestFrame.Timestamp) > 15*time.Second {
        return false // Frame is stale
    }

    // Subprocess check (for RealCamera)
    if rc, ok := fm.camera.(*RealCamera); ok {
        if rc.cmd == nil || rc.cmd.ProcessState != nil {
            if rc.cmd.ProcessState.ExitCode() != 0 {
                return false // Process exited with error
            }
        }
    }

    return true
}
```

### Lazy Camera Start

Don't start the camera subprocess until the first client connects (saves resources on Pi):

```go
type FrameManager struct {
    camera      Camera
    started     bool
    startMutex  sync.Mutex
}

func (fm *FrameManager) EnsureCameraRunning(ctx context.Context) error {
    fm.startMutex.Lock()
    defer fm.startMutex.Unlock()

    if fm.started {
        return nil // Already running
    }

    // Try real camera first
    cam := NewRealCamera(fm.config)
    if err := cam.Start(ctx); err != nil {
        log.Warnf("RealCamera failed: %v; trying mock", err)
        
        // Fallback to mock
        cam = NewMockCamera(640, 480, 30.0)
        if err := cam.Start(ctx); err != nil {
            return fmt.Errorf("both real and mock cameras failed: %w", err)
        }
    }

    fm.camera = cam
    fm.started = true
    return nil
}

// In handler:
func (fm *FrameManager) handleStream(w http.ResponseWriter, r *http.Request) {
    if err := fm.EnsureCameraRunning(r.Context()); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    // ... rest of handler
}
```

## Common Pitfalls

### Zombie Subprocess on Ungraceful Shutdown

**Symptom:** After server shutdown, `libcamera-vid` process still running; `ps aux | grep libcamera` shows orphaned process.

**Root cause:** subprocess wasn't sent SIGTERM or wasn't waited on; process becomes zombie.

**Fix:**
- Store `*exec.Cmd` or `*os.Process` handle in camera struct
- Call `Process.Signal(syscall.SIGTERM)` or use `cmd.Process.Kill()` in Stop()
- Always call `cmd.Wait()` to reap the process
- Defer cleanup before any potential error path

```go
func (rc *RealCamera) Stop() error {
    if rc.cmd == nil || rc.cmd.Process == nil {
        return nil
    }

    // Graceful SIGTERM
    rc.cmd.Process.Signal(syscall.SIGTERM)

    // Wait for exit with timeout
    done := make(chan error, 1)
    go func() {
        done <- rc.cmd.Wait()
    }()

    select {
    case <-done:
        return nil // Process exited cleanly
    case <-time.After(2 * time.Second):
        // Timeout; force kill
        rc.cmd.Process.Kill()
        <-done // Wait for actual exit
        return fmt.Errorf("had to force-kill subprocess")
    }
}
```

### Subprocess Stdout Pipe Buffering

**Symptom:** Encoder hangs after first frame; `libcamera-vid` stuck waiting for buffer drain.

**Root cause:** stdout pipe buffer is full (usually 64 KB); encoder never reads from pipe, deadlock occurs.

**Fix:**
- Read from stdout in a goroutine; don't expect all output to be available immediately
- Use `io.CopyBuffer` with reasonable buffer size (e.g., 256 KB) to drain pipe
- Always read stderr concurrently with stdout (both can fill buffers)

```go
// WRONG: reading from pipe in main goroutine blocks on buffer full
func (rc *RealCamera) Read(p []byte) (int, error) {
    return rc.stdout.Read(p) // deadlock if stdout buffer full
}

// CORRECT: pipe is read in a goroutine; main Read() returns from buffer
type RealCamera struct {
    stdoutPipe io.ReadCloser
    buffer     bytes.Buffer
}

func (rc *RealCamera) Start(ctx context.Context) error {
    // ... start subprocess ...

    // Drain pipe in goroutine
    go io.CopyBuffer(&rc.buffer, rc.stdoutPipe, make([]byte, 256*1024))
    return nil
}

func (rc *RealCamera) Read(p []byte) (int, error) {
    return rc.buffer.Read(p) // reads from in-memory buffer, never blocks
}
```

### Ignoring Early Stderr Errors

**Symptom:** Encoder waits indefinitely for first frame; subprocess has already failed due to invalid camera args.

**Root cause:** stderr is not monitored; error message is lost; subprocess exits silently.

**Fix:**
- Read stderr in a goroutine immediately after Start()
- Return error if stderr contains "error", "failed", "not found" within first few seconds
- Log all stderr output for debugging

```go
stderrScanner := bufio.NewScanner(stderr)
stderrErrors := make(chan error, 1)

go func() {
    for stderrScanner.Scan() {
        line := stderrScanner.Text()
        if strings.Contains(strings.ToLower(line), "error") {
            stderrErrors <- fmt.Errorf("subprocess error: %s", line)
            return
        }
    }
    if err := stderrScanner.Err(); err != nil {
        stderrErrors <- err
    }
}()

// Wait for startup
select {
case err := <-stderrErrors:
    return err // Error detected; fail fast
case <-time.After(2 * time.Second):
    // Assume OK if no error in 2 seconds
}
```

### Race Between Camera Stop and Encoder Read

**Symptom:** Encoder crashes with "read: bad file descriptor" when handler calls Stop().

**Root cause:** stdout pipe is closed while encoder is blocked in Read(); Read() returns error.

**Fix:**
- Call Stop() only from FrameManager cleanup, not from individual handlers
- Wrap encoder in goroutine that exits on pipe close
- Let FrameManager coordinate stop: decrement client count → if zero, stop camera

```go
// In FrameManager
func (fm *FrameManager) Stop() error {
    if fm.camera == nil {
        return nil
    }

    // Cancel encoder context first
    fm.encoderCancel()

    // Wait for encoder to exit
    time.Sleep(500 * time.Millisecond)

    // Then stop camera
    return fm.camera.Stop()
}

// In encoder loop
go func() {
    defer fm.camera.Stop()
    
    for {
        select {
        case <-fm.encoderCtx.Done():
            return // Stop encoder first
        default:
        }

        frame, err := fm.camera.Read(buf)
        if err != nil {
            // Pipe closed or camera stopped
            return
        }
        // Encode frame...
    }
}()
```

## Subprocess Checklist

Before implementing a new camera backend:

- [ ] Subprocess is started with `exec.CommandContext(ctx, ...)`
- [ ] Defer cleanup registered before Start() returns
- [ ] stdout and stderr pipes are created and read in separate goroutines
- [ ] Subprocess error (non-zero exit code) is detected and logged
- [ ] Stop() calls SIGTERM, waits, then SIGKILL with timeout
- [ ] Stop() always calls cmd.Wait() to reap process
- [ ] Mock fallback is implemented (MockCamera) and tested
- [ ] Health monitor detects stale frames (> 10 seconds old)
- [ ] Manual test: `MIO_MOCK=1 go run ./cmd/gogomio` shows mock camera
- [ ] Manual test: kill subprocess manually; verify fallback triggers
- [ ] Stress test: 100 reconnects; verify no zombie processes

## Verification via Tests

GoGoMio includes tests for real and mock cameras:

```bash
# Test real camera with mock subprocess (if available)
go test -v ./internal/camera -run TestRealCamera

# Test mock camera
go test -v ./internal/camera -run TestMockCamera

# Test camera fallback
go test -v ./internal/camera -run TestCameraFallback

# Benchmark frame encoding
go test -v ./internal/camera -bench=BenchmarkEncode -benchmem
```

## Litmus Checks

- **Can you start the server with `MIO_MOCK=1` and see generated frames?** Run `curl http://localhost:8000/snapshot.jpg` and verify a valid JPEG is returned.
- **Does the server detect a dead subprocess and fall back to mock?** Kill the camera subprocess manually (`pkill libcamera-vid`); verify logs show fallback and clients continue receiving frames.
- **Are there zombie processes after shutdown?** Run `ps aux | grep libcamera` after server stop; should be empty.
- **Does a new camera backend inherit the interface correctly?** Add a new camera type; verify it compiles and implements the Camera interface.
- **Can you swap real/mock cameras during runtime without client reconnection?** Trigger fallback while clients are connected; verify frames continue without interruption.

## Related Files

- [internal/camera/real_camera.go](../../../internal/camera/real_camera.go) — libcamera-vid/ffmpeg subprocess
- [internal/camera/mock_camera.go](../../../internal/camera/mock_camera.go) — Synthetic JPEG generator (fallback)
- [internal/camera/camera_interface.go](../../../internal/camera/camera_interface.go) — Camera interface definition
- [internal/camera/health_monitor_test.go](../../../internal/camera/health_monitor_test.go) — Health check tests
- [internal/camera/real_camera_test.go](../../../internal/camera/real_camera_test.go) — RealCamera tests
