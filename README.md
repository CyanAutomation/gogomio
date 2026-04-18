# Motion In Ocean - Go Edition 🌊

A Raspberry Pi CSI camera MJPEG streaming server written in Go. This is a high-performance, production-ready implementation of the Motion In Ocean project, focusing on the **client camera streaming mode**.

## Features

**v0.1.0 (Current - MVP)**

- ✅ MJPEG streaming (`/stream.mjpg`)
- ✅ Snapshot capture (`/snapshot.jpg`)
- ✅ REST API endpoints (`/api/config`, `/api/status`, `/health`, `/ready`)
- ✅ Mock camera mode for development (no Pi hardware needed)
- ✅ Configuration via environment variables
- ✅ Connection limiting (max concurrent streams)
- ✅ Thread-safe frame buffering with condition variables
- ✅ Real-time FPS calculation
- ✅ Docker support (multi-arch: arm64/amd64)
- ✅ 51+ unit and integration tests (TDD)

**Future (v0.2+)**

- Prometheus metrics endpoint
- Rate limiting per endpoint
- Web UI streaming viewer
- Advanced settings persistence  
- Performance profiling for different Pi models

## Quick Start

### Development (Mock Camera - No Hardware)

```bash
# Run in Docker (easiest)
docker-compose -f docker-compose.mock.yml up

# Or run locally with Go
MOCK_CAMERA=true go run ./cmd/gogomio

# Test endpoints
curl http://localhost:8000/snapshot.jpg -o frame.jpg
curl http://localhost:8000/api/config | jq
```

### Raspberry Pi (Real Camera - arm64 Optimized Build)

**Prerequisites:**

- Raspberry Pi 3B+, 4, or 5 (arm64)
- CSI camera connected and enabled
- Docker installed (`curl -sSL https://get.docker.com | sh`)

**Important:** The Dockerfile has been optimized for **Raspberry Pi arm64 deployments** with native libcamera support. See [Raspberry Pi Build Guide](docs/RASPBERRY_PI_BUILD.md) for:

- Build instructions
- Camera initialization troubleshooting  
- Performance optimization
- Cross-compilation from x86/amd64
- 64-bit Raspberry Pi OS (arm64) recommended so camera packages are available
- Camera backend package on host:
  - Preferred: `rpicam-apps` (`rpicam-vid`)
  - Fallback: `libcamera-tools` / `libcamera-apps-lite` (`libcamera-vid`)

**Deployment:**

```bash
# Clone the repository
git clone https://github.com/CyanAutomation/gogomio.git
cd gogomio

# Configure for your Pi (optional)
export MIO_RESOLUTION=1280x720
export MIO_FPS=24
export MIO_JPEG_QUALITY=90

# Run in Docker
docker-compose up -d

# View logs
docker-compose logs -f gogomio

# Optional: confirm backend binaries discovered at startup
docker-compose logs gogomio | grep camera-check

# Access streams
# - Streaming: http://YOUR_PI_IP:8000/stream.mjpg
# - Snapshot: http://YOUR_PI_IP:8000/snapshot.jpg
# - Status: http://YOUR_PI_IP:8000/api/config
```

## Architecture

```
┌─────────────────────────────────┐
│   Camera (Real or Mock)         │
│  - Picamera2 (future: real Pi)  │
│  - Synthetic generator (mock)   │
└───────────┬─────────────────────┘
            │ CaptureFrame()
            ▼
┌─────────────────────────────────┐
│   FrameBuffer (Thread-Safe)     │
│  - Condition variable signaling │
│  - FPS throttling               │
│  - Current frame storage        │
└───────────┬─────────────────────┘
            │ GetFrame()
            ▼
┌─────────────────────────────────┐
│   HTTP API Layer (Chi Router)   │
├─────────────────────────────────┤
│ /stream.mjpg    → MJPEG stream  │
│ /snapshot.jpg   → JPEG frame    │
│ /api/config     → Settings JSON │
│ /api/status     → Health JSON   │
│ /health         → Liveness      │
│ /ready          → Readiness     │
└─────────────────────────────────┘
```

