package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client wraps HTTP requests to the gogomio server API
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new API client with the given base URL
// If empty, defaults to http://localhost:8000
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// ClientFromEnv creates a client using GOGOMIO_URL env var or default
func ClientFromEnv() *Client {
	url := os.Getenv("GOGOMIO_URL")
	if url == "" {
		url = "http://localhost:8000"
	}
	return NewClient(url)
}

// getJSON performs a GET request and decodes JSON response
func (c *Client) getJSON(path string, v interface{}) error {
	url := c.baseURL + path
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to server at %s: %w", c.baseURL, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return nil
}

// getRaw performs a GET request and returns raw bytes
func (c *Client) getRaw(path string) ([]byte, error) {
	url := c.baseURL + path
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server at %s: %w", c.baseURL, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// postJSON performs a POST request with JSON body
func (c *Client) postJSON(path string, body interface{}, result interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + path
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to connect to server at %s: %w", c.baseURL, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// StatusResponse represents the /api/status endpoint response
type StatusResponse struct {
	Status      string  `json:"status"`
	Streaming   string  `json:"streaming"`
	FPS         float64 `json:"fps"`
	TargetFPS   int     `json:"target_fps"`
	Uptime      string  `json:"uptime"`
	Resolution  string  `json:"resolution"`
	JPEGQuality int     `json:"jpeg_quality"`
}

// GetStatus returns current streaming status
func (c *Client) GetStatus() (*StatusResponse, error) {
	var status StatusResponse
	err := c.getJSON("/v1/api/status", &status)
	return &status, err
}

// HealthResponse represents the /health endpoint response
type HealthResponse struct {
	Status            string  `json:"status"`
	CameraReady       bool    `json:"camera_ready"`
	Degraded          bool    `json:"degraded"`
	StreamConnections int     `json:"stream_connections"`
	FPSCurrent        float64 `json:"fps_current"`
	UptimeSeconds     int64   `json:"uptime_seconds"`
	TimestampISO8601  string  `json:"timestamp_iso8601"`
	APIVersion        string  `json:"api_version"`
}

// GetHealth returns basic health check
func (c *Client) GetHealth() (*HealthResponse, error) {
	var health HealthResponse
	err := c.getJSON("/v1/health", &health)
	return &health, err
}

// HealthDetailedResponse represents the /health/detailed endpoint response
type HealthDetailedResponse struct {
	Status                     string  `json:"status"`
	HealthStatus               string  `json:"health_status"`
	Message                    string  `json:"message"`
	CameraReady                bool    `json:"camera_ready"`
	Degraded                   bool    `json:"degraded"`
	UptimeSeconds              int64   `json:"uptime_seconds"`
	FPSCurrent                 float64 `json:"fps_current"`
	FPSConfigured              int     `json:"fps_configured"`
	FramesCaptured             int64   `json:"frames_captured"`
	StreamConnections          int     `json:"stream_connections"`
	LastFrameAgeSeconds        float64 `json:"last_frame_age_seconds"`
	Resolution                 string  `json:"resolution"`
	JPEGQuality                int     `json:"jpeg_quality"`
	MaxConnections             int     `json:"max_stream_connections"`
	CaptureFailuresConsecutive int64   `json:"capture_failures_consecutive"`
	CaptureFailuresTotal       int64   `json:"capture_failures_total"`
	CaptureRestartCount        int64   `json:"capture_restart_count"`
	ErrorRatePercent           float64 `json:"error_rate_percent"`
	FrameSequenceNumber        uint64  `json:"frame_sequence_number"`
	TimestampISO8601           string  `json:"timestamp_iso8601"`
	APIVersion                 string  `json:"api_version"`
}

// GetHealthDetailed returns detailed health information
func (c *Client) GetHealthDetailed() (*HealthDetailedResponse, error) {
	var health HealthDetailedResponse
	err := c.getJSON("/v1/health/detailed", &health)
	return &health, err
}

// ConfigResponse is the raw JSON response from /api/config
type ConfigResponse map[string]interface{}

// GetConfig returns the raw configuration as a map
func (c *Client) GetConfig() (ConfigResponse, error) {
	var config ConfigResponse
	err := c.getJSON("/v1/api/config", &config)
	return config, err
}

// GetSnapshot captures a single frame and returns JPEG bytes
func (c *Client) GetSnapshot() ([]byte, error) {
	return c.getRaw("/v1/snapshot.jpg")
}

// DiagnosticsResponse represents the /api/diagnostics endpoint response
type DiagnosticsResponse struct {
	Version    string                 `json:"version"`
	BuildTime  string                 `json:"build_time"`
	Camera     string                 `json:"camera"`
	Backend    string                 `json:"backend"`
	Uptime     string                 `json:"uptime"`
	Goroutines int                    `json:"goroutines"`
	MemoryMB   float64                `json:"memory_mb"`
	Config     map[string]interface{} `json:"config"`
}

// GetDiagnostics returns diagnostic information
func (c *Client) GetDiagnostics() (*DiagnosticsResponse, error) {
	var diag DiagnosticsResponse
	err := c.getJSON("/v1/api/diagnostics", &diag)
	return &diag, err
}

// MetricsResponse represents the /metrics/live endpoint response
type MetricsResponse struct {
	FPSCurrent          float64 `json:"fps_current"`
	FPSConfigured       int     `json:"fps_configured"`
	FramesCaptured      int64   `json:"frames_captured"`
	LastFrameAgeSeconds float64 `json:"last_frame_age_seconds"`
	UptimeSeconds       int64   `json:"uptime_seconds"`
	StreamConnections   int     `json:"stream_connections"`
	FrameSequenceNumber uint64  `json:"frame_sequence_number"`
	TimestampISO8601    string  `json:"timestamp_iso8601"`
	APIVersion          string  `json:"api_version"`
}

// GetMetrics returns real-time performance metrics
func (c *Client) GetMetrics() (*MetricsResponse, error) {
	var metrics MetricsResponse
	err := c.getJSON("/v1/metrics/live", &metrics)
	return &metrics, err
}

// SettingsResponse represents settings structure
type SettingsResponse map[string]interface{}

type settingsEnvelope struct {
	Settings SettingsResponse `json:"settings"`
}

// GetSettings returns all settings or a specific setting by key
func (c *Client) GetSettings(key string) (interface{}, error) {
	var envelope settingsEnvelope
	err := c.getJSON("/v1/api/settings", &envelope)
	if err != nil {
		return nil, err
	}

	settings := envelope.Settings
	if settings == nil {
		settings = SettingsResponse{}
	}

	if key == "" {
		return settings, nil
	}

	value, exists := settings[key]
	if !exists {
		return nil, fmt.Errorf("setting '%s' not found", key)
	}

	return value, nil
}

// SetSetting updates a setting value
func (c *Client) SetSetting(key string, value interface{}) error {
	body := map[string]interface{}{
		key: value,
	}
	return c.postJSON("/v1/api/settings", body, nil)
}

// StopStream stops active streams
func (c *Client) StopStream() error {
	return c.postJSON("/v1/api/stream/stop", nil, nil)
}

// ReadyResponse represents the /ready endpoint response
type ReadyResponse struct {
	Ready     bool   `json:"ready"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// GetReady checks if server is ready
func (c *Client) GetReady() (*ReadyResponse, error) {
	var ready ReadyResponse
	err := c.getJSON("/ready", &ready)
	return &ready, err
}
