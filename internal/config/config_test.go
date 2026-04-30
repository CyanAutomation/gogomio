package config

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestConfigFromEnv tests loading configuration from environment variables
func TestConfigFromEnv(t *testing.T) {
	// Save original env vars
	origResolution := os.Getenv("MIO_RESOLUTION")
	origFPS := os.Getenv("MIO_FPS")
	origPort := os.Getenv("MIO_PORT")

	defer func() {
		// Restore
		if origResolution != "" {
			_ = os.Setenv("MIO_RESOLUTION", origResolution)
		} else {
			_ = os.Unsetenv("MIO_RESOLUTION")
		}
		if origFPS != "" {
			_ = os.Setenv("MIO_FPS", origFPS)
		} else {
			_ = os.Unsetenv("MIO_FPS")
		}
		if origPort != "" {
			_ = os.Setenv("MIO_PORT", origPort)
		} else {
			_ = os.Unsetenv("MIO_PORT")
		}
	}()

	// Set test values
	_ = os.Setenv("MIO_RESOLUTION", "1280x720")
	_ = os.Setenv("MIO_FPS", "30")
	_ = os.Setenv("MIO_PORT", "8080")

	cfg := LoadFromEnv()

	if cfg.Resolution != [2]int{1280, 720} {
		t.Errorf("resolution is %v, want [1280 720]", cfg.Resolution)
	}
	if cfg.FPS != 30 {
		t.Errorf("FPS is %d, want 30", cfg.FPS)
	}
	if cfg.Port != 8080 {
		t.Errorf("port is %d, want 8080", cfg.Port)
	}
}

// TestConfigDefaults tests that default values are used when env vars are not set
func TestConfigDefaults(t *testing.T) {
	// Unset all config env vars
	_ = os.Unsetenv("MIO_RESOLUTION")
	_ = os.Unsetenv("MIO_FPS")
	_ = os.Unsetenv("MIO_JPEG_QUALITY")
	_ = os.Unsetenv("MIO_MAX_STREAM_CONNECTIONS")
	_ = os.Unsetenv("MIO_TARGET_FPS")
	_ = os.Unsetenv("MIO_PORT")
	_ = os.Unsetenv("MIO_BIND_HOST")

	cfg := LoadFromEnv()

	// Check defaults
	if cfg.Resolution != [2]int{640, 480} {
		t.Errorf("default resolution is %v, want [640 480]", cfg.Resolution)
	}
	if cfg.FPS != 24 {
		t.Errorf("default FPS is %d, want 24", cfg.FPS)
	}
	if cfg.JPEGQuality != 90 {
		t.Errorf("default JPEG quality is %d, want 90", cfg.JPEGQuality)
	}
	if cfg.MaxStreamConnections != 2 {
		t.Errorf("default max connections is %d, want 2", cfg.MaxStreamConnections)
	}
	if cfg.Port != 8000 {
		t.Errorf("default port is %d, want 8000", cfg.Port)
	}
	if cfg.BindHost != "0.0.0.0" {
		t.Errorf("default bind host is %s, want 0.0.0.0", cfg.BindHost)
	}
}

// TestParseResolution tests resolution string parsing
func TestParseResolution(t *testing.T) {
	tests := []struct {
		input    string
		expected [2]int
		wantErr  bool
	}{
		{"640x480", [2]int{640, 480}, false},
		{"1280x720", [2]int{1280, 720}, false},
		{"1920x1080", [2]int{1920, 1080}, false},
		{"invalid", [2]int{}, true},
		{"640", [2]int{}, true},
		{"640x", [2]int{}, true},
		{"x480", [2]int{}, true},
		{"640x480x32", [2]int{}, true},
	}

	for _, tt := range tests {
		res, err := parseResolution(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseResolution(%s) error = %v, want %v", tt.input, err != nil, tt.wantErr)
		}
		if err == nil && res != tt.expected {
			t.Errorf("parseResolution(%s) = %v, want %v", tt.input, res, tt.expected)
		}
	}
}

// TestConfigJSON tests JSON marshaling/unmarshaling
func TestConfigJSON(t *testing.T) {
	cfg := &Config{
		Resolution:           [2]int{1280, 720},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          85,
		MaxStreamConnections: 5,
		Port:                 8000,
		BindHost:             "0.0.0.0",
		MockCamera:           false,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(cfg)
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}

	// Unmarshal back
	cfg2 := &Config{}
	err = json.Unmarshal(jsonData, cfg2)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}

	// Compare
	if *cfg != *cfg2 {
		t.Errorf("Config mismatch after JSON round-trip: %+v vs %+v", cfg, cfg2)
	}
}

