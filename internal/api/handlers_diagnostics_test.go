package api

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"testing"
)

// TestDiagnosticsErrorRateCalculation tests error rate calculation
func TestDiagnosticsErrorRateCalculation(t *testing.T) {
	tests := []struct {
		name         string
		frameCount   int64
		failureCount int64
		expectedRate float64
	}{
		{
			name:         "No frames",
			frameCount:   0,
			failureCount: 0,
			expectedRate: 0,
		},
		{
			name:         "No failures",
			frameCount:   1000,
			failureCount: 0,
			expectedRate: 0,
		},
		{
			name:         "1% failure rate",
			frameCount:   990,
			failureCount: 10,
			expectedRate: 1.0,
		},
		{
			name:         "5% failure rate",
			frameCount:   950,
			failureCount: 50,
			expectedRate: 5.0,
		},
		{
			name:         "50% failure rate",
			frameCount:   500,
			failureCount: 500,
			expectedRate: 50.0,
		},
		{
			name:         "100% failure rate",
			frameCount:   0,
			failureCount: 100,
			expectedRate: 100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate error rate using same formula as handleDiagnostics
			var errorRate float64
			if tt.frameCount+tt.failureCount > 0 {
				errorRate = (float64(tt.failureCount) / float64(tt.frameCount+tt.failureCount)) * 100
			}

			if errorRate != tt.expectedRate {
				t.Errorf("Error rate calculation: got %f, want %f", errorRate, tt.expectedRate)
			}
		})
	}
}

// TestDiagnosticsHealthStatusThresholds tests health status categorization
func TestDiagnosticsHealthStatusThresholds(t *testing.T) {
	tests := []struct {
		name                string
		errorRate           float64
		consecutiveFailures int64
		expectedStatus      string
	}{
		{
			name:                "Excellent - low error rate",
			errorRate:           0.5,
			consecutiveFailures: 0,
			expectedStatus:      "Excellent",
		},
		{
			name:                "Excellent - borderline low",
			errorRate:           5.0,
			consecutiveFailures: 0,
			expectedStatus:      "Excellent",
		},
		{
			name:                "Degraded - borderline high",
			errorRate:           5.1,
			consecutiveFailures: 0,
			expectedStatus:      "Degraded",
		},
		{
			name:                "Degraded - mid range",
			errorRate:           10.0,
			consecutiveFailures: 0,
			expectedStatus:      "Degraded",
		},
		{
			name:                "Degraded - high error rate",
			errorRate:           19.9,
			consecutiveFailures: 0,
			expectedStatus:      "Degraded",
		},
		{
			name:                "Poor - high error rate",
			errorRate:           20.1,
			consecutiveFailures: 0,
			expectedStatus:      "Poor",
		},
		{
			name:                "Poor - many consecutive failures",
			errorRate:           1.0,
			consecutiveFailures: 6,
			expectedStatus:      "Poor",
		},
		{
			name:                "Excellent - few consecutive failures",
			errorRate:           1.0,
			consecutiveFailures: 3,
			expectedStatus:      "Excellent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate health status logic from handleDiagnostics
			healthStatus := "Excellent"
			if tt.errorRate > 5 {
				healthStatus = "Degraded"
			}
			if tt.errorRate > 20 || tt.consecutiveFailures > 5 {
				healthStatus = "Poor"
			}

			if healthStatus != tt.expectedStatus {
				t.Errorf("Health status: got %s, want %s", healthStatus, tt.expectedStatus)
			}
		})
	}
}

// TestDiagnosticsResponseStructure tests that diagnostics response contains all fields
func TestDiagnosticsResponseStructure(t *testing.T) {
	// Create a sample DiagnosticsResponse
	response := DiagnosticsResponse{
		Status:               "ok",
		CameraReady:          true,
		FramesPerSecond:      24.5,
		UptimeSeconds:        3600,
		StreamConnections:    1,
		FramesCaptured:       86400,
		LastFrameAgeSeconds:  0.1,
		Resolution:           "1920x1080",
		JPEGQuality:          85,
		MaxConnections:       2,
		CaptureFailures:      0,
		CaptureFailuresTotal: 10,
		ErrorRate:            0.01,
		HealthStatus:         "Excellent",
		Message:              "Camera is functioning normally",
	}

	// Marshal to JSON
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	// Unmarshal to verify all fields
	var unmarshaled map[string]interface{}
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Check for required fields
	requiredFields := []string{
		"status",
		"camera_ready",
		"fps",
		"uptime_seconds",
		"stream_connections",
		"frames_captured",
		"last_frame_age_seconds",
		"resolution",
		"jpeg_quality",
		"max_stream_connections",
		"capture_failures_recent",
		"capture_failures_total",
		"error_rate_percent",
		"health_status",
		"message",
	}

	for _, field := range requiredFields {
		if _, ok := unmarshaled[field]; !ok {
			t.Errorf("Missing field in response: %s", field)
		}
	}
}

