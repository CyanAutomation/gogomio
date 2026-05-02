// GoGoMio API
// @title GoGoMio API
// @version 0.1.0
// @description IP camera streaming and management API with MJPEG video streaming, real-time health monitoring, and camera configuration management. Designed for Raspberry Pi CSI cameras and compatible devices.
// @description
// @description ## Rate Limiting
// @description All API endpoints are subject to per-IP rate limiting: **100 requests per 10 seconds per IP address**. Requests exceeding this limit will receive HTTP 429 (Too Many Requests) responses.
// @description
// @description ## Authentication & Security
// @description ⚠️ **IMPORTANT**: This API has no built-in authentication. It is designed for private/internal networks only.
// @description - Do NOT expose this service directly to the internet
// @description - Deploy behind a firewall, VPN, or reverse proxy with authentication
// @description - Use HTTPS-terminating reverse proxy (nginx, Caddy, etc.) for HTTPS support
// @description - See Security section in README.md for deployment guidelines
// @description
// @description ## API Versioning
// @description - Current version: v0.1.0 (Preview/MVP)
// @description - Endpoints follow semantic versioning at /v1/ path
// @description - Legacy endpoints at / are maintained for backward compatibility but marked as deprecated
// @contact.name GoGoMio Support
// @contact.url https://github.com/CyanAutomation/gogomio
// @license.name MIT
// @license.url https://github.com/CyanAutomation/gogomio/blob/main/LICENSE
// @host localhost:8000
// @basePath /
// @schemes http https
package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/CyanAutomation/gogomio/internal/api"
	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/cli"
	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/go-chi/chi/v5"
)

var (
	Version   = "0.1.0-dev"
	BuildTime = "dev"
)

func main() {
	// Detect mode: CLI or server
	if len(os.Args) > 1 && os.Args[1] != "server" {
		// CLI mode: execute CLI command
		cli.Execute()
	} else {
		// Server mode (default): start the HTTP server
		startServer()
	}
}

// startServer initializes and runs the HTTP server
func startServer() {
	// Load configuration from environment variables
	cfg := config.LoadFromEnv()

	log.Printf("🌊 Motion In Ocean - Go Edition v%s", Version)
	log.Printf("Configuration: %s", cfg.String())

	// Initialize and start camera
	cam, backend, err := initializeCamera(
		cfg,
		func() camera.Camera { return camera.NewRealCamera() },
		func() camera.Camera { return camera.NewMockCamera() },
	)
	if err != nil {
		log.Fatalf("Failed to initialize camera: %v", err)
	}
	defer func() {
		if err := cam.Stop(); err != nil {
			log.Printf("Error stopping camera: %v", err)
		}
	}()

	log.Printf("Camera backend initialized: %s", backend)
	log.Printf("Camera capture started: %dx%d @ %d FPS", cfg.Resolution[0], cfg.Resolution[1], cfg.FPS)

	// Create HTTP router and register handlers
	router := chi.NewRouter()
	frameManager := api.NewFrameManager(cam, cfg)
	defer frameManager.Stop()

	api.RegisterHandlers(router, frameManager, cfg)

	// Start pprof profiling server on separate port
	go func() {
		log.Printf("🔍 Profiling server listening on http://localhost:6060/debug/pprof")
		if err := http.ListenAndServe(":6060", nil); err != nil && err != http.ErrServerClosed {
			log.Printf("Profiling server error: %v", err)
		}
	}()

	// Log goroutine count periodically
	go logGoroutineStats()

	// Setup HTTP server
	addr := cfg.AddressString()
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutdown signal received, stopping server...")
		if err := server.Close(); err != nil {
			log.Printf("Error closing server: %v", err)
		}
	}()

	// Start server
	log.Printf("Listening on http://%s", addr)
	log.Printf("Stream: http://%s/stream.mjpg", addr)
	log.Printf("Snapshot: http://%s/snapshot.jpg", addr)
	log.Printf("API: http://%s/api/config", addr)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	log.Println("Server stopped")
}

func initializeCamera(
	cfg *config.Config,
	newRealCamera func() camera.Camera,
	newMockCamera func() camera.Camera,
) (camera.Camera, string, error) {
	return initializeCameraWithLogger(log.Default(), cfg, newRealCamera, newMockCamera)
}

func initializeCameraWithLogger(
	logger *log.Logger,
	cfg *config.Config,
	newRealCamera func() camera.Camera,
	newMockCamera func() camera.Camera,
) (camera.Camera, string, error) {
	if cfg.MockCamera {
		logger.Println("🎬 Initializing camera backend: mock (development mode)")
		cam := newMockCamera()
		if err := cam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
			return nil, "", err
		}
		return cam, "mock", nil
	}

	// Try real camera first if device is available.
	logger.Println("📹 Initializing camera backend: real (Raspberry Pi CSI)")
	logger.Println("   Checking for CSI camera access...")
	realCam := newRealCamera()
	if err := realCam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
		attemptedBackend := "unknown"
		failureReason := err.Error()
		var initErr *camera.InitializationError
		if errors.As(err, &initErr) {
			if initErr.Backend != "" {
				attemptedBackend = initErr.Backend
			}
			if initErr.Reason != "" {
				failureReason = initErr.Reason
			}
		}

		logger.Printf("❌ Real camera initialization failed")
		logger.Printf("   Backend attempted: %s", attemptedBackend)
		logger.Printf("   Failure reason: %s", failureReason)
		if !errors.As(err, &initErr) || errors.Unwrap(err) != nil {
			logger.Printf("   Error details: %v", err)
		}
		logger.Println("   Troubleshooting steps:")
		logger.Println("   1. Verify CSI camera is physically connected to the camera port")
		logger.Println("   2. Enable camera in raspi-config: sudo raspi-config → Interface → Camera")
		logger.Println("   3. Check device permissions: ls -la /dev/video*")
		logger.Println("   4. Verify container has device access (see docker-compose.yml devices section)")
		logger.Println("")
		logger.Println("   For optimal performance (native CSI camera support):")
		logger.Println("   - libcamera-vid binary should be available in the container")
		logger.Println("   - Check: docker exec gogomio which libcamera-vid")
		logger.Println("   - If not found, libcamera-apps package may need to be installed from Raspberry Pi repos")
		logger.Println("")
		logger.Println("   Note: RealCamera may internally try FFmpeg/V4L2 as an alternative backend.")
		logger.Println("   Switching runtime camera backend to mock-fallback mode.")
		logger.Println("🎬 Initializing camera backend: mock-fallback (development mode)")
		cam := newMockCamera()
		if err := cam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
			return nil, "", err
		}
		return cam, "mock-fallback", nil
	}

	return realCam, "real", nil
}

// logGoroutineStats logs goroutine count periodically to track potential leaks
// logGoroutineStats logs goroutine count periodically to track potential leaks
func logGoroutineStats() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	done := make(chan struct{})
	defer close(done)
	logGoroutineStatsWithDeps(ticker.C, log.Default(), done)
}

func logGoroutineStatsWithDeps(tickerCh <-chan time.Time, logger *log.Logger, done <-chan struct{}) {
	if logger == nil {
		logger = log.Default()
	}

	var lastCount int
	for {
		select {
		case <-done:
			return
		case <-tickerCh:
			count := runtime.NumGoroutine()
			delta := count - lastCount
			deltaStr := ""
			if delta > 0 {
				deltaStr = fmt.Sprintf(" (+%d)", delta)
			} else if delta < 0 {
				deltaStr = fmt.Sprintf(" (%d)", delta)
			}
			logger.Printf("📊 Goroutines: %d%s", count, deltaStr)
			lastCount = count
		}
	}
}
