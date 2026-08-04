# GoGoMio golangci-lint Analysis & Improvement Plan

**Analysis Date**: 2026-08-04  
**Tool Version**: golangci-lint v2.12.2  
**Total Issues Found**: 141  
**Linters Used**: 24 (gosec, gocritic, gocyclo, gocognit, prealloc, goconst, unconvert, unparam, bodyclose, contextcheck, errname, errorlint, exhaustive, funlen, misspell, revive, whitespace, godot, godox, nilerr, nilnil, copyloopvar, dupl, durationcheck, perfsprint)

## Executive Summary

This document provides a comprehensive analysis of the GoGoMio codebase using golangci-lint with an extended set of linters. The analysis identified 141 issues across 16 different categories, ranging from critical security vulnerabilities to code quality improvements. Issues are prioritized by severity and impact.

## Issue Breakdown by Category

| Category | Count | Severity | Priority |
| ---------- | ------- | ---------- | ---------- |
| **gosec** (Security) | 23 | 🔴 Critical | P0 |
| **goconst** (Magic strings) | 41 | 🟡 High | P1 |
| **revive** (Documentation) | 24 | 🟡 High | P1 |
| **funlen** (Long functions) | 15 | 🟢 Medium | P2 |
| **perfsprint** (Performance) | 10 | 🟢 Medium | P2 |
| **dupl** (Code duplication) | 6 | 🟢 Medium | P2 |
| **gocognit** (Cognitive complexity) | 5 | 🟡 High | P1 |
| **gocritic** (Code issues) | 4 | 🟢 Medium | P2 |
| **godot** (Comment formatting) | 3 | 🟢 Low | P3 |
| **contextcheck** (Context propagation) | 3 | 🟡 High | P1 |
| **whitespace** | 2 | 🟢 Low | P3 |
| **errcheck** | 1 | 🟡 High | P1 |
| **ineffassign** | 1 | 🟡 High | P1 |
| **errorlint** | 1 | 🟢 Medium | P2 |
| **copyloopvar** | 1 | 🟢 Medium | P2 |
| **misspell** | 1 | 🟢 Low | P3 |

---

## 🔴 P0: Critical Security Issues (23 issues)

### 1. HTTP Server Timeouts Missing (G112) - CRITICAL

**Location**: `cmd/gogomio/main.go:110`  
**Issue**: HTTP server lacks timeout configurations, vulnerable to Slowloris attacks and resource exhaustion.

**Current Code**:

```go
server := &http.Server{
    Addr:    addr,
    Handler: router,
}
```

**Required Fix**:

```go
server := &http.Server{
    Addr:         addr,
    Handler:      router,
    ReadTimeout:  15 * time.Second,
    WriteTimeout: 15 * time.Second,
    IdleTimeout:  60 * time.Second,
    ReadHeaderTimeout: 5 * time.Second,
}
```

**Impact**: Prevents Slowloris attacks, connection exhaustion, and improves server resilience.

---

### 2. pprof Profiling Endpoint Exposed (G108)

**Location**: `cmd/gogomio/main.go:34`  
**Issue**: `/debug/pprof` endpoint automatically exposed in production builds.

**Current Code**:

```go
import (
    _ "net/http/pprof"
)
```

**Recommended Fix Options**:

1. **Option A**: Guard with build tag

```go
//go:build debug
// +build debug

package main

import _ "net/http/pprof"
```

1. **Option B**: Guard with environment variable

```go
if os.Getenv("MIO_ENABLE_PPROF") == "true" {
    go func() {
        log.Printf("🔬 pprof server starting on :6060")
        if err := http.ListenAndServe(":6060", nil); err != nil {
            log.Printf("pprof server error: %v", err)
        }
    }()
}
```

**Impact**: Prevents information disclosure and unauthorized profiling access.

---

### 3. File Permissions Too Permissive (G301, G302, G306) - 6 occurrences

**Issue**: Settings files and directories using overly permissive permissions (0644, 0755).

**Locations & Fixes**:

#### `internal/settings/settings.go:202`

```go
// Current
if err := os.MkdirAll(dir, 0755); err != nil {

// Fix
if err := os.MkdirAll(dir, 0750); err != nil {
```

#### `internal/settings/settings.go:274`

```go
// Current
lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)

// Fix
lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
```

#### `internal/settings/settings.go:324`

```go
// Current
return os.WriteFile(m.filePath, backupData, 0644)

// Fix
return os.WriteFile(m.filePath, backupData, 0600)
```

