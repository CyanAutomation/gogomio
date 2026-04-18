package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/CyanAutomation/gogomio/internal/settings"
	"github.com/CyanAutomation/gogomio/internal/web"
	"github.com/go-chi/chi/v5"
)

// FrameManager coordinates camera capture and serves frames to HTTP clients.
type FrameManager struct {
	cam             camera.Camera
	cfg             *config.Config
	frameBuffer     *camera.FrameBuffer
	streamStats     *camera.StreamStats
	connTracker     *camera.ConnectionTracker
	settingsM       *settings.Manager
	captureMu       sync.Mutex
	captureStarted  bool
	clientCount     int64 // atomic counter for connected clients
	clientImbalance int64 // atomic counter for decrement calls when clientCount is already 0

	// Channel to signal goroutine to stop
	doneChan    chan struct{}
	stopChancel chan struct{} // Channel to cancel pending stop

	idleStopDelay time.Duration
	captureStarts int64

	consecutiveCaptureFailures int64
	captureFailureTotal        int64
}

const defaultIdleStopDelay = 3 * time.Second

const (
	initialCaptureRetryDelay              = 100 * time.Millisecond
	maxCaptureRetryDelay                  = 2 * time.Second
	captureFailureDegradedThreshold int64 = 5
	captureFailureLogInterval       int64 = 10
)

var (
	mjpegBoundaryBytes      = []byte("--frame\r\n")
	mjpegContentTypeBytes   = []byte("Content-Type: image/jpeg\r\n")
	mjpegContentLengthBytes = []byte("Content-Length: ")
	mjpegHeaderEndBytes     = []byte("\r\n\r\n")
	mjpegTrailerBytes       = []byte("\r\n")
)

// NewFrameManager creates and initializes a new FrameManager.
func NewFrameManager(cam camera.Camera, cfg *config.Config) *FrameManager {
	return newFrameManager(cam, cfg, defaultIdleStopDelay)
}

func newFrameManager(cam camera.Camera, cfg *config.Config, idleStopDelay time.Duration) *FrameManager {
	stats := camera.NewStreamStats()
	bufferTargetFPS := cfg.TargetFPS
	if bufferTargetFPS <= 0 {
		bufferTargetFPS = cfg.FPS
	}

	fm := &FrameManager{
		cam:           cam,
		cfg:           cfg,
		frameBuffer:   camera.NewFrameBuffer(stats, bufferTargetFPS),
		streamStats:   stats,
		connTracker:   camera.NewConnectionTracker(),
		settingsM:     settings.NewManager("/tmp/gogomio/settings.json"),
		doneChan:      make(chan struct{}),
		stopChancel:   make(chan struct{}),
		clientCount:   0,
		idleStopDelay: idleStopDelay,
	}

	// Capture loop starts lazily when first client connects
	return fm
}

// IncrementClients increments the client count and starts capture if this is the first client.
func (fm *FrameManager) IncrementClients() {
	new := atomic.AddInt64(&fm.clientCount, 1)
	if new == 1 {
		fm.captureMu.Lock()
		// Cancel any pending stop by closing the stopChancel
		close(fm.stopChancel)
		fm.stopChancel = make(chan struct{})
		fm.captureMu.Unlock()
		fm.startCapture()
	}
}

// DecrementClients decrements the client count and stops capture if this is the last client.
func (fm *FrameManager) DecrementClients() {
	for attempts := 0; attempts < 100; attempts++ {
		current := atomic.LoadInt64(&fm.clientCount)
		if current <= 0 {
			if atomic.CompareAndSwapInt64(&fm.clientCount, current, 0) {
				imbalanceCount := atomic.AddInt64(&fm.clientImbalance, 1)
				log.Printf("frame manager client count imbalance detected: decrement at count=%d, clamped to 0 (total imbalances=%d)", current, imbalanceCount)
				return
			}
			continue
		}

		if atomic.CompareAndSwapInt64(&fm.clientCount, current, current-1) {
			if current-1 == 0 {
				fm.scheduleStopCapture()
			}
			return
		}
	}
	// Fallback: force clamp to 0 after max attempts
	atomic.StoreInt64(&fm.clientCount, 0)
}

// startCapture starts the capture loop if not already running.
func (fm *FrameManager) startCapture() {
	fm.captureMu.Lock()
	if fm.captureStarted {
		fm.captureMu.Unlock()
		return
	}

	done := make(chan struct{})
	fm.captureStarted = true
	fm.doneChan = done
	atomic.AddInt64(&fm.captureStarts, 1)
	fm.captureMu.Unlock()
	go fm.captureLoop(done)
}

