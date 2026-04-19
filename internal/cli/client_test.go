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
		if r.URL.Path != "/api/status" {
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
		if r.URL.Path != "/api/config" {
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
		if r.URL.Path != "/health" {
			t.Errorf("expected path /health, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"status": "ok",
			"timestamp": "2026-04-19T16:00:00Z"
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
}

func TestGetSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/snapshot.jpg" {
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
		if r.URL.Path != "/api/diagnostics" {
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
		if r.URL.Path != "/metrics/live" {
			t.Errorf("expected path /metrics/live, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"fps": 23.8,
			"frame_count": 5712,
			"active_connections": 1,
			"max_connections": 2,
			"average_frame_time": "41.8ms",
			"last_frame_time": "42ms",
			"timestamp": "2026-04-19T16:00:00Z"
		}`)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	metrics, err := client.GetMetrics()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metrics.FPS != 23.8 {
		t.Errorf("expected FPS 23.8, got %.1f", metrics.FPS)
	}
	if metrics.FrameCount != 5712 {
		t.Errorf("expected 5712 frames, got %d", metrics.FrameCount)
	}
}

func TestSetSetting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/settings" {
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
