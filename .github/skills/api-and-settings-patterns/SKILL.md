---
name: api-and-settings-patterns
description: Use when changing GoGoMio HTTP routes, middleware, request or response contracts, OpenAPI references, or persistent settings behavior.
---

# API and Settings Patterns

Use the current handlers, settings manager, and tests as the source of truth. Preserve compatibility deliberately when changing routes or stored settings.

## HTTP API contracts

- Routes are registered in [`internal/api/handlers.go`](../../../internal/api/handlers.go) using method-aware `http.ServeMux` patterns. `RegisterHandlers` applies CORS, logging, and rate limiting around the registered routes.
- The rate limiter applies to all routes. Only trust forwarded client addresses from CIDRs configured in `MIO_TRUSTED_PROXY_CIDRS`; do not parse forwarding headers directly in a handler.
- Keep versioned API routes and legacy aliases aligned when a change is intended to support both. Update the request and response contract in both [`docs/reference/openapi.json`](../../../docs/reference/openapi.json) and [`docs/reference/openapi.yaml`](../../../docs/reference/openapi.yaml); the server embeds these reference files.
- Preserve existing cache behavior for dynamic and error responses. Reuse the current `noStore` helper where appropriate instead of setting caching headers inconsistently.

## Persistent settings

- [`settings.Manager`](../../../internal/settings/settings.go) stores JSON in a file. Mutations read the latest on-disk state while holding the settings lock, write atomically, and update in-memory state only after persistence succeeds.
- Keep cross-process file locking and rollback behavior intact. The file lock is advisory, so it only coordinates writers that honor the same lock.
- `Get` and `GetAll` clone supported JSON-like maps and slices. Preserve that ownership boundary; callers must not be able to mutate the manager's stored maps or slices through returned values.
- Use temporary file paths in tests. Avoid relying on the default `/tmp/gogomio/settings.json` location in new tests.

## Verification

Run API and settings tests, then check that the documented API contract matches the registered handlers:

```bash
go test ./internal/api ./internal/settings
python3 scripts/test_documentation_contracts.py
```

For changes to rate limiting or trusted proxy handling, include the focused middleware tests in [`internal/api/handlers_test.go`](../../../internal/api/handlers_test.go). For persistence, concurrency, recovery, or rollback changes, use [`internal/settings/settings_test.go`](../../../internal/settings/settings_test.go) and [`internal/settings/settings_recovery_test.go`](../../../internal/settings/settings_recovery_test.go).

## Related files

- [`internal/api/handlers.go`](../../../internal/api/handlers.go) — route registration, middleware, and response behavior
- [`internal/settings/settings.go`](../../../internal/settings/settings.go) — settings persistence and value ownership
- [`internal/config/config.go`](../../../internal/config/config.go) — environment loading and validation
- [`docs/reference/openapi.json`](../../../docs/reference/openapi.json) and [`docs/reference/openapi.yaml`](../../../docs/reference/openapi.yaml) — published API contract
- [`scripts/test_documentation_contracts.py`](../../../scripts/test_documentation_contracts.py) — source-backed documentation checks