#### `internal/settings/settings.go:370`

```go
// Current
return os.WriteFile(dst, srcData, 0644)

// Fix
return os.WriteFile(dst, srcData, 0600)
```

#### Test files (informational - can remain permissive)

- `internal/settings/settings_recovery_test.go:34`
- `internal/settings/settings_test.go:352`
- `internal/settings/settings_test.go:540`

**Impact**: Prevents unauthorized access to sensitive configuration data.

---

### 4. Log Injection Risk (G706) - 2 occurrences

**Locations**:

- `internal/api/handlers.go:591`
- `internal/api/handlers.go:620`

**Issue**: User-controlled `r.RemoteAddr` logged without sanitization.

**Current Code** (`handlers.go:591`):

```go
log.Printf("🔗 Stream client connected (total clients: %d, remote: %s)", 
    atomic.LoadInt64(&fm.clientCount), r.RemoteAddr)
```

**Fix**:

```go
// Add sanitization helper
func sanitizeRemoteAddr(addr string) string {
    // Remove any control characters and limit length
    addr = strings.Map(func(r rune) rune {
        if r < 32 || r == 127 {
            return -1
        }
        return r
    }, addr)
    if len(addr) > 128 {
        addr = addr[:128]
    }
    return addr
}

// Use in log statements
log.Printf("🔗 Stream client connected (total clients: %d, remote: %s)", 
    atomic.LoadInt64(&fm.clientCount), sanitizeRemoteAddr(r.RemoteAddr))
```

**Impact**: Prevents log injection attacks.

---

### 5. Potential Integer Overflow (G115) - 4 occurrences

**Locations**:

- `internal/camera/health_monitor_test.go:185`
- `internal/camera/mock_camera.go:169,181,182,183`

**Issue**: Integer conversions without overflow checks.

**Fix Pattern**:

```go
// Before
hue := uint8((frameNum * 10) % 360)

// After - with bounds checking
hueVal := (frameNum * 10) % 360
if hueVal > 255 {
    hueVal = 255
}
hue := uint8(hueVal)
```

**Note**: These are in test code and mock camera, so risk is low. Consider adding bounds checks or use `math.Min()`.

---

### 6. Subprocess with Tainted Input (G204) - 3 occurrences

**Locations**:

- `internal/camera/real_camera.go:528`
- `internal/camera/real_camera.go:552`
- `internal/camera/real_camera.go:569`

**Issue**: Commands built from user-configurable parameters.

**Analysis**: These use validated configuration values (resolution, fps, device path). The inputs are constrained by config validation, so risk is **ACCEPTABLE** with current validation. However, add explicit validation if not already present.

**Recommended Action**:

- Add input validation documentation
- Consider adding explicit allowlists for device paths
- Add comments explaining security rationale

---

### 7. Potential File Inclusion (G304) - 2 occurrences

**Locations**:

- `internal/settings/settings.go:318`
- `internal/settings/settings.go:366`

**Issue**: Reading files using variables for paths.

**Analysis**: Both are for backup file operations using controlled paths. Risk is **LOW** but should validate paths don't escape expected directory.

**Recommended Fix**:

```go
// Add path validation helper
func validateBackupPath(path string) error {
    cleaned := filepath.Clean(path)
    // Ensure path is within expected directory
    if !strings.HasPrefix(cleaned, filepath.Clean(expectedBackupDir)) {
        return fmt.Errorf("backup path outside allowed directory")
    }
    return nil
}
```

---

## 🟡 P1: High Priority Issues

### 8. Magic Strings Should Be Constants (goconst) - 41 occurrences

**Pattern**: Repeated string literals that should be package-level constants for maintainability.

#### API & Network Constants

**File**: `internal/config/config.go`

```go
const (
    DefaultBindHost = "0.0.0.0"
    DefaultBindPort = 8000
)
```

**Occurrences**: `"0.0.0.0"` appears 5 times across `config.go` and `config_test.go`

#### HTTP Content Types

**File**: `internal/api/handlers.go` or new `internal/api/constants.go`

```go
const (
    ContentTypeJSON = "application/json"
    ContentTypeMJPEG = "multipart/x-mixed-replace; boundary=FRAME"
)
```

**Occurrences**: `"application/json"` appears 4 times in `handlers_test.go`

#### Camera Device Paths

**File**: `internal/camera/constants.go` (new file)

```go
const (
    DefaultDevicePath = "/dev/video0"
    DevNullPath = "/dev/null" // for testing
)
```

