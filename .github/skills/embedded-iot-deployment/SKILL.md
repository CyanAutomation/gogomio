---
name: embedded-iot-deployment
description: Use when optimizing for Raspberry Pi hardware constraints, cross-architecture Docker builds, resource management, graceful degradation under memory pressure, or tuning frame rates and resolutions. Ensures the server works on resource-constrained ARM devices and scales down gracefully without crashing.
---

# Embedded IoT Deployment Patterns

Use this skill when the quality of the work depends on efficient resource usage, adaptation to hardware constraints, cross-architecture compatibility, and graceful degradation under memory pressure.

Goal: ship deployments that work on Raspberry Pi with 1-2 GB RAM, scale frame resolution and rate based on available resources, and never OOM when running multiple streams. Default toward: environment-driven configuration, lazy resource allocation, connection limits tied to memory, multi-stage Docker builds for ARM64/ARM32v7, and explicit frame buffer sizing.

## Working Model

Embedded deployment in GoGoMio follows a constraint-aware startup:

```
Server starts with MIO_RESOLUTION env var
    ↓
Detect available RAM (via /proc/meminfo on Linux)
    ↓
Validate camera resolution:
    - If HD (1920×1080) and RAM < 512 MB → downgrade to VGA (640×480)
    - If HD and RAM < 256 MB → downgrade to QVGA (320×240)
    ↓
Set frame buffer size = resolution + 10% overhead
    ↓
Limit concurrent clients = RAM / (buffer_size * 2)
    ↓
Start encoder with frame rate = min(MIO_FPS, capability(camera, resolution))
    ↓
Monitor GC and OOM conditions
    ↓
On memory pressure:
    → Drop frame rate by 50%
    → Reduce resolution by 25%
    → Reject new clients with 503 Unavailable
```

Before deploying to Pi, answer three things:

- **What resolution can 256 MB RAM support?** (test with `MIO_MOCK=1` and monitor `top`)
- **How many concurrent clients can 512 MB RAM handle?** (stress test with `ab` or `Apache Benchmark`)
- **What's the fallback resolution chain?** (e.g., `1920x1080 → 1280x720 → 640x480 → 320x240`)

## Safe Defaults

### Configuration via Environment Variables

GoGoMio loads all settings from env vars; no hardcoded paths or config files needed.

**Key variables:**

- `MIO_RESOLUTION` = `1920x1080` | `1280x720` | `640x480` | `320x240` (default: `640x480`)
- `MIO_FPS` = integer 1–30 (default: 30)
- `MIO_TARGET_FPS` = integer, encoder target (default: same as MIO_FPS)
- `MIO_MOCK` = `1` to enable mock camera (default: `0`, use real camera)
- `MIO_HTTP_PORT` = integer (default: 8000)
- `MIO_PPROF_PORT` = integer (default: 6060)
- `MIO_CODEC` = `mjpeg` | `h264` (default: `mjpeg`)
- `MIO_BITRATE` = integer kbps (default: `2500`)
- `MIO_MAX_CLIENTS` = integer (default: auto-calculated from available RAM)

**Load and validate:**

```go
func LoadFromEnv() (*Config, error) {
    resolution := os.Getenv("MIO_RESOLUTION")
    if resolution == "" {
        resolution = "640x480"
    }

    fps, _ := strconv.Atoi(os.Getenv("MIO_FPS"))
    if fps < 1 || fps > 30 {
        fps = 30
    }

    maxClients, _ := strconv.Atoi(os.Getenv("MIO_MAX_CLIENTS"))
    if maxClients == 0 {
        maxClients = estimateMaxClientsFromRAM()
    }

    return &Config{
        Resolution: resolution,
        FPS:        fps,
        MaxClients: maxClients,
        // ...
    }, nil
}
```

### Memory-Aware Defaults

Calculate frame buffer size and client limit dynamically:

```go
func estimateMaxClientsFromRAM() int {
    // Read /proc/meminfo on Linux
    data, _ := ioutil.ReadFile("/proc/meminfo")
    // Parse MemAvailable (not MemTotal; MemAvailable accounts for caches)
    memMB := parseMemInfo(data) / 1024

    // Rule of thumb: 2 MB per client (frame buffer snapshot)
    maxClients := memMB / 2
    if maxClients < 2 {
        maxClients = 2 // Minimum: allow 2 clients
    }
    if maxClients > 10 {
        maxClients = 10 // Maximum: don't exceed 10 on single Pi
    }

    return maxClients
}

func framebufferSize(width, height int) int {
    // Estimate JPEG size = 1/4 of raw RGB
    // Raw = width * height * 3 bytes
    jpegEstimate := (width * height * 3) / 4
    // Add 10% for overhead
    return (jpegEstimate * 110) / 100
}
```

### Resolution Degradation

If hardware can't support requested resolution, degrade gracefully:

