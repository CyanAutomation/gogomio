---
name: go-cli-design
description: Use when building or extending Cobra CLI commands, designing client-server HTTP interactions, formatting output for terminals and JSON, handling errors with context, or implementing command discovery and help text. Ensures CLI commands are discoverable, fail gracefully, and provide output in both human-readable and machine-parseable formats.
---

# Go CLI Design Patterns

Use this skill when the quality of the work depends on intuitive command discovery, clear error messages, structured output formatting, and seamless client-server HTTP coordination.

Goal: ship CLIs that users can explore via `--help`, fail with actionable messages, provide both human and JSON output, and handle server unavailability gracefully. Default toward: declarative Cobra command tree, early flag validation, dual output modes (`--json` flag), timeout-aware HTTP clients, and examples in help text.

## Working Model

CLI design in GoGoMio follows a stateless query pattern:

```
User runs: gogomio status
    ↓
CLI parses flags and arguments via Cobra
    ↓
Cobra executes command's RunFunc
    ↓
Command queries running server via HTTP (default: localhost:8000)
    ↓
Server response is parsed and formatted
    ↓
Output printed to stdout (human or --json)
    ↓
Exit with status code (0 = success, non-zero = error)
```

The server (HTTP port 8000) and CLI (any machine, any shell) are decoupled. CLI never stores state; all state lives in server.

Before designing a CLI command, answer three things:

- **What endpoint does this command query?** (e.g., `/health`, `/v1/config`, `/v1/metrics/live`)
- **What output does the user expect?** (table? key-value? JSON array?)
- **What's the failure mode?** (server down? permission denied? timeout?)

## Safe Defaults

### Command Structure with Cobra

Organize commands in a tree using `cobra.Command`:

```go
var (
    // Root command (no args → help)
    rootCmd = &cobra.Command{
        Use:     "gogomio",
        Short:   "GoGoMio: MJPEG streaming server CLI",
        Long:    "Query and control a running GoGoMio server.",
        Version: Version,
    }

    // Subcommand: gogomio status
    statusCmd = &cobra.Command{
        Use:     "status",
        Short:   "Show server status and connected clients",
        Example: "gogomio status\ngogomio status --json",
        RunE:    runStatus,
    }

    // Subcommand: gogomio config
    configCmd = &cobra.Command{
        Use:     "config",
        Short:   "View or update server configuration",
        RunE:    runConfig,
    }
)

func Execute() error {
    rootCmd.AddCommand(statusCmd, configCmd, /* ... */)
    return rootCmd.Execute()
}
```

**Command organization rules:**
- Root command has `Version` set (enables `gogomio --version`)
- Each subcommand has `Use`, `Short`, `Long`, and `Example` strings
- Use `RunE` (returns error) instead of `Run` (panics on error)
- Add commands to root via `rootCmd.AddCommand()` before Execute()

### Flag Validation & Defaults

Validate flags early; provide helpful error messages:

```go
func runStatus(cmd *cobra.Command, args []string) error {
    // 1. Parse and validate flags
    serverURL, _ := cmd.Flags().GetString("server")
    jsonOutput, _ := cmd.Flags().GetBool("json")

    // 2. Apply defaults
    if serverURL == "" {
        serverURL = "http://localhost:8000"
    }

    // 3. Validate server URL
    if _, err := url.Parse(serverURL); err != nil {
        return fmt.Errorf("invalid --server URL: %w", err)
    }

    // 4. Create HTTP client with timeout
    client := &http.Client{
        Timeout: 5 * time.Second,
    }

    // 5. Query server
    status, err := queryStatus(client, serverURL)
    if err != nil {
        return fmt.Errorf("failed to query server: %w", err)
    }

    // 6. Format output
    if jsonOutput {
        return outputJSON(status)
    }
    return outputStatusTable(status)
}
```

### Global Flags

Define flags that apply to all commands (server, timeout, verbose):

```go
func init() {
    // Global flags (apply to all commands)
    rootCmd.PersistentFlags().StringP("server", "s", "http://localhost:8000",
        "Server URL (env: GOGOMIO_SERVER)")
    rootCmd.PersistentFlags().DurationP("timeout", "t", 5*time.Second,
        "Request timeout (env: GOGOMIO_TIMEOUT)")
    rootCmd.PersistentFlags().BoolP("verbose", "v", false,
        "Verbose output (env: GOGOMIO_VERBOSE)")

    // Bind to env vars for easy override
    viper.BindPFlag("server", rootCmd.PersistentFlags().Lookup("server"))
    viper.BindEnv("server", "GOGOMIO_SERVER")
}
```

### Dual Output Modes (Human + JSON)

