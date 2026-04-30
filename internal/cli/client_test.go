package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientFromEnv_DefaultURL(t *testing.T) {
	client := ClientFromEnv()
	if client.baseURL != "http://localhost:8000" {
		t.Errorf("expected default URL http://localhost:8000, got %s", client.baseURL)
	}
}

func TestGetStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/status" {
			t.Errorf("expected path /api/status, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"status": "ok",
			"streaming": "2/2",
			"fps": 24.5,
			"target_fps": 24,
			"uptime": "1h 30m",
			"resolution": "640x480",
			"jpeg_quality": 90
		}`)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	status, err := client.GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.Status != "ok" {
		t.Errorf("expected status 'ok', got %s", status.Status)
	}
	if status.FPS != 24.5 {
		t.Errorf("expected FPS 24.5, got %.1f", status.FPS)
	}
}

func TestGetConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/config" {
			t.Errorf("expected path /api/config, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"resolution": [640, 480],
			"fps": 24,
			"jpeg_quality": 90,
			"max_stream_connections": 2
		}`)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	config, err := client.GetConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config["fps"] != float64(24) {
		t.Errorf("expected fps 24, got %v", config["fps"])
	}
}

func TestGetHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health" {
			t.Errorf("expected path /health, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"status": "ok",
			"camera_ready": true,
			"degraded": false,
			"stream_connections": 1,
			"fps_current": 23.8,
			"uptime_seconds": 3600,
			"timestamp_iso8601": "2026-04-19T16:00:00Z",
			"api_version": "1"
		}`)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	health, err := client.GetHealth()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if health.Status != "ok" {
		t.Errorf("expected status 'ok', got %s", health.Status)
	}
	if !health.CameraReady {
		t.Errorf("expected camera_ready=true")
	}
}

func TestGetHealthDetailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health/detailed" {
			t.Errorf("expected path /v1/health/detailed, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"status": "ok",
			"health_status": "Excellent",
			"message": "Camera is functioning normally",
			"camera_ready": true,
			"degraded": false,
			"uptime_seconds": 3600,
			"fps_current": 23.8,
			"fps_configured": 24,
			"frames_captured": 5712,
			"stream_connections": 1,
			"last_frame_age_seconds": 0.042,
			"resolution": "640x480",
			"jpeg_quality": 90,
			"max_stream_connections": 2,
			"capture_failures_consecutive": 0,
			"capture_failures_total": 1,
			"capture_restart_count": 0,
			"error_rate_percent": 0.01,
			"frame_sequence_number": 5713,
			"timestamp_iso8601": "2026-04-19T16:00:00Z",
			"api_version": "1"
		}`)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	health, err := client.GetHealthDetailed()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if health.HealthStatus != "Excellent" {
		t.Errorf("expected health status Excellent, got %s", health.HealthStatus)
	}
	if health.FramesCaptured != 5712 {
		t.Errorf("expected 5712 frames, got %d", health.FramesCaptured)
	}
}

func TestGetSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/snapshot.jpg" {
			t.Errorf("expected path /snapshot.jpg, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0}) // JPEG magic bytes
	}))
	defer server.Close()

	client := NewClient(server.URL)
	frame, err := client.GetSnapshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(frame) == 0 {
		t.Errorf("expected non-empty frame data")
	}
	if frame[0] != 0xFF || frame[1] != 0xD8 {
		t.Errorf("expected JPEG magic bytes, got %x", frame[:2])
	}
}

func TestServerUnavailable(t *testing.T) {
	client := NewClient("http://127.0.0.1:1")
	_, err := client.GetStatus()
	if err == nil {
		t.Errorf("expected error for unavailable server, got nil")
	}
}

func TestServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "Internal Server Error")
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetStatus()
	if err == nil {
		t.Errorf("expected error for 500 status, got nil")
	}
}

func TestGetDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/diagnostics" {
			t.Errorf("expected path /api/diagnostics, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"version": "0.1.0",
			"build_time": "2026-04-19T12:00:00Z",
			"camera": "mock",
			"backend": "mock",
			"uptime": "1h",
			"goroutines": 12,
			"memory_mb": 45.3
		}`)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	diag, err := client.GetDiagnostics()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diag.Version != "0.1.0" {
		t.Errorf("expected version 0.1.0, got %s", diag.Version)
	}
	if diag.Goroutines != 12 {
		t.Errorf("expected 12 goroutines, got %d", diag.Goroutines)
	}
}

func TestGetMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics/live" {
			t.Errorf("expected path /metrics/live, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"fps_current": 23.8,
			"fps_configured": 24,
			"frames_captured": 5712,
			"last_frame_age_seconds": 0.042,
			"uptime_seconds": 3600,
			"stream_connections": 1,
			"frame_sequence_number": 5713,
			"timestamp_iso8601": "2026-04-19T16:00:00Z",
			"api_version": "1"
		}`)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	metrics, err := client.GetMetrics()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metrics.FPSCurrent != 23.8 {
		t.Errorf("expected FPS 23.8, got %.1f", metrics.FPSCurrent)
	}
	if metrics.FramesCaptured != 5712 {
		t.Errorf("expected 5712 frames, got %d", metrics.FramesCaptured)
	}
}

func TestSetSetting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/settings" {
			t.Errorf("expected path /api/settings, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"success": true}`)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.SetSetting("key", "value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetSettings_AllAndByKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/settings" {
			t.Errorf("expected path /api/settings, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"settings": {
				"brightness": 80,
				"contrast": 40
			}
		}`)
	}))
	defer server.Close()

	client := NewClient(server.URL)

	allSettings, err := client.GetSettings("")
	if err != nil {
		t.Fatalf("unexpected error getting all settings: %v", err)
	}
	settingsMap, ok := allSettings.(SettingsResponse)
	if !ok {
		t.Fatalf("expected map settings response, got %T", allSettings)
	}
	if settingsMap["brightness"] != float64(80) {
		t.Errorf("expected brightness=80, got %v", settingsMap["brightness"])
	}

	brightness, err := client.GetSettings("brightness")
	if err != nil {
		t.Fatalf("unexpected error getting setting by key: %v", err)
	}
	if brightness != float64(80) {
		t.Errorf("expected brightness=80, got %v", brightness)
	}
}