## Configuration

Environment variables (with defaults):

```bash
# Resolution (default: 640x480)
export MIO_RESOLUTION=1280x720

# Framerate (default: 24)
export MIO_FPS=24

# Target FPS for encoding (default: same as FPS)
export MIO_TARGET_FPS=24

# JPEG quality 1-100 (default: 90)
export MIO_JPEG_QUALITY=90

# Max concurrent stream connections (default: 10)
export MIO_MAX_STREAM_CONNECTIONS=5

# Server port (default: 8000)
export MIO_PORT=8000

# Server bind address (default: 0.0.0.0)
export MIO_BIND_HOST=0.0.0.0

# Mock camera for development (default: false)
export MOCK_CAMERA=false
```

### JPEG quality behavior by backend

`MIO_JPEG_QUALITY` is always configured as `1-100` at the app level, but camera backends interpret quality differently:

- **rpicam-vid / libcamera-vid (preferred on Raspberry Pi CSI):**
  - app value is passed as backend `--quality` and clamped to `1-100`,
  - scale direction is intuitive: **higher number = better quality** (usually higher CPU/bandwidth).
- **ffmpeg fallback (`video4linux2`):** app quality is intentionally remapped to FFmpeg MJPEG `-q:v` quantizer (`2-31`), where:
  - higher app quality => **lower** quantizer (better image, more CPU/bandwidth),
  - lower app quality => **higher** quantizer (smaller output, less CPU/bandwidth).

Practical ffmpeg mapping examples:

- `MIO_JPEG_QUALITY=100` → `-q:v 2` (highest quality)
- `MIO_JPEG_QUALITY=50` → `-q:v 17` (balanced)
- `MIO_JPEG_QUALITY=1` → `-q:v 31` (lowest quality)

For **low-CPU operation** on constrained devices, start around:

- `MIO_JPEG_QUALITY=35-60` for 640x480 / 720p,
- reduce FPS first (for example `15-20`) before raising quality.

Low-CPU preset example (change all three together):

```bash
export MIO_RESOLUTION=640x360
export MIO_FPS=15
export MIO_JPEG_QUALITY=45
```

## API Endpoints

### Streaming

- **GET `/stream.mjpg`** - Live MJPEG video stream
  - `Content-Type: multipart/x-mixed-replace`
  - Respects max connection limit (returns 429 if exceeded)
  - Example: `vlc http://localhost:8000/stream.mjpg`

- **GET `/snapshot.jpg`** - Current JPEG frame
  - `Content-Type: image/jpeg`
  - Returns 503 if camera not ready

### API

- **GET `/api/config`** - Server configuration and stats

  ```json
  {
    "resolution": [640, 480],
    "fps": 24,
    "target_fps": 24,
    "jpeg_quality": 90,
    "max_stream_connections": 10,
    "current_stream_connections": 2,
    "frames_captured": 12345,
    "current_fps": 23.8,
    "last_frame_age_seconds": 0.042
  }
  ```

- **GET `/api/status`** - Server health and uptime

  ```json
  {
    "status": "ok",
    "camera_ready": true,
    "uptime_seconds": 3600,
    "stream_connections": 2,
    "fps": 23.8
  }
  ```

### Health Checks

- **GET `/health`** - Liveness probe (always 200 OK if running)
- **GET `/ready`** - Readiness probe (200 if camera ready, 503 if initializing)

## Development

### Project Structure

```
gogomio/
├── cmd/gogomio/
│   └── main.go              # Application entry point
├── internal/
│   ├── api/
│   │   ├── handlers.go      # HTTP handlers
│   │   └── handlers_test.go # API tests (8 tests)
│   ├── camera/
│   │   ├── camera_interface.go
│   │   ├── frame_buffer.go  # Thread-safe frame buffer
│   │   ├── stream_stats.go  # FPS calculation & stats
│   │   ├── connection_tracker.go # Connection limiting
│   │   ├── mock_camera.go   # Synthetic frame generator
│   │   ├── frame_buffer_test.go (6 tests)
│   │   ├── stream_stats_test.go (8 tests)
│   │   ├── connection_tracker_test.go (10 tests)
│   │   └── mock_camera_test.go (10 tests)
│   └── config/
│       ├── config.go        # Configuration management
│       └── config_test.go   # Config tests (6 tests)
├── Dockerfile       # Multi-stage build
├── docker-compose.yml        # Real Pi configuration
├── docker-compose.mock.yml   # Development/testing
├── go.mod & go.sum # Dependency management
└── README.md        # This file
```

