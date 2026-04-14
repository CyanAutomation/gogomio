package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/go-chi/chi/v5"
)

// FrameManager coordinates camera capture and serves frames to HTTP clients.
type FrameManager struct {
	cam            camera.Camera
	cfg            *config.Config
	frameBuffer    *camera.FrameBuffer
	streamStats    *camera.StreamStats
	connTracker    *camera.ConnectionTracker
	mu             sync.RWMutex
	captureStarted bool

	// Channel to signal goroutine to stop
	doneChan chan struct{}
}

// NewFrameManager creates and initializes a new FrameManager.
func NewFrameManager(cam camera.Camera, cfg *config.Config) *FrameManager {
	stats := camera.NewStreamStats()
	bufferTargetFPS := cfg.TargetFPS
	if bufferTargetFPS <= 0 {
		bufferTargetFPS = cfg.FPS
	}

	fm := &FrameManager{
		cam:         cam,
		cfg:         cfg,
		frameBuffer: camera.NewFrameBuffer(stats, bufferTargetFPS),
		streamStats: stats,
		connTracker: camera.NewConnectionTracker(),
		doneChan:    make(chan struct{}),
	}

	// Start capture goroutine
	fm.captureStarted = true
	go fm.captureLoop()

	return fm
}

// captureLoop continuously captures frames from the camera and writes to the frame buffer.
func (fm *FrameManager) captureLoop() {
	defer func() {
		fm.mu.Lock()
		fm.captureStarted = false
		fm.mu.Unlock()
	}()

	for {
		select {
		case <-fm.doneChan:
			return
		default:
		}

		// Capture frame (with internal FPS throttling in mock camera)
		frame, err := fm.cam.CaptureFrame()
		if err != nil {
			// Camera error, wait before retry
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if frame != nil {
			fm.frameBuffer.Write(frame)
		}
	}
}

// Stop stops the frame capture loop.
func (fm *FrameManager) Stop() {
	close(fm.doneChan)
	time.Sleep(100 * time.Millisecond) // Allow goroutine to exit
}

// GetFrame returns a copy of the current frame for snapshot endpoints.
func (fm *FrameManager) GetFrame() []byte {
	return fm.frameBuffer.GetFrame()
}

// StreamFrame writes frames to an HTTP response in MJPEG format.
// Manages connection tracking and respects the max connection limit.
func (fm *FrameManager) StreamFrame(w http.ResponseWriter, maxConnections int) error {
	// Check connection limit
	if !fm.connTracker.TryIncrement(maxConnections) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Max stream connections reached"))
		return fmt.Errorf("connection limit exceeded")
	}
	defer fm.connTracker.Decrement()

	// Set MJPEG headers
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("response writer does not support flushing")
	}

	// Stream frames with timeout
	ticker := time.NewTicker(fm.cfg.FrameTimeout())
	defer ticker.Stop()

	for {
		select {
		case <-fm.doneChan:
			return fmt.Errorf("stream stopped")
		case <-ticker.C:
			// Frame timeout, continue (client may have disconnected)
			continue
		default:
		}

		// Try to get frame (non-blocking)
		frame := fm.GetFrame()
		if frame != nil {
			// Write MJPEG boundary and frame
			if _, err := w.Write([]byte("--frame\r\n")); err != nil {
				return err
			}
			if _, err := w.Write([]byte("Content-Type: image/jpeg\r\n")); err != nil {
				return err
			}
			if _, err := w.Write([]byte("Content-Length: " + fmt.Sprintf("%d", len(frame)) + "\r\n\r\n")); err != nil {
				return err
			}
			if _, err := w.Write(frame); err != nil {
				return err
			}
			if _, err := w.Write([]byte("\r\n")); err != nil {
				return err
			}

			flusher.Flush()
		} else {
			// No frame yet, sleep briefly
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// ConfigResponse is the JSON response for /api/config endpoint.
type ConfigResponse struct {
	Resolution           [2]int  `json:"resolution"`
	FPS                  int     `json:"fps"`
	TargetFPS            int     `json:"target_fps"`
	JPEGQuality          int     `json:"jpeg_quality"`
	MaxStreamConnections int     `json:"max_stream_connections"`
	CurrentStreamCount   int     `json:"current_stream_connections"`
	FrameCount           int64   `json:"frames_captured"`
	CurrentFPS           float64 `json:"current_fps"`
	LastFrameAgeSeconds  float64 `json:"last_frame_age_seconds"`
}

// HealthResponse is the JSON response for /health endpoint.
type HealthResponse struct {
	Status            string  `json:"status"`
	CameraReady       bool    `json:"camera_ready"`
	UptimeSeconds     int64   `json:"uptime_seconds"`
	StreamConnections int     `json:"stream_connections"`
	FramesPerSecond   float64 `json:"fps"`
}

// RegisterHandlers registers all API endpoints with the Chi router.
func RegisterHandlers(router *chi.Mux, fm *FrameManager, cfg *config.Config) {
	startTime := time.Now()

	// Middleware
	router.Use(loggingMiddleware)

	// Health check endpoints
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		handleHealth(w, r, fm, startTime)
	})

	router.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		handleReady(w, r, fm)
	})

	// Stream endpoints
	router.Get("/stream.mjpg", func(w http.ResponseWriter, r *http.Request) {
		if err := fm.StreamFrame(w, cfg.MaxStreamConnections); err != nil {
			// Client disconnected or error occurred
		}
	})

	router.Get("/snapshot.jpg", func(w http.ResponseWriter, r *http.Request) {
		handleSnapshot(w, r, fm)
	})

	// API endpoints
	router.Get("/api/config", func(w http.ResponseWriter, r *http.Request) {
		handleAPIConfigure(w, r, fm, cfg, startTime)
	})

	router.Get("/api/status", func(w http.ResponseWriter, r *http.Request) {
		handleAPIStatus(w, r, fm, startTime)
	})

	// Placeholder for future endpoints
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
  <title>Motion In Ocean - Go Edition</title>
  <style>
    body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
    .container { background: white; padding: 20px; border-radius: 5px; }
    h1 { color: #333; }
    .info { color: #666; margin: 10px 0; }
    a { color: #0066cc; text-decoration: none; }
    a:hover { text-decoration: underline; }
  </style>
</head>
<body>
  <div class="container">
    <h1>🌊 Motion In Ocean - Go Edition</h1>
    <p class="info">Raspberry Pi CSI Camera MJPEG Streaming Server</p>
    
    <h2>Endpoints</h2>
    <ul>
      <li><a href="/stream.mjpg">Live Stream: /stream.mjpg</a></li>
      <li><a href="/snapshot.jpg">Latest Snapshot: /snapshot.jpg</a></li>
      <li><a href="/api/config">Configuration: /api/config</a></li>
      <li><a href="/api/status">Status: /api/status</a></li>
      <li><a href="/health">Health Check: /health</a></li>
      <li><a href="/ready">Readiness Probe: /ready</a></li>
    </ul>

    <h2>Info</h2>
    <p class="info">Server is running and ready to stream.</p>
  </div>
</body>
</html>
`))
	})
}

// Handler functions

func handleHealth(w http.ResponseWriter, r *http.Request, fm *FrameManager, startTime time.Time) {
	_, _, fps := fm.streamStats.Snapshot()

	response := HealthResponse{
		Status:            "ok",
		CameraReady:       fm.cam.IsReady(),
		UptimeSeconds:     int64(time.Since(startTime).Seconds()),
		StreamConnections: fm.connTracker.Count(),
		FramesPerSecond:   fps,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleReady(w http.ResponseWriter, r *http.Request, fm *FrameManager) {
	if !fm.cam.IsReady() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "initializing"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func handleSnapshot(w http.ResponseWriter, r *http.Request, fm *FrameManager) {
	frame := fm.GetFrame()
	if frame == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Camera not ready"))
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Write(frame)
}

func handleAPIConfigure(w http.ResponseWriter, r *http.Request, fm *FrameManager, cfg *config.Config, startTime time.Time) {
	frameCount, _, fps := fm.streamStats.Snapshot()

	response := ConfigResponse{
		Resolution:           cfg.Resolution,
		FPS:                  cfg.FPS,
		TargetFPS:            cfg.TargetFPS,
		JPEGQuality:          cfg.JPEGQuality,
		MaxStreamConnections: cfg.MaxStreamConnections,
		CurrentStreamCount:   fm.connTracker.Count(),
		FrameCount:           frameCount,
		CurrentFPS:           fps,
		LastFrameAgeSeconds:  fm.streamStats.LastFrameAgeSeconds(time.Now().UnixNano()),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleAPIStatus(w http.ResponseWriter, r *http.Request, fm *FrameManager, startTime time.Time) {
	_, _, fps := fm.streamStats.Snapshot()

	response := HealthResponse{
		Status:            "ok",
		CameraReady:       fm.cam.IsReady(),
		UptimeSeconds:     int64(time.Since(startTime).Seconds()),
		StreamConnections: fm.connTracker.Count(),
		FramesPerSecond:   fps,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Middleware

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For now, just pass through
		// In production, would log requests
		next.ServeHTTP(w, r)
	})
}
