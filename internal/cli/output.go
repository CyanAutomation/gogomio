package cli

import (
	"encoding/json"
	"fmt"
)

// FormatJSON pretty-prints JSON data
func FormatJSON(data interface{}) string {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	return string(b)
}

// FormatStatus formats status response for display
func FormatStatus(status *StatusResponse) string {
	output := fmt.Sprintf(`Status: %s
Streaming: %s
FPS: %.1f (target: %d)
Uptime: %s
Resolution: %s
JPEG Quality: %d%%`,
		status.Status,
		status.Streaming,
		status.FPS,
		status.TargetFPS,
		status.Uptime,
		status.Resolution,
		status.JPEGQuality,
	)
	return output
}

// FormatHealth formats health response for display
func FormatHealth(health *HealthResponse) string {
	return fmt.Sprintf(`Health: %s
Camera Ready: %t
Degraded: %t
Connections: %d
FPS: %.1f
Uptime: %ds
Timestamp: %s`,
		health.Status,
		health.CameraReady,
		health.Degraded,
		health.StreamConnections,
		health.FPSCurrent,
		health.UptimeSeconds,
		health.TimestampISO8601,
	)
}

// FormatHealthDetailed formats detailed health response for display
func FormatHealthDetailed(health *HealthDetailedResponse) string {
	return fmt.Sprintf(`System Health:
  Status: %s (%s)
  Message: %s
  Camera Ready: %t
  FPS: %.1f/%d
  Frames: %d
  Connections: %d/%d
  Last Frame Age: %.3fs
  Capture Failures: %d consecutive, %d total
  Error Rate: %.2f%%
  Uptime: %ds`,
		health.Status,
		health.HealthStatus,
		health.Message,
		health.CameraReady,
		health.FPSCurrent,
		health.FPSConfigured,
		health.FramesCaptured,
		health.StreamConnections,
		health.MaxConnections,
		health.LastFrameAgeSeconds,
		health.CaptureFailuresConsecutive,
		health.CaptureFailuresTotal,
		health.ErrorRatePercent,
		health.UptimeSeconds,
	)
}

// FormatConfig formats config response for display
func FormatConfig(config ConfigResponse) string {
	output := "Configuration:\n"
	for k, v := range config {
		if k[0] != '_' { // Skip deprecated fields
			output += fmt.Sprintf("  %s: %v\n", k, v)
		}
	}
	return output
}

// FormatMetrics formats metrics response for display
func FormatMetrics(metrics *MetricsResponse) string {
	return fmt.Sprintf(`Stream Metrics:
  FPS: %.1f/%d
  Frame Count: %d
  Active Connections: %d
  Last Frame Age: %.3fs
  Uptime: %ds
  Timestamp: %s`,
		metrics.FPSCurrent,
		metrics.FPSConfigured,
		metrics.FramesCaptured,
		metrics.StreamConnections,
		metrics.LastFrameAgeSeconds,
		metrics.UptimeSeconds,
		metrics.TimestampISO8601,
	)
}

// FormatDiagnostics formats diagnostics response for display
func FormatDiagnostics(diag *DiagnosticsResponse) string {
	return fmt.Sprintf(`Diagnostics:
  Version: %s
  Build Time: %s
  Camera: %s
  Backend: %s
  Uptime: %s
  Goroutines: %d
  Memory: %.1f MB`,
		diag.Version,
		diag.BuildTime,
		diag.Camera,
		diag.Backend,
		diag.Uptime,
		diag.Goroutines,
		diag.MemoryMB,
	)
}

// FormatTable creates a simple ASCII table
func FormatTable(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}

	// Calculate column widths
	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	// Build output
	var output string

	// Header
	for i, h := range headers {
		if i > 0 {
			output += " | "
		}
		output += fmt.Sprintf("%-*s", colWidths[i], h)
	}
	output += "\n"

	// Separator
	for i := range headers {
		if i > 0 {
			output += "-+-"
		}
		for j := 0; j < colWidths[i]; j++ {
			output += "-"
		}
	}
	output += "\n"

	// Rows
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				output += " | "
			}
			if i < len(colWidths) {
				output += fmt.Sprintf("%-*s", colWidths[i], cell)
			} else {
				output += cell
			}
		}
		output += "\n"
	}

	return output
}