### Build & Test

```bash
# Run all tests
go test ./... -v

# Run tests with race detection
go test ./... -v -race

# Build binary
go build -o gogomio ./cmd/gogomio

# Run locally
./gogomio

# Build Docker image
docker build -t gogomio:latest .

# Test in Docker (mock mode)
docker-compose -f docker-compose.mock.yml up --build
```

### Testing

- **44+ unit tests** covering all core components
- Race condition detection enabled
- Mock camera for deterministic testing
- HTTP integration tests with Chi router
- Thread-safety tests for concurrent access

Run tests:

```bash
go test ./... -v -race -cover
```

## Performance

**Typical Performance (Raspberry Pi 4B)**

- Streaming: 24-30 FPS @ 1280x720
- Latency: ~80-120ms end-to-end
- Memory: ~50-80MB
- CPU: 15-25% @ 24 FPS, 720p

**Estimated Performance (Future - Real Camera)**

- Expected improvement: 30-50% lower latency with libcamera integration
- Better hardware acceleration for JPEG encoding

## Troubleshooting

**Camera not initializing**

- Ensure CSI cable is properly connected to Pi
- Check `/dev/video0` exists: `ls -la /dev/video0`
- In docker-compose, verify device mapping
- Check startup binary probe output: `docker-compose logs gogomio | grep camera-check`
- Prefer `rpicam-vid`; use `libcamera-vid` as fallback if `rpicam-vid` is unavailable

**High CPU usage**

- Lower `MIO_FPS` or `MIO_RESOLUTION`
- Increase `MIO_TARGET_FPS` to throttle encoder
- Check system load: `top`

**Timeout connecting to stream**

- Verify Pi is reachable: `ping YOUR_PI_IP`
- Check firewall allows port 8000
- Verify stream is running: `curl http://YOUR_PI_IP:8000/api/config`

## Integration Examples

### Home Assistant

Add to `configuration.yaml`:

```yaml
camera:
  - platform: mjpeg
    name: "Pi Camera"
    mjpeg_url: http://YOUR_PI_IP:8000/stream.mjpg
```

### OctoPrint

Set webcam stream URL: `http://YOUR_PI_IP:8000/stream.mjpg`

### VLC

```bash
vlc http://YOUR_PI_IP:8000/stream.mjpg
```

### curl for snapshots

```bash
curl http://YOUR_PI_IP:8000/snapshot.jpg -o image.jpg
```

## Dependencies

- Go 1.22+
- `chi` - HTTP router
- Runtime camera binaries (arm64 Raspberry Pi):
  - Preferred: `rpicam-vid` via `rpicam-apps`
  - Fallback: `libcamera-vid` via `libcamera-tools` / `libcamera-apps-lite`
- `ffmpeg` fallback backend is installed in Docker images for compatibility

## License

[Check LICENSE file](./LICENSE)

## Roadmap

| Phase | Features | Status |
|-------|----------|--------|
| **v0.1** | Mock camera, MJPEG stream, API, Docker | ✅ Current |
| **v0.2** | Real Pi camera (libcamera), Prometheus metrics | 🔄 Planned |
| **v0.3** | Web UI, Settings persistence | 📋 Planned |
| **v0.4** | Rate limiting, Performance profiles, Advanced config | 📋 Future |

## Contributing

Contributions welcome! See issues and project roadmap.

---

**Motion In Ocean Go** - Built with 🐹 Go, 🎥 for Raspberry Pi.
Made by [CyanAutomation](https://github.com/CyanAutomation)
Simple Raspberry PI Camera code streaming video for Docker, written in Go
