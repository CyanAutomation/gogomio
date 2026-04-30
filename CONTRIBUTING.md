# Contributing to GoGoMio

Thank you for your interest in contributing to GoGoMio! This document provides guidelines for contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Testing Requirements](#testing-requirements)
- [Code Style](#code-style)
- [Submitting Changes](#submitting-changes)
- [Release Process](#release-process)

## Code of Conduct

This project adheres to the Contributor Covenant. By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:

   ```bash
   git clone https://github.com/YOUR_USERNAME/gogomio.git
   cd gogomio
   ```

3. **Add upstream remote**:

   ```bash
   git remote add upstream https://github.com/CyanAutomation/gogomio.git
   ```

4. **Create a feature branch**:

   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Setup

### Prerequisites

- **Go 1.23+** — Install from [golang.org](https://golang.org/dl)
- **Docker** — For multi-architecture builds and deployment testing
- **Git** — For version control

### Local Environment

1. **Install dependencies**:

   ```bash
   go mod download
   ```

2. **Set up pre-commit hook** (optional but recommended):

   ```bash
   git config core.hooksPath .githooks
   chmod +x .githooks/pre-commit
   ```

3. **Configure environment** (for development):

   ```bash
   cp .env.example .env
   # Edit .env if needed; defaults are fine for mock camera mode
   ```

4. **Run tests locally** (before any commits):

   ```bash
   go test ./... -v -race -cover
   ```

## Making Changes

### Code Organization

- **`cmd/gogomio/`** — Application entry point (main.go)
- **`internal/camera/`** — Camera interfaces, frame buffering, mock camera
- **`internal/api/`** — HTTP handlers, routing, middleware
- **`internal/cli/`** — CLI commands (Cobra)
- **`internal/config/`** — Configuration management
- **`internal/settings/`** — Persistent settings
- **`internal/web/`** — Web UI and static assets

### Guidelines

1. **Follow Go idioms**: Use `gofmt` (automatically handled by most editors)
2. **Use interfaces**: Define interfaces for testability (e.g., `Camera` interface)
3. **Avoid globals**: Inject dependencies instead
4. **Handle errors explicitly**: Don't ignore errors unless intentional
5. **Add tests for new features**: Maintain ≥75% coverage
6. **Document public APIs**: Use Godoc comments for exported functions

### Example: Adding a New Feature

```go
// internal/camera/new_feature.go
package camera

// NewFeature does something important.
type NewFeature struct {
    // fields
}

// NewNewFeature creates and initializes a NewFeature.
func NewNewFeature() *NewFeature {
    return &NewFeature{}
}

// Do performs the feature operation.
func (nf *NewFeature) Do(ctx context.Context) error {
    // implementation
    return nil
}
```

**Corresponding test** (`internal/camera/new_feature_test.go`):

```go
func TestNewFeature(t *testing.T) {
    nf := NewNewFeature()
    if nf == nil {
        t.Fatal("expected non-nil NewFeature")
    }
    
    err := nf.Do(context.Background())
    if err != nil {
        t.Fatalf("Do() failed: %v", err)
    }
}

// Race condition test
func TestNewFeatureRace(t *testing.T) {
    nf := NewNewFeature()
    
    // Concurrent access to trigger race detector
    done := make(chan bool, 2)
    go func() {
        nf.Do(context.Background())
        done <- true
    }()
    go func() {
        nf.Do(context.Background())
        done <- true
    }()
    
    <-done
    <-done
}
```

## Testing Requirements

### Mandatory

1. **All tests must pass**:

   ```bash
   go test ./... -v -race -cover
   ```

2. **Race detection enabled** (`-race` flag):
   - Required for all concurrent code
   - Critical for `FrameBuffer` and `ConnectionTracker`

3. **Coverage ≥75%**:

   ```bash
   go test ./... -coverprofile=coverage.out
   go tool cover -func=coverage.out
   ```

4. **No test skips** in CI:
   - Temporary skips must have a tracked issue
   - Use `t.Skip("reason + issue #123")`

### Recommended

1. **Benchmarks for performance-critical code**:

   ```bash
   go test -bench=. -benchmem ./internal/camera ./internal/api
   ```

### Fault-Injection Logging Notes

- Some `internal/settings` fault-injection tests intentionally trigger persist failures to validate rollback and cleanup paths.
- These tests are expected to emit error-level log lines while still **passing**.
- Expected-path logs in these tests are prefixed with `[expected-failure-path]` to help CI readers distinguish intentional error logs from real regressions.

1. **Table-driven tests** for multiple scenarios:

   ```go
   func TestFrameBuffer(t *testing.T) {
       tests := []struct {
           name  string
           input []byte
           want  int
       }{
           {"small frame", make([]byte, 100), 100},
           {"large frame", make([]byte, 1000000), 1000000},
       }
       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) {
               // test logic
           })
       }
   }
   ```

### Pre-Commit Validation

If you set up the pre-commit hook, it runs automatically:

```bash
git commit -m "feat: add new feature"
# → Runs: go test ./... -v -race -cover
# → If tests fail, commit is blocked
```

To bypass (not recommended):

```bash
git commit --no-verify
```

## Code Style

### Go Format

Use `gofmt` (most editors do this automatically):

```bash
gofmt -w ./internal/camera
```

### Linting

Linting is enforced in CI via `golangci-lint`. Run locally before pushing:

```bash
golangci-lint run ./...
```

The project uses a [`.golangci.yml`](.golangci.yml) config with the default linters (`errcheck`, `govet`, `staticcheck`, `ineffassign`, `unused`) plus `gofmt`/`goimports` formatters. Fix any reported issues — the CI lint job will fail if they are present.

### Documentation

- **Exported functions/types** must have Godoc comments:

  ```go
  // FrameBuffer manages thread-safe frame storage for MJPEG clients.
  type FrameBuffer struct { ... }
  ```

- **Complex logic** should have inline comments:

  ```go
  // Use atomic CAS to prevent double-close
  if !atomic.CompareAndSwapInt32(&fb.closed, 0, 1) {
      return errClosed
  }
  ```

## Submitting Changes

### Pull Request Process

1. **Update your branch** from upstream:

   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Push to your fork**:

   ```bash
   git push origin feature/your-feature-name
   ```

3. **Create a Pull Request** on GitHub:
   - Title: `feat: add new feature` (use conventional commits)
   - Description: Explain what and why, not how
   - Link any related issues: `Closes #123`

4. **Respond to review feedback**:
   - Don't force-push after feedback (makes review history hard to follow)
   - Add new commits and push again

5. **Ensure CI passes**:
   - All checks must be green (tests, coverage, linting)
   - Coverage must not decrease

### PR Titles (Conventional Commits)

Use format: `<type>: <description>`

**Types**:

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `test`: Test additions/updates
- `refactor`: Code refactoring (no behavior change)
- `perf`: Performance improvement
- `chore`: Build, deps, etc.

**Examples**:

- `feat: add connection timeout configuration`
- `fix: prevent double-close race condition in FrameBuffer`
- `docs: update deployment guide for Bullseye`
- `test: add concurrent client stress test`

## Release Process

### Version Numbering

Uses [Semantic Versioning](https://semver.org/): `MAJOR.MINOR.PATCH`

- `0.1.0` — API stability not guaranteed (pre-1.0)
- `1.0.0` — First stable release
- `1.1.0` — New features added (backward compatible)
- `1.1.1` — Bug fixes only

### Releasing a New Version

1. **Update CHANGELOG.md**:

   ```markdown
   ## [0.2.0] - 2026-05-15
   
   ### Added
   - Real Pi camera support via libcamera
   - Prometheus metrics endpoint
   
   ### Fixed
   - Race condition in frame buffer cleanup
   
   ### Changed
   - Dockerfile uses golang:1.24 base image
   ```

2. **Update version in `go.mod`** (if applicable):

   ```bash
   # No version field in go.mod for apps, but mention in git tag
   ```

3. **Create git tag**:

   ```bash
   git tag -a v0.2.0 -m "Release version 0.2.0"
   git push origin v0.2.0
   ```

4. **GitHub Actions triggers**:
   - `build-multiarch.yml` builds and pushes to Docker Hub
   - `benchmark.yml` records baseline
   - Artifacts available in GitHub Releases

## Questions?

- Check [docs/README.md](docs/README.md) for architecture and guides
- See [CLAUDE.md](CLAUDE.md) for development commands
- Review [docs/repo-maturity.md](docs/repo-maturity.md) for project status
- Open an issue for clarification

## Recognition

Contributors will be acknowledged in releases and on the project homepage. Thank you for making GoGoMio better! 🌊
