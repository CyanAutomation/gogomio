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
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/CyanAutomation/gogomio/internal/api"
	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/cli"
	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/go-chi/chi/v5"
)

type application struct {
	camera       camera.Camera
	frameManager *api.FrameManager
	router       chi.Router
	listener     net.Listener
	server       *http.Server
	cleanupOnce  sync.Once
}

type applicationDependencies struct {
	newRealCamera func() camera.Camera
	newMockCamera func() camera.Camera
	listen        func(network, address string) (net.Listener, error)
}

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

	app, backend, err := initializeApplication(cfg, applicationDependencies{
		newRealCamera: func() camera.Camera { return camera.NewRealCamera() },
		newMockCamera: func() camera.Camera { return camera.NewMockCamera() },
		listen:        net.Listen,
	})
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}
	defer app.cleanup()

	log.Printf("Camera backend initialized: %s", backend)
	log.Printf("Camera capture started: %dx%d @ %d FPS", cfg.Resolution[0], cfg.Resolution[1], cfg.FPS)

	// Start pprof profiling server on separate port (only if explicitly enabled)
	if os.Getenv("MIO_ENABLE_PPROF") == "true" {
		go func() {
			log.Printf("🔍 Profiling server listening on http://localhost:6060/debug/pprof")
			if err := http.ListenAndServe(":6060", nil); err != nil && err != http.ErrServerClosed {
				log.Printf("Profiling server error: %v", err)
			}
		}()
	} else {
		log.Printf("ℹ️  pprof profiling disabled (set MIO_ENABLE_PPROF=true to enable)")
	}

	// Setup HTTP server with security timeouts
	addr := app.listener.Addr().String()

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	shutdownCtx, cancelShutdown := newShutdownContext(context.Background(), sigChan)
	defer cancelShutdown()

	// Log goroutine count until graceful shutdown begins.
	go logGoroutineStats(shutdownCtx.Done())

	go func() {
		<-shutdownCtx.Done()
		log.Println("Shutdown signal received, stopping server...")
		if err := app.server.Close(); err != nil {
			log.Printf("Error closing server: %v", err)
		}
	}()

	// Start server
	log.Printf("Listening on http://%s", addr)
	log.Printf("Stream: http://%s/stream.mjpg", addr)
	log.Printf("Snapshot: http://%s/snapshot.jpg", addr)
	log.Printf("API: http://%s/api/config", addr)

	if err := app.server.Serve(app.listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	log.Println("Server stopped")
}

func initializeApplication(cfg *config.Config, deps applicationDependencies) (*application, string, error) {
	cam, backend, err := initializeCamera(cfg, deps.newRealCamera, deps.newMockCamera)
	if err != nil {
		return nil, "", err
	}

	router := chi.NewRouter()
	frameManager := api.NewFrameManager(cam, cfg)
	api.RegisterHandlers(router, frameManager, cfg)

	listener, err := deps.listen("tcp", cfg.AddressString())
	if err != nil {
		frameManager.Stop()
		_ = cam.Stop()
		return nil, "", fmt.Errorf("listen on %s: %w", cfg.AddressString(), err)
	}

	return &application{
		camera:       cam,
		frameManager: frameManager,
		router:       router,
		listener:     listener,
		server: &http.Server{
			Addr:              listener.Addr().String(),
			Handler:           router,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}, backend, nil
}

func (app *application) cleanup() {
	app.cleanupOnce.Do(func() {
		app.frameManager.Stop()
		if err := app.camera.Stop(); err != nil {
			log.Printf("Error stopping camera: %v", err)
		}
		_ = app.listener.Close()
	})
}

func newShutdownContext(parent context.Context, signalSource <-chan os.Signal) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	go func() {
		select {
		case <-signalSource:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
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
	if configurable, ok := realCam.(interface{ SetSensorMode(int, int) }); ok {
		configurable.SetSensorMode(cfg.SensorMode[0], cfg.SensorMode[1])
	}
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
func logGoroutineStats(done <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var lastCount int
	logGoroutineStatsWithDeps(ticker.C, func(count int) {
		delta := count - lastCount
		deltaStr := ""
		if delta > 0 {
			deltaStr = fmt.Sprintf(" (+%d)", delta)
		} else if delta < 0 {
			deltaStr = fmt.Sprintf(" (%d)", delta)
		}
		log.Printf("📊 Goroutines: %d%s", count, deltaStr)
		lastCount = count
	}, done)
}

func logGoroutineStatsWithDeps(tickerCh <-chan time.Time, recordCount func(int), done <-chan struct{}) {
	if recordCount == nil {
		recordCount = func(int) {}
	}

	for {
		select {
		case <-done:
			return
		case <-tickerCh:
			recordCount(runtime.NumGoroutine())
		}
	}
}