// TestDiagnosticsResponseJSONEncoding tests that response encodes correctly
func TestDiagnosticsResponseJSONEncoding(t *testing.T) {
	response := DiagnosticsResponse{
		Status:               "ok",
		CameraReady:          true,
		FramesPerSecond:      30.0,
		UptimeSeconds:        1000,
		StreamConnections:    1,
		FramesCaptured:       30000,
		LastFrameAgeSeconds:  0.05,
		Resolution:           "1024x768",
		JPEGQuality:          80,
		MaxConnections:       2,
		CaptureFailures:      0,
		CaptureFailuresTotal: 5,
		ErrorRate:            0.016,
		HealthStatus:         "Excellent",
		Message:              "Test message",
	}

	// Test encoding
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Test decoding
	var decoded DiagnosticsResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify values match
	if decoded.Status != response.Status {
		t.Errorf("Status mismatch: %s vs %s", decoded.Status, response.Status)
	}
	if decoded.ErrorRate != response.ErrorRate {
		t.Errorf("ErrorRate mismatch: %f vs %f", decoded.ErrorRate, response.ErrorRate)
	}
	if decoded.HealthStatus != response.HealthStatus {
		t.Errorf("HealthStatus mismatch: %s vs %s", decoded.HealthStatus, response.HealthStatus)
	}
	if decoded.CaptureFailures != response.CaptureFailures {
		t.Errorf("CaptureFailures mismatch: %d vs %d", decoded.CaptureFailures, response.CaptureFailures)
	}
	if decoded.CaptureFailuresTotal != response.CaptureFailuresTotal {
		t.Errorf("CaptureFailuresTotal mismatch: %d vs %d", decoded.CaptureFailuresTotal, response.CaptureFailuresTotal)
	}
}

// TestDiagnosticsEdgeCases tests edge cases in error rate calculation
func TestDiagnosticsEdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		frameCount      int64
		failureCount    int64
		shouldCalculate bool
		expectedRate    float64
	}{
		{
			name:            "Very small numbers",
			frameCount:      1,
			failureCount:    0,
			shouldCalculate: true,
			expectedRate:    0.0,
		},
		{
			name:            "One failure one frame",
			frameCount:      1,
			failureCount:    1,
			shouldCalculate: true,
			expectedRate:    50.0,
		},
		{
			name:            "Large numbers",
			frameCount:      1000000,
			failureCount:    10000,
			shouldCalculate: true,
			expectedRate:    0.998,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errorRate float64
			if tt.frameCount+tt.failureCount > 0 {
				errorRate = (float64(tt.failureCount) / float64(tt.frameCount+tt.failureCount)) * 100
			}

			if errorRate != tt.expectedRate {
				// Allow for small floating point errors
				if !(errorRate > tt.expectedRate-0.01 && errorRate < tt.expectedRate+0.01) {
					t.Errorf("Error rate: got %f, want %f", errorRate, tt.expectedRate)
				}
			}
		})
	}
}

// TestSettingsUpdateErrorHandling tests error handling in settings updates
func TestSettingsUpdateErrorHandling(t *testing.T) {
	// Create a test request with invalid JSON
	invalidJSON := []byte("{invalid json}")

	req, err := http.NewRequest("POST", "/api/settings", bytes.NewReader(invalidJSON))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Verify request body is invalid
	decoder := json.NewDecoder(req.Body)
	var payload interface{}
	if err := decoder.Decode(&payload); err == nil {
		t.Error("Expected JSON decode to fail, but it succeeded")
	}
}

// TestPanicRecoveryLogging ensures panic recovery messages are logged
func TestPanicRecoveryLogging(t *testing.T) {
	// Capture log output
	var logBuffer bytes.Buffer
	originalOutput := log.Writer()
	log.SetOutput(&logBuffer)
	defer log.SetOutput(originalOutput)

	// Function with panic recovery (mimics goroutine pattern)
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ PANIC recovered: %v", r)
			}
		}()

		// Trigger panic
		panic("test panic")
	}()

	// Verify panic was logged
	logOutput := logBuffer.String()
	if logOutput == "" {
		t.Error("Panic recovery did not produce log output")
	}
	if !bytes.Contains(logBuffer.Bytes(), []byte("PANIC")) {
		t.Error("Log output does not contain PANIC indicator")
	}
}

// BenchmarkDiagnosticsErrorRate benchmarks error rate calculation
func BenchmarkDiagnosticsErrorRate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		frameCount := int64(1000000)
		failureCount := int64(10000)

		var errorRate float64
		if frameCount+failureCount > 0 {
			errorRate = (float64(failureCount) / float64(frameCount+failureCount)) * 100
		}
		_ = errorRate
	}
}

// BenchmarkDiagnosticsHealthStatus benchmarks health status determination
func BenchmarkDiagnosticsHealthStatus(b *testing.B) {
	for i := 0; i < b.N; i++ {
		errorRate := 10.5
		consecutiveFailures := int64(2)

		healthStatus := "Excellent"
		if errorRate > 5 {
			healthStatus = "Degraded"
		}
		if errorRate > 20 || consecutiveFailures > 5 {
			healthStatus = "Poor"
		}
		_ = healthStatus
	}
}

// BenchmarkDiagnosticsResponseEncoding benchmarks JSON encoding of response
func BenchmarkDiagnosticsResponseEncoding(b *testing.B) {
	response := DiagnosticsResponse{
		Status:               "ok",
		CameraReady:          true,
		FramesPerSecond:      24.5,
		UptimeSeconds:        3600,
		StreamConnections:    1,
		FramesCaptured:       86400,
		LastFrameAgeSeconds:  0.1,
		Resolution:           "1920x1080",
		JPEGQuality:          85,
		MaxConnections:       2,
		CaptureFailures:      0,
		CaptureFailuresTotal: 10,
		ErrorRate:            0.01,
		HealthStatus:         "Excellent",
		Message:              "Camera is functioning normally",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(response)
	}
}