All commands should support both `--json` and human-readable output:

```go
// Human-readable output
func outputStatusTable(status *Status) error {
    fmt.Printf("Server Status:\n")
    fmt.Printf("  Connected Clients: %d\n", status.ClientCount)
    fmt.Printf("  Frame Rate: %.1f FPS\n", status.FrameRate)
    fmt.Printf("  Resolution: %s\n", status.Resolution)
    fmt.Printf("  Uptime: %s\n", status.Uptime)
    return nil
}

// JSON output
func outputJSON(data interface{}) error {
    enc := json.NewEncoder(os.Stdout)
    enc.SetIndent("", "  ")
    return enc.Encode(data)
}

// In command's init()
statusCmd.Flags().BoolP("json", "j", false, "Output as JSON")
```

**Pattern:**
- Default: human-readable table with labels
- `--json` flag: structured JSON for scripts and other tools
- Both modes output the same data; only format differs

### Error Messages with Context

Errors should explain what happened and suggest a fix:

```go
// BAD: generic error
return err // "EOF"

// GOOD: contextual error
if err != nil {
    if os.IsTimeout(err) {
        return fmt.Errorf("server not responding (timeout: %v)\nTry: gogomio status --server http://192.168.1.100:8000", timeout)
    }
    if os.IsPermission(err) {
        return fmt.Errorf("access denied\nTry running with: sudo")
    }
    return fmt.Errorf("failed to query /health: %w\nDebug: curl http://localhost:8000/health", err)
}
```

**Error message guidelines:**
- Include the original error (`%w`)
- Explain what was being attempted
- Suggest the next action (e.g., "Try: gogomio config --help")
- If applicable, show the equivalent curl command for debugging

### HTTP Client Design

Reuse a single HTTP client for all commands; handle timeouts and retries:

```go
type Client struct {
    baseURL string
    client  *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
    return &Client{
        baseURL: baseURL,
        client: &http.Client{
            Timeout: timeout,
            Transport: &http.Transport{
                MaxIdleConns:       10,
                IdleConnTimeout:    30 * time.Second,
            },
        },
    }
}

// Helper to query endpoints
func (c *Client) Get(endpoint string, result interface{}) error {
    resp, err := c.client.Get(c.baseURL + endpoint)
    if err != nil {
        // Distinguish network errors (server down) from others
        if _, ok := err.(net.Error); ok {
            return fmt.Errorf("server unreachable at %s: %w", c.baseURL, err)
        }
        return fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := ioutil.ReadAll(resp.Body)
        return fmt.Errorf("server returned %d: %s", resp.StatusCode, body)
    }

    return json.NewDecoder(resp.Body).Decode(result)
}
```

### Command Aliases & Abbreviations

Make commands discoverable and fast to type:

```go
statusCmd.Aliases = []string{"stat", "s"} // gogomio s == gogomio status
configCmd.Aliases = []string{"cfg", "c"}  // gogomio c == gogomio config
healthCmd.Aliases = []string{"h"}         // gogomio h == gogomio health
```

**Benefits:**
- Users can type short forms: `gogomio s` instead of `gogomio status`
- First-time users run `gogomio --help` to discover full names
- Power users use aliases for speed

## Common Pitfalls

### No Server URL Configuration

**Symptom:** CLI only works on localhost; fails on remote Pi with "connection refused".

**Root cause:** Server URL is hardcoded as `localhost:8000`; no way to override.

**Fix:**
- Add global `--server` flag (short: `-s`)
- Support env var `GOGOMIO_SERVER` for scripting
- Default to `localhost:8000` if not specified

```go
rootCmd.PersistentFlags().StringP("server", "s", "http://localhost:8000",
    "Server URL (env: GOGOMIO_SERVER)")

// In command
serverURL := viper.GetString("server")
if serverURL == "" {
    serverURL = "http://localhost:8000"
}
```

### No Timeout on HTTP Client

**Symptom:** Command hangs forever if server is down; user can't interrupt.

**Root cause:** HTTP client has no timeout; read/write blocks indefinitely.

**Fix:**
- Always set `client.Timeout` (e.g., 5 seconds)
- Allow override via flag or env var: `--timeout` or `GOGOMIO_TIMEOUT`

```go
timeout, _ := cmd.Flags().GetDuration("timeout")
client := &http.Client{
    Timeout: timeout,
}
```

### Mixed Human and Structured Output

**Symptom:** User runs `gogomio status --json` but gets mixed human text and JSON.

**Root cause:** Some output is printed directly; `--json` flag is ignored in parts of code.

**Fix:**
- Check `--json` flag at start of command
- Route all output through single formatter (human or JSON)
- No ad-hoc `fmt.Printf()` calls after `--json` check

