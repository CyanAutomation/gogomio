package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the application configuration.
type Config struct {
	Resolution           [2]int `json:"resolution"`
	FPS                  int    `json:"fps"`
	TargetFPS            int    `json:"target_fps"`
	JPEGQuality          int    `json:"jpeg_quality"`
	MaxStreamConnections int    `json:"max_stream_connections"`
	Port                 int    `json:"port"`
	BindHost             string `json:"bind_host"`
	MockCamera           bool   `json:"mock_camera"`
}

// LoadFromEnv loads configuration from environment variables with defaults.
func LoadFromEnv() *Config {
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

	// Resolution
	if res := os.Getenv("MIO_RESOLUTION"); res != "" {
		if parsed, err := parseResolution(res); err == nil {
			cfg.Resolution = parsed
		}
	}

	// FPS and target FPS
	if fps := os.Getenv("MIO_FPS"); fps != "" {
		if f, err := strconv.Atoi(fps); err == nil && f > 0 {
			cfg.FPS = f
		}
	}

	if targetFPS := os.Getenv("MIO_TARGET_FPS"); targetFPS != "" {
		if f, err := strconv.Atoi(targetFPS); err == nil && f > 0 {
			cfg.TargetFPS = f
		}
	} else {
		cfg.TargetFPS = cfg.FPS
	}

	// JPEG quality
	if quality := os.Getenv("MIO_JPEG_QUALITY"); quality != "" {
		if q, err := strconv.Atoi(quality); err == nil && q >= 1 && q <= 100 {
			cfg.JPEGQuality = q
		}
	}

	// Max stream connections
	if maxConn := os.Getenv("MIO_MAX_STREAM_CONNECTIONS"); maxConn != "" {
		if m, err := strconv.Atoi(maxConn); err == nil && m > 0 {
			cfg.MaxStreamConnections = m
		}
	}

	// Port
	if port := os.Getenv("MIO_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil && p > 0 && p <= 65535 {
			cfg.Port = p
		}
	}

	// Bind host
	if host := os.Getenv("MIO_BIND_HOST"); host != "" {
		cfg.BindHost = host
	}

	// Mock camera
	if mock := os.Getenv("MOCK_CAMERA"); mock != "" {
		cfg.MockCamera = strings.ToLower(mock) == "true" || mock == "1"
	}

	return cfg
}

// parseResolution parses "WIDTHxHEIGHT" format into [2]int.
func parseResolution(s string) ([2]int, error) {
	parts := strings.Split(s, "x")
	if len(parts) != 2 {
		return [2]int{}, fmt.Errorf("invalid resolution format: %s", s)
	}

	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || width <= 0 {
		return [2]int{}, fmt.Errorf("invalid width: %s", parts[0])
	}

	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || height <= 0 {
		return [2]int{}, fmt.Errorf("invalid height: %s", parts[1])
	}

	return [2]int{width, height}, nil
}

// String returns a string representation of the config.
func (c *Config) String() string {
	data, _ := json.MarshalIndent(c, "", "  ")
	return string(data)
}

// FrameTimeout returns the timeout for waiting on a frame from the camera.
// Roughly 1.5x the frame interval to account for occasional delays.
func (c *Config) FrameTimeout() time.Duration {
	if c.TargetFPS <= 0 {
		return 5 * time.Second // Default if no FPS set
	}

	frameIntervalMS := 1000.0 / float64(c.TargetFPS)
	timeoutMS := frameIntervalMS * 1.5

	return time.Duration(int64(timeoutMS)) * time.Millisecond
}

// AddressString returns the full address string for the HTTP server.
func (c *Config) AddressString() string {
	return fmt.Sprintf("%s:%d", c.BindHost, c.Port)
}
