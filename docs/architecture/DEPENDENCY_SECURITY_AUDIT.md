# Dependency and Build Tooling Review

**Date:** 2026-09-24
**Project:** GoGoMio
**Go module requirement:** Go 1.25

## Go dependencies

The application uses internal packages and the Go standard library. It has no third-party Go module requirements, so `go.mod` has no `require` block and `go.sum` is not needed. The HTTP router, command dispatch, and API reference serving use standard-library and project code.

This removes application module downloads from Docker builds and the normal build and test jobs. The benchmark comparison job still installs `golang.org/x/perf/cmd/benchstat` when it runs. It does not remove the trust placed in the Go toolchain, container base images, OS packages, or build and release services.

## Runtime and delivery tools

- Raspberry Pi deployments use `rpicam-vid` or `libcamera-vid`; `ffmpeg` is the compatibility fallback. These tools provide access to camera drivers and codecs.
- Docker and Docker Compose provide the supported deployment path. Buildx creates multi-architecture images.
- GitHub Actions runs CI. GoReleaser packages binary releases, and Codecov publishes coverage reports.
- The benchmark workflow installs `benchstat` on demand to compare benchmark results; it is a CI-only tool, not an application dependency.

These tools have distinct delivery or hardware roles and are not duplicated by the in-process Go application.

## CI checks

Normal CI checks use Go's built-in `go test`, `go vet`, and `gofmt`. The deployment workflows still use runner-provided `jq` to process service responses; the Go builder no longer installs `jq` to summarize test logs. Dependency updates are not configured in Dependabot because the application module has no third-party Go packages; GitHub Actions updates remain enabled. The separately installed benchmark tool is not tracked by Dependabot.

## Scope and maintenance

This document records the dependency and tool inventory from repository manifests. It is not a live vulnerability scan. Keep this inventory current when adding a Go module, changing runtime camera backends, or changing build and release services. Evaluate the standard library and existing project code before adding a module.
