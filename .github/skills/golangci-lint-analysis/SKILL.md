---
name: golangci-lint-analysis
description: Use when performing static code analysis, finding code health issues, detecting security vulnerabilities, enforcing code quality standards, or preparing code for review. Runs comprehensive linter analysis with golangci-lint and provides actionable recommendations prioritized by severity.
---

# golangci-lint Analysis

Use this skill when the quality of the work depends on code health, security best practices, performance optimization, or adherence to Go idioms and style guidelines.

Goal: identify code issues before they reach production, enforce consistent code quality standards, detect security vulnerabilities, and provide clear, actionable fixes with examples. Default toward: comprehensive linter sets for new analysis, focused linter sets for targeted checks, always include security linters (gosec), always verify fixes don't break tests.

## Working Model

Static analysis with golangci-lint follows this workflow:

```
1. Select Linter Set
   ↓
2. Run Analysis (golangci-lint run)
   ↓
3. Categorize Issues by Priority
   ↓
4. Generate Actionable Recommendations
   ↓
5. Implement Fixes (highest priority first)
   ↓
6. Verify with Tests (go test ./... -race)
   ↓
7. Re-run Analysis (confirm fixes)
   ↓
8. Document Findings (optional)
```

Before running analysis, answer three things:

- **What's the goal?** (comprehensive audit, pre-commit check, security scan, performance review)
- **What's the baseline?** (existing issues to track, new issues only, all issues)
- **What's acceptable?** (zero issues, security issues only, team-agreed thresholds)

## Installation & Verification

### Check Installation
```bash
golangci-lint --version
```

Expected output format: `golangci-lint has version X.Y.Z built with go1.XX...`

### Install if Missing
```bash
# macOS/Linux via script
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# Or via go install
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

## Linter Sets by Use Case

### 1. Quick Security Scan (Fastest)
**Use when**: Pre-commit hook, quick check, CI fast path
```bash
golangci-lint run --enable=gosec,errcheck ./...
```
**Checks**: Security vulnerabilities, unchecked errors

### 2. Code Quality Check (Recommended)
**Use when**: Pre-PR review, daily development, code health monitoring
```bash
golangci-lint run --enable=gosec,errcheck,govet,staticcheck,ineffassign,unused,gocritic,revive ./...
```
**Checks**: Security + correctness + style + best practices

### 3. Comprehensive Audit (Most Thorough)
**Use when**: New project setup, major refactor, quarterly review
```bash
golangci-lint run \
  --enable=gosec,gocritic,gocyclo,gocognit,prealloc,goconst,unconvert,unparam,bodyclose,contextcheck,errname,errorlint,exhaustive,funlen,misspell,revive,whitespace,godot,godox,nilerr,nilnil,copyloopvar,dupl,durationcheck,perfsprint \
  ./...
```
**Checks**: Everything—security, performance, style, complexity, duplication

### 4. Performance Focus
**Use when**: Optimizing hot paths, reducing allocations, improving throughput
```bash
golangci-lint run --enable=prealloc,unconvert,perfsprint,gocritic ./...
```
**Checks**: Allocation optimization, unnecessary conversions, sprintf performance

### 5. Test Code Quality
**Use when**: Reviewing test files, improving test maintainability
```bash
golangci-lint run --enable=testifylint,paralleltest,thelper,tparallel ./...
```
**Checks**: Test patterns, parallel execution, test helpers

## Priority-Based Issue Triage

### 🔴 P0: Critical (Fix Immediately)
**Category**: Security vulnerabilities, data races, resource leaks
**Linters**: gosec, errcheck, bodyclose, sqlclosecheck, rowserrcheck
**Action**: Stop development, fix before merge

**Common Issues**:
- Missing HTTP server timeouts (G112)
- Exposed profiling endpoints (G108)
- File permissions too permissive (G301, G302, G306)
- Log injection vulnerabilities (G706)
- Unchecked errors that can cause panics

**Example Fix**:
```go
// BEFORE: Missing timeouts (gosec G112)
server := &http.Server{
    Addr:    ":8000",
    Handler: router,
}

