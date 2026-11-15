---
name: configuration-and-validation
description: Use when changing GoGoMio environment variables, configuration defaults, parsing, validation, or startup configuration behavior.
---

# Configuration and Validation

Treat configuration as an operator-facing contract. Read the loader, validator, startup path, tests, and deployment examples before changing a setting.

## Current configuration flow

- [`config.LoadFromEnv`](../../../internal/config/config.go) loads environment variables into defaults. Many malformed numeric or resolution values retain a default or fall back to a related setting rather than returning an error directly.
- [`Config.Validate`](../../../internal/config/config.go) performs strict checks for ranges and cross-field shape. The application calls validation before camera initialization and listener setup in [`cmd/gogomio/main.go`](../../../cmd/gogomio/main.go).
- Preserve the distinction between parsing/default fallback and startup validation. Do not silently change invalid-input behavior without updating tests and operator documentation.
- `MIO_RESOLUTION` selects encoded output dimensions; `MIO_SENSOR_MODE` independently selects an optional native sensor readout mode. `MIO_FPS` and `MIO_TARGET_FPS` have separate capture and processing roles.
- Secrets and security controls need extra care: do not add passwords or tokens to ordinary configuration output, examples, or logs. Coordinate auth, CORS, proxy, and pprof changes with [HTTP Security and Authentication](../http-security-and-authentication/SKILL.md).

## Changing a setting

- Update the `Config` field and env loader, validation rules, and focused tests together.
- Update [`.env.example`](../../../.env.example), the README configuration table, and deployment docs when the variable name, default, accepted values, or practical behavior changes.
- Check Docker Compose and workflow environment wiring when a setting is consumed by those paths. Do not assume a Go setting is automatically propagated into containers.
- Keep resource limits explicit and evidence-based. Do not claim automatic adaptation to memory, CPU, camera capabilities, or architecture unless code implements it.
- Avoid duplicate parsing or validation rules across startup paths. Use a shared helper when the same configuration concept genuinely has one contract.

## Verification

Run configuration tests and startup tests when validation timing or startup behavior changes:

```bash
go test ./internal/config
go test ./cmd/gogomio
```

For changes that affect a backend or runtime settings, also follow the corresponding [Camera Integration](../camera-integration-patterns/SKILL.md), [Embedded Deployment](../embedded-iot-deployment/SKILL.md), or [API and Settings](../api-and-settings-patterns/SKILL.md) guidance.

## Related files

- [`internal/config/config.go`](../../../internal/config/config.go) — environment loading, validation, and derived values
- [`internal/config/config_test.go`](../../../internal/config/config_test.go) — defaults, parsing, validation, and fallback tests
- [`cmd/gogomio/main.go`](../../../cmd/gogomio/main.go) — startup validation order
- [`.env.example`](../../../.env.example) — operator-facing environment template
- [`README.md`](../../../README.md) and [`docs/guides/DEPLOYMENT_GUIDE.md`](../../../docs/guides/DEPLOYMENT_GUIDE.md) — configuration and deployment guidance