**Occurrences**:

- `"/dev/video0"` appears 6 times in `real_camera.go` and `real_camera_test.go`
- `"/dev/null"` appears 8 times in `real_camera_test.go`

#### Backend Names

**File**: `internal/camera/constants.go`

```go
const (
    BackendRpiCam = "rpicam-vid"
    BackendLibCamera = "libcamera-vid"
    BackendFFmpeg = "ffmpeg"
)
```

**Occurrences**:

- `"rpicam-vid"` appears 6 times
- `"ffmpeg"` appears 3 times

#### Test Fixture Strings

**File**: Test files can use test-scoped constants

```go
const (
    testStatusDegraded = "degraded"
    testQualityLow = "low quality"
    testQualityMid = "mid quality"
    testQualityHigh = "high quality"
)
```

#### API Endpoints

**File**: `internal/api/constants.go` or `internal/cli/constants.go`

```go
const (
    EndpointSettings = "/v1/api/settings"
    DefaultServerURL = "http://localhost:8000"
)
```

**Occurrences**:

- `"/v1/api/settings"` appears 6 times
- `"http://localhost:8000"` appears 3 times

**Complete List of Magic Strings**:

| String | Count | Locations |
| -------- | ------- | ----------- |
| `"0.0.0.0"` | 5 | `config.go`, `config_test.go` |
| `"/dev/video0"` | 6 | `real_camera.go`, `real_camera_test.go` |
| `"/dev/null"` | 8 | `real_camera_test.go` |
| `"rpicam-vid"` | 6 | `real_camera.go`, `real_camera_test.go` |
| `"ffmpeg"` | 3 | `real_camera.go`, `real_camera_test.go` |
| `"application/json"` | 4 | `handlers_test.go` |
| `"http://localhost:8000"` | 3 | `client.go`, `client_test.go` |
| `"/v1/api/settings"` | 6 | `client_test.go`, `commands_test.go` |
| `"degraded"` | 6 | `handlers_diagnostics_test.go` |
| `"stream stopped"` | 3 | `handlers_test.go` |
| Various test strings | 20+ | Multiple test files |

---

### 9. Missing Documentation (revive) - 24 occurrences

**Pattern**: Exported functions, variables, and packages missing godoc comments.

#### Missing Package Comments

**Files**:

- `internal/config/config.go:1`
- `internal/settings/filelock_unix.go:3`
- `internal/web/web.go:1`

**Fix Template**:

```go
// Package config provides configuration management for GoGoMio.
// It handles environment variable loading, validation, and default values.
package config
```

```go
// Package settings provides persistent file-based settings storage with OS-appropriate file locking.
package settings
```

```go
// Package web provides the embedded web UI for GoGoMio.
package web
```

#### Missing Exported Variable Comments

**File**: `internal/camera/real_camera.go:31`

```go
// Current
var ErrFirstFrameTimeout = errors.New("camera first frame timeout")

// Fix
// ErrFirstFrameTimeout is returned when the camera fails to produce the first frame within the expected timeout period.
var ErrFirstFrameTimeout = errors.New("camera first frame timeout")
```

#### Missing Exported Method Comments

**File**: `internal/camera/real_camera.go:134`

```go
// Current
func (rc *RealCamera) SetLogger(logger *log.Logger) {

// Fix
// SetLogger sets a custom logger for the RealCamera instance.
// This is primarily used for testing and debugging purposes.
func (rc *RealCamera) SetLogger(logger *log.Logger) {
```

#### Unused Parameters (revive) - 3 occurrences

**File**: `internal/cli/commands.go`
**Locations**: Lines 30, 46, 63

**Current**:

```go
RunE: func(cmd *cobra.Command, args []string) error {
```

**Fix**:

```go
RunE: func(_ *cobra.Command, args []string) error {
```

**Impact**: Silence linter warnings for intentionally unused Cobra command parameters.

---

### 10. Context Not Passed (contextcheck) - 3 occurrences

**Locations**:

- `internal/api/handlers.go:588` - `IncrementClients->startCapture`
- `internal/api/handlers.go:1054` - `handleSnapshot->GetFrame->IncrementClients->startCapture`
- `internal/api/handlers.go:1115` - Similar chain

**Issue**: Camera operations don't propagate context for cancellation.

**Recommended Fix**:
Add context parameter to camera interface methods:

