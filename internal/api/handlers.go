package api

import (
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
	httpSwagger "github.com/swaggo/http-swagger"
	_ "github.com/CyanAutomation/gogomio/docs"
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

	// Cleanup infrastructure for idle timeout
	cleanupCh       chan cleanupRequest
	cleanupStop     chan struct{}
	cleanupDone     chan struct{}
	cleanupStopOnce sync.Once
	cleanupChOnce   sync.Once // Protects cleanupCh close from double-close panic
	fallbackWG      sync.WaitGroup
	stopChancelMu   sync.Mutex // Protects stopChancel access

	idleStopDelay time.Duration
	captureStarts int64

	consecutiveCaptureFailures int64
	captureFailureTotal        int64
}

// cleanupRequest represents a pending cleanup task
type cleanupRequest struct {
	delay  time.Duration
	stopCh chan struct{}
	done   chan struct{}
}

const defaultIdleStopDelay = 3 * time.Second
const cleanupEnqueueTimeout = 100 * time.Millisecond

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
		cleanupCh:     make(chan cleanupRequest, 8),
		cleanupStop:   make(chan struct{}),
		cleanupDone:   make(chan struct{}),
		clientCount:   0,
		idleStopDelay: idleStopDelay,
	}

	// Start background cleanup goroutine
	go fm.cleanupLoop()

	// Capture loop starts lazily when first client connects
	return fm
}

// IncrementClients increments the client count and starts capture if this is the first client.
func (fm *FrameManager) IncrementClients() {
	new := atomic.AddInt64(&fm.clientCount, 1)
	log.Printf("🔗 Client count incremented to: %d", new)
	if new == 1 {
		fm.captureMu.Lock()
		// Cancel any pending stop by closing the stopChancel
		close(fm.stopChancel)
		fm.stopChancel = make(chan struct{})
		fm.captureMu.Unlock()
		log.Printf("🎬 Starting capture (first client)")
		fm.startCapture()
	}
}

// DecrementClients decrements the client count and stops capture if this is the last client.
func (fm *FrameManager) DecrementClients() {
	newCount := atomic.AddInt64(&fm.clientCount, -1)
	if newCount < 0 {
		// Imbalance: tried to decrement below zero, clamp to zero
		atomic.StoreInt64(&fm.clientCount, 0)
		imbalanceCount := atomic.AddInt64(&fm.clientImbalance, 1)
		log.Printf("frame manager client count imbalance detected: decrement at count < 0, clamped to 0 (total imbalances=%d)", imbalanceCount)
		return
	}
	log.Printf("🔗 Client count decremented to: %d", newCount)
	if newCount == 0 {
		log.Printf("🛑 Last client disconnected, scheduling capture stop")
		fm.scheduleStopCapture()
	}
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
	// Create a new stopChancel for the new capture cycle
	fm.stopChancel = make(chan struct{})
	atomic.AddInt64(&fm.captureStarts, 1)
	fm.captureMu.Unlock()
	go fm.captureLoop(done)
}

func (fm *FrameManager) scheduleStopCapture() {
	fm.captureMu.Lock()
	if !fm.captureStarted {
		fm.captureMu.Unlock()
		log.Printf("📊 Capture already stopped, no need to schedule stop")
		return
	}

	delay := fm.idleStopDelay
	if delay <= 0 {
		delay = defaultIdleStopDelay
	}

	// Get current stopChancel and done to check against in the cleanup loop
	stopChancel := fm.stopChancel
	done := fm.doneChan
	fm.captureMu.Unlock()

	log.Printf("⏳ Scheduling capture stop in %v (will stop if no new clients connect)", delay)

	// Send stop request to cleanup goroutine instead of spawning new goroutine
	req := cleanupRequest{
		delay:  delay,
		stopCh: stopChancel,
		done:   done,
	}

	timer := time.NewTimer(cleanupEnqueueTimeout)
	defer timer.Stop()

	select {
	case fm.cleanupCh <- req:
		// Request queued successfully
	case <-timer.C:
		// Cleanup queue saturated or cleanupCh closed: fallback to a direct delayed stop check
		// so that stop intent is never dropped.
		log.Printf("⚠️  Cleanup channel enqueue timed out; using fallback stop path")
		fm.fallbackWG.Add(1)
		go fm.delayedStopFallback(req)
	}
}

