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
	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/go-chi/chi/v5"
)

var (
	Version   = "0.1.0-dev"
	BuildTime = "dev"
)

func main() {
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
	if cfg.MockCamera {
		log.Println("🎬 Initializing camera backend: mock (development mode)")
		cam := newMockCamera()
		if err := cam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
			return nil, "", err
		}
		return cam, "mock", nil
	}

	// Try real camera first if device is available.
	log.Println("📹 Initializing camera backend: real (Raspberry Pi CSI)")
	log.Println("   Checking for CSI camera access...")
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

		log.Printf("❌ Real camera initialization failed")
		log.Printf("   Backend attempted: %s", attemptedBackend)
		log.Printf("   Failure reason: %s", failureReason)
		if !errors.As(err, &initErr) || errors.Unwrap(err) != nil {
			log.Printf("   Error details: %v", err)
		}
		log.Println("   Troubleshooting steps:")
		log.Println("   1. Verify CSI camera is physically connected to the camera port")
		log.Println("   2. Enable camera in raspi-config: sudo raspi-config → Interface → Camera")
		log.Println("   3. Check device permissions: ls -la /dev/video*")
		log.Println("   4. Verify container has device access (see docker-compose.yml devices section)")
		log.Println("")
		log.Println("   For optimal performance (native CSI camera support):")
		log.Println("   - libcamera-vid binary should be available in the container")
		log.Println("   - Check: docker exec gogomio which libcamera-vid")
		log.Println("   - If not found, libcamera-apps package may need to be installed from Raspberry Pi repos")
		log.Println("")
		log.Println("   Falling back to FFmpeg V4L2 mode (may have lower performance)...")
		log.Println("🎬 Initializing camera backend: mock fallback (development mode)")
		cam := newMockCamera()
		if err := cam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
			return nil, "", err
		}
		return cam, "mock-fallback", nil
	}

	return realCam, "real", nil
}

// logGoroutineStats logs goroutine count periodically to track potential leaks
func logGoroutineStats() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var lastCount int
	for range ticker.C {
		count := runtime.NumGoroutine()
		delta := count - lastCount
		deltaStr := ""
		if delta > 0 {
			deltaStr = fmt.Sprintf(" (+%d)", delta)
		} else if delta < 0 {
			deltaStr = fmt.Sprintf(" (%d)", delta)
		}
		log.Printf("📊 Goroutines: %d%s", count, deltaStr)
		lastCount = count
	}
}
