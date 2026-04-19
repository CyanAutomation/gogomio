package cli

import (
	"strings"
	"testing"
)

func TestFormatStatus(t *testing.T) {
	status := &StatusResponse{
		Status:      "ok",
		Streaming:   "1/2",
		FPS:         24.5,
		TargetFPS:   24,
		Uptime:      "30m",
		Resolution:  "640x480",
		JPEGQuality: 90,
	}

	output := FormatStatus(status)

	if !strings.Contains(output, "Status: ok") {
		t.Errorf("expected 'Status: ok', got: %s", output)
	}
	if !strings.Contains(output, "24.5") {
		t.Errorf("expected FPS 24.5, got: %s", output)
	}
}

func TestFormatHealth(t *testing.T) {
	health := &HealthResponse{
		Status:    "ok",
		Timestamp: "2026-04-19T16:00:00Z",
	}

	output := FormatHealth(health)

	if !strings.Contains(output, "Health: ok") {
		t.Errorf("expected 'Health: ok', got: %s", output)
	}
}

func TestFormatHealthDetailed(t *testing.T) {
	health := &HealthDetailedResponse{
		Overall:     "✓ Healthy",
		Memory:      "245MB / 512MB (47%)",
		Camera:      "✓ Connected",
		FrameBuffer: "✓ Operating",
		LastFrame:   "45ms",
	}

	output := FormatHealthDetailed(health)

	if !strings.Contains(output, "System Health:") {
		t.Errorf("expected 'System Health:' in output, got: %s", output)
	}
	if !strings.Contains(output, "Overall:") {
		t.Errorf("expected 'Overall:' in output, got: %s", output)
	}
}

func TestFormatConfig(t *testing.T) {
	config := ConfigResponse{
		"fps":          24,
		"resolution":   "[640 480]",
		"jpeg_quality": 90,
		"_deprecated":  "use v1 API", // Should be skipped
	}

	output := FormatConfig(config)

	if !strings.Contains(output, "Configuration:") {
		t.Errorf("expected 'Configuration:' in output, got: %s", output)
	}
	if !strings.Contains(output, "fps") {
		t.Errorf("expected 'fps' in output, got: %s", output)
	}
	if strings.Contains(output, "_deprecated") {
		t.Errorf("should skip deprecated fields, got: %s", output)
	}
}

func TestFormatMetrics(t *testing.T) {
	metrics := &MetricsResponse{
		FPS:               24.5,
		FrameCount:        43200,
		ActiveConnections: 1,
		MaxConnections:    2,
		AverageFrameTime:  "41.7ms",
		LastFrameTime:     "41.8ms",
		Timestamp:         "2026-04-19T16:00:00Z",
	}

	output := FormatMetrics(metrics)

	if !strings.Contains(output, "Stream Metrics:") {
		t.Errorf("expected 'Stream Metrics:' in output, got: %s", output)
	}
	if !strings.Contains(output, "24.5") {
		t.Errorf("expected FPS 24.5 in output, got: %s", output)
	}
	if !strings.Contains(output, "1/2") {
		t.Errorf("expected '1/2' connections, got: %s", output)
	}
}

func TestFormatDiagnostics(t *testing.T) {
	diag := &DiagnosticsResponse{
		Version:    "0.1.0",
		BuildTime:  "2026-04-19T12:00:00Z",
		Camera:     "mock",
		Backend:    "mock",
		Uptime:     "30m",
		Goroutines: 15,
		MemoryMB:   48.5,
	}

	output := FormatDiagnostics(diag)

	if !strings.Contains(output, "Diagnostics:") {
		t.Errorf("expected 'Diagnostics:' in output, got: %s", output)
	}
	if !strings.Contains(output, "0.1.0") {
		t.Errorf("expected version in output, got: %s", output)
	}
	if !strings.Contains(output, "15") {
		t.Errorf("expected goroutine count in output, got: %s", output)
	}
}

func TestFormatTable(t *testing.T) {
	headers := []string{"Name", "Value"}
	rows := [][]string{
		{"FPS", "24"},
		{"Resolution", "640x480"},
	}

	output := FormatTable(headers, rows)

	if !strings.Contains(output, "Name") {
		t.Errorf("expected 'Name' header, got: %s", output)
	}
	if !strings.Contains(output, "FPS") {
		t.Errorf("expected 'FPS' row, got: %s", output)
	}
	if !strings.Contains(output, "640x480") {
		t.Errorf("expected 'Resolution' value, got: %s", output)
	}
}

func TestFormatJSON(t *testing.T) {
	data := map[string]interface{}{
		"fps":    24,
		"status": "ok",
	}

	output := FormatJSON(data)

	if !strings.Contains(output, "fps") {
		t.Errorf("expected 'fps' in JSON output, got: %s", output)
	}
}