func (fm *FrameManager) delayedStopFallback(req cleanupRequest) {
	defer fm.fallbackWG.Done()

	timer := time.NewTimer(req.delay)

	select {
	case <-timer.C:
		if fm.stopCaptureIfIdle(req.done) {
			log.Printf("🛑 Stopping capture via fallback idle-stop path")
		}
	case <-req.stopCh:
		if !timer.Stop() {
			<-timer.C
		}
		log.Printf("📊 Fallback stop cancelled: new client connected")
	case <-fm.cleanupStop:
		if !timer.Stop() {
			<-timer.C
		}
	}
}

func (fm *FrameManager) stopCaptureIfIdle(expectedDone chan struct{}) bool {
	fm.captureMu.Lock()
	if !fm.captureStarted || atomic.LoadInt64(&fm.clientCount) > 0 || fm.doneChan != expectedDone {
		fm.captureMu.Unlock()
		return false
	}
	fm.captureStarted = false
	done := fm.doneChan
	fm.captureMu.Unlock()

	close(done)
	return true
}

// cleanupLoop runs in a background goroutine and handles deferred capture stops
func (fm *FrameManager) cleanupLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ PANIC in cleanupLoop: %v", r)
		}
		log.Printf("📊 Cleanup loop EXIT")
		close(fm.cleanupDone)
	}()

	log.Printf("📊 Cleanup loop STARTED")

	for { //nolint:staticcheck // for-select is idiomatic for multi-channel listening goroutine
		select {
		case <-fm.cleanupStop:
			return

		case req, ok := <-fm.cleanupCh:
			if !ok {
				// Channel closed, exit
				return
			}

			timer := time.NewTimer(req.delay)
			select {
			case <-timer.C:
				// Delay expired, proceed with stopping if conditions still met
				if !fm.stopCaptureIfIdle(req.done) {
					log.Printf("📊 Stop cancelled: capture already restarted or new client connected")
					continue
				}
				log.Printf("🛑 Stopping capture (idle timeout expired)")

			case <-req.stopCh:
				if !timer.Stop() {
					<-timer.C
				}
				// Stop was cancelled (new client connected)
				log.Printf("📊 Stop cancelled: new client connected")

			case <-fm.cleanupStop:
				if !timer.Stop() {
					<-timer.C
				}
				return
			}
		}
	}
}

// stopCapture stops the capture loop if currently running.
func (fm *FrameManager) stopCapture() {
	fm.captureMu.Lock()
	if !fm.captureStarted {
		fm.captureMu.Unlock()
		log.Printf("📊 stopCapture called but captureStarted=false, already stopped")
		return
	}
	fm.captureStarted = false
	done := fm.doneChan
	fm.captureMu.Unlock()
	log.Printf("📊 stopCapture: closing done channel to signal captureLoop to exit")
	close(done)
	time.Sleep(50 * time.Millisecond) // Allow goroutine to exit cleanly
	log.Printf("✓ stopCapture: done channel closed")
}

// captureLoop continuously captures frames from the camera and writes to the frame buffer.
func (fm *FrameManager) captureLoop(done <-chan struct{}) {
	log.Printf("🎬 Capture loop STARTED")
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ PANIC in captureLoop: %v", r)
		}
		log.Printf("🎬 Capture loop EXITING")
		fm.captureMu.Lock()
		if fm.doneChan == done {
			fm.captureStarted = false
			log.Printf("🎬 Marked captureStarted=false in defer")
		}
		fm.captureMu.Unlock()
	}()

	retryDelay := initialCaptureRetryDelay
	captureCount := 0

	for {
		select {
		case <-done:
			log.Printf("🎬 Capture loop: done signal received, exiting (captured %d frames)", captureCount)
			return
		default:
		}

		// CaptureFrame blocks until a newer frame sequence is published (or timeout/error),
		// so this loop tracks upstream frame cadence without CPU spinning.
		// Add minimal 1ms sleep to prevent tight spinning on error path retries.
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
				log.Printf("🎬 Capture loop: done signal received during error retry, exiting")
				return
			case <-timer.C:
			}

			retryDelay *= 2
			if retryDelay > maxCaptureRetryDelay {
				retryDelay = maxCaptureRetryDelay
			}
			time.Sleep(1 * time.Millisecond) // Minimal sleep on error to prevent tight spinning
			continue
		}

		if frame != nil {
			captureCount++
			if consecutive := atomic.SwapInt64(&fm.consecutiveCaptureFailures, 0); consecutive > 0 {
				log.Printf("camera capture recovered after %d consecutive failures", consecutive)
			}
			retryDelay = initialCaptureRetryDelay
			_, _ = fm.frameBuffer.WriteImmutable(frame)
		}
	}
}

func (fm *FrameManager) captureFailureStats() (int64, int64, bool) {
	consecutive := atomic.LoadInt64(&fm.consecutiveCaptureFailures)
	total := atomic.LoadInt64(&fm.captureFailureTotal)
	return consecutive, total, consecutive >= captureFailureDegradedThreshold
}

