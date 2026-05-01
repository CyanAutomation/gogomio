# Changelog

All notable changes to GoGoMio are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added

- CI test workflows now trigger on every push and pull request to `main`
- Coverage gate enforcing ≥75% total test coverage in CI
- `golangci-lint` linting job added to CI pipeline
- `.golangci.yml` linter configuration
- GitHub issue templates (bug report, feature request)
- GitHub PR template with test/coverage checklist
- `CODEOWNERS` defining review authority per package

### Changed

- `code-coverage-test.yml` triggers updated from manual/weekly-only to push + PR + weekly + manual

---

## [0.1.0] - 2026-04-30

Initial public release of GoGoMio — a high-performance MJPEG streaming server for Raspberry Pi CSI cameras.

### Added

- MJPEG streaming server on port 8000 (`/stream.mjpg`)
- Snapshot endpoint (`/snapshot.jpg`)
- Live metrics endpoint (`/v1/metrics/live`)
- Detailed health endpoint (`/v1/health/detailed`) and basic `/health`, `/ready` endpoints
- Swagger UI at `/docs`
- Web UI at `/` with live stream preview
- `RealCamera` backend using `libcamera-vid` or `ffmpeg` subprocess
- `MockCamera` backend for development without hardware (`MIO_MOCK=true`)
- `FrameBuffer` with condition-variable fan-out for concurrent MJPEG clients
- `ConnectionTracker` for enforcing max concurrent stream limit
- `StreamStats` for FPS and frame metrics
- `FrameManager` coordinating camera lifecycle, idle stop/start, and cleanup
- Cobra CLI with `status`, `config`, `health`, `snapshot`, `diagnostics` commands
- Persistent file-based settings with OS-appropriate file locking
- Rate limiting (100 req/10 s per IP) on `/v1/` endpoints
- Environment-based configuration via `.env` / `MIO_*` variables
- Multi-architecture Docker images for `linux/amd64` and `linux/arm64`
- `docker-compose.yml` for production and `docker-compose.mock.yml` for development
- Race condition tests (`*_race_test.go`) and benchmark tests (`*_benchmark_test.go`)
- GitHub Actions workflows: multi-arch Docker build, test + coverage, benchmarks
- Dependabot configuration for weekly Go module and GitHub Actions updates
- Comprehensive documentation: deployment guide, Raspberry Pi build guide, CLI guide, architecture docs
- Maturity scoring rubric and self-assessment

### Security

- Rate limiting per IP on versioned API routes
- Minimal dependency footprint (4 direct dependencies)
- No credentials or secrets in repository
- Designed for internal network deployment; reverse proxy guidance documented

[Unreleased]: https://github.com/CyanAutomation/gogomio/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/CyanAutomation/gogomio/releases/tag/v0.1.0
