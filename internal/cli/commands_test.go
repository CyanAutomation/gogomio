package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/status":
			fmt.Fprintf(w, `{
				"status": "ok",
				"streaming": "1/2",
				"fps": 24.0,
				"target_fps": 24,
				"uptime": "30m",
				"resolution": "640x480",
				"jpeg_quality": 90
			}`)
		case "/api/config":
			fmt.Fprintf(w, `{
				"resolution": [640, 480],
				"fps": 24,
				"target_fps": 24,
				"jpeg_quality": 90,
				"max_stream_connections": 2,
				"current_stream_connections": 1,
				"frames_captured": 43200,
				"current_fps": 24.0,
				"last_frame_age_seconds": 0.041,
				"timestamp_iso8601": "2026-04-19T16:00:00Z",
				"api_version": "1"
			}`)
		case "/health":
			fmt.Fprintf(w, `{
				"status": "ok",
				"timestamp": "2026-04-19T16:00:00Z"
			}`)
		case "/snapshot.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0})
		case "/api/diagnostics":
			fmt.Fprintf(w, `{
				"version": "0.1.0",
				"build_time": "2026-04-19T12:00:00Z",
				"camera": "mock",
				"backend": "mock",
				"uptime": "30m",
				"goroutines": 15,
				"memory_mb": 48.5
			}`)
		case "/metrics/live":
			fmt.Fprintf(w, `{
				"fps": 24.0,
				"frame_count": 43200,
				"active_connections": 1,
				"max_connections": 2,
				"average_frame_time": "41.7ms",
				"last_frame_time": "41.8ms",
				"timestamp": "2026-04-19T16:00:00Z"
			}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

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