// AFTER: With timeouts
server := &http.Server{
    Addr:              ":8000",
    Handler:           router,
    ReadTimeout:       15 * time.Second,
    WriteTimeout:      15 * time.Second,
    IdleTimeout:       60 * time.Second,
    ReadHeaderTimeout: 5 * time.Second,
}
```

### 🟡 P1: High Priority (Fix This Sprint)
**Category**: Code maintainability, correctness, missing documentation
**Linters**: goconst, revive, contextcheck, ineffassign, staticcheck
**Action**: Schedule for current sprint, include in PR

**Common Issues**:
- Magic strings repeated throughout code (goconst)
- Missing documentation on exported items (revive)
- Context not propagated for cancellation (contextcheck)
- Variables assigned but never used (ineffassign)

**Example Fix**:
```go
// BEFORE: Magic strings (goconst)
if device != "/dev/video0" {
    return errors.New("invalid device")
}
// ... later ...
log.Printf("Opening %s", "/dev/video0")

// AFTER: Extract to constant
const DefaultDevicePath = "/dev/video0"

if device != DefaultDevicePath {
    return errors.New("invalid device")
}
log.Printf("Opening %s", DefaultDevicePath)
```

### 🟢 P2: Medium Priority (Fix Next Sprint)
**Category**: Performance, code duplication, complexity
**Linters**: perfsprint, dupl, funlen, gocognit, gocyclo
**Action**: Refactor during cleanup cycles

**Common Issues**:
- Using fmt.Errorf for static strings instead of errors.New (perfsprint)
- Duplicate code blocks (dupl)
- Functions too long or complex (funlen, gocognit)

**Example Fix**:
```go
// BEFORE: Unnecessary allocation (perfsprint)
return fmt.Errorf("connection limit exceeded")

// AFTER: Use errors.New for static strings
return errors.New("connection limit exceeded")
```

### 🟢 P3: Low Priority (Fix When Convenient)
**Category**: Style, formatting, comments
**Linters**: godot, whitespace, misspell
**Action**: Fix during refactoring, batch with other changes

## Configuration File

Create `.golangci.yml` in repository root for consistent CI/local runs:

```yaml
run:
  timeout: 5m
  tests: true
  build-tags: []

linters:
  disable-all: true
  enable:
    # Essential - always enabled
    - errcheck       # Unchecked errors
    - gosec         # Security issues
    - govet         # Suspicious constructs
    - ineffassign   # Ineffectual assignments
    - staticcheck   # Static analysis
    - unused        # Unused code
    
    # Code Quality - recommended
    - gocritic      # Opinionated checks
    - gocognit      # Cognitive complexity
    - goconst       # Repeated strings
    - misspell      # Spelling errors
    - revive        # Golint replacement
    - whitespace    # Whitespace issues
    
    # Context & Errors - recommended for services
    - contextcheck  # Context propagation
    - errorlint     # Error wrapping
    - errname       # Error naming
    
    # Performance - optional
    - perfsprint    # Sprint optimization
    - prealloc      # Slice preallocation
    
    # Style - optional
    - godot         # Comment punctuation
    - godox         # TODO/FIXME detection

linters-settings:
  gosec:
    excludes:
      - G115 # Integer overflow (often false positives in tests)
    severity: medium
  
  goconst:
    min-len: 3
    min-occurrences: 3
    ignore-tests: false
  
  funlen:
    lines: 100
    statements: 50
  
  gocognit:
    min-complexity: 15
  
  revive:
    rules:
      - name: exported
        severity: warning
      - name: package-comments
        severity: warning
      - name: unexported-return
        severity: warning

