---
name: embedded-iot-deployment
description: Use when deploying GoGoMio to Raspberry Pi or changing its resource limits, Docker image, or cross-architecture build.
---

# Embedded Deployment

Use the checked-in configuration, Dockerfile, and deployment guides as the source of truth. This skill describes current behavior; do not assume the service automatically adapts to memory or CPU pressure.

## Current configuration

These settings are loaded by [`internal/config/config.go`](../../../internal/config/config.go) and documented in [`.env.example`](../../../.env.example):

| Setting | Current behavior |
| --- | --- |
| `MIO_RESOLUTION` | Output size; defaults to `640x480`, validated up to `3840x2160`. |
| `MIO_SENSOR_MODE` | Optional sensor readout size; independent of output size. |
| `MIO_FPS` | Capture rate; defaults to `24`, validated from 1 to 120. |
| `MIO_TARGET_FPS` | Processing target; defaults to `MIO_FPS`, validated from 1 to 120. |
| `MIO_JPEG_QUALITY` | JPEG quality from 1 to 100; defaults to `90`. |
| `MIO_MAX_STREAM_CONNECTIONS` | Concurrent stream limit; defaults to `2`, with a maximum of 100. It is not calculated from available RAM. |
| `MIO_PORT` / `MIO_BIND_HOST` | Server port and bind host; defaults are `8000` and `0.0.0.0`. |
| `MOCK_CAMERA` | Set to `true` or `1` to use generated frames instead of hardware. |
| `MIO_TRUSTED_PROXY_CIDRS` | Optional comma-separated trusted proxy ranges. |

`MIO_ENABLE_PPROF=true` enables pprof on port `6060`; there is no configurable pprof port. See [`cmd/gogomio/main.go`](../../../cmd/gogomio/main.go).

## Resource behavior

- The frame buffer retains the latest immutable frame and its sequence. `Write` copies caller data; `WriteImmutable` transfers ownership, so callers must not mutate the published slice. See [`internal/camera/frame_buffer.go`](../../../internal/camera/frame_buffer.go).
- Streams share the latest-frame buffer rather than maintaining a frame queue per client. The configured stream limit bounds concurrent stream handlers; it does not adapt to device memory.
- Resolution and FPS are validated at startup. The application does not lower them automatically when RAM is low, cap FPS by CPU count, or change settings in response to memory pressure.
- Measure on the target device before proposing new resource thresholds. Keep estimates, benchmark results, and measured device limits distinct.

## Container and architecture changes

- Match the existing [`Dockerfile`](../../../Dockerfile), [multi-architecture guide](../../../docs/guides/MULTI_ARCH_BUILD.md), and [build workflow](../../../.github/workflows/build-multiarch.yml). The checked-in build script currently targets `linux/amd64` and `linux/arm64`, not ARM32v7.
- Do not copy package versions, CGO flags, camera packages, or build commands from generic examples without checking the current image and workflow.
- For camera startup problems, use the [Raspberry Pi build guide](../../../docs/guides/RASPBERRY_PI_BUILD.md) and [deployment guide](../../../docs/guides/DEPLOYMENT_GUIDE.md).

## Verification

Run the packages affected by configuration, camera, or stream resource changes:

```bash
go test ./internal/config ./internal/camera ./internal/api
go test ./internal/camera -run 'TestFrameBuffer|TestMockCamera' -race
```

Use the repository's documented build path for a target architecture. Building or publishing images can require Docker, emulation, registry credentials, or hardware; inspect the script and workflow before choosing a command.

## Related files

- [`internal/config/config.go`](../../../internal/config/config.go) — environment loading and validation
- [`internal/camera/frame_buffer.go`](../../../internal/camera/frame_buffer.go) — frame retention and ownership
- [`Dockerfile`](../../../Dockerfile) and [`scripts/build-multiarch.sh`](../../../scripts/build-multiarch.sh) — image and build targets
- [`docs/guides/DEPLOYMENT_GUIDE.md`](../../../docs/guides/DEPLOYMENT_GUIDE.md) — deployment setup
