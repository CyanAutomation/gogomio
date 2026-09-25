---
name: camera-integration-patterns
description: Use when changing GoGoMio camera backends, capture lifecycle, subprocess handling, startup fallback, or camera health reporting.
---

# Camera Integration

Follow the existing camera contract and lifecycle. Read the implementation and its tests before changing subprocess behavior; several lifecycle races have dedicated regression tests.

## Current contract and startup

- A backend implements [`camera.Camera`](../../../internal/camera/camera_interface.go): `Start(width, height, fps, jpegQuality int) error`, `CaptureFrame`, `CaptureFrameWithContext`, `Stop`, and `IsReady`.
- `initializeCamera` starts the selected camera before the HTTP listener. `MOCK_CAMERA=true` selects `MockCamera`; a failed real-camera startup falls back to `MockCamera`. This is a startup fallback, not a runtime camera swap. See [`cmd/gogomio/main.go`](../../../cmd/gogomio/main.go).
- `RealCamera` prefers `rpicam-vid`, then `libcamera-vid`, then its FFmpeg/V4L2 path. Check the exact selection and probe rules in [`internal/camera/real_camera.go`](../../../internal/camera/real_camera.go) before changing backend order or error messages.
- The `FrameManager` capture loop starts lazily when a frame consumer connects. The real camera backend may already be initialized; do not describe lazy capture-loop startup as lazy subprocess startup.

## Subprocess lifecycle invariants

- `Start` owns setup of stdin, stdout, and stderr pipes and starts the process. The reader, stderr drainer, process waiter, and health monitor each have explicit completion signals.
- A process handle is available only after a successful `Start`. Register failure cleanup after acquisition and preserve one owner for `Wait` so the child is reaped exactly once.
- Do not infer process failure just because stderr contains words like “error” or “failed.” Treat stderr as diagnostics; use process exit, reader errors, and first-frame startup timeout as lifecycle signals.
- `Stop` cancels the current generation, closes stdin, kills the process through the process abstraction, then waits with bounded waits for process and worker completion. Preserve the generation checks and stop-timeout behavior when editing this order.
- Do not promise a SIGTERM-to-SIGKILL sequence unless implementing and testing it. The current process abstraction calls `Process.Kill`.
- Keep backend commands as argument arrays passed to `exec.Command`; do not build shell command strings from configuration.

## Frame and health behavior

- JPEG extraction and first-frame readiness happen in `RealCamera`; its public capture methods return shared frame bytes that must remain read-only.
- The application-level mock fallback runs when real camera initialization fails. Adding recovery from a camera failure after startup requires a separate lifecycle design: it must coordinate capture-loop generations, frame ownership, and concurrent `Stop`/restart operations.
- Avoid adding a second health loop without checking the existing backend monitor and frame-manager failure/restart behavior.

## Verification

Run camera lifecycle and mock tests:

```bash
go test ./internal/camera -run 'Test(RealCamera|MockCamera)' -race
go test ./cmd/gogomio -run 'TestInitializeCamera' -race
```

For changes to process startup, stop, or restart, include the relevant regression tests in [`internal/camera/real_camera_test.go`](../../../internal/camera/real_camera_test.go). For startup fallback behavior, use [`cmd/gogomio/main_test.go`](../../../cmd/gogomio/main_test.go). Test hardware-specific behavior on the target device when available; mock tests do not validate camera drivers or device permissions.

## Related files

- [`internal/camera/camera_interface.go`](../../../internal/camera/camera_interface.go) — backend contract
- [`internal/camera/real_camera.go`](../../../internal/camera/real_camera.go) — process and frame lifecycle
- [`internal/camera/mock_camera.go`](../../../internal/camera/mock_camera.go) — generated test/development frames
- [`internal/camera/real_camera_test.go`](../../../internal/camera/real_camera_test.go) — subprocess lifecycle regression tests
- [`cmd/gogomio/main.go`](../../../cmd/gogomio/main.go) — backend selection and startup fallback