```go
func (cfg *Config) OptimizeForHardware() error {
    memMB := getAvailableRAM()
    width, height := parseResolution(cfg.Resolution)

    // Degradation chain
    resolutions := []string{
        "1920x1080", // Full HD: ~1.5 MB per frame
        "1280x720",  // HD: ~750 KB per frame
        "640x480",   // VGA: ~300 KB per frame
        "320x240",   // QVGA: ~75 KB per frame
    }

    for _, res := range resolutions {
        w, h := parseResolution(res)
        frameSize := framebufferSize(w, h)
        neededMem := frameSize * cfg.MaxClients
        neededMem += 50 * 1024 * 1024 // 50 MB buffer for runtime

        if neededMem < memMB {
            cfg.Resolution = res
            return nil
        }
    }

    // Fallback: QVGA always works
    cfg.Resolution = "320x240"
    return nil
}
```

**Log degradation:**

```go
if originalRes != cfg.Resolution {
    log.Infof("Degraded resolution from %s to %s (available RAM: %d MB)",
        originalRes, cfg.Resolution, memMB)
}
```

### Frame Rate Scaling

Adjust FPS based on available CPU and memory:

```go
func (cfg *Config) OptimizeFrameRate() {
    // Detect CPU cores
    cores := runtime.NumCPU()

    // Rule: 1 core can handle ~15 FPS HD; 4 cores can handle ~30 FPS
    maxFPS := (cores * 30) / 4
    if cfg.FPS > maxFPS {
        cfg.FPS = maxFPS
        log.Warnf("Reduced FPS from %d to %d (CPU cores: %d)", 
            cfg.TargetFPS, cfg.FPS, cores)
    }
}
```

### Connection Limits Tied to Resources

Enforce `MaxClients` in handler; reject on overload:

```go
func (fm *FrameManager) handleStream(w http.ResponseWriter, r *http.Request) {
    if fm.tracker.ClientCount() >= fm.config.MaxClients {
        http.Error(w, "Server at max capacity", http.StatusServiceUnavailable)
        return
    }

    if err := fm.tracker.IncrementClients(); err != nil {
        http.Error(w, "Too many clients", http.StatusServiceUnavailable)
        return
    }
    defer fm.tracker.DecrementClients()

    // ... stream handler ...
}
```

### Docker Multi-Stage Builds for ARM

Use multi-stage Dockerfile to build Go binary once, then run on smaller runtime image:

```dockerfile
# Stage 1: Build
FROM golang:1.21-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -o gogomio ./cmd/gogomio

# Stage 2: Runtime (smaller image)
FROM arm64v8/debian:bookworm-slim
RUN apt-get update && apt-get install -y \
    libcamera0 \
    libcamera-tools \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/gogomio /usr/local/bin/
ENTRYPOINT ["gogomio"]
```

**Build for multiple architectures:**

```bash
# In scripts/build-multiarch.sh
docker buildx build \
    --platform linux/arm64,linux/arm/v7 \
    -t myregistry/gogomio:latest \
    --push \
    .
```

## Common Pitfalls

### Unbounded Frame Buffer Allocation

**Symptom:** Server OOMs after 1000 frames; memory grows linearly with frame count.

**Root cause:** Frame buffer stores all frames instead of just the latest; no garbage collection.

**Fix:**
- FrameBuffer stores only the latest frame atomically (immutable snapshot)
- All clients read from the same latest-frame pointer; no per-client buffer
- Old frames are immediately unreachable and GC'd

**Pattern (correct):**
```go
type FrameBuffer struct {
    latestFrame unsafe.Pointer // *Frame, atomic, SINGLE FRAME ONLY
}

func (fb *FrameBuffer) Publish(frame *Frame) {
    oldFrame := atomic.SwapPointer(&fb.latestFrame, unsafe.Pointer(frame))
    // oldFrame is unreachable; GC cleans up
}
```

### Ignoring Resource Constraints During Development

**Symptom:** App works on developer's x86 laptop (16 GB RAM) but crashes on Pi (512 MB).

**Root cause:** Hardcoded limits (e.g., `maxClients = 100`); no testing on constrained hardware.

**Fix:**
- Always test with `MIO_MOCK=1` and memory limits: `docker run --memory=256m ...`
- Monitor heap size with pprof: `curl http://localhost:6060/debug/pprof/heap`
- Set `GOGC=50` (more aggressive GC) for Pi deployments

**Test on constrained Pi:**
```bash
# Simulate 256 MB RAM Pi
docker run --memory=256m \
    -e MIO_RESOLUTION=1920x1080 \
    -e MIO_MAX_CLIENTS=10 \
    gogomio:latest

# If it doesn't auto-degrade resolution, config is broken
```

### Cross-Compilation Flags Forgotten

**Symptom:** ARM binary fails with `exec format error`; build succeeded but binary is x86.

**Root cause:** Missing `GOOS=linux GOARCH=arm64` flags in `go build`.

**Fix:**
- Always set `GOOS` and `GOARCH` explicitly in Dockerfile or build scripts
- Verify with `file` command: `file gogomio | grep ARM`

```bash
# Correct
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -o gogomio ./cmd/gogomio

# Wrong (builds for local OS)
go build -o gogomio ./cmd/gogomio
```