// Stop stops the frame capture loop and cleanup infrastructure.
func (fm *FrameManager) Stop() {
	fm.stopCapture()

	// Close cleanup channel to signal cleanup loop to exit (protect against double-close)
	fm.cleanupChOnce.Do(func() {
		close(fm.cleanupCh)
	})

	fm.cleanupStopOnce.Do(func() {
		close(fm.cleanupStop)
	})

	// Wait for cleanup loop to exit
	<-fm.cleanupDone
	fm.fallbackWG.Wait()
	log.Printf("✓ Cleanup loop exited cleanly")
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
// Manages connection tracking and respects the configurable max connection limit.
func (fm *FrameManager) StreamFrame(w http.ResponseWriter, r *http.Request, maxConnections int) error {
	// Check connection limit
	if !fm.connTracker.TryIncrement(maxConnections) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(fmt.Sprintf("Max stream connections reached (limit: %d)", maxConnections)))
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
	ctx := r.Context()
	contentLengthScratch := make([]byte, 0, 20)

	framesSent := 0
	timeoutCount := 0
	startTime := time.Now()

	for {
		// Re-check streamDone on each iteration to detect capture restarts
		// This prevents stale channel reference when capture cycles
		fm.captureMu.Lock()
		streamDone := fm.doneChan
		fm.captureMu.Unlock()

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

		if err := writeMultipartFrame(w, &contentLengthScratch, frame); err != nil {
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

func writeMultipartFrame(w http.ResponseWriter, contentLengthScratch *[]byte, frame []byte) error {
	// frame is shared immutable bytes from FrameBuffer; only read from it.
	*contentLengthScratch = strconv.AppendInt((*contentLengthScratch)[:0], int64(len(frame)), 10)
	if _, err := w.Write(mjpegBoundaryBytes); err != nil {
		return err
	}
	if _, err := w.Write(mjpegContentTypeBytes); err != nil {
		return err
	}
	if _, err := w.Write(mjpegContentLengthBytes); err != nil {
		return err
	}
	if _, err := w.Write(*contentLengthScratch); err != nil {
		return err
	}
	if _, err := w.Write(mjpegHeaderEndBytes); err != nil {
		return err
	}
	if _, err := w.Write(frame); err != nil {
		return err
	}
	_, err := w.Write(mjpegTrailerBytes)
	return err
}

// ConfigResponse is the JSON response for /api/config endpoint.
// ConfigResponse contains camera configuration and streaming statistics
// @Description Camera configuration, resolution, and streaming performance metrics
type ConfigResponse struct {
	// Camera resolution in pixels [width, height]
	Resolution [2]int `json:"resolution"`
	// Configured frames per second
	FPS int `json:"fps" example:"30"`
	// Target FPS for frame buffer
	TargetFPS int `json:"target_fps" example:"30"`
	// JPEG compression quality (0-100)
	JPEGQuality int `json:"jpeg_quality" example:"85"`
	// Maximum concurrent stream connections allowed
	MaxStreamConnections int `json:"max_stream_connections" example:"5"`
	// Current number of active stream connections
	CurrentStreamCount int `json:"current_stream_connections" example:"1"`
	// Total number of frames captured
	FrameCount int64 `json:"frames_captured" example:"1500"`
	// Current FPS being delivered
	CurrentFPS float64 `json:"current_fps" example:"29.5"`
	// Age of the last frame in seconds
	LastFrameAgeSeconds float64 `json:"last_frame_age_seconds" example:"0.033"`
}

// HealthResponse is the JSON response for /health endpoint.
// @Description Health status of the camera system including connectivity and performance metrics
type HealthResponse struct {
	// Overall status: ok, degraded, error
	Status string `json:"status" example:"ok"`
	// Whether the camera is initialized and ready
	CameraReady bool `json:"camera_ready" example:"true"`
	// System uptime in seconds
	UptimeSeconds int64 `json:"uptime_seconds" example:"3600"`
	// Current number of active stream connections
	StreamConnections int `json:"stream_connections" example:"2"`
	// Frames per second being captured
	FramesPerSecond float64 `json:"fps" example:"29.8"`
	// Whether system is operating in degraded mode (optional)
	Degraded bool `json:"degraded,omitempty" example:"false"`
	// Number of consecutive capture failures (optional)
	CaptureFailures int64 `json:"capture_consecutive_failures,omitempty" example:"0"`
	// Total number of capture failures (optional)
	CaptureErrorsTotal int64 `json:"capture_failures_total,omitempty" example:"5"`
}

// DiagnosticsResponse is the JSON response for /api/diagnostics endpoint.
// @Description Comprehensive diagnostics including health metrics, error rates, and system status
type DiagnosticsResponse struct {
	// Overall status: ok, degraded, error
	Status string `json:"status" example:"ok"`
	// Whether the camera is ready
	CameraReady bool `json:"camera_ready" example:"true"`
	// Current FPS being delivered
	FramesPerSecond float64 `json:"fps" example:"29.8"`
	// System uptime in seconds
	UptimeSeconds int64 `json:"uptime_seconds" example:"7200"`
	// Current stream connection count
	StreamConnections int `json:"stream_connections" example:"1"`
	// Total frames successfully captured
	FramesCaptured int64 `json:"frames_captured" example:"216000"`
	// Age of the last frame in seconds
	LastFrameAgeSeconds float64 `json:"last_frame_age_seconds" example:"0.034"`
	// Resolution as string (e.g., '1920x1080')
	Resolution string `json:"resolution" example:"1920x1080"`
	// JPEG quality setting (0-100)
	JPEGQuality int `json:"jpeg_quality" example:"85"`
	// Maximum concurrent connections
	MaxConnections int `json:"max_stream_connections" example:"5"`
	// Recent consecutive capture failures
	CaptureFailures int64 `json:"capture_failures_recent" example:"0"`
	// Total capture failures
	CaptureFailuresTotal int64 `json:"capture_failures_total" example:"10"`
	// Error rate as percentage
	ErrorRate float64 `json:"error_rate_percent" example:"0.5"`
	// Health status text: Excellent, Degraded, Poor
	HealthStatus string `json:"health_status" example:"Excellent"`
	// Detailed status message
	Message string `json:"message" example:"Camera is functioning normally"`
}

// RegisterHandlers registers all API endpoints with the Chi router.
// RegisterHandlers registers all API routes with versioning support
func RegisterHandlers(router *chi.Mux, fm *FrameManager, cfg *config.Config) {
	startTime := time.Now()

	// Middleware
	router.Use(corsMiddleware)
	router.Use(loggingMiddleware)

	// Register web UI (must be before other handlers for proper routing)
	web.RegisterStaticFiles(router)

	// Register Swagger UI
	router.Get("/docs/*", httpSwagger.Handler())

	// Register API routes with versioning
	router.Route("/v1", func(r chi.Router) {
		registerV1Handlers(r, fm, cfg, startTime)
	})

	// Register unversioned legacy endpoints for backward compatibility
	registerLegacyHandlers(router, fm, cfg, startTime)
}

// registerV1Handlers registers all v1 API endpoints
func registerV1Handlers(router chi.Router, fm *FrameManager, cfg *config.Config, startTime time.Time) {
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

	// OctoPrint-compatible stream endpoint
	router.Get("/webcam", func(w http.ResponseWriter, r *http.Request) {
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

	router.Post("/api/stream/stop", func(w http.ResponseWriter, r *http.Request) {
		handleStopStream(w, r, fm)
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

// registerLegacyHandlers registers unversioned endpoints for backward compatibility
func registerLegacyHandlers(router chi.Router, fm *FrameManager, cfg *config.Config, startTime time.Time) {
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
			_ = err
		}
	})

	// OctoPrint-compatible stream endpoint
	router.Get("/webcam", func(w http.ResponseWriter, r *http.Request) {
		if err := fm.StreamFrame(w, r, cfg.MaxStreamConnections); err != nil {
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

	router.Post("/api/stream/stop", func(w http.ResponseWriter, r *http.Request) {
		handleStopStream(w, r, fm)
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

// @Summary Health check endpoint
// @Description Returns the current health status of the camera system including uptime, FPS, and connection count
// @Tags Health
// @Accept  json
// @Produce json
// @Success 200 {object} HealthResponse
// @Failure 503 {object} map[string]string
// @Router /v1/health [get]
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

// @Summary Readiness probe
// @Description Returns 200 if camera is ready, 503 if still initializing
// @Tags Health
// @Accept  json
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /v1/ready [get]
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

// @Summary Get current snapshot
// @Description Returns the latest captured frame as a JPEG image
// @Tags Streaming
// @Accept  json
// @Produce image/jpeg
// @Success 200 {file} binary "JPEG image data"
// @Failure 503 {string} string "Camera not ready"
// @Router /v1/snapshot.jpg [get]
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

// @Summary Get configuration and statistics
// @Description Returns camera configuration including resolution, FPS, stream connections, and performance metrics
// @Tags Configuration
// @Accept  json
// @Produce json
// @Success 200 {object} ConfigResponse
// @Router /v1/api/config [get]
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

// @Summary Get system status
// @Description Returns detailed status including health state, capture failures, and degradation status
// @Tags Health
// @Accept  json
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /v1/api/status [get]
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

// @Summary Get all settings
// @Description Retrieves all saved application settings
// @Tags Settings
// @Accept  json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/api/settings [get]
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
// @Description Request body for updating multiple settings at once
type SettingsUpdateRequest struct {
	// Map of setting keys to values
	Settings map[string]interface{} `json:"settings"`
}

// ErrorResponse is a standardized error response for all API errors
// @Description Standard error response format for API errors
type ErrorResponse struct {
	// HTTP status code
	Code int `json:"code" example:"400"`
	// Error message
	Message string `json:"message" example:"Invalid request"`
	// Optional detailed error information
	Details string `json:"details,omitempty" example:"Settings map is empty"`
}

// @Summary Get comprehensive diagnostics
// @Description Returns complete system diagnostics including health status, error metrics, and performance data
// @Tags Diagnostics
// @Accept  json
// @Produce json
// @Success 200 {object} DiagnosticsResponse
// @Router /v1/api/diagnostics [get]
func handleDiagnostics(w http.ResponseWriter, r *http.Request, fm *FrameManager, cfg *config.Config, startTime time.Time) {
	frameCount, _, fps := fm.streamStats.Snapshot()

	status := "ok"
	message := "Camera is functioning normally"
	if !fm.cam.IsReady() {
		status = "error"
		message = "Camera is not ready or failed to initialize"
	}

	// Calculate capture error metrics
	consecutiveFailures, totalFailures, degraded := fm.captureFailureStats()

	// Calculate error rate
	var errorRate float64
	if frameCount > 0 {
		errorRate = (float64(totalFailures) / (float64(totalFailures) + float64(frameCount))) * 100
	}

	// Determine health status
	healthStatus := "Excellent"
	if errorRate > 5 {
		healthStatus = "Degraded"
		status = "degraded"
	}
	if errorRate > 20 || consecutiveFailures > 5 {
		healthStatus = "Poor"
		status = "error"
	}
	if degraded && status != "error" {
		healthStatus = "Degraded"
		status = "degraded"
	}

	resolution := fmt.Sprintf("%dx%d", cfg.Resolution[0], cfg.Resolution[1])

	response := DiagnosticsResponse{
		Status:               status,
		CameraReady:          fm.cam.IsReady(),
		FramesPerSecond:      fps,
		UptimeSeconds:        int64(time.Since(startTime).Seconds()),
		StreamConnections:    fm.connTracker.Count(),
		FramesCaptured:       frameCount,
		LastFrameAgeSeconds:  fm.streamStats.LastFrameAgeSeconds(time.Now().UnixNano()),
		Resolution:           resolution,
		JPEGQuality:          cfg.JPEGQuality,
		MaxConnections:       cfg.MaxStreamConnections,
		CaptureFailures:      consecutiveFailures,
		CaptureFailuresTotal: totalFailures,
		ErrorRate:            errorRate,
		HealthStatus:         healthStatus,
		Message:              message,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		_ = err
	}
}

// @Summary Update settings
// @Description Updates one or more application settings. Accepts both POST and PUT requests
// @Tags Settings
// @Accept  json
// @Produce json
// @Param   request body SettingsUpdateRequest true "Settings to update"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/api/settings [post]
// @Router /v1/api/settings [put]
func handleSettingsUpdate(w http.ResponseWriter, r *http.Request, fm *FrameManager) {
	var req SettingsUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := fm.settingsM.SetMany(req.Settings); err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to save settings", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("saved %d settings", len(req.Settings)),
	})
}

// @Summary Stop stream capture
// @Description Stops the camera stream capture and disconnects all clients
// @Tags Streaming
// @Accept  json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /v1/api/stream/stop [post]
func handleStopStream(w http.ResponseWriter, r *http.Request, fm *FrameManager) {
	log.Printf("🛑 API: Stop stream requested by client")

	// Use the proper stopCapture method to cleanly shut down
	fm.stopCapture()

	// Also reset client count to ensure cleanup
	atomic.StoreInt64(&fm.clientCount, 0)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "stream stopped",
	})
}

// Middleware

func writeErrorResponse(w http.ResponseWriter, statusCode int, message string, details string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	err := json.NewEncoder(w).Encode(ErrorResponse{
		Code:    statusCode,
		Message: message,
		Details: details,
	})
	if err != nil {
		// Client disconnected, ignore
		_ = err
	}
}

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
