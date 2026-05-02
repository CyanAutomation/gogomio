package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
		Status:           "ok",
		CameraReady:      true,
		TimestampISO8601: "2026-04-19T16:00:00Z",
	}

	output := FormatHealth(health)

	lines := strings.Split(strings.TrimSpace(output), "\n")
	fields := make(map[string]string, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		label := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		fields[label] = value
	}

	if got := fields["Health"]; got != "ok" {
		t.Errorf("expected Health=ok, got: %q (output=%q)", got, output)
	}
	if got := fields["Camera Ready"]; got != "true" {
		t.Errorf("expected Camera Ready=true, got: %q (output=%q)", got, output)
	}
	if got := fields["Timestamp"]; got != "2026-04-19T16:00:00Z" {
		t.Errorf("expected Timestamp=2026-04-19T16:00:00Z, got: %q (output=%q)", got, output)
	}

	// Negative assertions: key fields should not be malformed or empty.
	if strings.TrimSpace(fields["Health"]) == "" {
		t.Errorf("expected non-empty Health value, got: %q (output=%q)", fields["Health"], output)
	}
	if strings.TrimSpace(fields["Timestamp"]) == "" {
		t.Errorf("expected non-empty Timestamp value, got: %q (output=%q)", fields["Timestamp"], output)
	}
	if strings.EqualFold(fields["Timestamp"], "null") || strings.EqualFold(fields["Timestamp"], "none") {
		t.Errorf("expected concrete Timestamp representation, got: %q (output=%q)", fields["Timestamp"], output)
	}
}

func TestFormatHealthDetailed(t *testing.T) {
	health := &HealthDetailedResponse{
		Status:               "ok",
		HealthStatus:         "Excellent",
		Message:              "Camera is functioning normally",
		FPSCurrent:           24.5,
		FPSConfigured:        24,
		FramesCaptured:       43200,
		StreamConnections:    1,
		MaxConnections:       2,
		LastFrameAgeSeconds:  0.045,
		CaptureFailuresTotal: 0,
	}

	output := FormatHealthDetailed(health)

	if !strings.Contains(output, "System Health:") {
		t.Errorf("expected 'System Health:' in output, got: %s", output)
	}
	if !strings.Contains(output, "Status:") {
		t.Errorf("expected 'Status:' in output, got: %s", output)
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
		FPSCurrent:        24.5,
		FPSConfigured:     24,
		FramesCaptured:    43200,
		StreamConnections: 1,
		UptimeSeconds:     120,
		TimestampISO8601:  "2026-04-19T16:00:00Z",
	}

	output := FormatMetrics(metrics)

	if !strings.Contains(output, "Stream Metrics:") {
		t.Errorf("expected 'Stream Metrics:' in output, got: %s", output)
	}
	if !strings.Contains(output, "24.5") {
		t.Errorf("expected FPS 24.5 in output, got: %s", output)
	}
	if !strings.Contains(output, "Active Connections: 1") {
		t.Errorf("expected active connections in output, got: %s", output)
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

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("expected valid JSON output, got parse error: %v; output=%q", err, output)
	}

	if got, ok := parsed["status"].(string); !ok || got != "ok" {
		t.Errorf("expected status=ok, got: %#v", parsed["status"])
	}
	if got, ok := parsed["fps"].(float64); !ok || got != 24 {
		t.Errorf("expected fps=24, got: %#v", parsed["fps"])
	}
}

func TestSettingsGetCmd_PrintsAllKeysAndSpecificValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/settings" {
			t.Errorf("expected path /api/settings, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"settings":{"brightness":80,"contrast":40}}`))
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	allOutput, err := captureStdout(func() error {
		return settingsGetCmd.RunE(settingsGetCmd, []string{})
	})
	if err != nil {
		t.Fatalf("unexpected error running settings get all: %v", err)
	}
	if !strings.Contains(allOutput, "brightness: 80") {
		t.Errorf("expected all settings output to include brightness key, got: %s", allOutput)
	}
	if !strings.Contains(allOutput, "contrast: 40") {
		t.Errorf("expected all settings output to include contrast key, got: %s", allOutput)
	}

	brightnessOutput, err := captureStdout(func() error {
		return settingsGetCmd.RunE(settingsGetCmd, []string{"brightness"})
	})
	if err != nil {
		t.Fatalf("unexpected error running settings get brightness: %v", err)
	}
	if strings.TrimSpace(brightnessOutput) != "80" {
		t.Errorf("expected brightness output '80', got: %q", brightnessOutput)
	}
}

