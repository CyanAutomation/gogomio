package config

import (
	"encoding/json"
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

	// Frame timeout should be ~2 frame intervals
	timeout := cfg.FrameTimeout()
	if timeout <= 0 {
		t.Errorf("FrameTimeout is %v, want positive duration", timeout)
	}

	// Should be roughly 1/24 * 2 = ~83ms at 24 FPS
	frameIntervalMS := 1000.0 / float64(cfg.TargetFPS)
	expected := time.Duration(int64(frameIntervalMS*2)) * time.Millisecond
	if timeout < expected-50*time.Millisecond || timeout > expected+50*time.Millisecond {
		t.Logf("FrameTimeout is %v (expected ~%v)", timeout, expected)
	}
}