issues:
  exclude-rules:
    # Relax rules for test files
    - path: _test\.go
      linters:
        - gosec
        - funlen
        - gocognit
        - dupl
    
    # Relax rules for generated code
    - path: docs/docs\.go
      linters:
        - all
    
    # Allow long functions in main
    - path: cmd/.*main\.go
      linters:
        - funlen
  
  max-issues-per-linter: 0
  max-same-issues: 0
  
output:
  sort-results: true
```

## Common Analysis Workflows

### Workflow 1: Pre-Commit Check
**Goal**: Catch issues before committing
```bash
# Check only modified files (fast)
golangci-lint run --new-from-rev=HEAD

# Or check entire repo (slower but thorough)
golangci-lint run ./...
```

### Workflow 2: Pre-PR Review
**Goal**: Ensure PR meets quality standards
```bash
# Run comprehensive check
golangci-lint run --enable=gosec,gocritic,goconst,revive,errcheck,staticcheck ./...

# Verify tests pass with race detector
go test ./... -v -race -cover

# Check coverage
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

### Workflow 3: Security Audit
**Goal**: Find security vulnerabilities
```bash
# Run security-focused linters
golangci-lint run --enable=gosec ./...

# Also run govulncheck for dependency vulnerabilities
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

### Workflow 4: Baseline Establishment
**Goal**: Document current state, track improvements
```bash
# Run full analysis, save to file
golangci-lint run \
  --enable=gosec,gocritic,gocognit,goconst,revive \
  ./... > analysis_baseline_$(date +%Y%m%d).txt

# Create tracking document
cat > docs/CODE_QUALITY_BASELINE.md << 'EOF'
# Code Quality Baseline
Date: $(date)
Total Issues: $(grep -c "^" analysis_baseline_*.txt)

## By Category
$(grep -o "[a-z]*:" analysis_baseline_*.txt | sort | uniq -c | sort -rn)
EOF
```

### Workflow 5: Fix Verification
**Goal**: Confirm fixes don't introduce regressions
```bash
# Before fixes - capture baseline
golangci-lint run ./... > before.txt

# After fixes - compare
golangci-lint run ./... > after.txt

# Show diff (issues resolved)
diff before.txt after.txt

# Verify tests still pass
go test ./... -v -race -cover
```

## Integration Patterns

### GitHub Actions CI
Create `.github/workflows/lint.yml`:
```yaml
name: Lint
on: [push, pull_request]
jobs:
  golangci:
    name: lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest
          args: --enable=gosec,gocritic,goconst,revive,errcheck
```

### Pre-Commit Hook
Create `.git/hooks/pre-commit`:
```bash
#!/bin/bash
set -e

echo "Running golangci-lint..."
golangci-lint run --new-from-rev=HEAD

echo "Running tests..."
go test ./... -short

echo "✅ Pre-commit checks passed"
```

### VS Code Integration
Add to `.vscode/settings.json`:
```json
{
  "go.lintTool": "golangci-lint",
  "go.lintFlags": [
    "--enable=gosec,gocritic,goconst,revive,errcheck,staticcheck",
    "--fast"
  ],
  "go.lintOnSave": "package"
}
```

## Interpreting Results

### Output Format
```
path/to/file.go:123:45: issue description (linter-name)
    code line shown here
                                            ^
```

**Components**:
- `path/to/file.go` - File path (relative to repo root)
- `123` - Line number
- `45` - Column number (if available)
- `issue description` - What's wrong
- `(linter-name)` - Which linter reported it
- `^` - Points to issue location

### Suppressing False Positives

**Inline suppression** (use sparingly):
```go
// Suppress specific linter
//nolint:gosec // G204: Command arguments are validated by config
cmd := exec.Command(binary, args...)

// Suppress all linters (avoid)
//nolint:all
riskyCode()

// Better: Fix the root cause or add to .golangci.yml exclude-rules
```

**Configuration suppression** (preferred):
```yaml
issues:
  exclude-rules:
    - path: internal/camera/real_camera.go
      linters:
        - gosec
      text: "G204.*subprocess.*" # Suppress specific gosec rule