```go
func runStatus(cmd *cobra.Command, args []string) error {
    jsonOutput, _ := cmd.Flags().GetBool("json")

    status, _ := queryStatus(...)

    // Single output path based on flag
    if jsonOutput {
        return outputJSON(status)
    }
    return outputStatusTable(status)
    // No printf() after this point
}
```

### Poor Error Messages

**Symptom:** `gogomio config` returns "400 Bad Request"; user doesn't know what went wrong.

**Root cause:** HTTP error status is returned without explaining the cause.

**Fix:**
- Read response body on error
- Suggest debugging steps (e.g., "Try: curl ...")
- Include the original endpoint in error

```go
if resp.StatusCode != http.StatusOK {
    body, _ := ioutil.ReadAll(resp.Body)
    return fmt.Errorf("server error %d on %s: %s\nDebug: curl %s%s",
        resp.StatusCode, endpoint, body, baseURL, endpoint)
}
```

### No Command Examples in Help

**Symptom:** User types `gogomio config --help` but sees no usage examples.

**Root cause:** Command definition has no `Example` field.

**Fix:**
- Add `Example` field to cobra.Command with real use cases

```go
configCmd := &cobra.Command{
    Use:     "config",
    Short:   "View or update configuration",
    Example: `
  # Show current config
  gogomio config

  # Show as JSON
  gogomio config --json

  # Update resolution
  gogomio config set MIO_RESOLUTION 1280x720
    `,
    RunE: runConfig,
}
```

### Command Aliases Not Discoverable

**Symptom:** User doesn't know that `gogomio s` works; types full `status` every time.

**Root cause:** Aliases are set but not mentioned in help text.

**Fix:**
- Set `Aliases` on cobra.Command
- Mention in `Short` or `Example` that aliases exist

```go
statusCmd := &cobra.Command{
    Use:    "status",
    Short:  "Show server status [aliases: s, stat]",
    Aliases: []string{"s", "stat"},
    // ...
}
```

## CLI Checklist

Before shipping a new command:

- [ ] Command has `Use`, `Short`, `Long`, `Example` strings
- [ ] Command has `RunE` (not `Run`) for proper error handling
- [ ] `--json` flag supported for all data-returning commands
- [ ] Global `--server` and `--timeout` flags present
- [ ] HTTP client has 5-second timeout (or configurable)
- [ ] Error messages explain the cause and suggest next step
- [ ] Server URL can be overridden via flag or env var
- [ ] Manual test: `gogomio help` shows all commands
- [ ] Manual test: `gogomio <cmd> --help` shows examples
- [ ] Manual test: `gogomio <cmd> --json` outputs valid JSON
- [ ] Stress test: 100 rapid requests don't hang or leak goroutines
- [ ] Timeout test: `--timeout 1s` on slow endpoint; returns error quickly

## Verification via Tests

GoGoMio includes tests for CLI parsing and output:

```bash
# Test command parsing
go test -v ./internal/cli -run TestCommandParsing

# Test output formatting
go test -v ./internal/cli -run TestOutputFormatting

# Test server unavailability handling
go test -v ./internal/cli -run TestServerDown

# Benchmark: measure command latency
go test -v ./internal/cli -bench=. -benchmem
```

## Litmus Checks

- **Can you discover all commands via `gogomio --help`?** Run the command; output should list all subcommands with short descriptions.
- **Does `gogomio status --server 192.168.1.100:8000` work?** Verify the flag overrides the default localhost.
- **Does `gogomio status --json` output valid JSON?** Run the command and pipe to `jq` or `python -m json.tool`; should parse without errors.
- **Do error messages help debug the problem?** Stop the server; run `gogomio status`; error should suggest how to fix (e.g., "Try: gogomio --server http://pi.local:8000").
- **Does the command exit quickly on timeout?** Run `gogomio --timeout 1s status` against a slow/dead server; should return within 2 seconds, not hang.
- **Can you add a new command in 30 seconds?** Copy an existing command struct, change `Use` and `Short`, implement `RunE`; test with `gogomio <cmd> --help`.

## Related Files

- [internal/cli/commands.go](../../../internal/cli/commands.go) — Cobra command definitions
- [internal/cli/client.go](../../../internal/cli/client.go) — HTTP client for server queries
- [internal/cli/output.go](../../../internal/cli/output.go) — Output formatting (human and JSON)
- [internal/cli/commands_test.go](../../../internal/cli/commands_test.go) — Command tests
- [CLI_GUIDE.md](../../../docs/guides/CLI_GUIDE.md) — Full CLI user guide