### No Graceful Degradation on OOM

**Symptom:** Server crashes with no warning when memory runs out.

**Root cause:** No monitoring of available memory; no action on pressure.

**Fix:**
- Monitor GC stats: if GC runs > 30% of time, reduce FPS
- Read `/proc/meminfo` periodically; reject new clients if available RAM drops below threshold
- Set `GOGC=50` to force more aggressive GC

```go
func (fm *FrameManager) MonitorMemory(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    var m runtime.MemStats

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            runtime.ReadMemStats(&m)
            
            heapUsageMB := m.Alloc / 1024 / 1024
            totalMB := m.TotalAlloc / 1024 / 1024

            if heapUsageMB > totalMB/2 {
                // Heap is > 50% of total allocated; GC pressure is high
                log.Warnf("High memory usage: %d MB / %d MB", heapUsageMB, totalMB)
                
                // Reduce FPS or reject new clients
                fm.config.FPS = (fm.config.FPS * 3) / 4
                log.Infof("Reduced FPS to %d due to memory pressure", fm.config.FPS)
            }
        }
    }
}
```

### Cold Start Slow on Pi

**Symptom:** First frame takes 5+ seconds to appear after server start.

**Root cause:** libcamera initialization, JPEG encoder warmup, or subprocess startup delay.

**Fix:**
- Start camera in background; allow clients to connect while camera is initializing
- Serve mock frames during camera startup (fast JPEG generation)
- Pre-warm encoder with dummy frame before accepting streams

```go
func (fm *FrameManager) Start(ctx context.Context) error {
    // Start camera in goroutine; don't wait
    go func() {
        if err := fm.camera.Start(ctx); err != nil {
            log.Errorf("Camera start failed: %v", err)
            fm.recoverToMockCamera()
        }
    }()

    // Immediately switch to mock camera for fast startup
    fm.camera = NewMockCamera(...)
    fm.camera.Start(ctx)

    return nil
}
```

## Deployment Checklist

Before shipping to production on Pi:

- [ ] Config loads all settings from env vars (no hardcoded paths)
- [ ] Resolution degrades automatically if available RAM is low
- [ ] Frame rate adjusts based on CPU cores
- [ ] Connection limit calculated from available RAM, not hardcoded
- [ ] FrameBuffer stores only latest frame (no accumulation)
- [ ] Docker build uses multi-stage for ARM64 and ARM32v7
- [ ] Build script uses `CGO_ENABLED=1 GOOS=linux GOARCH=arm64`
- [ ] Memory monitor logs heap usage and can reduce FPS on pressure
- [ ] Test with `docker run --memory=256m` simulating Pi RAM
- [ ] Graceful degradation: server works at reduced perf, not crash
- [ ] Manual test: `MIO_RESOLUTION=1920x1080 MIO_FPS=30` on 512 MB Pi; verify auto-degradation
- [ ] Stress test: 10 concurrent clients on Pi for 5 minutes; memory stable

## Verification via Tests & Benchmarks

GoGoMio includes benchmarks for memory and CPU:

```bash
# Memory profile: see which code allocates most
go test -v ./internal/camera -bench=. -benchmem -memprofile=mem.out
go tool pprof mem.out

# GC analysis
GODEBUG=gctrace=1 go test -v ./internal/camera -run TestFrameBuffer -count=1

# Frame buffer allocation
go test -v ./internal/camera -run TestFrameBufferGC -race
```

## Litmus Checks

- **Does the server auto-degrade resolution on low-RAM Pi?** Set `MIO_RESOLUTION=1920x1080` on 256 MB Pi; verify logs show degradation to 320×240.
- **Can the server run 100 FPS with QVGA on Pi?** Profile with `go tool pprof http://localhost:6060/debug/pprof/profile`; verify CPU < 80%.
- **Does `docker run --memory=128m` still work?** Start server with hard memory limit; verify it doesn't crash (may degrade resolution/FPS).
- **Is memory stable over 1000 frames?** Monitor `/proc/[pid]/status` while streaming; heap should not grow linearly.
- **Do ARM64 and ARM32v7 binaries work on real Pi?** Cross-compile and test on actual hardware (Raspberry Pi 4 for ARM64, Pi Zero for ARM32v7).

## Related Files

- [internal/config/config.go](../../../internal/config/config.go) — Env var loading and validation
- [docs/architecture/FRAME_BUFFER_GC_ANALYSIS.md](../../../docs/architecture/FRAME_BUFFER_GC_ANALYSIS.md) — GC and memory analysis
- [docker-compose.mock.yml](../../../docker-compose.mock.yml) — Mock camera Docker setup
- [Dockerfile](../../../Dockerfile) — Multi-stage build for ARM
- [scripts/build-multiarch.sh](../../../scripts/build-multiarch.sh) — Cross-architecture build script
- [DEPLOYMENT_GUIDE.md](../../../docs/guides/DEPLOYMENT_GUIDE.md) — Full deployment walkthrough
