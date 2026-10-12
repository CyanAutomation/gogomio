# Motion In Ocean - Go Edition 🌊

[![Test](https://github.com/CyanAutomation/gogomio/actions/workflows/code-coverage-test.yml/badge.svg?branch=main)](https://github.com/CyanAutomation/gogomio/actions/workflows/code-coverage-test.yml)
[![Build Multi-Architecture Docker Image](https://github.com/CyanAutomation/gogomio/actions/workflows/build-multiarch.yml/badge.svg?branch=main)](https://github.com/CyanAutomation/gogomio/actions/workflows/build-multiarch.yml)
[![Benchmarks](https://github.com/CyanAutomation/gogomio/actions/workflows/benchmark.yml/badge.svg?branch=main)](https://github.com/CyanAutomation/gogomio/actions/workflows/benchmark.yml)

GoGoMio is a Go service that streams Raspberry Pi CSI camera video over MJPEG/HTTP. It includes a mock camera for development, an embedded web interface, and a CLI for managing a running server. It is intended for Raspberry Pi camera operators and developers integrating camera streams.

For the canonical documentation map and archive policy, see [docs/README.md](docs/README.md).

## Table of Contents

- [Features](#features)
- [Getting Started](#getting-started)
- [CLI Usage](#cli-usage)
- [API Endpoints](#api-endpoints)
- [Runtime Settings](#runtime-settings)
- [Configuration](#configuration)
- [Architecture](#architecture)
- [Development](#development)
- [Performance](#performance)
- [Troubleshooting](#troubleshooting)
- [Dependencies](#dependencies)
- [Integration Examples](#integration-examples)
- [Contributing](#contributing)
- [Security](#security)
- [Community](#community)
- [License](#license)
- [Release History](#release-history)

## Features

The current `main` branch provides:

- ✅ MJPEG streaming (`/stream.mjpg`)
- ✅ Snapshot capture (`/snapshot.jpg`)
- ✅ REST API endpoints (v1 versioned + legacy)
- ✅ Per-IP rate limiting (100 requests per 10 seconds)
- ✅ Mock camera mode for development (no Pi hardware needed)
- ✅ Raspberry Pi CSI camera capture using `rpicam-vid` or `libcamera-vid`, with an FFmpeg/V4L2 fallback where compatible
- ✅ Embedded web UI at `/`
- ✅ Prometheus-compatible metrics at `/metrics`
- ✅ File-backed runtime settings API
- ✅ Configuration via environment variables
- ✅ Connection limiting (max concurrent streams)
- ✅ Thread-safe frame buffering with condition variables
- ✅ Real-time FPS calculation and health monitoring
- ✅ Docker support (multi-arch: arm64/amd64)
- ✅ OpenAPI reference page at `/docs/` and checked-in JSON/YAML specifications

For release history and unreleased changes, see [CHANGELOG.md](CHANGELOG.md).

## Getting Started

### Quick Start (Mock Camera, Go)

Prerequisite: Go 1.27.2 or newer.

```bash
# Start the server without Raspberry Pi hardware
MOCK_CAMERA=true go run ./cmd/gogomio

# In another terminal, check readiness and capture a frame
curl http://localhost:8000/ready
curl http://localhost:8000/snapshot.jpg -o frame.jpg
```

The web interface is available at <http://localhost:8000/>. Stop the Go process with Ctrl+C.

### Quick Start (Published Mock Container)

Prerequisite: Docker Compose and access to the published GoGoMio image.

```bash
docker-compose -f docker-compose.mock.yml up
```

This Compose file runs `docker.io/cyanautomation/gogomio:latest`; it does not build the source in your checkout. The optional `./scripts/bootstrap.sh` helper prepares `.env`, checks for `docker-compose`, and prints this start command. It does **not** start the service itself.
The mock Compose configuration pins the container to `linux/amd64`; ARM hosts may need emulation. The published image supports both `amd64` and `arm64` for normal deployments.

### Development (Mock Camera - No Hardware)

```bash
# Optional: copy the deployment example for local configuration
cp .env.example .env

# Run the published mock image with Docker Compose
docker-compose -f docker-compose.mock.yml up

# Or run locally with Go
set -a && source .env && set +a
MOCK_CAMERA=true go run ./cmd/gogomio

# Test endpoints
curl http://localhost:8000/snapshot.jpg -o frame.jpg
curl http://localhost:8000/api/config | jq
```

### Raspberry Pi (Real Camera - arm64 Optimized Build)

**Prerequisites:**

- Raspberry Pi 3B+, 4, or 5 (arm64)
- CSI camera connected and enabled
- Docker Engine and Docker Compose installed using the official instructions for your Raspberry Pi OS release

**Important:** The arm64 Docker image installs `rpicam-apps` or `libcamera-apps` in the container. The Compose deployment also exposes Raspberry Pi device nodes. See the [Raspberry Pi Build Guide](docs/guides/RASPBERRY_PI_BUILD.md) for:

- Build instructions
- Camera initialization troubleshooting  
- Performance optimization
- Cross-compilation from x86/amd64
- 64-bit Raspberry Pi OS (arm64) is required for the native camera packages included in the image

**Deployment:**

```bash
# Clone the repository
git clone https://github.com/CyanAutomation/gogomio.git
cd gogomio

# Copy and edit environment settings for your Pi
cp .env.example .env
# Then update values in .env (for example: MOCK_CAMERA=false, MIO_RESOLUTION=1280x720)

# Run in Docker
docker-compose --env-file .env up -d

# View logs
docker-compose logs -f gogomio

# Optional: confirm backend binaries discovered at startup
docker-compose logs gogomio | grep camera-check

# Access streams
# - Streaming: http://YOUR_PI_IP:8000/stream.mjpg
# - Snapshot: http://YOUR_PI_IP:8000/snapshot.jpg
# - Status: http://YOUR_PI_IP:8000/api/config
```

## CLI Usage

GoGoMio supports a **two-mode binary**: serve HTTP or execute CLI commands.
Build the binary from this checkout before using the `./gogomio` examples:

```bash
go build -o gogomio ./cmd/gogomio
```

### Server Mode (Default)

```bash
./gogomio                    # Start HTTP server
./gogomio server             # Explicit server mode
MOCK_CAMERA=true ./gogomio   # Development mode
```

### CLI Mode

Query a running server with CLI commands:

```bash
# Status and diagnostics
./gogomio status                           # Show streaming status
./gogomio diagnostics                      # System info
./gogomio health check                     # Quick health check
./gogomio version                          # Version info

# Configuration
./gogomio config                           # Show all config
./gogomio config get fps                   # Get specific value
./gogomio config get resolution

# Stream management
./gogomio stream info                      # Metrics
./gogomio stream stop                      # Stop streams

# Snapshots
./gogomio snapshot save /tmp/frame.jpg     # Capture and save
./gogomio snapshot capture                 # Output to stdout
```

### Docker Compose CLI Usage

```bash
docker-compose up -d                       # Start server
docker-compose exec gogomio gogomio status # Run CLI command
docker-compose exec gogomio gogomio config get fps
docker-compose exec gogomio gogomio health check
```

For complete CLI documentation, see [CLI Guide](docs/guides/CLI_GUIDE.md).

## API Endpoints

The embedded browser UI is served at **GET `/`**. The API reference is served at `/docs/`.

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

  This example uses application config defaults; the runtime stats are illustrative. The published mock Compose file overrides `max_stream_connections` to 10.

  ```json
  {
    "resolution": [640, 480],
    "fps": 24,
    "target_fps": 24,
    "jpeg_quality": 90,
    "max_stream_connections": 2,
    "current_stream_connections": 0,
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

### API Documentation

- **GET `/docs/`** - API reference with links to the OpenAPI specifications
- **GET `/docs/index.html`** - API reference page
- **GET `/swagger.json`** - Raw OpenAPI 2.0 specification (JSON)
- **GET `/swagger.yaml`** - Raw OpenAPI 2.0 specification (YAML)

### API Rate Limiting

The per-IP rate limiter allows **100 requests per 10 seconds**. Middleware wraps every HTTP route, including the web UI, API reference, metrics, versioned API, and legacy API. A long-lived stream is counted when its HTTP request starts, not per video frame.

- **Exceeded**: HTTP 429 Too Many Requests with `Retry-After: 1`
- **Client address**: Forwarded client IP headers are used only when the immediate peer belongs to a configured trusted proxy CIDR; see `MIO_TRUSTED_PROXY_CIDRS` below.

**Example Rate Limit Exceeded Response**:

```json
{
  "code": 429,
  "message": "Rate limit exceeded",
  "details": "Too many requests from this IP address"
}
```

**Best Practices**:

1. Implement exponential backoff when you receive 429 responses
2. Avoid unnecessary polling; use `/v1/health` for quick probes instead of `/v1/health/detailed`
3. Monitor your request rate to stay within limits
4. Use the MJPEG stream for video rather than repeatedly requesting snapshots

### API Migration Guide

Legacy endpoints remain available for backward compatibility. Prefer the versioned endpoints for new clients; see the [OpenAPI reference](docs/reference/swagger.yaml) for the current API contract. The code continues to register the legacy paths; this README does not promise a removal version.

**Deprecated Endpoints → Migration Path**:

| Old Endpoint | Status | New Endpoint(s) | Migration Notes |
| --- | --- | --- | --- |
| `GET /api/config` | ⚠️ Deprecated | `GET /v1/config/camera` + `GET /v1/metrics/live` | Combines static config + live metrics; split into two endpoints for better separation of concerns |
| `GET /api/status` | ⚠️ Deprecated | `GET /v1/health/detailed` | Identical response format; same data, clearer naming |
| `GET /api/diagnostics` | ⚠️ Deprecated | `GET /v1/health/detailed` | Identical response format; same data, clearer naming |

**Migration Example**:

**Old (Deprecated)**:

```bash
# Get both config and metrics from one endpoint
curl http://localhost:8000/api/config | jq
```

**New (Recommended)**:

```bash
# Get static config
curl http://localhost:8000/v1/config/camera | jq '.resolution, .fps, .jpeg_quality'

# Get live metrics (frequently changing data)
curl http://localhost:8000/v1/metrics/live | jq '.fps_current, .stream_connections, .uptime_seconds'

# Get comprehensive health info when needed
curl http://localhost:8000/v1/health/detailed | jq
```

**Benefits of Migration**:

- **Clear Separation**: Config is static (cacheable), metrics are dynamic (request frequently)
- **Stable Configuration**: `/v1/config/camera` describes settings that normally change only after a restart; its ETag and `Cache-Control` headers support revalidation
- **Clearer API**: Each endpoint has a single responsibility
- **Versioned Contract**: New clients can use the documented v1 routes

`GET /api/config` and `GET /api/diagnostics` currently emit a `Deprecation: true` response header. `GET /api/status` is marked deprecated in code but does not currently emit that header. Use the versioned paths above for new integrations.

### Runtime Settings

The versioned settings API reads and writes arbitrary JSON key/value settings. These values are stored separately from startup configuration and do not change camera or server environment variables:

```bash
curl http://localhost:8000/v1/api/settings

curl -X PUT http://localhost:8000/v1/api/settings \
  -H 'Content-Type: application/json' \
  -d '{"settings":{"example":"value"}}'
```

The service writes settings to `/tmp/gogomio/settings.json`. The provided Raspberry Pi `docker-compose.yml` does not mount this path, so settings may be lost when that container is removed or recreated. Configure persistent storage if settings must survive container replacement.

## Architecture

```text
┌─────────────────────────────────┐
│ Camera backend                  │
│ rpicam/libcamera, FFmpeg, mock  │
└───────────┬─────────────────────┘
            │ CaptureFrame()
            ▼
┌─────────────────────────────────┐
│ Frame manager and frame buffer  │
│ capture lifecycle, frame stats  │
└───────────┬─────────────────────┘
            │ GetFrame()
            ▼
┌─────────────────────────────────┐
│ HTTP handlers (net/http)        │
├─────────────────────────────────┤
│ MJPEG, snapshots, versioned API │
│ web UI, OpenAPI, metrics        │
└─────────────────────────────────┘
           ▲
           │ HTTP requests
       GoGoMio CLI
```

## Configuration

Application environment variables and defaults:

| Variable | Default | Notes |
| --- | --- | --- |
| `MIO_RESOLUTION` | `640x480` | Encoded output size; width up to 3840 and height up to 2160 |
| `MIO_SENSOR_MODE` | Automatic | Optional native camera sensor readout mode as `WIDTHxHEIGHT`; ignored by FFmpeg/V4L2 fallback |
| `MIO_FPS` | `24` | Capture rate, valid range 1–120 |
| `MIO_TARGET_FPS` | Same as `MIO_FPS` | Frame-buffer target rate, valid range 1–120 |
| `MIO_JPEG_QUALITY` | `90` | Quality from 1–100; meaning differs by camera backend below |
| `MIO_MAX_STREAM_CONNECTIONS` | `2` | Valid range 1–100. `docker-compose.mock.yml` overrides this to 10. |
| `MIO_PORT` | `8000` | HTTP server port |
| `MIO_BIND_HOST` | `0.0.0.0` | Binds all interfaces by default; restrict network access as described in Security. |
| `MOCK_CAMERA` | `false` | Set to `true` or `1` to use the synthetic camera. |
| `MIO_TRUSTED_PROXY_CIDRS` | Unset | Optional comma-separated CIDRs for trusted reverse proxies; only trust forwarded client IP headers from these peers. |
| `MIO_ENABLE_PPROF` | Disabled | Set to `true` to start Go profiling on `:6060`; do not expose this port outside a trusted network. |

The Pi Compose file also reads deployment variables such as `GOGOMIO_PORT` and `TZ` from `.env`; see [.env.example](.env.example). `./scripts/bootstrap.sh` checks for the legacy `docker-compose` command; Docker Compose plugin users can run `docker compose` directly.

`MIO_RESOLUTION` controls only the encoded output dimensions; it does **not**
guarantee a particular sensor field of view. `MIO_SENSOR_MODE` is passed to the
native `rpicam-vid`/`libcamera-vid` backend as its sensor readout mode and can be
used to avoid an automatically selected, more-cropped mode. It has no effect on
the FFmpeg/V4L2 fallback. Available native modes depend on the camera module
(inspect them with `rpicam-hello --list-cameras` or `libcamera-hello --list-cameras`).

For the full vertical field of view, choose a 4:3 sensor mode and usually a 4:3
output such as `MIO_RESOLUTION=1280x960`. A 16:9 output such as 1280x720 commonly
crops the top and bottom; select a wide 16:9 sensor mode when that composition
and its potentially higher frame rate are desired. A 4:3 sensor mode can still
feed a 16:9 output, but the backend may crop or scale it to fit.

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

## Development

### Project Structure

| Path | Responsibility |
| --- | --- |
| `cmd/gogomio/` | Server startup and CLI entry point |
| `internal/api/` | HTTP routes, middleware, stream coordination, and API responses |
| `internal/camera/` | Real/mock camera backends, frame buffering, and stream statistics |
| `internal/cli/` | CLI commands and HTTP client |
| `internal/config/` | Environment loading, defaults, and validation |
| `internal/settings/` | File-backed runtime settings |
| `internal/web/` | Embedded HTML, JavaScript, and image assets |
| `docs/reference/` | Embedded OpenAPI specifications and API reference page |
| `docs/guides/` | Deployment, CLI, and build guides |

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

# Build this checkout and run it in mock mode
docker build -t gogomio:local .
docker run --rm --name gogomio-local -e MOCK_CAMERA=true -p 8000:8000 gogomio:local
```

### Testing

The suite covers configuration, CLI behavior, API handlers, camera/frame lifecycle, runtime settings, and the embedded web UI. CI runs race detection, coverage, and `go vet`; see the [test workflow](.github/workflows/code-coverage-test.yml).

Run tests:

```bash
go test ./... -v -race -cover
```

## Performance

The [benchmark workflow](.github/workflows/benchmark.yml) runs Go microbenchmarks on GitHub-hosted runners and records runner metadata. These measurements are not end-to-end Raspberry Pi camera benchmarks. Throughput, latency, CPU, and memory use depend on the Pi model, camera, backend, resolution, and deployment; no Pi performance figures are published here without a reproducible hardware test.

## Troubleshooting

### Camera not initializing

- Ensure CSI cable is properly connected to Pi
- Check `/dev/video0` exists: `ls -la /dev/video0`
- In docker-compose, verify device mapping
- Check startup binary probe output: `docker-compose logs gogomio | grep camera-check`
- Prefer `rpicam-vid`; use `libcamera-vid` as fallback if `rpicam-vid` is unavailable

### High CPU usage

- Lower `MIO_FPS` or `MIO_RESOLUTION`
- Increase `MIO_TARGET_FPS` to throttle encoder
- Check system load: `top`

### Timeout connecting to stream

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

- Go 1.27.2 or newer, as declared in `go.mod`
- The Go application has no third-party Go module dependencies.
- Runtime camera binaries (arm64 Raspberry Pi):
  - Preferred: `rpicam-vid` via `rpicam-apps`
  - Fallback: `libcamera-vid` via `libcamera-tools` / `libcamera-apps-lite`
- `ffmpeg` fallback backend is installed in Docker images for compatibility

## Release History

See [CHANGELOG.md](CHANGELOG.md) for released capabilities and changes on `main`.

## Security

### ⚠️ Important Security Notice

**This service has NO built-in authentication and is designed for private/trusted networks only.**

**DO NOT expose this service directly to the internet.** Deploy it behind:

- Firewall with restricted access
- VPN or private network
- Reverse proxy with authentication (nginx, Caddy, etc.)
- HTTPS-terminating proxy

### Deployment Security Checklist

- [ ] Network is private/behind firewall
- [ ] Firewall restricts access to trusted IPs only
- [ ] For remote access: Use VPN tunnel (e.g., WireGuard, OpenVPN)
- [ ] For public access: Use reverse proxy with authentication
- [ ] HTTPS enabled via reverse proxy (never HTTP publicly)
- [ ] Rate limiting understood (100 req/10sec per IP)
- [ ] Camera is on trusted device/network

The supplied Raspberry Pi `docker-compose.yml` runs the container with `privileged: true` and maps camera device nodes. Review those permissions for your host and keep the service on a trusted network. Enabling `MIO_ENABLE_PPROF=true` also opens an unauthenticated profiling listener on port 6060; restrict it to trusted local access.

### Recommended Deployment Patterns

**Local Network Only (Recommended for Raspberry Pi)**:

```bash
# Run on local network, firewall restricts access
docker-compose up -d
# Access only from trusted devices on same network
```

**Remote Access with VPN**:

```bash
# Expose only to VPN network
# Users connect via VPN to access the camera
# Example: WireGuard, OpenVPN
```

**Reverse Proxy with Auth (Advanced)**:

```bash
# Nginx/Caddy in front with authentication
# Reverse proxy example:
# - Caddy with basic auth
# - Nginx with client certificate validation
# - Traefik with OAuth2 proxy
```

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for local setup, coding conventions, validation commands, and the pull request workflow. For a small change:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/your-feature`)
3. Write tests for your changes
4. Ensure all tests pass (`go test ./... -v -race`)
5. Commit and push your changes
6. Open a pull request

For larger changes, open a [GitHub issue](https://github.com/CyanAutomation/gogomio/issues) first to discuss the approach.

## Community

Join us in building better camera streaming for Raspberry Pi!

- **Support and issue reports**: [GitHub Issues](https://github.com/CyanAutomation/gogomio/issues)
- **Main Repository**: [CyanAutomation/gogomio](https://github.com/CyanAutomation/gogomio)

Contributions and feedback are appreciated!

## License

This project is licensed under the [BSD 3-Clause License](./LICENSE). See the LICENSE file for details.

---

**Motion In Ocean Go** - Built with 🐹 Go, 🎥 for Raspberry Pi  
Made by [CyanAutomation](https://github.com/CyanAutomation)