func captureStdout(fn func() error) (string, error) {
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}

	os.Stdout = writer
	runErr := fn()
	_ = writer.Close()
	os.Stdout = originalStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, reader)
	_ = reader.Close()
	return buf.String(), runErr
}

// TestStatusCmd tests the status command
func TestStatusCmd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/status" {
			t.Errorf("expected path /api/status, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "ok",
			"streaming": "1/2",
			"fps": 24.5,
			"target_fps": 24,
			"uptime": "1h30m",
			"resolution": "640x480",
			"jpeg_quality": 90
		}`))
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	output, err := captureStdout(func() error {
		return statusCmd.RunE(statusCmd, []string{})
	})
	if err != nil {
		t.Fatalf("status command failed: %v", err)
	}
	if !strings.Contains(output, "Status: ok") {
		t.Errorf("expected status output, got: %s", output)
	}
	if !strings.Contains(output, "24.5") {
		t.Errorf("expected FPS in output, got: %s", output)
	}
}

// TestStatusCmd_ServerError tests status command with server error
func TestStatusCmd_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("service unavailable"))
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	_, err := captureStdout(func() error {
		return statusCmd.RunE(statusCmd, []string{})
	})
	if err == nil {
		t.Fatalf("expected error for server error response")
	}
}

// TestConfigCmd tests the config command
func TestConfigCmd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/config" {
			t.Errorf("expected path /api/config, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"resolution": "[640 480]",
			"fps": 24,
			"jpeg_quality": 90,
			"max_stream_connections": 2
		}`))
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	output, err := captureStdout(func() error {
		return configCmd.RunE(configCmd, []string{})
	})
	if err != nil {
		t.Fatalf("config command failed: %v", err)
	}
	if !strings.Contains(output, "Configuration:") {
		t.Errorf("expected config output, got: %s", output)
	}
	if !strings.Contains(output, "fps") {
		t.Errorf("expected fps in output, got: %s", output)
	}
}

// TestConfigGetCmd tests the config get subcommand
func TestConfigGetCmd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"resolution": "[640 480]",
			"fps": 24,
			"jpeg_quality": 90
		}`))
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	// Test getting specific key
	output, err := captureStdout(func() error {
		return configGetCmd.RunE(configGetCmd, []string{"fps"})
	})
	if err != nil {
		t.Fatalf("config get command failed: %v", err)
	}
	if strings.TrimSpace(output) != "24" {
		t.Errorf("expected fps value 24, got: %q", output)
	}
}

// TestConfigGetCmd_InvalidKey tests config get with invalid key
func TestConfigGetCmd_InvalidKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"resolution": "[640 480]",
			"fps": 24
		}`))
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	_, err := captureStdout(func() error {
		return configGetCmd.RunE(configGetCmd, []string{"nonexistent"})
	})
	if err == nil {
		t.Fatalf("expected error for nonexistent config key")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("expected 'unknown config key' error, got: %v", err)
	}
}

// TestHealthCheckCmd tests the health check command
func TestHealthCheckCmd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health" {
			t.Errorf("expected path /health, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "ok",
			"timestamp": "2026-04-19T16:00:00Z"
		}`))
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	output, err := captureStdout(func() error {
		return healthCheckCmd.RunE(healthCheckCmd, []string{})
	})
	if err != nil {
		t.Fatalf("health check command failed: %v", err)
	}
	if !strings.Contains(output, "Health: ok") {
		t.Errorf("expected health output, got: %s", output)
	}
}

// TestHealthDetailedCmd tests the health detailed command
func TestHealthDetailedCmd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health/detailed" {
			t.Errorf("expected path /health/detailed, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"overall": "✓ Healthy",
			"memory": "245MB / 512MB (47%)",
			"camera": "✓ Connected",
			"frame_buffer": "✓ Operating",
			"last_frame": "45ms"
		}`))
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	output, err := captureStdout(func() error {
		return healthDetailedCmd.RunE(healthDetailedCmd, []string{})
	})
	if err != nil {
		t.Fatalf("health detailed command failed: %v", err)
	}
	if !strings.Contains(output, "System Health:") {
		t.Errorf("expected health detailed output, got: %s", output)
	}
}