func (fm *FrameManager) scheduleStopCapture() {
	fm.captureMu.Lock()
	if !fm.captureStarted {
		fm.captureMu.Unlock()
		return
	}

	delay := fm.idleStopDelay
	if delay <= 0 {
		delay = defaultIdleStopDelay
	}

	// Get current stopChancel and done to check against in the goroutine
	stopChancel := fm.stopChancel
	done := fm.doneChan
	fm.captureMu.Unlock()

	// Spawn a goroutine that will signal stop after the idle delay
	// unless the stopChancel is replaced (indicating a new client connected)
	go func() {
		select {
		case <-time.After(delay):
			// Delay expired, proceed with stopping
			fm.captureMu.Lock()
			// Verify that the done channel is still the same and capture is still running
			if !fm.captureStarted || atomic.LoadInt64(&fm.clientCount) > 0 || fm.doneChan != done {
				fm.captureMu.Unlock()
				return
			}
			// Still no clients, close capture
			fm.captureStarted = false
			fm.captureMu.Unlock()
			// Safe to close because this goroutine owns this done channel
			close(done)
		case <-stopChancel:
			// Stop was cancelled (new client connected)
			return
		}
	}()
}

// stopCapture stops the capture loop if currently running.
func (fm *FrameManager) stopCapture() {
	fm.captureMu.Lock()
	if !fm.captureStarted {
		fm.captureMu.Unlock()
		return
	}
	fm.captureStarted = false
	done := fm.doneChan
	fm.captureMu.Unlock()
	close(done)
	time.Sleep(50 * time.Millisecond) // Allow goroutine to exit cleanly
}

// captureLoop continuously captures frames from the camera and writes to the frame buffer.
func (fm *FrameManager) captureLoop(done <-chan struct{}) {
	defer func() {
		fm.captureMu.Lock()
		if fm.doneChan == done {
			fm.captureStarted = false
		}
		fm.captureMu.Unlock()
	}()

	retryDelay := initialCaptureRetryDelay

	for {
		select {
		case <-done:
			return
		default:
		}

		// Capture frame (with internal FPS throttling in mock camera)
		frame, err := fm.cam.CaptureFrame()
		if err != nil {
			consecutive := atomic.AddInt64(&fm.consecutiveCaptureFailures, 1)
			total := atomic.AddInt64(&fm.captureFailureTotal, 1)
			if consecutive == 1 || consecutive%captureFailureLogInterval == 0 {
				log.Printf("camera capture failure: consecutive=%d total=%d retry_delay=%s err=%v", consecutive, total, retryDelay, err)
			}

			timer := time.NewTimer(retryDelay)
			select {
			case <-done:
				timer.Stop()
				return
			case <-timer.C:
			}

			retryDelay *= 2
			if retryDelay > maxCaptureRetryDelay {
				retryDelay = maxCaptureRetryDelay
			}
			continue
		}

		if frame != nil {
			if consecutive := atomic.SwapInt64(&fm.consecutiveCaptureFailures, 0); consecutive > 0 {
				log.Printf("camera capture recovered after %d consecutive failures", consecutive)
			}
			retryDelay = initialCaptureRetryDelay
			_, _ = fm.frameBuffer.Write(frame)
		}
	}
}

func (fm *FrameManager) captureFailureStats() (int64, int64, bool) {
	consecutive := atomic.LoadInt64(&fm.consecutiveCaptureFailures)
	total := atomic.LoadInt64(&fm.captureFailureTotal)
	return consecutive, total, consecutive >= captureFailureDegradedThreshold
}

// Stop stops the frame capture loop.
func (fm *FrameManager) Stop() {
	fm.stopCapture()
}

// GetFrame returns a copy of the current frame for snapshot endpoints.
// Ensures capture is running to provide current frames on-demand.
func (fm *FrameManager) GetFrame() []byte {
	// Temporarily increment client count to ensure capture is running
	fm.IncrementClients()
	defer fm.DecrementClients()

	// Wait briefly for a frame to become available
	// Wait briefly for a frame to become available
	frame, _ := fm.frameBuffer.WaitFrame(100*time.Millisecond, 0)
	if frame != nil {
		return frame
	}

	// Fall back to existing frame if available
	return fm.frameBuffer.GetFrame()
}

