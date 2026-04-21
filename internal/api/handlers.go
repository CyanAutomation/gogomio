package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/CyanAutomation/gogomio/docs"
	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/CyanAutomation/gogomio/internal/settings"
	"github.com/CyanAutomation/gogomio/internal/web"
	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger"
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
	fm.doneChan = nil
	fm.captureMu.Unlock()

	if done != nil {
		close(done)
	}
	return true
}

// cleanupLoop runs in a background goroutine and handles deferred capture stops
func (fm *FrameManager) cleanupLoop() {
	defer func() {
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
				continue

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
	fm.doneChan = nil
	fm.captureMu.Unlock()
	log.Printf("📊 stopCapture: closing done channel to signal captureLoop to exit")
	if done != nil {
		close(done)
	}
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

// GetFrameSequence returns the current frame sequence number
func (fm *FrameManager) GetFrameSequence() uint64 {
	return fm.frameBuffer.CurrentSequence()
}

// GetCaptureRestartCount returns the number of times capture has been restarted
func (fm *FrameManager) GetCaptureRestartCount() int64 {
	return atomic.LoadInt64(&fm.captureStarts)
}

// GetCaptureFailures returns consecutive failures, total failures, and degraded state
func (fm *FrameManager) GetCaptureFailures() (consecutive, total int64, degraded bool) {
	return fm.captureFailureStats()
}

// GetClientImbalance returns the client imbalance counter (debug metric)
func (fm *FrameManager) GetClientImbalance() int64 {
	return atomic.LoadInt64(&fm.clientImbalance)
}

// StreamFrame writes frames to an HTTP response in MJPEG format.
// Manages connection tracking and respects the configurable max connection limit.
func (fm *FrameManager) StreamFrame(w http.ResponseWriter, r *http.Request, maxConnections int) error {
	// Check connection limit
	if !fm.connTracker.TryIncrement(maxConnections) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprintf(w, "Max stream connections reached (limit: %d)", maxConnections)
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
// CameraConfigResponse contains static camera configuration
// @Description Static camera configuration settings (resolution, quality, limits)
type CameraConfigResponse struct {
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
	// ISO8601 timestamp of this response
	TimestampISO8601 string `json:"timestamp_iso8601" example:"2026-04-19T15:30:45Z"`
	// API version
	APIVersion string `json:"api_version" example:"1"`
}

// LiveMetricsResponse contains dynamic, real-time metrics
// @Description Current performance and connection metrics
type LiveMetricsResponse struct {
	// Current frames per second being delivered
	FPSCurrent float64 `json:"fps_current" example:"29.8"`
	// Configured target frames per second
	FPSConfigured int `json:"fps_configured" example:"30"`
	// Total frames captured since startup
	FramesCaptured int64 `json:"frames_captured" example:"216000"`
	// Age of the most recent frame in seconds
	LastFrameAgeSeconds float64 `json:"last_frame_age_seconds" example:"0.034"`
	// System uptime in seconds
	UptimeSeconds int64 `json:"uptime_seconds" example:"7200"`
	// Current number of active stream connections
	StreamConnections int `json:"stream_connections" example:"2"`
	// Frame sequence number (increments per frame, useful for detecting gaps)
	FrameSequenceNumber uint64 `json:"frame_sequence_number" example:"216001"`
	// ISO8601 timestamp of this response
	TimestampISO8601 string `json:"timestamp_iso8601" example:"2026-04-19T15:30:45Z"`
	// API version
	APIVersion string `json:"api_version" example:"1"`
}

// HealthResponse is a quick health check response
// @Description Quick health status (suitable for Kubernetes probes)
type HealthResponse struct {
	// Overall status: ok, degraded, error
	Status string `json:"status" example:"ok"`
	// Whether the camera is initialized and ready
	CameraReady bool `json:"camera_ready" example:"true"`
	// Whether system is operating in degraded mode
	Degraded bool `json:"degraded" example:"false"`
	// Current number of active stream connections
	StreamConnections int `json:"stream_connections" example:"2"`
	// Current frames per second
	FPSCurrent float64 `json:"fps_current" example:"29.8"`
	// System uptime in seconds
	UptimeSeconds int64 `json:"uptime_seconds" example:"3600"`
	// ISO8601 timestamp of this response
	TimestampISO8601 string `json:"timestamp_iso8601" example:"2026-04-19T15:30:45Z"`
	// API version
	APIVersion string `json:"api_version" example:"1"`
}

// DetailedHealthResponse provides comprehensive health and metrics information
// @Description Comprehensive health status with detailed metrics, error tracking, and diagnostics
type DetailedHealthResponse struct {
	// Overall status: ok, degraded, error
	Status string `json:"status" example:"ok"`
	// Health status text: Excellent, Degraded, Poor
	HealthStatus string `json:"health_status" example:"Excellent"`
	// Detailed status message
	Message string `json:"message" example:"Camera is functioning normally"`
	// Whether the camera is initialized and ready
	CameraReady bool `json:"camera_ready" example:"true"`
	// Whether system is operating in degraded mode
	Degraded bool `json:"degraded" example:"false"`
	// System uptime in seconds
	UptimeSeconds int64 `json:"uptime_seconds" example:"7200"`
	// Current frames per second
	FPSCurrent float64 `json:"fps_current" example:"29.8"`
	// Configured frames per second
	FPSConfigured int `json:"fps_configured" example:"30"`
	// Total frames captured since startup
	FramesCaptured int64 `json:"frames_captured" example:"216000"`
	// Current number of active stream connections
	StreamConnections int `json:"stream_connections" example:"1"`
	// Age of the most recent frame in seconds
	LastFrameAgeSeconds float64 `json:"last_frame_age_seconds" example:"0.034"`
	// Resolution as string (e.g., '1920x1080')
	Resolution string `json:"resolution" example:"1920x1080"`
	// JPEG quality setting (0-100)
	JPEGQuality int `json:"jpeg_quality" example:"85"`
	// Maximum concurrent connections
	MaxConnections int `json:"max_stream_connections" example:"5"`
	// Consecutive capture failures (resets on success)
	CaptureFailuresConsecutive int64 `json:"capture_failures_consecutive" example:"0"`
	// Total capture failures since startup
	CaptureFailuresTotal int64 `json:"capture_failures_total" example:"10"`
	// Number of times capture has been restarted
	CaptureRestartCount int64 `json:"capture_restart_count" example:"2"`
	// Error rate as percentage
	ErrorRatePercent float64 `json:"error_rate_percent" example:"0.5"`
	// Frame sequence number (increments per frame)
	FrameSequenceNumber uint64 `json:"frame_sequence_number" example:"216001"`
	// ISO8601 timestamp of this response
	TimestampISO8601 string `json:"timestamp_iso8601" example:"2026-04-19T15:30:45Z"`
	// API version
	APIVersion string `json:"api_version" example:"1"`
}

// RateLimiter implements per-IP rate limiting for API endpoints
type RateLimiter struct {
	mu        sync.Mutex
	requests  map[string]*ipRequestTracker
	maxReqSec int
	window    time.Duration
}

// ipRequestTracker tracks requests for a single IP
type ipRequestTracker struct {
	count     int
	lastReset time.Time
}

// NewRateLimiter creates a new rate limiter (maxReqSec requests per window duration)
func NewRateLimiter(maxReqSec int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests:  make(map[string]*ipRequestTracker),
		maxReqSec: maxReqSec,
		window:    window,
	}
}

// Allow checks if an IP is within rate limit
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	tracker, exists := rl.requests[ip]

	// Initialize or reset if window expired
	if !exists || now.Sub(tracker.lastReset) > rl.window {
		rl.requests[ip] = &ipRequestTracker{
			count:     1,
			lastReset: now,
		}
		return true
	}

	// Check limit
	if tracker.count < rl.maxReqSec {
		tracker.count++
		return true
	}

	return false
}

// rateLimitMiddleware enforces per-IP rate limiting on API endpoints
func rateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get normalized client IP key
			ip := ""
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				for _, token := range strings.Split(forwarded, ",") {
					candidate := strings.TrimSpace(token)
					if candidate != "" {
						ip = candidate
						break
					}
				}
			}
			if ip == "" {
				host, _, err := net.SplitHostPort(r.RemoteAddr)
				if err == nil && host != "" {
					ip = host
				} else {
					ip = r.RemoteAddr
				}
			}

			// Check rate limit
			if !limiter.Allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(ErrorResponse{
					Code:    http.StatusTooManyRequests,
					Message: "Rate limit exceeded",
					Details: "Too many requests from this IP address",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RegisterHandlers registers all API endpoints with the Chi router.
// RegisterHandlers registers all API routes with versioning support
func RegisterHandlers(router *chi.Mux, fm *FrameManager, cfg *config.Config) {
	startTime := time.Now()

	// Create rate limiter: 100 requests per 10 seconds per IP
	rateLimiter := NewRateLimiter(100, 10*time.Second)

	// Middleware
	router.Use(corsMiddleware)
	router.Use(loggingMiddleware)

	// Register web UI (must be before other handlers for proper routing)
	web.RegisterStaticFiles(router)

	// Register Swagger UI
	router.Get("/docs/*", httpSwagger.Handler())

	// Register API routes with versioning
	router.Route("/v1", func(r chi.Router) {
		// Apply rate limiting to v1 endpoints
		r.Use(rateLimitMiddleware(rateLimiter))
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

	// New refactored config and metrics endpoints
	router.Get("/config/camera", func(w http.ResponseWriter, r *http.Request) {
		handleCameraConfig(w, r, cfg)
	})

	router.Get("/metrics/live", func(w http.ResponseWriter, r *http.Request) {
		handleLiveMetrics(w, r, fm, cfg, startTime)
	})

	router.Get("/health/detailed", func(w http.ResponseWriter, r *http.Request) {
		handleDetailedHealth(w, r, fm, cfg, startTime)
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

// @Summary Quick health check (Kubernetes probe)
// @Description Returns quick camera health status suitable for Kubernetes liveness probes. Returns 200 OK if running (regardless of camera state), and includes status, fps, uptime, and connection count. For detailed health with metrics, use /v1/health/detailed instead.
// @Tags Health
// @Accept  json
// @Produce json
// @Success 200 {object} HealthResponse "Camera status ok/degraded/error"
// @Failure 503 {object} ErrorResponse "Service unavailable (should not occur in normal operation)"
// @Router /v1/health [get]
func handleHealth(w http.ResponseWriter, r *http.Request, fm *FrameManager, startTime time.Time) {
	_, _, fps := fm.streamStats.Snapshot()
	_, _, degraded := fm.GetCaptureFailures()

	status := "ok"
	if !fm.cam.IsReady() {
		status = "error"
	} else if degraded {
		status = "degraded"
	}

	response := HealthResponse{
		Status:            status,
		CameraReady:       fm.cam.IsReady(),
		Degraded:          degraded,
		StreamConnections: fm.connTracker.Count(),
		FPSCurrent:        fps,
		UptimeSeconds:     int64(time.Since(startTime).Seconds()),
		TimestampISO8601:  time.Now().UTC().Format(time.RFC3339),
		APIVersion:        "1",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		_ = err
	}
}

// @Summary Get camera configuration
// @Description Returns static camera configuration (resolution, FPS, quality settings, connection limits). Use this for UI configuration displays or to understand camera capabilities. Does not include runtime metrics; use /v1/metrics/live for current performance data.
// @Tags Configuration
// @Accept  json
// @Produce json
// @Success 200 {object} CameraConfigResponse "Static configuration settings"
// @Router /v1/config/camera [get]
func handleCameraConfig(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	response := CameraConfigResponse{
		Resolution:           cfg.Resolution,
		FPS:                  cfg.FPS,
		TargetFPS:            cfg.TargetFPS,
		JPEGQuality:          cfg.JPEGQuality,
		MaxStreamConnections: cfg.MaxStreamConnections,
		TimestampISO8601:     time.Now().UTC().Format(time.RFC3339),
		APIVersion:           "1",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		_ = err
	}
}

// @Summary Get live performance metrics
// @Description Returns current runtime performance metrics: frame rate, captured frames, stream connections, and frame sequence number. Lightweight and suitable for frequent polling (combine with rate limiting awareness). Use /v1/config/camera for static configuration values.
// @Tags Metrics
// @Accept  json
// @Produce json
// @Success 200 {object} LiveMetricsResponse "Current performance metrics"
// @Failure 429 {object} ErrorResponse "Rate limit exceeded (100 req/10sec per IP)"
// @Router /v1/metrics/live [get]
func handleLiveMetrics(w http.ResponseWriter, r *http.Request, fm *FrameManager, cfg *config.Config, startTime time.Time) {
	frameCount, _, fps := fm.streamStats.Snapshot()

	response := LiveMetricsResponse{
		FPSCurrent:          fps,
		FPSConfigured:       cfg.FPS,
		FramesCaptured:      frameCount,
		LastFrameAgeSeconds: fm.streamStats.LastFrameAgeSeconds(time.Now().UnixNano()),
		UptimeSeconds:       int64(time.Since(startTime).Seconds()),
		StreamConnections:   fm.connTracker.Count(),
		FrameSequenceNumber: fm.GetFrameSequence(),
		TimestampISO8601:    time.Now().UTC().Format(time.RFC3339),
		APIVersion:          "1",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		_ = err
	}
}

// @Summary Get comprehensive health and diagnostics
// @Description Returns all available health information, metrics, and diagnostics in a single request: status, uptime, FPS, frames captured, connections, error rates, restart count, frame sequence, and detailed failure tracking. Recommended for dashboards, monitoring systems, and troubleshooting. Note: more data than /v1/health or /v1/metrics/live, use appropriate endpoint for your use case.
// @Tags Health
// @Accept  json
// @Produce json
// @Success 200 {object} DetailedHealthResponse "Complete health and metrics snapshot"
// @Failure 429 {object} ErrorResponse "Rate limit exceeded (100 req/10sec per IP)"
// @Router /v1/health/detailed [get]
func handleDetailedHealth(w http.ResponseWriter, r *http.Request, fm *FrameManager, cfg *config.Config, startTime time.Time) {
	frameCount, _, fps := fm.streamStats.Snapshot()
	consecutiveFailures, totalFailures, degraded := fm.GetCaptureFailures()

	status := "ok"
	message := "Camera is functioning normally"
	if !fm.cam.IsReady() {
		status = "error"
		message = "Camera is not ready or failed to initialize"
	} else if degraded {
		status = "degraded"
		message = "Camera is functioning but experiencing degradation"
	}

	// Calculate error rate
	var errorRate float64
	if frameCount > 0 {
		errorRate = (float64(totalFailures) / (float64(totalFailures) + float64(frameCount))) * 100
	}

	// Determine health status
	healthStatus := "Excellent"
	if errorRate > 5 {
		healthStatus = "Degraded"
		if status == "ok" {
			status = "degraded"
		}
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

	response := DetailedHealthResponse{
		Status:                     status,
		HealthStatus:               healthStatus,
		Message:                    message,
		CameraReady:                fm.cam.IsReady(),
		Degraded:                   degraded,
		UptimeSeconds:              int64(time.Since(startTime).Seconds()),
		FPSCurrent:                 fps,
		FPSConfigured:              cfg.FPS,
		FramesCaptured:             frameCount,
		StreamConnections:          fm.connTracker.Count(),
		LastFrameAgeSeconds:        fm.streamStats.LastFrameAgeSeconds(time.Now().UnixNano()),
		Resolution:                 resolution,
		JPEGQuality:                cfg.JPEGQuality,
		MaxConnections:             cfg.MaxStreamConnections,
		CaptureFailuresConsecutive: consecutiveFailures,
		CaptureFailuresTotal:       totalFailures,
		CaptureRestartCount:        fm.GetCaptureRestartCount(),
		ErrorRatePercent:           errorRate,
		FrameSequenceNumber:        fm.GetFrameSequence(),
		TimestampISO8601:           time.Now().UTC().Format(time.RFC3339),
		APIVersion:                 "1",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		_ = err
	}
}

// @Summary Readiness probe (Kubernetes)
// @Description Returns 200 only when camera is fully initialized and ready to stream. Returns 503 during startup. Suitable for Kubernetes readiness probes to control traffic routing. Use /v1/health for liveness checks instead.
// @Tags Health
// @Accept  json
// @Produce json
// @Success 200 {object} map[string]string "Status: ready"
// @Failure 503 {object} map[string]string "Camera still initializing"
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

// @Summary Get current snapshot as JPEG
// @Description Returns the latest captured video frame as a JPEG image. Suitable for web UI embedding, thumbnails, or periodic frame capture. Respects JPEG quality setting from /v1/config/camera. Returns 503 if camera not yet initialized.
// @Tags Streaming
// @Accept  json
// @Produce image/jpeg
// @Success 200 {file} binary "JPEG image data (raw binary, Content-Type: image/jpeg)"
// @Failure 503 {object} ErrorResponse "Camera not ready or not initialized yet"
// @Failure 429 {object} ErrorResponse "Rate limit exceeded (100 req/10sec per IP)"
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

// @Summary Get configuration and metrics (DEPRECATED)
// @Description **DEPRECATED in v0.1.0** - This endpoint mixes two concerns (static config + live metrics). **Migrate to**: Use /v1/config/camera for static settings, and /v1/metrics/live for performance metrics. Legacy endpoint will be removed in v0.3.0. See README for migration guide.
// @Tags Configuration
// @Accept  json
// @Produce json
// @Success 200 {object} map[string]interface{} "Contains all config + metrics fields (see examples)"
// @Failure 429 {object} ErrorResponse "Rate limit exceeded (100 req/10sec per IP)"
// @Deprecated true
// @Router /v1/api/config [get]
func handleAPIConfigure(w http.ResponseWriter, r *http.Request, fm *FrameManager, cfg *config.Config, startTime time.Time) {
	// Deprecated: combines static config with live metrics
	// Return a merged response for backward compatibility
	frameCount, _, fps := fm.streamStats.Snapshot()

	response := map[string]interface{}{
		"resolution":                 cfg.Resolution,
		"fps":                        cfg.FPS,
		"target_fps":                 cfg.TargetFPS,
		"jpeg_quality":               cfg.JPEGQuality,
		"max_stream_connections":     cfg.MaxStreamConnections,
		"current_stream_connections": fm.connTracker.Count(),
		"frames_captured":            frameCount,
		"current_fps":                fps,
		"last_frame_age_seconds":     fm.streamStats.LastFrameAgeSeconds(time.Now().UnixNano()),
		"timestamp_iso8601":          time.Now().UTC().Format(time.RFC3339),
		"api_version":                "1",
		"_deprecated":                "use /v1/config/camera and /v1/metrics/live instead",
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Deprecation", "true")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		_ = err
	}
}

// @Summary Get system status (DEPRECATED)
// @Description **DEPRECATED in v0.1.0** - **Migrate to**: Use /v1/health/detailed instead (identical response). Legacy endpoint will be removed in v0.3.0. See README for migration guide.
// @Tags Health
// @Accept  json
// @Produce json
// @Success 200 {object} DetailedHealthResponse "Complete health snapshot"
// @Failure 429 {object} ErrorResponse "Rate limit exceeded (100 req/10sec per IP)"
// @Deprecated true
// @Router /v1/api/status [get]
func handleAPIStatus(w http.ResponseWriter, r *http.Request, fm *FrameManager, startTime time.Time) {
	// Deprecated: use /v1/health/detailed instead
	handleDetailedHealth(w, r, fm, &config.Config{}, startTime) // Note: passing empty config for now - this is deprecated anyway
}

// Settings handlers

// @Summary Get all settings
// @Description Retrieves all saved application settings as a JSON object. Settings can include brightness, contrast, saturation, and other camera-specific parameters. Returns empty object if no settings saved yet. Use PUT or POST with /v1/api/settings to update.
// @Tags Settings
// @Accept  json
// @Produce json
// @Success 200 {object} map[string]interface{} "Object containing all saved settings (example: {brightness: 100, contrast: 120})"
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

// @Summary Get comprehensive diagnostics (DEPRECATED)
// @Description **DEPRECATED in v0.1.0** - **Migrate to**: Use /v1/health/detailed instead (identical response). Provides complete system diagnostics including health status, metrics, error tracking, and failure statistics. Legacy endpoint will be removed in v0.3.0. See README for migration guide.
// @Tags Diagnostics
// @Accept  json
// @Produce json
// @Success 200 {object} DetailedHealthResponse "Complete health and diagnostics snapshot"
// @Failure 429 {object} ErrorResponse "Rate limit exceeded (100 req/10sec per IP)"
// @Deprecated true
// @Router /v1/api/diagnostics [get]
func handleDiagnostics(w http.ResponseWriter, r *http.Request, fm *FrameManager, cfg *config.Config, startTime time.Time) {
	// Deprecated: use /v1/health/detailed instead
	w.Header().Set("Deprecation", "true")
	handleDetailedHealth(w, r, fm, cfg, startTime)
}

// @Summary Update camera settings
// @Description Updates one or more application settings (brightness, contrast, saturation, etc). Accepts POST and PUT requests. Only specified settings are updated; omitted settings are preserved. Returns updated settings. Settings persist across restarts.
// @Tags Settings
// @Accept  json
// @Produce json
// @Param   request body SettingsUpdateRequest true "Settings to update (example: {\"brightness\": 150, \"contrast\": 120})"
// @Success 200 {object} map[string]string "Updated settings"
// @Failure 400 {object} ErrorResponse "Invalid JSON or request format"
// @Failure 500 {object} ErrorResponse "Failed to save settings to persistent storage"
// @Failure 429 {object} ErrorResponse "Rate limit exceeded (100 req/10sec per IP)"
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