// TestConfigToString tests string representation
func TestConfigToString(t *testing.T) {
	cfg := &Config{
		Resolution:           [2]int{1280, 720},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 10,
		Port:                 8000,
		BindHost:             "0.0.0.0",
		MockCamera:           false,
	}

	str := cfg.String()
	if str == "" {
		t.Error("Config.String() returned empty string")
	}

	// Check that labeled key/value pairs are present.
	if !contains(str, "\"fps\": 24") {
		t.Errorf("FPS key/value pair not in string: %s", str)
	}
	if !contains(str, "\"resolution\":") || !contains(str, "1280") || !contains(str, "720") {
		t.Errorf("Resolution key/value pair not in string: %s", str)
	}

	// Negative case: a field that is not set should not be implied in output.
	cfg.BindHost = ""
	strWithoutBindHost := cfg.String()
	if contains(strWithoutBindHost, "\"bind_host\": \"0.0.0.0\"") {
		t.Errorf("unexpected default bind_host implied in string: %s", strWithoutBindHost)
	}
}

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestConfigTimeouts tests timeout computation
func TestConfigTimeouts(t *testing.T) {
	cfg := &Config{
		Resolution:           [2]int{640, 480},
		FPS:                  24,
		TargetFPS:            24,
		JPEGQuality:          90,
		MaxStreamConnections: 10,
		Port:                 8000,
		BindHost:             "0.0.0.0",
		MockCamera:           false,
	}

	// Frame timeout should be ~3 frame intervals
	timeout := cfg.FrameTimeout()
	if timeout <= 0 {
		t.Errorf("FrameTimeout is %v, want positive duration", timeout)
	}

	// Should be roughly 1/24 * 3 = ~125ms at 24 FPS
	expected := (time.Second / time.Duration(cfg.TargetFPS)) * 3
	if timeout < expected-50*time.Millisecond || timeout > expected+50*time.Millisecond {
		t.Logf("FrameTimeout is %v (expected ~%v)", timeout, expected)
	}
}

func TestFrameTimeout_HighFPSFloor(t *testing.T) {
	tests := []struct {
		name string
		fps  int
	}{
		{name: "1000fps", fps: 1000},
		{name: "5000fps", fps: 5000},
		{name: "10000fps", fps: 10000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{TargetFPS: tt.fps}
			timeout := cfg.FrameTimeout()

			if timeout <= 0 {
				t.Fatalf("FrameTimeout(%d) = %v, want positive non-zero duration", tt.fps, timeout)
			}

			if timeout < 10*time.Millisecond {
				t.Fatalf("FrameTimeout(%d) = %v, want timeout floor of at least 10ms", tt.fps, timeout)
			}
		})
	}
}

// ====================== PHASE 4: Config Validation Tests ======================

// TestConfig_InvalidResolution_ZeroWidth tests zero width in resolution
func TestConfig_InvalidResolution_ZeroWidth(t *testing.T) {
	_ = os.Setenv("MIO_RESOLUTION", "0x480")
	defer func() { _ = os.Unsetenv("MIO_RESOLUTION") }()

	cfg := LoadFromEnv()
	// Should fall back to default when invalid
	if cfg.Resolution != [2]int{640, 480} {
		t.Errorf("invalid resolution should use default, got %v", cfg.Resolution)
	}
}

// TestConfig_InvalidResolution_ZeroHeight tests zero height in resolution
func TestConfig_InvalidResolution_ZeroHeight(t *testing.T) {
	_ = os.Setenv("MIO_RESOLUTION", "640x0")
	defer func() { _ = os.Unsetenv("MIO_RESOLUTION") }()

	cfg := LoadFromEnv()
	// Should fall back to default when invalid
	if cfg.Resolution != [2]int{640, 480} {
		t.Errorf("invalid resolution should use default, got %v", cfg.Resolution)
	}
}

// TestConfig_InvalidResolution_NegativeWidth tests negative width
func TestConfig_InvalidResolution_NegativeWidth(t *testing.T) {
	_ = os.Setenv("MIO_RESOLUTION", "-640x480")
	defer func() { _ = os.Unsetenv("MIO_RESOLUTION") }()

	cfg := LoadFromEnv()
	if cfg.Resolution != [2]int{640, 480} {
		t.Errorf("invalid resolution should use default, got %v", cfg.Resolution)
	}
}

