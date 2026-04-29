# GoGoMio Skills

This directory contains domain-specific coding skills that provide expert guidance on common tasks and patterns in the GoGoMio codebase.

Each skill is a specialized guide focusing on one area of complexity. Use skills to avoid common pitfalls, understand architectural patterns, and write code that fits the project's conventions and performance constraints.

## Available Skills

### 🔄 [Go Concurrency Patterns](./go-concurrency-patterns/SKILL.md)

**Use when:** Writing or debugging concurrent Go code, managing graceful shutdown, avoiding race conditions, coordinating multiple goroutines, or using condition variables and atomic operations.

**Key patterns:**
- Shutdown orchestration and cleanup registration
- `sync.Cond` for fan-out to multiple concurrent readers (MJPEG clients)
- Atomic operations for hot-path counters (client tracking)
- Safe send-on-closed-channel prevention
- FrameBuffer synchronization model (immutable frame snapshots)
- Race detection with `-race` flag

**Critical problems this solves:**
- Panics on "send on closed channel" during shutdown
- Goroutine leaks and zombie processes
- Race between client connect/disconnect and camera start/stop
- Indefinite blocking on closed channels

---

### 📡 [HTTP Streaming Patterns](./http-streaming-patterns/SKILL.md)

**Use when:** Implementing MJPEG streaming endpoints, handling multipart HTTP responses, managing concurrent client connections, or dealing with frame publishing and backpressure.

**Key patterns:**
- MJPEG multipart boundary markers and encoding
- Frame publishing via `sync.Cond` broadcast to all waiting clients
- Flushing data to clients immediately (avoiding buffering)
- Graceful client disconnect detection
- Per-IP rate limiting and connection pooling
- FrameBuffer's io.Writer interface for direct encoder writes

**Critical problems this solves:**
- Missing or malformed boundary markers causing client frame skips
- Slow clients blocking other clients (TCP backpressure)
- Memory leaks on per-client buffering
- Connection count growing indefinitely

---

### 📷 [Camera Integration Patterns](./camera-integration-patterns/SKILL.md)

**Use when:** Implementing camera backends, managing subprocess lifecycle, handling errors, swapping real/mock cameras, or monitoring camera health.

**Key patterns:**
- Camera interface abstraction (RealCamera vs MockCamera)
- Subprocess lifecycle: Start → monitor stderr → Watch for errors → Stop with SIGTERM/SIGKILL
- Graceful fallback to mock camera on real camera failure
- Early error detection from subprocess stderr
- Health monitoring for stale frames and process crashes
- Lazy camera start (only on first client connect)

**Critical problems this solves:**
- Zombie subprocess after ungraceful shutdown
- Hangs due to subprocess stdout buffer overflow
- Race between camera stop and encoder read
- No fallback when camera hardware unavailable

---

### 🖥️ [Embedded IoT Deployment](./embedded-iot-deployment/SKILL.md)

**Use when:** Optimizing for Raspberry Pi resource constraints, building cross-architecture Docker images, tuning memory and CPU usage, or implementing graceful degradation.

**Key patterns:**
- Environment-driven configuration (MIO_RESOLUTION, MIO_FPS, etc.)
- Dynamic resolution degradation based on available RAM
- Frame rate scaling based on CPU cores
- Connection limits tied to available memory
- Multi-stage Docker builds for ARM64/ARM32v7
- Memory profiling and garbage collection tuning
- Frame buffer size calculation for constrained hardware

**Critical problems this solves:**
- OOM crashes on 256 MB Raspberry Pi
- Cross-compilation failures (wrong GOARCH)
- Unbounded GC pressure and frame buffer growth
- No graceful degradation under memory pressure
- Cold start delays on Pi hardware

---

### 🎯 [Go CLI Design](./go-cli-design/SKILL.md)

**Use when:** Building Cobra CLI commands, designing client-server interactions, formatting output for both humans and machines, or handling errors with actionable messages.

**Key patterns:**
- Cobra command tree structure with Use, Short, Long, Example
- Global flags (`--server`, `--timeout`) with env var support
- Dual output modes (`--json` flag for structured output)
- Error messages that explain the cause and suggest next steps
- HTTP client configuration with timeouts
- Command aliases for speed and discoverability

**Critical problems this solves:**
- CLI only works on localhost (no remote server support)
- Commands hang indefinitely on timeout
- Mixed human and JSON output breaking parsers
- Unclear error messages offering no debugging help
- No way to discover available commands

---

## How to Use Skills

### In Your Editor

Skills are available as context for AI coding assistants like GitHub Copilot. When working on code related to a skill's domain:

