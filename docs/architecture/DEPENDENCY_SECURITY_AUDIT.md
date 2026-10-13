# Dependency and Build Tooling Review

**Date:** 2026-10-13
**Project:** GoGoMio
**Go module requirement:** Go 1.27.2

## Go dependencies

The application uses internal packages and the Go standard library. It has no third-party Go module requirements, so `go.mod` has no `require` block and `go.sum` is not needed. The HTTP router, command dispatch, and API reference serving use standard-library and project code.

This removes application module downloads from Docker builds and normal build and test jobs. It does not remove the trust placed in the Go toolchain, container base images, OS packages, or build and release services.

## Runtime and delivery tools

- Raspberry Pi deployments use `rpicam-vid` or `libcamera-vid`; `ffmpeg` is the compatibility fallback. These tools provide access to camera drivers and codecs.
- Docker and Docker Compose provide the supported deployment path. Buildx creates multi-architecture images.
- GitHub Actions runs CI and stores coverage reports as workflow artifacts. GoReleaser packages binary releases.

These tools have distinct delivery or hardware roles and are not duplicated by the in-process Go application.

## CI checks

Normal CI checks use Go's built-in `go test`, `go vet`, and `gofmt`. The deployment workflows use runner-provided `jq` to process service responses. Dependency updates are not configured in Dependabot because the application module has no third-party Go packages; GitHub Actions updates remain enabled.

## Scope and maintenance

This document records the dependency and tool inventory from repository manifests. It is not a live vulnerability scan. Keep this inventory current when adding a Go module, changing runtime camera backends, or changing build and release services. Evaluate the standard library and existing project code before adding a module.
