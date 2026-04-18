# Raspberry Pi CSI Camera Build

This document explains the Raspberry Pi-optimized Docker build for native CSI camera support.

## Overview

The Dockerfile has been updated to support **native Raspberry Pi CSI cameras** via `libcamera-vid`/`rpicam-apps`. This is an **arm64-only build** optimized for Raspberry Pi 4/5.

## Key Changes

### Base Image
- **Old:** `debian:trixie` (multi-architecture: amd64 + arm64)
- **New:** `arm64v8/debian:bookworm` (arm64 only)

### Camera Tools
- **Added:** Raspberry Pi OS apt repository (`archive.raspberrypi.org/debian`)
- **Installs:** `libcamera-apps` (provides `libcamera-vid`) or `rpicam-apps`
- **Result:** Native CSI camera support works out of the box

## Building

### For Raspberry Pi Hardware (Recommended)

```bash
# Build locally on Raspberry Pi for maximum compatibility
docker-compose build --no-cache

# Or build with explicit arm64 platform
docker buildx build --platform linux/arm64 -t gogomio:pi-latest .
```

### For Cross-Compilation from x86/amd64

If building on non-Pi hardware, use `buildx` with Raspberry Pi emulation:

```bash
# Install buildx if not available
docker buildx create --name gogomio-builder

# Build for arm64 with emulation
docker buildx build \
  --platform linux/arm64 \
  --builder gogomio-builder \
  -t cyanautomation/gogomio:pi-latest \
  --push \
  .
```

**Note:** Cross-compilation is slower due to QEMU emulation. For production builds, consider building directly on Raspberry Pi or using a dedicated ARM build server.

## What's Different

### With Native libcamera-vid
- ✅ Direct CSI camera access without intermediaries
- ✅ Optimal performance and frame rate
- ✅ Full Raspberry Pi GPU ISP (Image Signal Processor) utilization
- ✅ Logs show: `✓ Selected camera backend binary: libcamera-vid`

### With FFmpeg Fallback
- ⚠️ Limited V4L2 device compatibility
- ⚠️ May timeout waiting for frames
- ⚠️ Falls back to mock camera
- ⚠️ Logs show: `⚠️ Attempting FFmpeg fallback (limited compatibility)`

## Troubleshooting

### Camera Not Working

Check if `libcamera-vid` is installed:

```bash
docker exec gogomio which libcamera-vid
```

If not found, check build logs for repository setup issues:

```bash
docker logs gogomio | grep -i "libcamera\|rpicam\|raspberry"
```

### Device Access Issues

Verify device mappings:

```bash
docker exec gogomio ls -la /dev/video* /dev/vchiq /dev/dma_heap
```

Verify container can access host udev:

```bash
docker exec gogomio cat /run/udev/db/c189:0 2>/dev/null | head
```

### Diagnostics Modal

Access the web UI diagnostics at `http://<pi-ip>:8000` and click the **📊 Diagnostics** button to see:
- Real-time camera status
- Frame rates
- Camera backend in use
- System uptime
- Active stream connections

## Performance Notes

| Metric | libcamera-vid | FFmpeg V4L2 | Mock Mode |
|--------|---------------|------------|-----------|
| Startup | ~500ms | ~2s | <100ms |
| Frame Rate | 60+ FPS possible | Varies | 24 FPS (synthetic) |
| CPU Usage | Low (hardware ISP) | Medium | Very Low |
| Latency | <50ms | 100-500ms | <10ms |
| CSI Support | ✅ Native | ⚠️ Limited | ✅ Simulated |

## Going Back to Multi-Architecture Build

If you need both amd64 and arm64 support (for development on x86):

1. Revert Dockerfile to use `debian:trixie`
2. Accept that CSI camera won't work (FFmpeg fallback only)
3. Use application's mock camera mode for development
4. For production Pi deployments, rebuild with this Raspberry Pi-optimized Dockerfile

## Repository Details

- **Raspberry Pi Repo:** `archive.raspberrypi.org/debian`
- **Suite:** `bookworm` (current stable)
- **Components:** `main`
- **Packages:**
  - `libcamera-apps` - Preferred (includes libcamera-vid)
  - `rpicam-apps` - Alternative (newer tool)
  - `libcamera-tools` - Fallback (limited functionality)

## Environment Variables

All existing environment variables work unchanged:

- `MIO_RESOLUTION` - Video resolution (default: 640x480)
- `MIO_FPS` - Frames per second (default: 24)
- `MIO_JPEG_QUALITY` - JPEG compression (default: 90)
- `MIO_PORT` - HTTP port (default: 8000)
- `MIO_MAX_CONNECTIONS` - Max simultaneous streams (default: 2)

## Additional Resources

- [Raspberry Pi libcamera Documentation](https://www.raspberrypi.com/documentation/computers/camera_software.html)
- [libcamera-vid Usage](https://www.raspberrypi.com/documentation/computers/camera_software.html#libcamera-vid-options)
- [Docker Build Documentation](https://docs.docker.com/engine/reference/builder/)
- [BuildX Cross-Compilation](https://docs.docker.com/build/buildx/)
