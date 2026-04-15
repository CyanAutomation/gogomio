package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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
	cam, err := initializeCamera(
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

	log.Printf("Camera started: %dx%d @ %d FPS", cfg.Resolution[0], cfg.Resolution[1], cfg.FPS)

	// Create HTTP router and register handlers
	router := chi.NewRouter()
	frameManager := api.NewFrameManager(cam, cfg)
	defer frameManager.Stop()

	api.RegisterHandlers(router, frameManager, cfg)

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
) (camera.Camera, error) {
	if cfg.MockCamera {
		log.Println("Using mock camera (development mode)")
		cam := newMockCamera()
		if err := cam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
			return nil, err
		}
		return cam, nil
	}

	// Try real camera first if device is available.
	realCam := newRealCamera()
	if err := realCam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
		log.Printf("Real camera unavailable (%v), falling back to mock camera", err)
		cam := newMockCamera()
		if err := cam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality); err != nil {
			return nil, err
		}
		log.Println("Using mock camera fallback (development mode)")
		return cam, nil
	}

	log.Println("Using real camera (Raspberry Pi CSI)")
	return realCam, nil
}