// TestSnapshotCaptureCmd tests snapshot capture command
func TestSnapshotCaptureCmd(t *testing.T) {
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xD9} // Minimal JPEG

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/snapshot.jpg" {
			t.Errorf("expected path /snapshot.jpg, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegData)
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	// Capture stdout to verify JPEG is written
	output, err := captureStdout(func() error {
		// Redirect stdout to capture binary data
		return snapshotCaptureCmd.RunE(snapshotCaptureCmd, []string{})
	})
	if err != nil {
		t.Fatalf("snapshot capture command failed: %v", err)
	}

	// Output contains JPEG data (when redirected through bytes)
	if len(output) == 0 {
		t.Errorf("expected snapshot data in output")
	}
}

// TestSnapshotSaveCmd tests snapshot save command
func TestSnapshotSaveCmd(t *testing.T) {
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xD9} // Minimal JPEG

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegData)
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/snapshot.jpg"

	output, err := captureStdout(func() error {
		return snapshotSaveCmd.RunE(snapshotSaveCmd, []string{tmpFile})
	})
	if err != nil {
		t.Fatalf("snapshot save command failed: %v", err)
	}

	if !strings.Contains(output, "Snapshot saved to") {
		t.Errorf("expected save confirmation in output, got: %s", output)
	}

	// Verify file was written
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read saved snapshot: %v", err)
	}
	if len(data) == 0 {
		t.Errorf("expected snapshot data in file")
	}
}

// TestDiagnosticsCmd tests the diagnostics command
func TestDiagnosticsCmd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/diagnostics" {
			t.Errorf("expected path /api/diagnostics, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version": "0.1.0",
			"build_time": "2026-04-19T12:00:00Z",
			"camera": "mock",
			"backend": "mock",
			"uptime": "1h30m",
			"goroutines": 15,
			"memory_mb": 48.5
		}`))
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	output, err := captureStdout(func() error {
		return diagnosticsCmd.RunE(diagnosticsCmd, []string{})
	})
	if err != nil {
		t.Fatalf("diagnostics command failed: %v", err)
	}
	if !strings.Contains(output, "Diagnostics:") {
		t.Errorf("expected diagnostics output, got: %s", output)
	}
	if !strings.Contains(output, "0.1.0") {
		t.Errorf("expected version in output, got: %s", output)
	}
}

// TestStreamInfoCmd tests the stream info command
func TestStreamInfoCmd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics/live" {
			t.Errorf("expected path /metrics/live, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"fps_current": 24.5,
			"fps_configured": 24,
			"frames_captured": 43200,
			"last_frame_age_seconds": 0.04,
			"uptime_seconds": 120,
			"stream_connections": 1,
			"frame_sequence_number": 43201,
			"timestamp_iso8601": "2026-04-19T16:00:00Z",
			"api_version": "1"
		}`))
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	output, err := captureStdout(func() error {
		return streamInfoCmd.RunE(streamInfoCmd, []string{})
	})
	if err != nil {
		t.Fatalf("stream info command failed: %v", err)
	}
	if !strings.Contains(output, "Stream Metrics:") {
		t.Errorf("expected stream metrics output, got: %s", output)
	}
	if !strings.Contains(output, "24.5") {
		t.Errorf("expected FPS in output, got: %s", output)
	}
}

// TestStreamStopCmd tests the stream stop command
func TestStreamStopCmd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/stream/stop" {
			t.Errorf("expected path /api/stream/stop, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"stopped"}`))
	}))
	defer server.Close()

	t.Setenv("GOGOMIO_URL", server.URL)

	output, err := captureStdout(func() error {
		return streamStopCmd.RunE(streamStopCmd, []string{})
	})
	if err != nil {
		t.Fatalf("stream stop command failed: %v", err)
	}
	if !strings.Contains(output, "Streams stopped") {
		t.Errorf("expected stop confirmation, got: %s", output)
	}
}

// TestClientConnectionError tests client error handling when server is unreachable
func TestClientConnectionError(t *testing.T) {
	t.Setenv("GOGOMIO_URL", "http://localhost:1234") // Unused port

	_, err := captureStdout(func() error {
		return statusCmd.RunE(statusCmd, []string{})
	})
	if err == nil {
		t.Fatalf("expected error when server is unreachable")
	}
	if !strings.Contains(err.Error(), "failed to connect") {
		t.Errorf("expected connection error message, got: %v", err)
	}
}
