package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const rootHelp = `Usage: gogomio <command> [arguments]

Commands:
  status                 Show current streaming status
  config [get [key]]     Show configuration or get a value
  snapshot capture       Capture a frame to stdout
  snapshot save PATH     Capture a frame to a file
  health check           Check system health
  health detailed        Show detailed health information
  stream info            Show stream metrics
  stream stop            Stop active streams
  diagnostics            Show diagnostic information
  settings get [key]     Show settings or get a value
  settings set KEY=VALUE Update a setting
  version                Show version information

Use "gogomio <command> --help" for command help.`

var commandHelp = map[string]string{
	"config":           "Usage: gogomio config [get [key]]\nShow configuration or get a configuration value.",
	"config get":       "Usage: gogomio config get [key]\nGet all configuration values or one configuration value.",
	"snapshot":         "Usage: gogomio snapshot <capture|save PATH>\nCapture a frame from the camera.",
	"snapshot capture": "Usage: gogomio snapshot capture\nCapture a frame and write it to stdout.",
	"snapshot save":    "Usage: gogomio snapshot save PATH\nCapture a frame and save it to PATH.",
	"health":           "Usage: gogomio health <check|detailed>\nCheck system health.",
	"health check":     "Usage: gogomio health check\nPerform a quick health check.",
	"health detailed":  "Usage: gogomio health detailed\nShow detailed health information.",
	"stream":           "Usage: gogomio stream <info|stop>\nManage stream operations.",
	"stream info":      "Usage: gogomio stream info\nShow stream metrics.",
	"stream stop":      "Usage: gogomio stream stop\nStop active streams.",
	"settings":         "Usage: gogomio settings <get [key]|set KEY=VALUE>\nGet or set persistent settings.",
	"settings get":     "Usage: gogomio settings get [key]\nGet all settings or one setting value.",
	"settings set":     "Usage: gogomio settings set KEY=VALUE\nSet a persistent setting.",
	"status":           "Usage: gogomio status\nShow current streaming status.",
	"diagnostics":      "Usage: gogomio diagnostics\nShow diagnostic information.",
	"version":          "Usage: gogomio version\nShow version information.",
}

// Execute runs the command line interface and exits non-zero when a command fails.
func Execute() {
	if err := dispatch(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func dispatch(args []string, output io.Writer) error {
	if len(args) == 0 {
		_, err := fmt.Fprintln(output, rootHelp)
		return err
	}

	if args[0] == "help" {
		return writeHelp(args[1:], output)
	}
	for i, arg := range args {
		if arg == "-h" || arg == "--help" {
			return writeHelp(args[:i], output)
		}
	}

	command, rest := args[0], args[1:]
	switch command {
	case "status":
		return runStatus(rest)
	case "config":
		if len(rest) == 0 {
			return runConfig(rest)
		}
		if rest[0] != "get" {
			return unknownSubcommand("config", rest[0])
		}
		return runConfigGet(rest[1:])
	case "snapshot":
		if len(rest) == 0 {
			return missingSubcommand("snapshot", "capture, save")
		}
		switch rest[0] {
		case "capture":
			return runSnapshotCapture(rest[1:])
		case "save":
			return runSnapshotSave(rest[1:])
		default:
			return unknownSubcommand("snapshot", rest[0])
		}
	case "health":
		if len(rest) == 0 {
			return missingSubcommand("health", "check, detailed")
		}
		switch rest[0] {
		case "check":
			return runHealthCheck(rest[1:])
		case "detailed":
			return runHealthDetailed(rest[1:])
		default:
			return unknownSubcommand("health", rest[0])
		}
	case "stream":
		if len(rest) == 0 {
			return missingSubcommand("stream", "info, stop")
		}
		switch rest[0] {
		case "info":
			return runStreamInfo(rest[1:])
		case "stop":
			return runStreamStop(rest[1:])
		default:
			return unknownSubcommand("stream", rest[0])
		}
	case "diagnostics":
		return runDiagnostics(rest)
	case "settings":
		if len(rest) == 0 {
			return missingSubcommand("settings", "get, set")
		}
		switch rest[0] {
		case "get":
			return runSettingsGet(rest[1:])
		case "set":
			return runSettingsSet(rest[1:])
		default:
			return unknownSubcommand("settings", rest[0])
		}
	case "version":
		return runVersion(rest)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", command, rootHelp)
	}
}

func writeHelp(target []string, output io.Writer) error {
	if len(target) == 0 {
		_, err := fmt.Fprintln(output, rootHelp)
		return err
	}
	text, ok := commandHelp[strings.Join(target, " ")]
	if !ok {
		return fmt.Errorf("unknown help topic %q", strings.Join(target, " "))
	}
	_, err := fmt.Fprintln(output, text)
	return err
}

func unknownSubcommand(parent, name string) error {
	return fmt.Errorf("unknown %s subcommand %q", parent, name)
}

func missingSubcommand(parent, choices string) error {
	return fmt.Errorf("%s requires a subcommand (%s)", parent, choices)
}

func requireOneArgument(command string, args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("%s requires exactly one argument", command)
	}
	return args[0], nil
}

func runStatus(_ []string) error {
	status, err := ClientFromEnv().GetStatus()
	if err != nil {
		return err
	}
	fmt.Println(FormatStatus(status))
	return nil
}

func runConfig(_ []string) error {
	config, err := ClientFromEnv().GetConfig()
	if err != nil {
		return err
	}
	fmt.Println(FormatConfig(config))
	return nil
}

func runConfigGet(args []string) error {
	config, err := ClientFromEnv().GetConfig()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		for key, value := range config {
			fmt.Printf("%s: %v\n", key, value)
		}
		return nil
	}
	value, exists := config[args[0]]
	if !exists {
		return fmt.Errorf("unknown config key: %s", args[0])
	}
	fmt.Println(value)
	return nil
}