// StreamFrame writes frames to an HTTP response in MJPEG format.
// Manages connection tracking and respects the max connection limit (max 2 concurrent streams).
func (fm *FrameManager) StreamFrame(w http.ResponseWriter, r *http.Request, maxConnections int) error {
	// Check connection limit
	if !fm.connTracker.TryIncrement(maxConnections) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("Max stream connections reached (limit: 2)"))
		log.Printf("⚠️  Stream client rejected: connection limit exceeded")
		return fmt.Errorf("connection limit exceeded")
	}
	defer fm.connTracker.Decrement()

	// Track client lifecycle for lazy capture
	fm.IncrementClients()
	defer fm.DecrementClients()

	log.Printf("🔗 Stream client connected (total clients: %d, remote: %s)", atomic.LoadInt64(&fm.clientCount), r.RemoteAddr)

	// Set MJPEG headers
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "close")

	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("❌ Stream client: response writer does not support flushing")
		return fmt.Errorf("response writer does not support flushing")
	}

	frameTimeout := fm.cfg.FrameTimeout()
	lastSeenSeq := fm.frameBuffer.CurrentSequence()
	fm.captureMu.Lock()
	streamDone := fm.doneChan
	fm.captureMu.Unlock()
	ctx := r.Context()
	var frameWriteBuf bytes.Buffer
	contentLengthScratch := make([]byte, 0, 20)

	framesSent := 0
	timeoutCount := 0
	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			duration := time.Since(startTime)
			log.Printf("🔗 Stream client disconnected: %d frames sent in %v (remote: %s)", framesSent, duration, r.RemoteAddr)
			return ctx.Err()
		case <-streamDone:
			log.Printf("⚠️  Stream stopped for client (frames sent: %d)", framesSent)
			return fmt.Errorf("stream stopped")
		default:
		}

		frame, seq := fm.frameBuffer.WaitFrameWithContext(ctx, frameTimeout, lastSeenSeq)

		select {
		case <-ctx.Done():
			log.Printf("🔗 Stream client disconnected during frame wait")
			return ctx.Err()
		case <-streamDone:
			return fmt.Errorf("stream stopped")
		default:
		}

		if frame == nil {
			// Timeout waiting for frame, keep connection open or retry.
			timeoutCount++
			if timeoutCount == 1 {
				log.Printf("⏱️  Stream: waiting for first frame (initial timeout)")
			} else if timeoutCount%10 == 0 {
				log.Printf("⏱️  Stream: timeout waiting for frames (%d timeouts, %d frames sent)", timeoutCount, framesSent)
			}
			continue
		}

		lastSeenSeq = seq

		if err := writeMultipartFrame(w, &frameWriteBuf, &contentLengthScratch, frame); err != nil {
			log.Printf("❌ Stream write error after %d frames: %v", framesSent, err)
			return err
		}

		framesSent++
		if framesSent == 1 {
			log.Printf("✓ First MJPEG frame sent to client after %v", time.Since(startTime))
		}

		flusher.Flush()
	}
}

func writeMultipartFrame(w http.ResponseWriter, frameWriteBuf *bytes.Buffer, contentLengthScratch *[]byte, frame []byte) error {
	frameWriteBuf.Reset()
	frameWriteBuf.Grow(len(frame) + 128)

	frameWriteBuf.Write(mjpegBoundaryBytes)
	frameWriteBuf.Write(mjpegContentTypeBytes)
	frameWriteBuf.Write(mjpegContentLengthBytes)
	*contentLengthScratch = strconv.AppendInt((*contentLengthScratch)[:0], int64(len(frame)), 10)
	frameWriteBuf.Write(*contentLengthScratch)
	frameWriteBuf.Write(mjpegHeaderEndBytes)
	frameWriteBuf.Write(frame)
	frameWriteBuf.Write(mjpegTrailerBytes)

	_, err := w.Write(frameWriteBuf.Bytes())
	return err
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
	Status             string  `json:"status"`
	CameraReady        bool    `json:"camera_ready"`
	UptimeSeconds      int64   `json:"uptime_seconds"`
	StreamConnections  int     `json:"stream_connections"`
	FramesPerSecond    float64 `json:"fps"`
	Degraded           bool    `json:"degraded,omitempty"`
	CaptureFailures    int64   `json:"capture_consecutive_failures,omitempty"`
	CaptureErrorsTotal int64   `json:"capture_failures_total,omitempty"`
}