```

## Fix Patterns

### Pattern 1: Extract Magic Strings to Constants
```go
// BEFORE
func connect() {
    dial("http://localhost:8000")
}
func status() {
    req, _ := http.Get("http://localhost:8000/status")
}

// AFTER
const DefaultServerURL = "http://localhost:8000"

func connect() {
    dial(DefaultServerURL)
}
func status() {
    req, _ := http.Get(DefaultServerURL + "/status")
}
```

### Pattern 2: Add Missing Documentation
```go
// BEFORE
var ErrTimeout = errors.New("timeout")

func ProcessFrame(data []byte) error {
    // ...
}

// AFTER
// ErrTimeout is returned when an operation exceeds its deadline.
var ErrTimeout = errors.New("timeout")

// ProcessFrame decodes and validates a JPEG frame from the camera.
// Returns an error if the frame is malformed or exceeds size limits.
func ProcessFrame(data []byte) error {
    // ...
}
```

### Pattern 3: Propagate Context
```go
// BEFORE
func (fm *FrameManager) Start() error {
    return fm.camera.Start()
}

// AFTER
func (fm *FrameManager) Start(ctx context.Context) error {
    return fm.camera.Start(ctx)
}
```

### Pattern 4: Check Errors
```go
// BEFORE
func cleanup() {
    os.Remove(tempFile)
}

// AFTER - Option A: Handle error
func cleanup() error {
    if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("cleanup failed: %w", err)
    }
    return nil
}

// AFTER - Option B: Explicitly ignore (tests/cleanup)
func cleanup() {
    _ = os.Remove(tempFile) // Error ignored: best-effort cleanup
}
```

### Pattern 5: Fix File Permissions
```go
// BEFORE
os.WriteFile(path, data, 0644) // Too permissive
os.MkdirAll(dir, 0755)         // Too permissive

// AFTER
os.WriteFile(path, data, 0600) // Owner read/write only
os.MkdirAll(dir, 0750)         // Owner full, group read/execute
```

## Common Pitfalls

### Pitfall 1: Ignoring Test Files
**Problem**: Tests have same bugs as production code
**Solution**: Analyze tests but relax some rules (funlen, dupl, gosec)
```yaml
issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - funlen  # Test tables can be long
        - dupl    # Test setup often duplicated
```

### Pitfall 2: Fixing Everything at Once
**Problem**: Huge PRs are hard to review, risky to merge
**Solution**: Fix by priority (P0 → P1 → P2), batch by file or package
```bash
# Fix P0 issues only
golangci-lint run --enable=gosec,errcheck ./...
# [fix issues]
git commit -m "fix: P0 security issues from golangci-lint"

# Then P1 issues
golangci-lint run --enable=goconst,revive ./...
# [fix issues]
git commit -m "refactor: extract magic strings to constants"
```

### Pitfall 3: Breaking Tests While Fixing Linter Issues
**Problem**: Refactoring introduces regressions
**Solution**: Run tests after each logical group of fixes
```bash
# Fix issues in package
golangci-lint run ./internal/camera

# Immediately verify tests pass
go test ./internal/camera -v -race

# Check coverage didn't drop
go test ./internal/camera -cover
```

### Pitfall 4: Suppressing Real Issues
**Problem**: Using //nolint to avoid fixing root cause
**Solution**: Only suppress validated false positives, document why
```go
// BAD: Hiding real issue
//nolint:errcheck
os.Remove(file)

// GOOD: Documented suppression of false positive
//nolint:gosec // G204: Command args from validated config, not user input
cmd := exec.Command(binary, validatedArgs...)
```

### Pitfall 5: Not Configuring for Your Codebase
**Problem**: Using default linters may not match your standards
**Solution**: Create `.golangci.yml` tuned to your team's practices
```yaml
linters-settings:
  funlen:
    lines: 100  # Adjust based on team preference
  gocognit:
    min-complexity: 15  # Lower = stricter
  goconst:
    min-occurrences: 3  # Higher = fewer false positives
