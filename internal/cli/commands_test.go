package cli

import (
	"bytes"
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

func TestSettingsGetCmd_PrintsAllKeysAndSpecificValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/settings" {
			t.Errorf("expected path /api/settings, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"settings":{"brightness":80,"contrast":40}}`))
	}))
	defer server.Close()

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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
		if r.URL.Path != "/api/status" {
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

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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
		if r.URL.Path != "/api/config" {
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

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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
		if r.URL.Path != "/health" {
			t.Errorf("expected path /health, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "ok",
			"timestamp": "2026-04-19T16:00:00Z"
		}`))
	}))
	defer server.Close()

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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
		if r.URL.Path != "/health/detailed" {
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

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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
		if r.URL.Path != "/snapshot.jpg" {
			t.Errorf("expected path /snapshot.jpg, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegData)
	}))
	defer server.Close()

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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
		if r.URL.Path != "/api/diagnostics" {
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

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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
		if r.URL.Path != "/metrics/live" {
			t.Errorf("expected path /metrics/live, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"fps": 24.5,
			"frame_count": 43200,
			"active_connections": 1,
			"max_connections": 2,
			"average_frame_time": "41.7ms",
			"last_frame_time": "41.8ms",
			"timestamp": "2026-04-19T16:00:00Z"
		}`))
	}))
	defer server.Close()

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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
		if r.URL.Path != "/api/stream/stop" {
			t.Errorf("expected path /api/stream/stop, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"stopped"}`))
	}))
	defer server.Close()

	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", server.URL)

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
	originalURL := os.Getenv("GOGOMIO_URL")
	t.Cleanup(func() {
		_ = os.Setenv("GOGOMIO_URL", originalURL)
	})
	_ = os.Setenv("GOGOMIO_URL", "http://localhost:1234") // Unused port

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
