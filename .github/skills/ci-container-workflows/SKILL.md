---
name: ci-container-workflows
description: Use when changing GitHub Actions workflows, CI helper scripts, Docker image builds or publication, or architecture verification.
---

# CI and Container Workflows

Read the caller workflow, reusable workflow, helper scripts, and their tests before changing CI behavior. Workflow triggers, permissions, concurrency, source selection, and publication order are part of the contract.

## Workflow contracts

- Keep workflow permissions least-privilege and preserve pinned third-party action references. Use `persist-credentials: false` for checkout unless a step explicitly needs credentials.
- The main test workflow runs Go tests with race detection and coverage, enforces the configured coverage floor, and runs static checks. Pull requests also build both container architectures and smoke-test an amd64 mock-camera image.
- The Docker publish workflow validates its selected source and requested tags, builds a candidate multi-architecture image, verifies and scans the immutable digest, then promotes that verified digest to final tags. Preserve this order and do not publish an unverified candidate.
- The published image targets `linux/amd64` and `linux/arm64`. Keep architecture claims aligned with the Dockerfile, workflows, and deployment docs.
- Reusable Kaseki workflows validate controller configuration and changed-file allowlists. Keep caller allowlists narrow and ensure task prompts agree with the permitted paths and validation command.
- When a workflow runs scripts or generates embedded UI assets, preserve the repository's Node and Go version sources (`.nvmrc` and `go.mod`) and the source-backed contract checks.

## Safe workflow changes

- Quote untrusted event inputs through environment variables and validate them before using them in shell commands, tags, paths, or outputs. Avoid interpolating untrusted GitHub expressions directly into shell source.
- Keep secrets in the narrowest step that needs them. Do not print tokens, credentials, or secret-bearing command output.
- Preserve concurrency groups that serialize publication and avoid canceling an in-progress publish.
- Update helper tests when changing tag parsing, release-source checks, manifest verification, Kaseki URL validation, or changed-file rules. Prefer testing pure helpers locally over requiring a registry publish.
- Update workflow contract tests and documentation when triggers, required checks, publication semantics, or architecture targets change.

## Verification

Run CI helper tests and lint all workflows:

```bash
bash scripts/test-ci-helpers.sh
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.11 -color
```

The helper script also runs the skill checks, Python script tests, and web checks. A full publish additionally requires workflow dispatch inputs, registry credentials, and GitHub Actions; do not substitute a local build for the workflow's digest verification and promotion checks.

## Related files

- [`.github/workflows/code-coverage-test.yml`](../../workflows/code-coverage-test.yml) — tests, coverage, static checks, and pull request container checks
- [`.github/workflows/build-docker.yml`](../../workflows/build-docker.yml) — candidate build, scan, verification, and promotion
- [`.github/workflows/codeql.yml`](../../workflows/codeql.yml) — Go and JavaScript/TypeScript analysis
- [`.github/workflows/kaseki-sweep.yml`](../../workflows/kaseki-sweep.yml) — reusable Kaseki workflow
- [`scripts/test-ci-helpers.sh`](../../../scripts/test-ci-helpers.sh) — shared CI helper validation
- [`scripts/test_workflow_contracts.py`](../../../scripts/test_workflow_contracts.py) — workflow behavior contracts
- [`scripts/verify_docker_manifest.py`](../../../scripts/verify_docker_manifest.py) — multi-architecture manifest checks