func runSnapshotCapture(_ []string) error {
	frame, err := ClientFromEnv().GetSnapshot()
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(frame)
	return err
}

func runSnapshotSave(args []string) error {
	path, err := requireOneArgument("snapshot save", args)
	if err != nil {
		return err
	}
	frame, err := ClientFromEnv().GetSnapshot()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, frame, 0644); err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}
	fmt.Printf("Snapshot saved to %s\n", path)
	return nil
}

func runHealthCheck(_ []string) error {
	health, err := ClientFromEnv().GetHealth()
	if err != nil {
		return err
	}
	fmt.Println(FormatHealth(health))
	return nil
}

func runHealthDetailed(_ []string) error {
	health, err := ClientFromEnv().GetHealthDetailed()
	if err != nil {
		return err
	}
	fmt.Println(FormatHealthDetailed(health))
	return nil
}

func runStreamInfo(_ []string) error {
	metrics, err := ClientFromEnv().GetMetrics()
	if err != nil {
		return err
	}
	fmt.Println(FormatMetrics(metrics))
	return nil
}

func runStreamStop(_ []string) error {
	if err := ClientFromEnv().StopStream(); err != nil {
		return err
	}
	fmt.Println("Streams stopped")
	return nil
}

func runDiagnostics(_ []string) error {
	diagnostics, err := ClientFromEnv().GetDiagnostics()
	if err != nil {
		return err
	}
	fmt.Println(FormatDiagnostics(diagnostics))
	return nil
}

func runSettingsGet(args []string) error {
	key := ""
	if len(args) > 0 {
		key = args[0]
	}
	settings, err := ClientFromEnv().GetSettings(key)
	if err != nil {
		return err
	}
	if key == "" {
		settingsMap, ok := settings.(SettingsResponse)
		if !ok {
			return fmt.Errorf("unexpected settings response type: %T", settings)
		}
		for name, value := range settingsMap {
			fmt.Printf("%s: %v\n", name, value)
		}
		return nil
	}
	fmt.Printf("%v\n", settings)
	return nil
}

func runSettingsSet(args []string) error {
	arg, err := requireOneArgument("settings set", args)
	if err != nil {
		return err
	}
	key, value, found := strings.Cut(arg, "=")
	if !found || key == "" {
		return fmt.Errorf("invalid format, use KEY=VALUE")
	}
	if err := ClientFromEnv().SetSetting(key, value); err != nil {
		return err
	}
	fmt.Printf("Setting '%s' updated to '%s'\n", key, value)
	return nil
}

func runVersion(_ []string) error {
	diagnostics, err := ClientFromEnv().GetDiagnostics()
	if err != nil {
		return err
	}
	fmt.Printf("gogomio version %s\n", diagnostics.Version)
	fmt.Printf("Build Time: %s\n", diagnostics.BuildTime)
	return nil
}