```

## Integration with GoGoMio Workflows

### For GoGoMio Contributors

**Before starting work**:
```bash
# Check baseline
golangci-lint run ./...
```

**During development**:
```bash
# Check only files you changed
golangci-lint run --new-from-rev=origin/main
```

**Before pushing PR**:
```bash
# Full check with common linters
golangci-lint run --enable=gosec,gocritic,goconst,revive,errcheck ./...

# Run full test suite
go test ./... -v -race -cover

# Check benchmarks haven't regressed
go test -bench=. -benchmem ./internal/camera ./internal/api
```

**CI Integration**: GoGoMio's GitHub Actions should run golangci-lint on every PR. If not configured, add to `.github/workflows/lint.yml`.

## Advanced Usage

### Custom Linters
For project-specific rules, consider writing custom analyzers:
```go
// internal/tools/analyze/main.go
package main

import (
    "golang.org/x/tools/go/analysis"
    "golang.org/x/tools/go/analysis/singlechecker"
)

var Analyzer = &analysis.Analyzer{
    Name: "gogomiolint",
    Doc:  "checks GoGoMio-specific patterns",
    Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
    // Custom rules for GoGoMio
    return nil, nil
}

func main() {
    singlechecker.Main(Analyzer)
}
```

### Benchmark Impact
Measure linter performance:
```bash
# Time different linter sets
time golangci-lint run --enable=gosec,errcheck ./...
time golangci-lint run --enable=gosec,gocritic,goconst,revive ./...

# Profile linter execution
golangci-lint run --enable-all --print-resources-usage ./...
```

### Batch Fixing with Auto-Fix
Some linters support automatic fixes:
```bash
# See which linters support auto-fix
golangci-lint linters | grep "auto-fix"

# Run with fixes enabled (DANGEROUS - review changes)
golangci-lint run --fix \
  --enable=gofmt,goimports,misspell,whitespace \
  ./...
```

## Metrics & Tracking

### Establish Quality Metrics
```bash
# Count total issues
golangci-lint run ./... | wc -l

# Count by severity (if using .golangci.yml severity settings)
golangci-lint run ./... | grep -c "CRITICAL"
golangci-lint run ./... | grep -c "WARNING"

# Track over time
echo "$(date +%Y-%m-%d),$(golangci-lint run ./... | wc -l)" >> .quality-metrics.csv
```

### Quality Gates for CI
```bash
#!/bin/bash
# ci-quality-gate.sh

ISSUES=$(golangci-lint run --enable=gosec,errcheck ./... | wc -l)

if [ "$ISSUES" -gt 0 ]; then
    echo "❌ Quality gate failed: $ISSUES critical issues found"
    exit 1
fi

echo "✅ Quality gate passed: 0 critical issues"
```

## Reference

### Essential Linters
- **errcheck**: Unchecked errors (correctness)
- **gosec**: Security vulnerabilities (security)
- **govet**: Suspicious constructs (correctness)
- **staticcheck**: Static analysis (correctness)
- **unused**: Dead code (maintainability)

### Recommended Linters
- **gocritic**: Opinionated best practices
- **goconst**: Repeated strings → constants
- **revive**: Style guide enforcement
- **contextcheck**: Context propagation
- **errorlint**: Error wrapping patterns

### Optional Linters
- **funlen**: Function length limits
- **gocognit**: Cognitive complexity
- **dupl**: Code duplication
- **perfsprint**: Performance optimizations
- **misspell**: Spelling errors

### Documentation
- [golangci-lint docs](https://golangci-lint.run/)
- [Linters list](https://golangci-lint.run/docs/linters/)
- [Configuration](https://golangci-lint.run/docs/configuration/file/)
- [gosec rules](https://github.com/securego/gosec#available-rules)

---

**End of Skill**