// DiagnosticsResponse is the JSON response for /api/diagnostics endpoint.
type DiagnosticsResponse struct {
	Status              string  `json:"status"`
	CameraReady         bool    `json:"camera_ready"`
	FramesPerSecond     float64 `json:"fps"`
	UptimeSeconds       int64   `json:"uptime_seconds"`
	StreamConnections   int     `json:"stream_connections"`
	FramesCaptured      int64   `json:"frames_captured"`
	LastFrameAgeSeconds float64 `json:"last_frame_age_seconds"`
	Resolution          string  `json:"resolution"`
	JPEGQuality         int     `json:"jpeg_quality"`
	MaxConnections      int     `json:"max_stream_connections"`
	Message             string  `json:"message"`
}

// RegisterHandlers registers all API endpoints with the Chi router.
func RegisterHandlers(router *chi.Mux, fm *FrameManager, cfg *config.Config) {
	startTime := time.Now()

	// Middleware
	router.Use(corsMiddleware)
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
		if err := fm.StreamFrame(w, r, cfg.MaxStreamConnections); err != nil {
			// Client disconnected or error occurred - this is normal
			_ = err
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

	// Diagnostics endpoint
	router.Get("/api/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		handleDiagnostics(w, r, fm, cfg, startTime)
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
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Client likely disconnected, ignore error
		_ = err
	}
}

func handleReady(w http.ResponseWriter, r *http.Request, fm *FrameManager) {
	if !fm.cam.IsReady() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "initializing"}); err != nil {
			_ = err
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ready"}); err != nil {
		_ = err
	}
}

func handleSnapshot(w http.ResponseWriter, r *http.Request, fm *FrameManager) {
	frame := fm.GetFrame()
	if frame == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("Camera not ready"))
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = w.Write(frame)
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
	if err := json.NewEncoder(w).Encode(response); err != nil {
		_ = err
	}
}

func handleAPIStatus(w http.ResponseWriter, r *http.Request, fm *FrameManager, startTime time.Time) {
	_, _, fps := fm.streamStats.Snapshot()
	consecutiveFailures, captureFailuresTotal, degraded := fm.captureFailureStats()

	status := "ok"
	if !fm.cam.IsReady() {
		status = "error"
	} else if degraded {
		status = "degraded"
	}

	response := HealthResponse{
		Status:             status,
		CameraReady:        fm.cam.IsReady(),
		UptimeSeconds:      int64(time.Since(startTime).Seconds()),
		StreamConnections:  fm.connTracker.Count(),
		FramesPerSecond:    fps,
		Degraded:           degraded,
		CaptureFailures:    consecutiveFailures,
		CaptureErrorsTotal: captureFailuresTotal,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		_ = err
	}
}

// Settings handlers

func handleSettingsGet(w http.ResponseWriter, r *http.Request, fm *FrameManager) {
	settings := fm.settingsM.GetAll()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"settings": settings,
	}); err != nil {
		_ = err
	}
}

// SettingsUpdateRequest represents a request body for updating settings
type SettingsUpdateRequest struct {
	Settings map[string]interface{} `json:"settings"`
}

func handleDiagnostics(w http.ResponseWriter, r *http.Request, fm *FrameManager, cfg *config.Config, startTime time.Time) {
	frameCount, _, fps := fm.streamStats.Snapshot()

	status := "ok"
	message := "Camera is functioning normally"
	if !fm.cam.IsReady() {
		status = "error"
		message = "Camera is not ready or failed to initialize"
	}

	resolution := fmt.Sprintf("%dx%d", cfg.Resolution[0], cfg.Resolution[1])

	response := DiagnosticsResponse{
		Status:              status,
		CameraReady:         fm.cam.IsReady(),
		FramesPerSecond:     fps,
		UptimeSeconds:       int64(time.Since(startTime).Seconds()),
		StreamConnections:   fm.connTracker.Count(),
		FramesCaptured:      frameCount,
		LastFrameAgeSeconds: fm.streamStats.LastFrameAgeSeconds(time.Now().UnixNano()),
		Resolution:          resolution,
		JPEGQuality:         cfg.JPEGQuality,
		MaxConnections:      cfg.MaxStreamConnections,
		Message:             message,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		_ = err
	}
}

func handleSettingsUpdate(w http.ResponseWriter, r *http.Request, fm *FrameManager) {
	var req SettingsUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	// Save each setting
	for key, value := range req.Settings {
		if err := fm.settingsM.Set(key, value); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to save setting: " + key})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
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

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			reqMethod := r.Header.Get("Access-Control-Request-Method")
			switch reqMethod {
			case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodOptions:
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}

		next.ServeHTTP(w, r)
	})
}