// TestConfig_InvalidResolution_Malformed tests malformed resolution string
func TestConfig_InvalidResolution_Malformed(t *testing.T) {
	tests := []string{
		"invalid",
		"640",
		"640x",
		"x480",
		"640y480",
		"640 x 480",
		"640x480x32",
		"",
		"abc x def",
	}

	for _, malformed := range tests {
		_ = os.Setenv("MIO_RESOLUTION", malformed)
		cfg := LoadFromEnv()
		if cfg.Resolution != [2]int{640, 480} {
			t.Errorf("malformed resolution %q should use default, got %v", malformed, cfg.Resolution)
		}
	}
	_ = os.Unsetenv("MIO_RESOLUTION")
}

// TestConfig_InvalidFPS_Zero tests zero FPS
func TestConfig_InvalidFPS_Zero(t *testing.T) {
	_ = os.Setenv("MIO_FPS", "0")
	defer func() { _ = os.Unsetenv("MIO_FPS") }()

	cfg := LoadFromEnv()
	// Zero FPS should be ignored, use default
	if cfg.FPS != 24 {
		t.Errorf("zero FPS should use default, got %d", cfg.FPS)
	}
}

// TestConfig_InvalidFPS_Negative tests negative FPS
func TestConfig_InvalidFPS_Negative(t *testing.T) {
	_ = os.Setenv("MIO_FPS", "-30")
	defer func() { _ = os.Unsetenv("MIO_FPS") }()

	cfg := LoadFromEnv()
	if cfg.FPS != 24 {
		t.Errorf("negative FPS should use default, got %d", cfg.FPS)
	}
}

// TestConfig_InvalidFPS_NonNumeric tests non-numeric FPS
func TestConfig_InvalidFPS_NonNumeric(t *testing.T) {
	tests := []string{
		"abc",
		"30fps",
		"30.5",
		"thirty",
		"",
	}

	for _, invalid := range tests {
		_ = os.Setenv("MIO_FPS", invalid)
		cfg := LoadFromEnv()
		if cfg.FPS != 24 {
			t.Errorf("invalid FPS %q should use default, got %d", invalid, cfg.FPS)
		}
	}
	_ = os.Unsetenv("MIO_FPS")
}

// TestConfig_TargetFPS_DefaultsToFPS tests target FPS defaults to FPS when not set
func TestConfig_TargetFPS_DefaultsToFPS(t *testing.T) {
	_ = os.Setenv("MIO_FPS", "30")
	_ = os.Unsetenv("MIO_TARGET_FPS")
	defer func() {
		_ = os.Unsetenv("MIO_FPS")
		_ = os.Unsetenv("MIO_TARGET_FPS")
	}()

	cfg := LoadFromEnv()
	if cfg.TargetFPS != cfg.FPS {
		t.Errorf("TargetFPS should default to FPS, got TargetFPS=%d, FPS=%d", cfg.TargetFPS, cfg.FPS)
	}
}

// TestConfig_InvalidTargetFPS_Zero tests zero target FPS
func TestConfig_InvalidTargetFPS_Zero(t *testing.T) {
	_ = os.Setenv("MIO_TARGET_FPS", "0")
	defer func() { _ = os.Unsetenv("MIO_TARGET_FPS") }()

	cfg := LoadFromEnv()
	// Zero target FPS should be ignored, defaults to regular FPS
	if cfg.TargetFPS == 0 {
		t.Errorf("zero TargetFPS should be ignored, got %d", cfg.TargetFPS)
	}
}

// TestConfig_InvalidJPEGQuality_Below1 tests JPEG quality below 1
func TestConfig_InvalidJPEGQuality_Below1(t *testing.T) {
	_ = os.Setenv("MIO_JPEG_QUALITY", "0")
	defer func() { _ = os.Unsetenv("MIO_JPEG_QUALITY") }()

	cfg := LoadFromEnv()
	// Should use default when outside 1-100 range
	if cfg.JPEGQuality != 90 {
		t.Errorf("invalid JPEG quality should use default, got %d", cfg.JPEGQuality)
	}
}