```go
// Update Camera interface
type Camera interface {
    Start(ctx context.Context) error
    Stop() error
    Write(p []byte) (n int, err error)
    IsHealthy() bool
}

// Update handlers to pass request context
func handleMJPEGStream(w http.ResponseWriter, r *http.Request, fm *FrameManager) {
    ctx := r.Context()
    if err := fm.IncrementClientsWithContext(ctx); err != nil {
        http.Error(w, err.Error(), http.StatusServiceUnavailable)
        return
    }
    // ...
}
```

**Impact**: Allows proper request cancellation and resource cleanup.

---

### 11. Unchecked Error (errcheck) - 1 occurrence

**Location**: `internal/cli/client_test.go:53`

**Current**:

```go
os.Unsetenv(envKey)
```

**Fix**:

```go
_ = os.Unsetenv(envKey) // Error ignored: test cleanup best-effort
```

**Impact**: Explicit acknowledgment that error is intentionally ignored.

---

### 12. Ineffectual Assignment (ineffassign) - 1 occurrence

**Location**: `internal/api/handlers_e2e_test.go:312`

**Issue**: Variable assigned but never used.

**Fix**: Remove the assignment or use the variable as intended. Need to examine context to determine correct fix.

---

### 13. Cognitive Complexity (gocognit) - 5 occurrences

**High cognitive complexity functions that should be refactored**:

- Long functions with multiple nested conditionals
- Extract helper functions to improve readability

**Recommendation**: Review these during refactoring sessions, not critical for immediate fix.

---

## 🟢 P2: Medium Priority Issues

### 14. Performance - Use errors.New Instead of fmt.Errorf (perfsprint) - 10 occurrences

**Pattern**: Static error messages using `fmt.Errorf` instead of `errors.New`.

**Example**: `internal/api/handlers.go:583`

```go
// Current
return fmt.Errorf("connection limit exceeded")

// Fix
return errors.New("connection limit exceeded")
```

**Locations**: Search for `fmt.Errorf` with static strings and replace with `errors.New()`.

**Impact**: Minor performance improvement, reduced allocations.

---

### 15. Long Functions (funlen) - 15 occurrences

**Issue**: Functions exceeding recommended length limits.

**Recommendation**:

- Extract helper functions
- Break into smaller, testable units
- Improve readability

**Approach**: Review during refactoring cycles, not immediate priority.

---

### 16. Code Duplication (dupl) - 6 occurrences

**Issue**: Similar code blocks that could be refactored into shared functions.

**Recommendation**:

- Identify common patterns
- Extract into helper functions
- Consider generics for type-specific duplicates

---

### 17. Error Wrapping (errorlint) - 1 occurrence

**Issue**: Error not properly wrapped for error chain inspection.

**Pattern**:

```go
// Before
return fmt.Errorf("failed to start: %s", err.Error())

// After
return fmt.Errorf("failed to start: %w", err)
```

---

### 18. Copy Loop Variable (copyloopvar) - 1 occurrence

**Issue**: Loop variable captured by reference instead of value.

**Pattern**:

```go
// Before
for _, item := range items {
    go func() {
        process(item) // captures reference
    }()
}

// After
for _, item := range items {
    item := item // capture value
    go func() {
        process(item)
    }()
}
```

---

## 🟢 P3: Low Priority Issues

### 19. Comment Formatting (godot) - 3 occurrences

**Locations**:

- `cmd/gogomio/main.go:64`
- `cmd/gogomio/main.go:214`
- `docs/docs.go:740`

**Issue**: Comments should end with a period.

**Examples**:

```go
// Current
// startServer initializes and runs the HTTP server

// Fix
// startServer initializes and runs the HTTP server.
```

---

### 20. Code Style (gocritic) - 4 occurrences

**Issues**:

- `cmd/gogomio/main.go:134` - `exitAfterDefer`: `log.Fatalf` will exit, defer won't run
- `internal/api/handlers_e2e_test.go:352` - `ifElseChain`: rewrite if-else to switch
- `internal/camera/real_camera.go:297,896` - `elseif`: can replace `else {if` with `else if`

**Fixes**: Minor style improvements, low priority.

---

### 21. Whitespace Issues - 2 occurrences

**Locations**:

- `internal/settings/settings.go:215` - unnecessary leading newline
- `internal/web/web_test.go:208` - unnecessary trailing newline

**Fix**: Remove extra whitespace.

---

### 22. Misspelling (misspell) - 1 occurrence

**Location**: `internal/camera/connection_tracker_test.go:146`

