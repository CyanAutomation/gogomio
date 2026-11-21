package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/config"
)

const (
	realCameraStartupAttempts = 2
	realCameraRetryDelay      = time.Second
)

func configureRealCamera(cam camera.Camera, cfg *config.Config) {
	if configurable, ok := cam.(interface{ SetSensorMode(int, int) }); ok {
		configurable.SetSensorMode(cfg.SensorMode[0], cfg.SensorMode[1])
	}
	if configurable, ok := cam.(interface{ SetStartupTimeout(time.Duration) }); ok && cfg.CameraStartupTimeoutSeconds > 0 {
		configurable.SetStartupTimeout(time.Duration(cfg.CameraStartupTimeoutSeconds) * time.Second)
	}
}

// startRealCameraWithRetry retries only a first-frame timeout. Other failures
// such as a missing device or failed process launch are not transient startup delays.
func startRealCameraWithRetry(
	logger *log.Logger,
	cfg *config.Config,
	cam camera.Camera,
	wait func(time.Duration) bool,
) error {
	if logger == nil {
		logger = log.Default()
	}
	configureRealCamera(cam, cfg)
	for attempt := 1; attempt <= realCameraStartupAttempts; attempt++ {
		err := cam.Start(cfg.Resolution[0], cfg.Resolution[1], cfg.FPS, cfg.JPEGQuality)
		if err == nil {
			return nil
		}
		if !errors.Is(err, camera.ErrFirstFrameTimeout) || attempt == realCameraStartupAttempts {
			return err
		}
		logger.Printf("⚠️ Native camera produced no first frame on attempt %d/%d; retrying after %v: %v", attempt, realCameraStartupAttempts, realCameraRetryDelay, err)
		if wait == nil || !wait(realCameraRetryDelay) {
			return context.Canceled
		}
	}
	return nil
}

func sleepForRetry(delay time.Duration) bool {
	time.Sleep(delay)
	return true
}
