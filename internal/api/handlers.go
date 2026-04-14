package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/CyanAutomation/gogomio/internal/settings"
	"github.com/CyanAutomation/gogomio/internal/web"
	"github.com/go-chi/chi/v5"
)

// FrameManager coordinates camera capture and serves frames to HTTP clients.
type FrameManager struct {
	cam            camera.Camera
	cfg            *config.Config
	frameBuffer    *camera.FrameBuffer
	streamStats    *camera.StreamStats
	connTracker    *camera.ConnectionTracker
	settingsM      *settings.Manager
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
		settingsM:   settings.NewManager("/tmp/gogomio/settings.json"),
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
	w.Header().Set("Connection", "close")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("response writer does not support flushing")
	}

	frameTimeout := fm.cfg.FrameTimeout()
	lastFrame := fm.frameBuffer.GetFrame()

	for {
		select {
		case <-fm.doneChan:
			return fmt.Errorf("stream stopped")
		default:
		}

		// Wait for new frame with timeout
		frame := fm.frameBuffer.WaitFrame(frameTimeout)
		if frame == nil {
			// Timeout waiting for frame, keep connection open or retry
			continue
		}

		// Skip if same frame as last (no new frames)
		if len(frame) == len(lastFrame) {
			same := true
			for i := range frame {
				if frame[i] != lastFrame[i] {
					same = false
					break
				}
			}
			if same {
				continue
			}
		}
		lastFrame = frame

		// Write MJPEG boundary and frame
		boundary := []byte("--frame\r\n")
		headers := []byte("Content-Type: image/jpeg\r\nContent-Length: " + fmt.Sprintf("%d", len(frame)) + "\r\n\r\n")
		trailer := []byte("\r\n")

		if _, err := w.Write(boundary); err != nil {
			return err
		}
		if _, err := w.Write(headers); err != nil {
			return err
		}
		if _, err := w.Write(frame); err != nil {
			return err
		}
		if _, err := w.Write(trailer); err != nil {
			return err
		}

		flusher.Flush()
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

	// Register web UI (must be before other handlers for proper routing)
	web.RegisterStaticFiles(router)

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

	// Settings management endpoints
	router.Get("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		handleSettingsGet(w, r, fm)
	})

	router.Post("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		handleSettingsUpdate(w, r, fm)
	})

	router.Put("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		handleSettingsUpdate(w, r, fm)
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

// Settings handlers

func handleSettingsGet(w http.ResponseWriter, r *http.Request, fm *FrameManager) {
	settings := fm.settingsM.GetAll()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"settings": settings,
	})
}

// SettingsUpdateRequest represents a request body for updating settings
type SettingsUpdateRequest struct {
	Settings map[string]interface{} `json:"settings"`
}

func handleSettingsUpdate(w http.ResponseWriter, r *http.Request, fm *FrameManager) {
	var req SettingsUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	// Save each setting
	for key, value := range req.Settings {
		if err := fm.settingsM.Set(key, value); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to save setting: " + key})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("saved %d settings", len(req.Settings)),
	})
}

// Middleware

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For now, just pass through
		// In production, would log requests
		next.ServeHTTP(w, r)
	})
}