1. **Reference the skill in your question:**
   ```
   "I'm implementing a new streaming endpoint. Use the http-streaming-patterns skill 
   to review my boundary marker logic."
   ```

2. **Ask for a pattern review:**
   ```
   "Review this concurrency code using the go-concurrency-patterns skill for race conditions."
   ```

3. **Ask for help on a specific litmus check:**
   ```
   "How do I verify that my camera code passes the zombie process litmus check from 
   the camera-integration-patterns skill?"
   ```

### For Your Team

1. **Share skill links in code review comments:**
   - "This looks like it might have a race condition. See go-concurrency-patterns#litmus-checks for validation."

2. **Reference skills in PR descriptions:**
   - Use the skill checklist when submitting code that touches that domain.

3. **Onboard new developers:**
   - Point new team members to relevant skills when they're working on their first feature in that area.

---

## Skill Structure

Each skill follows a consistent format:

| Section | Purpose |
|---------|---------|
| **Description** | When to use this skill and what problem it solves |
| **Working Model** | High-level flow diagram and key questions to ask before starting |
| **Safe Defaults** | Recommended patterns and code examples (copy-paste ready) |
| **Common Pitfalls** | Real problems with root causes and fixes |
| **Checklist** | Before shipping: validations to check |
| **Verification** | How to test and validate your work |
| **Litmus Checks** | Quick pass/fail tests to verify correctness |
| **Related Files** | Links to relevant source code in the repo |

---

## Quick Reference

**By area of work:**

| Task | Skill |
|------|-------|
| Writing concurrent code, fixing race conditions | [Go Concurrency Patterns](./go-concurrency-patterns/SKILL.md) |
| Implementing HTTP streaming, MJPEG endpoints | [HTTP Streaming Patterns](./http-streaming-patterns/SKILL.md) |
| Adding camera backends, subprocess management | [Camera Integration Patterns](./camera-integration-patterns/SKILL.md) |
| Optimizing for Pi, reducing memory, cross-compilation | [Embedded IoT Deployment](./embedded-iot-deployment/SKILL.md) |
| Building CLI commands, client-server interactions | [Go CLI Design](./go-cli-design/SKILL.md) |

**By technology:**

| Technology | Skill |
|-----------|-------|
| `sync.Cond`, atomics, goroutines | [Go Concurrency Patterns](./go-concurrency-patterns/SKILL.md) |
| HTTP, multipart, io.Writer | [HTTP Streaming Patterns](./http-streaming-patterns/SKILL.md) |
| Subprocess, channels, error handling | [Camera Integration Patterns](./camera-integration-patterns/SKILL.md) |
| Docker, GOOS/GOARCH, memory profiling | [Embedded IoT Deployment](./embedded-iot-deployment/SKILL.md) |
| Cobra, HTTP client, flag parsing | [Go CLI Design](./go-cli-design/SKILL.md) |

---

## When to Create a New Skill

Consider creating a new skill if:

1. **Complexity cluster**: There's a set of related patterns that developers frequently get wrong
2. **Repeated mistakes**: The team encounters the same bug or antipattern multiple times
3. **Domain expertise required**: Deep knowledge is needed that's not obvious from docs
4. **Teachable concepts**: The skill can provide clear defaults, examples, and litmus checks

Do not create a skill for:
- Simple coding patterns (one or two examples are enough)
- Framework documentation (refer to official docs instead)
- One-off workarounds specific to a bug

---

## Feedback & Updates

Skills are living documents. If you discover:
- **A missing pattern**: Add it to the skill's "Safe Defaults" section
- **A new pitfall**: Document it with root cause and fix
- **A better example**: Update the related files reference
- **A broken link**: Fix the file path

Skills should be updated when architecture changes significantly or new patterns emerge.

---

## See Also

- [CLAUDE.md](../../CLAUDE.md) — High-level project overview, architecture, execution flow
- [docs/architecture/RACE_CONDITIONS_ANALYSIS.md](../../docs/architecture/RACE_CONDITIONS_ANALYSIS.md) — Deep dive on concurrency bugs
- [docs/architecture/FRAME_BUFFER_GC_ANALYSIS.md](../../docs/architecture/FRAME_BUFFER_GC_ANALYSIS.md) — Memory and GC analysis
- [docs/guides/DEPLOYMENT_GUIDE.md](../../docs/guides/DEPLOYMENT_GUIDE.md) — Deployment walkthrough
- [docs/guides/CLI_GUIDE.md](../../docs/guides/CLI_GUIDE.md) — CLI user guide