**Current**:

```go
// Launch concurrent incrementers
```

**Fix**:

```go
// Launch concurrent incrementors
```

---

## Implementation Checklist

### Phase 1: Security (P0) - Immediate Action Required

- [ ] Add HTTP server timeouts (`cmd/gogomio/main.go`)
- [ ] Guard pprof endpoint behind environment variable
- [ ] Fix file permissions (6 locations in `settings.go`)
- [ ] Add log sanitization for `r.RemoteAddr` (2 locations)
- [ ] Document security rationale for subprocess commands
- [ ] Add path validation for backup files

### Phase 2: Code Quality (P1) - High Priority

- [ ] Extract 41 magic strings to constants
  - [ ] Create `internal/api/constants.go`
  - [ ] Create `internal/camera/constants.go`
  - [ ] Update `internal/config/config.go`
  - [ ] Update test files with test-scoped constants
- [ ] Add missing package documentation (3 packages)
- [ ] Add missing exported function/variable documentation (24 items)
- [ ] Fix unused Cobra command parameters (3 locations)
- [ ] Add context propagation to camera operations
- [ ] Fix unchecked `os.Unsetenv` error
- [ ] Fix ineffectual assignment in test

### Phase 3: Performance & Refactoring (P2) - Medium Priority

- [ ] Replace `fmt.Errorf` with `errors.New` for static strings (10 locations)
- [ ] Refactor long functions (15 functions)
- [ ] Eliminate code duplication (6 locations)
- [ ] Fix error wrapping (1 location)
- [ ] Fix loop variable capture (1 location)

### Phase 4: Polish (P3) - Low Priority

- [ ] Add periods to comments (3 locations)
- [ ] Fix style issues (4 locations)
- [ ] Remove extra whitespace (2 locations)
- [ ] Fix typo: "incrementers" → "incrementors"

---

## Configuration File Recommendation

Create `.golangci.yml` in repository root to enforce these checks in CI:

```yaml
run:
  timeout: 5m
  tests: true

linters:
  enable:
    - errcheck
    - gosec
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gocritic
    - gocognit
    - goconst
    - misspell
    - revive
    - godot
    - contextcheck
    - errorlint
    - perfsprint
    - whitespace

linters-settings:
  gosec:
    excludes:
      - G115 # Allow integer conversions in test/mock code
      - G204 # Allow subprocess with validated config
  goconst:
    min-len: 3
    min-occurrences: 3
  funlen:
    lines: 100
    statements: 50
  gocognit:
    min-complexity: 15

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - gosec
        - funlen
        - gocognit
    - path: mock_camera\.go
      linters:
        - gosec
  max-issues-per-linter: 0
  max-same-issues: 0
```

---

## Testing Strategy

After implementing fixes:

1. **Run full linter suite**:

   ```bash
   golangci-lint run --enable=gosec,gocritic,gocyclo,gocognit,prealloc,goconst,unconvert,unparam,bodyclose,contextcheck,errname,errorlint,exhaustive,funlen,misspell,revive,whitespace,godot,godox,nilerr,nilnil,copyloopvar,dupl,durationcheck,perfsprint ./...
   ```

2. **Verify all tests pass**:

   ```bash
   go test ./... -v -race -cover
   ```

3. **Run benchmarks to ensure no performance regression**:

   ```bash
   go test -bench=. -benchmem ./internal/camera ./internal/api
   ```

4. **Manual testing**:
   - Test mock camera mode
   - Test connection limits
   - Test settings persistence
   - Verify pprof only accessible when enabled

---

## Success Criteria

- [ ] All P0 security issues resolved
- [ ] golangci-lint passes with extended linter set
- [ ] Test coverage remains ≥75%
- [ ] No performance regressions in benchmarks
- [ ] Documentation complete for all exported items
- [ ] CI integration with `.golangci.yml`

---

## Additional Notes

### Deferred/Optional Improvements

These can be addressed in future iterations:

1. **Add more linters**: Consider `nilaway`, `rowserrcheck`, `sqlclosecheck` if applicable
2. **Benchmark tracking**: Integrate benchmark regression detection in CI
3. **Security scanning**: Add `govulncheck` to CI pipeline
4. **Code coverage gates**: Enforce coverage thresholds per package

### References

- [golangci-lint Documentation](https://golangci-lint.run/)
- [gosec Security Rules](https://github.com/securego/gosec#available-rules)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://golang.org/doc/effective_go)

---

**End of Analysis**
