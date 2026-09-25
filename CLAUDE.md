# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

GoGoMio is a high-performance MJPEG streaming server for Raspberry Pi CSI cameras, written in Go. The binary runs in two modes: **server mode** (no args → starts HTTP server) and **CLI mode** (any subcommand → queries a running server over HTTP).

**Project Maturity**: See [docs/repo-maturity.md](docs/repo-maturity.md) for a comprehensive assessment against the maturity rubric, identified gaps, and a prioritized improvement roadmap. Current score: **93/100 (Mature Product)**.

## Commands

```bash
# Build
go build -o gogomio ./cmd/gogomio

# Test (all)
go test ./... -v -race -cover

# Test with coverage report
go test ./... -v -race -coverprofile=coverage.out && go tool cover -html=coverage.out -o coverage.html

# Test (single test)
go test -v ./internal/camera -run TestFrameBuffer

# Test (single package)
go test -v ./internal/api

# Benchmarks
go test -v ./internal/camera -bench=. -benchmem

# Benchmarks with detailed output
go test -bench=. -benchmem -benchtime=2s ./internal/camera ./internal/api

# Development with mock camera (no hardware needed)
docker-compose -f docker-compose.mock.yml up --build

# Static checks (Go standard library)
go vet ./...
gofmt -w ./cmd ./internal ./docs/reference
```

**Code Quality**: CI runs `go vet ./...` and checks tracked Go files with `gofmt -l`.

**Performance Baselines** (April 2026):

- **FrameBuffer.Write()**: ~150ns per frame (optimal; lock-free atomic operation)
- **MJPEG Handler throughput**: ~10,000 concurrent frames/sec on single core
- **GC Impact**: <1ms pause per request (see [FRAME_BUFFER_GC_ANALYSIS.md](docs/architecture/FRAME_BUFFER_GC_ANALYSIS.md))
- **Connection tracking overhead**: <100μs per new connection

Benchmark regression checks run on pull requests, pushes to `main`, weekly, and on demand via [.github/workflows/benchmark.yml](.github/workflows/benchmark.yml). The workflow compares against the PR/push base or the most recent successful main artifact and fails for a statistically significant regression above 15% after multiple-comparison correction. See [FRAME_BUFFER_GC_ANALYSIS.md](docs/architecture/FRAME_BUFFER_GC_ANALYSIS.md) for details.

**CI/CD Note**: Tests run automatically on every push/PR via [.github/workflows/code-coverage-test.yml](.github/workflows/code-coverage-test.yml). All tests must pass and coverage must be ≥75% to merge to `main`. Coverage is tracked on [Codecov](https://codecov.io/gh/CyanAutomation/gogomio). Benchmarks tracked via [.github/workflows/benchmark.yml](.github/workflows/benchmark.yml).

There is no Makefile. The repo uses standard Go tooling and GitHub Actions for multi-arch Docker builds via `./scripts/build-multiarch.sh`.

## Architecture

### Execution Flow

```
cmd/gogomio/main.go
 ├─ CLI mode  → cli.Execute()          (standard-library dispatch; commands query server via HTTP)
 └─ Server mode
     ├─ config.LoadFromEnv()
     ├─ camera.NewRealCamera() or NewMockCamera()
     ├─ api.NewFrameManager(camera)
     ├─ api.RegisterHandlers(mux, frameManager, config)
     └─ http.Server :8000
```

### Key Packages

**`internal/camera/`** — Core streaming logic. The `Camera` interface is implemented by `RealCamera` (spawns `libcamera-vid` or `ffmpeg` subprocess) and `MockCamera` (synthetic JPEG generator for development). `FrameBuffer` is the critical thread-safe latest-frame store using condition variables and atomic operations — it supports concurrent MJPEG clients via `GetFrame()` and implements `io.Writer` so the encoder writes directly into it.

**`internal/api/`** — Go standard-library `http.ServeMux` routes and middleware. `FrameManager` coordinates the camera, frame buffering, and streaming. Key endpoints: `/stream.mjpg` (MJPEG multipart), `/snapshot.jpg`, `/v1/metrics/live`, `/v1/health/detailed`, `/health`, `/ready`, `/docs/` (API reference). Rate limiting: 100 req/10s per IP.

**`internal/cli/`** — Standard-library command dispatcher. Commands (`status`, `config`, `health`, `snapshot`, `diagnostics`) send HTTP requests to the running server.

**`internal/config/`** — Env var loading and validation. Key vars: `MIO_RESOLUTION`, `MIO_FPS`, `MIO_TARGET_FPS`, `MIO_MOCK` (enable mock camera). See `.env.example` for full list.

**`internal/settings/`** — Persistent file-based settings with OS-appropriate file locking (`filelock_unix.go` / `filelock_windows.go`).

### Concurrency Model

`FrameBuffer` uses a condition variable (`sync.Cond`) for fan-out to multiple concurrent MJPEG clients. Frames are published atomically as immutable values. `ConnectionTracker` enforces max concurrent streams. Race condition tests live in `*_race_test.go` files and should always be run with `-race`.

### Swagger / OpenAPI

The checked-in OpenAPI 2.0 JSON and YAML files in `docs/reference/` are the API specification source. The server embeds and serves these files at `/swagger.json` and `/swagger.yaml`; update them when API routes or response shapes change. `/docs/` links to both specifications.
