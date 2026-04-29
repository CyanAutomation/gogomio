# Documentation Index

This directory is organized around **canonical, current documentation** and a separate archive for historical snapshots.

## Structure

- `docs/guides/` — user/operator runbooks and how-to guides
- `docs/reference/` — API/CLI reference artifacts
- `docs/architecture/` — design notes and technical analysis
- `docs/archive/` — point-in-time completion reports and implementation summaries

## Canonical Documentation Policy

- **Keep current operational docs in `guides/` and `reference/`.**
- **Move point-in-time completion reports to `archive/`.**
- **Use one canonical file per topic** (avoid duplicate guidance split across multiple locations).

## Canonical Entry Points

### Guides

- [CLI Guide](./guides/CLI_GUIDE.md)
- [Quick Reference](./guides/QUICK_REFERENCE.md)
- [Deployment Guide](./guides/DEPLOYMENT_GUIDE.md)
- [Raspberry Pi Build](./guides/RASPBERRY_PI_BUILD.md)
- [Docker Build & Push](./guides/DOCKER_BUILD_PUSH.md)
- [Docker Hub Deployment](./guides/DOCKER_HUB_DEPLOYMENT.md)
- [Multi-Arch Build](./guides/MULTI_ARCH_BUILD.md)

### Reference

- [OpenAPI JSON](./reference/swagger.json)
- [OpenAPI YAML](./reference/swagger.yaml)

### Architecture

- [Repository Maturity Analysis](./architecture/repo-maturity.md)
- [Race Conditions Analysis](./architecture/RACE_CONDITIONS_ANALYSIS.md)
- [Dependency Security Audit](./architecture/DEPENDENCY_SECURITY_AUDIT.md)
- [Frame Buffer GC Analysis](./architecture/FRAME_BUFFER_GC_ANALYSIS.md)
- [ARM64 Build Issue Analysis](./architecture/ARM64_BUILD_ISSUE.md)
- [MIO Sprite Migration](./architecture/MIO_SPRITE_MIGRATION.md)

### Archive

- [Comprehensive Improvement Summary](./archive/COMPREHENSIVE_IMPROVEMENT_SUMMARY.md)
- [Docker Deployment Summary](./archive/DOCKER_DEPLOYMENT_SUMMARY.md)
- [Option 2 Implementation](./archive/OPTION2_IMPLEMENTATION.md)
- [Phase 1 Summary](./archive/PHASE1_SUMMARY.md)
- [Phase 2 Complete](./archive/PHASE2_COMPLETE.md)
- [Phase 2 Implementation Details](./archive/PHASE2_IMPLEMENTATION_DETAILS.md)
- [Phase 3 Summary](./archive/PHASE3_SUMMARY.md)
- [Project Summary](./archive/PROJECT_SUMMARY.md)
- [Race Conditions Fixed](./archive/RACE_CONDITIONS_FIXED.md)
