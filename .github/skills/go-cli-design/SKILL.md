---
name: go-cli-design
description: Design and maintain GoGoMio's standard-library CLI dispatcher and HTTP client commands.
---

# Go CLI Design

Use this guide when changing `internal/cli/` or the CLI entrypoint in `cmd/gogomio/`.

## Current structure

- `cmd/gogomio/main.go` selects server mode or CLI mode.
- `internal/cli/commands.go` dispatches command and subcommand names, prints help, validates required arguments, and calls command functions.
- `internal/cli/client.go` owns HTTP requests to the running server.
- `internal/cli/output.go` formats responses for people.
- `ClientFromEnv` reads `GOGOMIO_URL` (default `http://localhost:8000`); the HTTP client timeout is currently five seconds.

## Guidelines

- Keep dispatch explicit and small. Add a command branch and a focused command function instead of introducing a command framework.
- Keep the existing command names and endpoint paths stable unless a change is intentional and documented.
- The dispatcher currently supports help flags, not global `--server`, `--timeout`, or `--json` options. Add and document such options only as an intentional CLI feature.
- Show help for `-h`, `--help`, and `help`; return useful errors for unknown commands and invalid required arguments.
- Keep HTTP timeouts and response-body cleanup in the client layer.
- Keep output formatting separate from request handling where practical.
- Test the dispatcher with help, unknown commands, nested commands, and required-argument cases. Test command behavior with `httptest` servers.

## Verification

Run the CLI package tests for dispatch, command behavior, and HTTP responses. If changing how the executable chooses server or CLI mode, also run the entrypoint tests:

```bash
go test ./internal/cli
go test ./cmd/gogomio
```

See [`internal/cli/commands_test.go`](../../../internal/cli/commands_test.go), [`internal/cli/client_test.go`](../../../internal/cli/client_test.go), and [`cmd/gogomio/main_test.go`](../../../cmd/gogomio/main_test.go).