// TestConfig_InvalidJPEGQuality_Above100 tests JPEG quality above 100
func TestConfig_InvalidJPEGQuality_Above100(t *testing.T) {
	_ = os.Setenv("MIO_JPEG_QUALITY", "101")
	defer func() { _ = os.Unsetenv("MIO_JPEG_QUALITY") }()

	cfg := LoadFromEnv()
	if cfg.JPEGQuality != 90 {
		t.Errorf("JPEG quality > 100 should use default, got %d", cfg.JPEGQuality)
	}
}

// TestConfig_InvalidJPEGQuality_Negative tests negative JPEG quality
func TestConfig_InvalidJPEGQuality_Negative(t *testing.T) {
	_ = os.Setenv("MIO_JPEG_QUALITY", "-50")
	defer func() { _ = os.Unsetenv("MIO_JPEG_QUALITY") }()

	cfg := LoadFromEnv()
	if cfg.JPEGQuality != 90 {
		t.Errorf("negative JPEG quality should use default, got %d", cfg.JPEGQuality)
	}
}

// TestConfig_InvalidJPEGQuality_NonNumeric tests non-numeric JPEG quality
func TestConfig_InvalidJPEGQuality_NonNumeric(t *testing.T) {
	tests := []string{
		"abc",
		"90%",
		"ninety",
		"",
	}

	for _, invalid := range tests {
		_ = os.Setenv("MIO_JPEG_QUALITY", invalid)
		cfg := LoadFromEnv()
		if cfg.JPEGQuality != 90 {
			t.Errorf("invalid JPEG quality %q should use default, got %d", invalid, cfg.JPEGQuality)
		}
	}
	_ = os.Unsetenv("MIO_JPEG_QUALITY")
}

// TestConfig_ValidJPEGQuality_BoundaryValues tests valid JPEG quality boundary values
func TestConfig_ValidJPEGQuality_BoundaryValues(t *testing.T) {
	tests := []int{1, 50, 100}

	for _, quality := range tests {
		qualityStr := fmt.Sprintf("%d", quality)
		_ = os.Setenv("MIO_JPEG_QUALITY", qualityStr)
		cfg := LoadFromEnv()
		if cfg.JPEGQuality != quality {
			t.Errorf("JPEG quality %d should be accepted, got %d", quality, cfg.JPEGQuality)
		}
	}
	_ = os.Unsetenv("MIO_JPEG_QUALITY")
}

// TestConfig_InvalidPort_Zero tests port 0
func TestConfig_InvalidPort_Zero(t *testing.T) {
	_ = os.Setenv("MIO_PORT", "0")
	defer func() { _ = os.Unsetenv("MIO_PORT") }()

	cfg := LoadFromEnv()
	// Port 0 should use default
	if cfg.Port != 8000 {
		t.Errorf("port 0 should use default, got %d", cfg.Port)
	}
}

// TestConfig_InvalidPort_Negative tests negative port
func TestConfig_InvalidPort_Negative(t *testing.T) {
	_ = os.Setenv("MIO_PORT", "-8000")
	defer func() { _ = os.Unsetenv("MIO_PORT") }()

	cfg := LoadFromEnv()
	if cfg.Port != 8000 {
		t.Errorf("negative port should use default, got %d", cfg.Port)
	}
}

// TestConfig_InvalidPort_TooHigh tests port > 65535
func TestConfig_InvalidPort_TooHigh(t *testing.T) {
	_ = os.Setenv("MIO_PORT", "65536")
	defer func() { _ = os.Unsetenv("MIO_PORT") }()

	cfg := LoadFromEnv()
	if cfg.Port != 8000 {
		t.Errorf("port > 65535 should use default, got %d", cfg.Port)
	}
}

// TestConfig_InvalidPort_NonNumeric tests non-numeric port
func TestConfig_InvalidPort_NonNumeric(t *testing.T) {
	tests := []string{
		"abc",
		"8000port",
		"eight-thousand",
		"",
		"8000.5",
	}

	for _, invalid := range tests {
		_ = os.Setenv("MIO_PORT", invalid)
		cfg := LoadFromEnv()
		if cfg.Port != 8000 {
			t.Errorf("invalid port %q should use default, got %d", invalid, cfg.Port)
		}
	}
	_ = os.Unsetenv("MIO_PORT")
}

// TestConfig_ValidPort_BoundaryValues tests valid port boundary values
func TestConfig_ValidPort_BoundaryValues(t *testing.T) {
	tests := []int{1, 8000, 65535}

	for _, port := range tests {
		portStr := fmt.Sprintf("%d", port)
		_ = os.Setenv("MIO_PORT", portStr)
		cfg := LoadFromEnv()
		if cfg.Port != port {
			t.Errorf("port %d should be accepted, got %d", port, cfg.Port)
		}
	}
	_ = os.Unsetenv("MIO_PORT")
}

