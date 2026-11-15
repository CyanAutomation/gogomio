---
name: http-security-and-authentication
description: Use when changing GoGoMio HTTP authentication, route access, CORS, same-origin mutation checks, trusted proxies, or request rate limits.
---

# HTTP Security and Authentication

Treat the registered routes, middleware order, and tests as one security boundary. Read the implementation and the focused tests before changing access or client identity behavior.

## Current access and middleware contract

- Basic Auth is optional and disabled by default. `LoadBasicAuthFromEnv` accepts one configured account using `MIO_AUTH_MODE=basic`, `MIO_AUTH_USERNAME`, and `MIO_AUTH_PASSWORD_FILE`. Passwords belong in a permission-restricted file, not source, logs, command arguments, or `.env`.
- Basic Auth does not encrypt credentials or camera traffic. Keep access on a trusted network or behind a TLS-terminating proxy/VPN; do not expose the service directly to the internet.
- With auth enabled, only `GET /ready` and `GET /v1/ready` bypass authentication. Liveness, metrics, docs, the UI, streams, and legacy routes are protected too.
- The global limiter wraps authentication and applies to every route. The separate failed-credential limiter allows 20 supplied invalid credential attempts per minute per client IP; missing credentials receive a challenge but do not consume that second limit.
- Authenticated mutations require a matching `Origin`, an explicitly configured exact CORS origin, or `Sec-Fetch-Site: same-origin` when `Origin` is absent. The bundled CLI sends an `Origin` header. Do not weaken this check to make a client work; update that client to follow the contract.
- Wildcard CORS origins are not accepted. Keep preflight behavior, credential headers, and `Vary` handling consistent with the existing middleware.

## Client identity and sensitive endpoints

- Only trust forwarded client IP and HTTPS scheme information from peers in `MIO_TRUSTED_PROXY_CIDRS`. Preserve the existing proxy-chain walk and do not read `X-Forwarded-For` or `X-Forwarded-Proto` directly in handlers.
- Keep generic authentication errors and password-file errors free of credentials or secret contents. `internal/auth` bounds password-file size and compares credential digests in constant time; preserve those protections.
- `MIO_ENABLE_PPROF=true` starts unauthenticated pprof on `127.0.0.1:6060`. Keep it disabled by default and do not describe it as a remotely accessible authenticated endpoint.
- When changing middleware order, evaluate cache headers, rate-limit identity, CORS preflight, authentication, and same-origin checks together. Avoid adding a second source of truth for route policy.

## Verification

Run the authentication primitives and route/middleware tests:

```bash
go test ./internal/auth
go test ./internal/api -run 'Test(CORSHeaders|BasicAuth|RateLimitMiddleware)' -race
```

Update tests in [`internal/auth/basic_test.go`](../../../internal/auth/basic_test.go) and [`internal/api/handlers_test.go`](../../../internal/api/handlers_test.go) for any changed policy. Update the security and configuration guidance in [README.md](../../../README.md) and [`.env.example`](../../../.env.example) when the operator-facing contract changes.

## Related files

- [`internal/auth/basic.go`](../../../internal/auth/basic.go) — credential loading, password-file limits, and verification
- [`internal/api/handlers.go`](../../../internal/api/handlers.go) — middleware composition, route exemptions, proxy identity, and mutation checks
- [`internal/config/config.go`](../../../internal/config/config.go) — exact CORS origin configuration
- [`cmd/gogomio/main.go`](../../../cmd/gogomio/main.go) — pprof listener and auth initialization
- [API and Settings Patterns](../api-and-settings-patterns/SKILL.md) — route and settings contract changes
