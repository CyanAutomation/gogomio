package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CyanAutomation/gogomio/internal/config"
)

// TestDiagnosticsErrorRateFromHandler verifies the documented diagnostics rate
// through the routed handler. Contract: TC-DIAG-01 (docs/testing/test-contracts.md).
func TestDiagnosticsErrorRateFromHandler(t *testing.T) {
	tests := []struct {
		name              string
		frameCount        int64
		failureCount      int64
		expectedErrorRate float64
	}{
		{name: "No frames or failures", frameCount: 0, failureCount: 0, expectedErrorRate: 0},
		{name: "Failure before first captured frame", frameCount: 0, failureCount: 1, expectedErrorRate: 100.0},
		{name: "No failures", frameCount: 1000, failureCount: 0, expectedErrorRate: 0},
		{name: "One failure and one captured frame", frameCount: 1, failureCount: 1, expectedErrorRate: 50.0},
		{name: "One percent failures", frameCount: 990, failureCount: 10, expectedErrorRate: 1.0},
		{name: "Half failures", frameCount: 500, failureCount: 500, expectedErrorRate: 50.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cam := &readinessCamera{ready: true}
			fm := NewFrameManager(cam, &config.Config{Resolution: [2]int{640, 480}, JPEGQuality: 80, MaxStreamConnections: 2})
			t.Cleanup(fm.Stop)

			for i := int64(0); i < tt.frameCount; i++ {
				fm.streamStats.RecordFrame(time.Now().Add(time.Duration(i) * time.Millisecond).UnixNano())
			}
			atomic.StoreInt64(&fm.captureFailureTotal, tt.failureCount)
			atomic.StoreInt64(&fm.consecutiveCaptureFailures, 0)

			router := RegisterHandlers(http.NewServeMux(), fm, fm.cfg)

			req, err := http.NewRequest(http.MethodGet, "/api/diagnostics", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status code: got %d, want %d", rr.Code, http.StatusOK)
			}
			if got := rr.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("content type: got %q, want %q", got, "application/json")
			}

			var response DetailedHealthResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to decode response JSON: %v", err)
			}
			if response.FramesCaptured != tt.frameCount {
				t.Fatalf("frames_captured: got %d, want %d", response.FramesCaptured, tt.frameCount)
			}
			if response.CaptureFailuresTotal != tt.failureCount {
				t.Fatalf("capture_failures_total: got %d, want %d", response.CaptureFailuresTotal, tt.failureCount)
			}
			if math.Abs(response.ErrorRatePercent-tt.expectedErrorRate) > 0.001 {
				t.Fatalf("error_rate_percent: got %f, want %f", response.ErrorRatePercent, tt.expectedErrorRate)
			}
		})
	}
}

// TestDiagnosticsHealthStatusThresholds verifies documented status boundaries
// through the diagnostics handler. Contract: TC-DIAG-02 (docs/testing/test-contracts.md).
func TestDiagnosticsHealthStatusThresholds(t *testing.T) {
	tests := []struct {
		name                string
		frameCount          int64
		failureCount        int64
		consecutiveFailures int64
		expectedStatus      string
	}{
		{name: "Excellent - boundary 5.0", frameCount: 95, failureCount: 5, consecutiveFailures: 0, expectedStatus: "Excellent"},
		{name: "Degraded - boundary 5.1", frameCount: 949, failureCount: 51, consecutiveFailures: 0, expectedStatus: "Degraded"},
		{name: "Degraded - boundary 20.0", frameCount: 80, failureCount: 20, consecutiveFailures: 0, expectedStatus: "Degraded"},
		{name: "Poor - boundary 20.1", frameCount: 799, failureCount: 201, consecutiveFailures: 0, expectedStatus: "Poor"},
		{name: "Poor - consecutive failures > 5", frameCount: 99, failureCount: 1, consecutiveFailures: 6, expectedStatus: "Poor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cam := &readinessCamera{ready: true}
			fm := NewFrameManager(cam, &config.Config{Resolution: [2]int{640, 480}, JPEGQuality: 80, MaxStreamConnections: 2})
			t.Cleanup(fm.Stop)

			for i := int64(0); i < tt.frameCount; i++ {
				fm.streamStats.RecordFrame(time.Now().Add(time.Duration(i) * time.Millisecond).UnixNano())
			}
			atomic.StoreInt64(&fm.captureFailureTotal, tt.failureCount)
			atomic.StoreInt64(&fm.consecutiveCaptureFailures, tt.consecutiveFailures)

			router := RegisterHandlers(http.NewServeMux(), fm, fm.cfg)

			req, err := http.NewRequest(http.MethodGet, "/api/diagnostics", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status code: got %d, want %d", rr.Code, http.StatusOK)
			}

			var response DetailedHealthResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to decode response JSON: %v", err)
			}

			if response.HealthStatus != tt.expectedStatus {
				t.Fatalf("health_status: got %q, want %q", response.HealthStatus, tt.expectedStatus)
			}
		})
	}
}

// TestDiagnosticsResponseStructure verifies JSON includes client-critical fields
// with expected semantic values rather than only tag-mapping mechanics.
func TestDiagnosticsResponseStructure(t *testing.T) {
	response := DetailedHealthResponse{
		Status:                     "degraded",
		CameraReady:                false,
		FPSCurrent:                 4.2,
		UptimeSeconds:              900,
		StreamConnections:          1,
		FramesCaptured:             200,
		LastFrameAgeSeconds:        2.5,
		Resolution:                 "1280x720",
		JPEGQuality:                80,
		MaxConnections:             2,
		CaptureFailuresConsecutive: 6,
		CaptureFailuresTotal:       40,
		ErrorRatePercent:           16.67,
		HealthStatus:               "Poor",
		Message:                    "Capture reliability degraded",
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if got := body["status"]; got != "degraded" {
		t.Fatalf("status: got %v, want %q", got, "degraded")
	}
	if got := body["health_status"]; got != "Poor" {
		t.Fatalf("health_status: got %v, want %q", got, "Poor")
	}
	if got := body["capture_failures_total"]; got != float64(40) {
		t.Fatalf("capture_failures_total: got %v, want %v", got, float64(40))
	}
	if got := body["capture_failures_consecutive"]; got != float64(6) {
		t.Fatalf("capture_failures_consecutive: got %v, want %v", got, float64(6))
	}
	errorRate, ok := body["error_rate_percent"].(float64)
	if !ok {
		t.Fatalf("error_rate_percent is not a float64: got type %T", body["error_rate_percent"])
	}
	if math.Abs(errorRate-16.67) > 0.001 {
		t.Fatalf("error_rate_percent: got %v, want approximately %v", errorRate, 16.67)
	}
	if got := body["message"]; got != "Capture reliability degraded" {
		t.Fatalf("message: got %v, want %q", got, "Capture reliability degraded")
	}
}

// BenchmarkDiagnosticsResponseEncoding benchmarks JSON encoding of response
func BenchmarkDiagnosticsResponseEncoding(b *testing.B) {
	response := DetailedHealthResponse{
		Status:                     "ok",
		CameraReady:                true,
		FPSCurrent:                 24.5,
		UptimeSeconds:              3600,
		StreamConnections:          1,
		FramesCaptured:             86400,
		LastFrameAgeSeconds:        0.1,
		Resolution:                 "1920x1080",
		JPEGQuality:                85,
		MaxConnections:             2,
		CaptureFailuresConsecutive: 0,
		CaptureFailuresTotal:       10,
		ErrorRatePercent:           0.01,
		HealthStatus:               "Excellent",
		Message:                    "Camera is functioning normally",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(response)
	}
}
