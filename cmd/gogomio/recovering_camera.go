package main

import (
	"context"
	"errors"
	"log"
	"reflect"
	"sync"
	"time"

	"github.com/CyanAutomation/gogomio/internal/camera"
	"github.com/CyanAutomation/gogomio/internal/config"
)

const (
	initialCameraRecoveryDelay   = 5 * time.Second
	maximumCameraRecoveryDelay   = time.Minute
	realCameraHealthPollInterval = 5 * time.Second
)

// recoveringCamera supervises the active camera. A startup or runtime failure
// falls back to simulated frames while the CSI camera is retried in the
// background; a real backend is activated only after its first valid frame.
type recoveringCamera struct {
	mu sync.RWMutex

	active       camera.Camera
	candidate    camera.Camera
	backend      string
	lastError    string
	stopped      bool
	cfg          *config.Config
	newReal      func() camera.Camera
	newMock      func() camera.Camera
	logger       *log.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	recoveryDone chan struct{}
	retryDelay   time.Duration
}

func newRecoveringCamera(
	active camera.Camera,
	backend string,
	cfg *config.Config,
	newReal func() camera.Camera,
	newMock func() camera.Camera,
	logger *log.Logger,
	initialError error,
	retryDelay time.Duration,
) *recoveringCamera {
	if logger == nil {
		logger = log.Default()
	}
	if retryDelay <= 0 {
		retryDelay = initialCameraRecoveryDelay
	}
	ctx, cancel := context.WithCancel(context.Background())
	rc := &recoveringCamera{
		active:       active,
		backend:      backend,
		cfg:          cfg,
		newReal:      newReal,
		newMock:      newMock,
		logger:       logger,
		ctx:          ctx,
		cancel:       cancel,
		recoveryDone: make(chan struct{}),
		retryDelay:   retryDelay,
	}
	if initialError != nil {
		rc.lastError = initialError.Error()
	}
	go rc.supervise()
	return rc
}

func (rc *recoveringCamera) Start(int, int, int, int) error {
	return camera.ErrCameraAlreadyStarted
}

func (rc *recoveringCamera) CaptureFrame() ([]byte, error) {
	return rc.CaptureFrameWithContext(context.Background())
}

func (rc *recoveringCamera) CaptureFrameWithContext(ctx context.Context) ([]byte, error) {
	rc.mu.RLock()
	active := rc.active
	rc.mu.RUnlock()
	if active == nil {
		return nil, context.Canceled
	}
	return active.CaptureFrameWithContext(ctx)
}

func (rc *recoveringCamera) IsReady() bool {
	rc.mu.RLock()
	active := rc.active
	rc.mu.RUnlock()
	return active != nil && active.IsReady()
}

func (rc *recoveringCamera) CameraBackend() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.backend
}

func (rc *recoveringCamera) CameraStatusMessage() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	switch rc.backend {
	case "mock-fallback":
		return "The real camera is unavailable; GoGoMio is serving simulated frames and retrying."
	case "real":
		return "The real Raspberry Pi camera is active."
	default:
		return "The camera backend is unavailable."
	}
}

func (rc *recoveringCamera) LastCameraError() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.lastError
}

func (rc *recoveringCamera) Stop() error {
	rc.mu.Lock()
	if rc.stopped {
		rc.mu.Unlock()
		return nil
	}
	rc.stopped = true
	active := rc.active
	candidate := rc.candidate
	done := rc.recoveryDone
	rc.mu.Unlock()

	rc.cancel()
	if candidate != nil && !sameCamera(candidate, active) {
		_ = candidate.Stop()
	}
	if active != nil {
		_ = active.Stop()
	}
	<-done
	return nil
}

func (rc *recoveringCamera) supervise() {
	defer close(rc.recoveryDone)
	delay := rc.retryDelay

	for {
		if rc.CameraBackend() == "real" {
			if !waitForContext(rc.ctx, delay) {
				return
			}
			if rc.IsReady() {
				delay = rc.retryDelay
				continue
			}
			rc.failoverToMock(&delay)
			continue
		}

		if !waitForContext(rc.ctx, delay) {
			return
		}
		candidate := rc.newReal()
		rc.mu.Lock()
		if rc.stopped {
			rc.mu.Unlock()
			_ = candidate.Stop()
			return
		}
		rc.candidate = candidate
		rc.mu.Unlock()

		err := startRealCameraWithRetry(rc.logger, rc.cfg, candidate, func(wait time.Duration) bool {
			return waitForContext(rc.ctx, wait)
		})
		if err != nil {
			rc.mu.Lock()
			rc.candidate = nil
			if !rc.stopped {
				rc.lastError = err.Error()
			}
			rc.mu.Unlock()
			_ = candidate.Stop()
			rc.logger.Printf("⚠️ Real camera recovery attempt failed; retrying in %v: %v", delay, err)
			delay = nextCameraRecoveryDelay(delay)
			continue
		}

		rc.mu.Lock()
		if rc.stopped {
			rc.candidate = nil
			rc.mu.Unlock()
			_ = candidate.Stop()
			return
		}
		previous := rc.active
		rc.active = candidate
		rc.candidate = nil
		rc.backend = "real"
		rc.lastError = ""
		rc.mu.Unlock()

		if previous != nil && !sameCamera(previous, candidate) {
			_ = previous.Stop()
		}
		rc.logger.Println("✅ Real camera recovered; switched to the CSI camera.")
		delay = rc.retryDelay
	}
}

func (rc *recoveringCamera) failoverToMock(delay *time.Duration) {
	message := errors.New("real camera stopped producing frames")
	mock, _, err := startMockCamera(rc.logger, rc.cfg, rc.newMock, "mock-fallback", true)
	if err != nil {
		rc.mu.Lock()
		if !rc.stopped {
			rc.lastError = err.Error()
		}
		rc.mu.Unlock()
		rc.logger.Printf("⚠️ Could not start simulated camera after real-camera failure: %v", err)
		*delay = nextCameraRecoveryDelay(*delay)
		return
	}

	rc.mu.Lock()
	if rc.stopped {
		rc.mu.Unlock()
		_ = mock.Stop()
		return
	}
	previous := rc.active
	rc.active = mock
	rc.backend = "mock-fallback"
	rc.lastError = message.Error()
	rc.mu.Unlock()

	if previous != nil && !sameCamera(previous, mock) {
		_ = previous.Stop()
	}
	rc.logger.Printf("⚠️ Real camera became unavailable; switched to mock fallback and will retry in %v.", *delay)
	*delay = rc.retryDelay
}

func nextCameraRecoveryDelay(delay time.Duration) time.Duration {
	delay *= 2
	if delay > maximumCameraRecoveryDelay {
		return maximumCameraRecoveryDelay
	}
	return delay
}

func sameCamera(a, b camera.Camera) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	typeOfA := reflect.TypeOf(a)
	return typeOfA == reflect.TypeOf(b) && typeOfA.Comparable() && a == b
}

func waitForContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
