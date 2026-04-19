package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// RootCmd is the main entry point for the CLI
var RootCmd = &cobra.Command{
	Use:   "gogomio",
	Short: "gogomio - Motion In Ocean streaming server",
	Long:  "A high-performance MJPEG streaming server for Raspberry Pi CSI cameras",
}

// Execute runs the CLI command
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current streaming status",
	Long:  `Display current streaming status, FPS, resolution, and configuration`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := ClientFromEnv()
		status, err := client.GetStatus()
		if err != nil {
			return err
		}
		fmt.Println(FormatStatus(status))
		return nil
	},
}

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  `Get or display configuration settings`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default to 'get' if no subcommand specified
		client := ClientFromEnv()
		config, err := client.GetConfig()
		if err != nil {
			return err
		}
		fmt.Println(FormatConfig(config))
		return nil
	},
}

// configGetCmd gets a specific config value
var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a configuration value",
	Long:  `Get a specific configuration value or all config if no key specified`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := ClientFromEnv()

		config, err := client.GetConfig()
		if err != nil {
			return err
		}

		if len(args) == 0 {
			// Display all config
			for k, v := range config {
				fmt.Printf("%s: %v\n", k, v)
			}
			return nil
		}

		key := args[0]
		value, exists := config[key]
		if !exists {
			return fmt.Errorf("unknown config key: %s", key)
		}

		fmt.Println(value)
		return nil
	},
}

// snapshotCmd represents the snapshot command
var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Capture a snapshot",
	Long:  `Capture a single frame from the camera`,
}

// snapshotCaptureCmd captures and outputs to stdout
var snapshotCaptureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Capture and output to stdout",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := ClientFromEnv()
		frame, err := client.GetSnapshot()
		if err != nil {
			return err
		}
		os.Stdout.Write(frame)
		return nil
	},
}

// snapshotSaveCmd captures and saves to file
var snapshotSaveCmd = &cobra.Command{
	Use:   "save [path]",
	Short: "Capture and save to file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := ClientFromEnv()
		frame, err := client.GetSnapshot()
		if err != nil {
			return err
		}
		if err := os.WriteFile(args[0], frame, 0644); err != nil {
			return fmt.Errorf("failed to save snapshot: %w", err)
		}
		fmt.Printf("Snapshot saved to %s\n", args[0])
		return nil
	},
}

// healthCmd represents the health command
var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check system health",
	Long:  `Display health status of the gogomio system`,
}

// healthCheckCmd performs a quick health check
var healthCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Perform a health check",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := ClientFromEnv()
		health, err := client.GetHealth()
		if err != nil {
			return err
		}
		fmt.Println(FormatHealth(health))
		return nil
	},
}

// healthDetailedCmd shows detailed health information
var healthDetailedCmd = &cobra.Command{
	Use:   "detailed",
	Short: "Show detailed health information",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := ClientFromEnv()
		health, err := client.GetHealthDetailed()
		if err != nil {
			return err
		}
		fmt.Println(FormatHealthDetailed(health))
		return nil
	},
}

// streamCmd represents the stream command
var streamCmd = &cobra.Command{
	Use:   "stream",
	Short: "Manage streaming",
	Long:  `Manage streaming operations`,
}

// streamInfoCmd shows stream metrics
var streamInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show stream metrics",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := ClientFromEnv()
		metrics, err := client.GetMetrics()
		if err != nil {
			return err
		}
		fmt.Println(FormatMetrics(metrics))
		return nil
	},
}

// streamStopCmd stops active streams
var streamStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop active streams",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := ClientFromEnv()
		if err := client.StopStream(); err != nil {
			return err
		}
		fmt.Println("Streams stopped")
		return nil
	},
}

// diagnosticsCmd shows diagnostic information
var diagnosticsCmd = &cobra.Command{
	Use:   "diagnostics",
	Short: "Show diagnostic information",
	Long:  `Display comprehensive diagnostic information about the system`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := ClientFromEnv()
		diag, err := client.GetDiagnostics()
		if err != nil {
			return err
		}
		fmt.Println(FormatDiagnostics(diag))
		return nil
	},
}

// settingsCmd represents the settings command
var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage settings",
	Long:  `Get or set persistent settings`,
}

// settingsGetCmd gets a setting value
var settingsGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a setting value",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := ClientFromEnv()

		if len(args) == 0 {
			settings, err := client.GetSettings("")
			if err != nil {
				return err
			}
			if settingsMap, ok := settings.(map[string]interface{}); ok {
				for k, v := range settingsMap {
					fmt.Printf("%s: %v\n", k, v)
				}
			} else {
				fmt.Printf("%v\n", settings)
			}
			return nil
		}

		value, err := client.GetSettings(args[0])
		if err != nil {
			return err
		}
		fmt.Printf("%v\n", value)
		return nil
	},
}

// settingsSetCmd sets a setting value
var settingsSetCmd = &cobra.Command{
	Use:   "set KEY=VALUE",
	Short: "Set a setting value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse KEY=VALUE format
		arg := args[0]
		key, value := "", ""
		for i, c := range arg {
			if c == '=' {
				key = arg[:i]
				value = arg[i+1:]
				break
			}
		}
		if key == "" {
			return fmt.Errorf("invalid format, use KEY=VALUE")
		}

		client := ClientFromEnv()
		if err := client.SetSetting(key, value); err != nil {
			return err
		}
		fmt.Printf("Setting '%s' updated to '%s'\n", key, value)
		return nil
	},
}

// versionCmd shows version information
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := ClientFromEnv()
		diag, err := client.GetDiagnostics()
		if err != nil {
			return err
		}
		fmt.Printf("gogomio version %s\n", diag.Version)
		fmt.Printf("Build Time: %s\n", diag.BuildTime)
		return nil
	},
}

func init() {
	// Add top-level commands
	RootCmd.AddCommand(statusCmd)
	RootCmd.AddCommand(configCmd)
	RootCmd.AddCommand(snapshotCmd)
	RootCmd.AddCommand(healthCmd)
	RootCmd.AddCommand(streamCmd)
	RootCmd.AddCommand(diagnosticsCmd)
	RootCmd.AddCommand(settingsCmd)
	RootCmd.AddCommand(versionCmd)

	// Add config subcommands
	configCmd.AddCommand(configGetCmd)

	// Add snapshot subcommands
	snapshotCmd.AddCommand(snapshotCaptureCmd)
	snapshotCmd.AddCommand(snapshotSaveCmd)

	// Add health subcommands
	healthCmd.AddCommand(healthCheckCmd)
	healthCmd.AddCommand(healthDetailedCmd)

	// Add stream subcommands
	streamCmd.AddCommand(streamInfoCmd)
	streamCmd.AddCommand(streamStopCmd)

	// Add settings subcommands
	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsSetCmd)
}