// TestConfig_InvalidMaxConnections_Zero tests max connections 0
func TestConfig_InvalidMaxConnections_Zero(t *testing.T) {
	_ = os.Setenv("MIO_MAX_STREAM_CONNECTIONS", "0")
	defer func() { _ = os.Unsetenv("MIO_MAX_STREAM_CONNECTIONS") }()

	cfg := LoadFromEnv()
	// Zero should use default
	if cfg.MaxStreamConnections != 2 {
		t.Errorf("max connections 0 should use default, got %d", cfg.MaxStreamConnections)
	}
}

// TestConfig_InvalidMaxConnections_Negative tests negative max connections
func TestConfig_InvalidMaxConnections_Negative(t *testing.T) {
	_ = os.Setenv("MIO_MAX_STREAM_CONNECTIONS", "-5")
	defer func() { _ = os.Unsetenv("MIO_MAX_STREAM_CONNECTIONS") }()

	cfg := LoadFromEnv()
	if cfg.MaxStreamConnections != 2 {
		t.Errorf("negative max connections should use default, got %d", cfg.MaxStreamConnections)
	}
}

// TestConfig_InvalidMaxConnections_NonNumeric tests non-numeric max connections
func TestConfig_InvalidMaxConnections_NonNumeric(t *testing.T) {
	tests := []string{
		"abc",
		"5 connections",
		"five",
		"",
	}

	for _, invalid := range tests {
		_ = os.Setenv("MIO_MAX_STREAM_CONNECTIONS", invalid)
		cfg := LoadFromEnv()
		if cfg.MaxStreamConnections != 2 {
			t.Errorf("invalid max connections %q should use default, got %d", invalid, cfg.MaxStreamConnections)
		}
	}
	_ = os.Unsetenv("MIO_MAX_STREAM_CONNECTIONS")
}

// TestConfig_MockCamera_EnabledViaEnv tests mock camera is enabled via env var
func TestConfig_MockCamera_EnabledViaEnv(t *testing.T) {
	tests := []struct {
		env      string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"false", false},
		{"False", false},
		{"0", false},
		{"", false},
		{"yes", false}, // Invalid should use default (false)
	}

	for _, tt := range tests {
		if tt.env != "" {
			_ = os.Setenv("MOCK_CAMERA", tt.env)
		} else {
			_ = os.Unsetenv("MOCK_CAMERA")
		}

		cfg := LoadFromEnv()
		if cfg.MockCamera != tt.expected {
			t.Errorf("MockCamera env=%q expected %v, got %v", tt.env, tt.expected, cfg.MockCamera)
		}
	}
	_ = os.Unsetenv("MOCK_CAMERA")
}

// TestConfig_AddressString tests address string formatting
func TestConfig_AddressString(t *testing.T) {
	cfg := &Config{
		Port:     8080,
		BindHost: "127.0.0.1",
	}

	addr := cfg.AddressString()
	if addr != "127.0.0.1:8080" {
		t.Errorf("AddressString() = %q, want %q", addr, "127.0.0.1:8080")
	}
}

// TestConfig_FrameTimeout_EdgeCases tests frame timeout with various FPS values
func TestConfig_FrameTimeout_EdgeCases(t *testing.T) {
	tests := []struct {
		fps       int
		minExpect time.Duration
		maxExpect time.Duration
	}{
		{1, 2800 * time.Millisecond, 3200 * time.Millisecond}, // 1 FPS: ~3000ms
		{24, 100 * time.Millisecond, 150 * time.Millisecond},  // 24 FPS: ~125ms
		{60, 40 * time.Millisecond, 60 * time.Millisecond},    // 60 FPS: ~50ms
		{0, 4900 * time.Millisecond, 5100 * time.Millisecond}, // 0 FPS: default 5s
	}

	for _, tt := range tests {
		cfg := &Config{TargetFPS: tt.fps}
		timeout := cfg.FrameTimeout()

		if timeout < tt.minExpect || timeout > tt.maxExpect {
			t.Errorf("FrameTimeout for %d FPS = %v, want between %v and %v", tt.fps, timeout, tt.minExpect, tt.maxExpect)
		}
	}
}
