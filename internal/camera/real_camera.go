package camera

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultCaptureWaitTimeout = 2 * time.Second
	readChunkSize             = 32 * 1024
	maxReadBufferSize         = 10 * 1024 * 1024
	v4l2ProbeTimeout          = 3 * time.Second
)

// RealCamera captures frames from a Raspberry Pi CSI camera via a long-lived
// process that emits an MJPEG byte stream.
type RealCamera struct {
	width       int
	height      int
	fps         int
	jpegQuality int
	devicePath  string

	isReady    atomic.Bool
	isStopping atomic.Bool

	captureMutex sync.Mutex

	// Process management
	proc       *exec.Cmd
	procStdin  io.WriteCloser
	procStdout io.ReadCloser
	procStderr io.ReadCloser

	// Frame stream management
	frameMutex         sync.Mutex
	latestFrame        []byte
	frameSeq           uint64
	readerErr          error
	readBuffer         []byte
	captureWaitTimeout time.Duration

	readerDone chan struct{}
	stderrDone chan struct{}

	backendAttempted string

	// test hooks
	lookPath func(string) (string, error)
	statFn   func(string) (os.FileInfo, error)
	launchFn func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error)
	runCmdFn func(*exec.Cmd) ([]byte, error)
}

// InitializationError describes why real camera startup failed.
type InitializationError struct {
	Backend string
	Reason  string
	Cause   error
}

func (e *InitializationError) Error() string {
	if e == nil {
		return "camera initialization failed"
	}
	causeText := ""
	if e.Cause != nil {
		causeText = fmt.Sprintf(": %v", e.Cause)
	}
	if e.Backend != "" {
		return fmt.Sprintf("camera initialization failed (backend: %s): %s%s", e.Backend, e.Reason, causeText)
	}
	return fmt.Sprintf("camera initialization failed: %s%s", e.Reason, causeText)
}

func (e *InitializationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewRealCamera creates a new camera instance for Raspberry Pi CSI camera.
func NewRealCamera() *RealCamera {
	rc := &RealCamera{
		width:              640,
		height:             480,
		fps:                24,
		jpegQuality:        80,
		devicePath:         "/dev/video0",
		captureWaitTimeout: defaultCaptureWaitTimeout,
	}
	rc.lookPath = exec.LookPath
	rc.statFn = os.Stat
	rc.launchFn = rc.launchContinuousProducer
	rc.runCmdFn = func(cmd *exec.Cmd) ([]byte, error) {
		return cmd.CombinedOutput()
	}
	return rc
}

// Start initializes camera configuration and starts the long-lived capture process.
func (rc *RealCamera) Start(width, height, fps, jpegQuality int) error {
	rc.captureMutex.Lock()
	if rc.isReady.Load() {
		rc.captureMutex.Unlock()
		return fmt.Errorf("camera already started")
	}

	rc.width = width
	rc.height = height
	rc.fps = fps
	if jpegQuality < 1 || jpegQuality > 100 {
		jpegQuality = 80
	}
	rc.jpegQuality = jpegQuality
	rc.backendAttempted = "preflight"
	rc.captureMutex.Unlock()

	// Check if camera device exists
	if _, err := rc.statFn(rc.devicePath); err != nil {
		log.Printf("❌ Camera device not found at %s: %v", rc.devicePath, err)
		log.Printf("   Please ensure:")
		log.Printf("   - CSI camera is physically connected")
		log.Printf("   - Camera is enabled in raspi-config")
		log.Printf("   - Device permissions allow access to %s", rc.devicePath)
		return &InitializationError{
			Backend: rc.getBackendAttempted(),
			Reason:  fmt.Sprintf("camera device not found at %s", rc.devicePath),
			Cause:   err,
		}
	}
	log.Printf("✓ Camera device found at %s", rc.devicePath)

	rc.frameMutex.Lock()
	rc.latestFrame = nil
	rc.frameSeq = 0
	rc.readerErr = nil
	rc.readBuffer = nil
	rc.frameMutex.Unlock()

	cmd, stdin, stdout, stderr, err := rc.launchFn()
	if err != nil {
		log.Printf("❌ Failed to launch camera backend: %v", err)
		return &InitializationError{
			Backend: rc.getBackendAttempted(),
			Reason:  "failed to launch camera backend",
			Cause:   err,
		}
	}

	rc.captureMutex.Lock()
	rc.proc = cmd
	rc.procStdin = stdin
	rc.procStdout = stdout
	rc.procStderr = stderr
	rc.readerDone = make(chan struct{})
	rc.stderrDone = make(chan struct{})

	rc.isStopping.Store(false)
	rc.isReady.Store(false)
	rc.captureMutex.Unlock()

	go rc.readMJPEGStream()
	go rc.drainStderr()

	if err := rc.waitForFirstFrame(); err != nil {
		_ = rc.Stop()
		return err
	}

	rc.isReady.Store(true)

	return nil
}

func (rc *RealCamera) waitForFirstFrame() error {
	timeout := rc.firstFrameTimeout()
	deadline := time.Now().Add(timeout)

	for {
		// Check if stopping to allow clean shutdown during initialization
		if rc.isStopping.Load() {
			return &InitializationError{
				Backend: rc.getBackendAttempted(),
				Reason:  "camera stopped during initialization",
			}
		}

		rc.frameMutex.Lock()
		frame := append([]byte(nil), rc.latestFrame...)
		readerErr := rc.readerErr
		rc.frameMutex.Unlock()

		if len(frame) > 0 {
			if _, err := jpeg.DecodeConfig(bytes.NewReader(frame)); err == nil {
				return nil
			}
		}

		if readerErr != nil {
			reason := "frame reader stopped before first JPEG frame"
			if errors.Is(readerErr, io.EOF) {
				reason = "camera backend exited before first JPEG frame"
			}
			return &InitializationError{
				Backend: rc.getBackendAttempted(),
				Reason:  reason,
				Cause:   readerErr,
			}
		}

		if time.Now().After(deadline) {
			return &InitializationError{
				Backend: rc.getBackendAttempted(),
				Reason:  fmt.Sprintf("timed out waiting %s for first JPEG frame (fps=%d)", timeout.Round(10*time.Millisecond), rc.fps),
			}
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func (rc *RealCamera) firstFrameTimeout() time.Duration {
	fps := rc.fps
	if fps <= 0 {
		fps = 1
	}
	timeout := 3 * (time.Second / time.Duration(fps))

	// rpicam-vid and libcamera-vid need extra time to initialize libcamera daemon,
	// detect camera, configure ISP pipeline, and produce first frame
	if rc.backendAttempted == "rpicam-vid" || rc.backendAttempted == "libcamera-vid" {
		minTimeout := 3 * time.Second
		if timeout < minTimeout {
			timeout = minTimeout
		}
	} else {
		// FFmpeg and other backends use shorter timeout
		if timeout < 500*time.Millisecond {
			timeout = 500 * time.Millisecond
		}
	}

	if rc.captureWaitTimeout > 0 && timeout > rc.captureWaitTimeout {
		timeout = rc.captureWaitTimeout
	}
	return timeout
}

func (rc *RealCamera) setBackendAttempted(name string) {
	rc.captureMutex.Lock()
	rc.backendAttempted = name
	rc.captureMutex.Unlock()
}

func (rc *RealCamera) getBackendAttempted() string {
	rc.captureMutex.Lock()
	defer rc.captureMutex.Unlock()
	if rc.backendAttempted == "" {
		return "unknown"
	}
	return rc.backendAttempted
}

// CaptureFrame returns the latest buffered frame, waiting for an initial frame
// when necessary.
func (rc *RealCamera) CaptureFrame() ([]byte, error) {
	if !rc.isReady.Load() {
		return nil, fmt.Errorf("camera not ready")
	}
	if rc.isStopping.Load() {
		return nil, fmt.Errorf("camera stopped")
	}

	deadline := time.Now().Add(rc.captureWaitTimeout)
	for {
		rc.frameMutex.Lock()
		if len(rc.latestFrame) > 0 {
			frame := append([]byte(nil), rc.latestFrame...)
			rc.frameMutex.Unlock()
			return frame, nil
		}
		readerErr := rc.readerErr
		rc.frameMutex.Unlock()

		if readerErr != nil {
			return nil, fmt.Errorf("frame stream stopped: %w", readerErr)
		}
		if !rc.isReady.Load() || rc.isStopping.Load() {
			return nil, fmt.Errorf("camera stopped")
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for frame")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Stop stops the camera process and reader goroutines.
func (rc *RealCamera) Stop() error {
	rc.isStopping.Store(true)
	rc.isReady.Store(false)

	rc.captureMutex.Lock()
	proc := rc.proc
	stdin := rc.procStdin
	readerDone := rc.readerDone
	stderrDone := rc.stderrDone

	rc.proc = nil
	rc.procStdin = nil
	rc.procStdout = nil
	rc.procStderr = nil
	rc.captureMutex.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}

	if proc != nil {
		if proc.Process != nil {
			_ = proc.Process.Kill()
		}
		// Use a timeout to prevent indefinite blocking on proc.Wait()
		waitDone := make(chan error, 1)
		go func() {
			waitDone <- proc.Wait()
		}()

		select {
		case <-waitDone:
			// Process exited cleanly
		case <-time.After(5 * time.Second):
			// Timeout waiting for process to exit
			log.Printf("⚠️  Timeout waiting for camera process to exit")
		}
	}

	// Wait for reader goroutines to exit, with timeout
	if readerDone != nil {
		select {
		case <-readerDone:
			// Reader exited
		case <-time.After(5 * time.Second):
			// Timeout waiting for reader
			log.Printf("⚠️  Timeout waiting for reader goroutine to exit")
		}
	}
	if stderrDone != nil {
		select {
		case <-stderrDone:
			// Stderr drainer exited
		case <-time.After(5 * time.Second):
			// Timeout waiting for stderr drainer
			log.Printf("⚠️  Timeout waiting for stderr drainer goroutine to exit")
		}
	}

	return nil
}

// IsReady returns true if camera is initialized and ready to capture.
func (rc *RealCamera) IsReady() bool {
	return rc.isReady.Load() && !rc.isStopping.Load()
}

func (rc *RealCamera) launchContinuousProducer() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	if _, err := rc.lookPath("rpicam-vid"); err == nil {
		rc.setBackendAttempted("rpicam-vid")
		log.Printf("✓ Selected camera backend binary: rpicam-vid")
		log.Printf("  Resolution: %dx%d | FPS: %d | Quality: %d%%", rc.width, rc.height, rc.fps, rc.jpegQuality)
		cmd := rc.buildRpiCamVidCommand()
		return rc.startCommand(cmd, "rpicam-vid")
	}

	if _, err := rc.lookPath("libcamera-vid"); err == nil {
		rc.setBackendAttempted("libcamera-vid")
		log.Printf("✓ Selected camera backend binary: libcamera-vid")
		log.Printf("  Resolution: %dx%d | FPS: %d | Quality: %d%%", rc.width, rc.height, rc.fps, rc.jpegQuality)
		cmd := rc.buildLibcameraVidCommand()
		return rc.startCommand(cmd, "libcamera-vid")
	}

	if _, err := rc.lookPath("ffmpeg"); err != nil {
		rc.setBackendAttempted("none")
		log.Printf("❌ None of rpicam-vid, libcamera-vid, or ffmpeg were found in PATH")
		log.Printf("   rpicam-vid/libcamera-vid: Check if libcamera-apps package is installed in container")
		log.Printf("   ffmpeg: Check if ffmpeg package is installed in container")
		return nil, nil, nil, nil, fmt.Errorf("none of rpicam-vid, libcamera-vid, or ffmpeg found in PATH")
	}
	rc.setBackendAttempted("ffmpeg")

	// Skip V4L2 probe for libcamera CSI cameras without libcamera-vid
	// These devices require libcamera initialization which FFmpeg can't do
	// The probe would fail anyway, so we skip it and provide clear guidance
	if rc.devicePath == "/dev/video0" {
		log.Printf("ℹ️  CSI camera detected at /dev/video0 without native libcamera tools")
		log.Printf("⚠️  Attempting FFmpeg fallback (limited compatibility)")
		log.Printf("✓ For optimal performance, install rpicam-vid/libcamera-vid in the container")
	} else {
		// For other V4L2 devices, probe first to verify they work with FFmpeg
		if err := rc.probeV4L2CaptureNode(); err != nil {
			return nil, nil, nil, nil, err
		}
	}

	log.Printf("✓ Selected camera backend binary: ffmpeg")
	log.Printf("⚠️  Falling back to ffmpeg (V4L2 mode) because rpicam-vid/libcamera-vid were not found")
	log.Printf("  Note: native CSI camera tools may not be installed or available in container")
	log.Printf("  Using device: %s | Resolution: %dx%d | FPS: %d | Quality: %d%%", rc.devicePath, rc.width, rc.height, rc.fps, rc.jpegQuality)

	cmd := rc.buildFFmpegCommand()
	return rc.startCommand(cmd, "ffmpeg")
}

func (rc *RealCamera) buildRpiCamVidCommand() *exec.Cmd {
	return exec.Command(
		"rpicam-vid",
		"--codec", "mjpeg",
		"--nopreview",
		"--timeout", "0",
		"--width", fmt.Sprintf("%d", rc.width),
		"--height", fmt.Sprintf("%d", rc.height),
		"--framerate", fmt.Sprintf("%d", rc.fps),
		"-o", "-",
	)
}

func (rc *RealCamera) buildLibcameraVidCommand() *exec.Cmd {
	return exec.Command(
		"libcamera-vid",
		"--codec", "mjpeg",
		"--nopreview",
		"--timeout", "0",
		"--width", fmt.Sprintf("%d", rc.width),
		"--height", fmt.Sprintf("%d", rc.height),
		"--framerate", fmt.Sprintf("%d", rc.fps),
		"-o", "-",
	)
}

func (rc *RealCamera) buildFFmpegCommand() *exec.Cmd {
	// For libcamera V4L2 devices, avoid strict format constraints
	// Let FFmpeg auto-detect the device's native format
	return exec.Command(
		"ffmpeg",
		"-f", "video4linux2",
		"-video_size", fmt.Sprintf("%dx%d", rc.width, rc.height),
		"-framerate", fmt.Sprintf("%d", rc.fps),
		"-i", rc.devicePath,
		"-c:v", "mjpeg",
		"-q:v", fmt.Sprintf("%d", rc.jpegQuality),
		"-f", "mjpeg",
		"pipe:1",
	)
}

func (rc *RealCamera) probeV4L2CaptureNode() error {
	ctx, cancel := context.WithTimeout(context.Background(), v4l2ProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-f", "video4linux2",
		"-video_size", fmt.Sprintf("%dx%d", rc.width, rc.height),
		"-framerate", fmt.Sprintf("%d", rc.fps),
		"-i", rc.devicePath,
		"-frames:v", "1",
		"-f", "null",
		"-",
	)

	output, err := rc.runCmdFn(cmd)
	if err == nil {
		log.Printf("✓ V4L2 probe succeeded for %s", rc.devicePath)
		return nil
	}

	probeOutput := strings.TrimSpace(string(output))
	log.Printf("❌ V4L2 probe failed for %s: %v", rc.devicePath, err)
	if probeOutput != "" {
		log.Printf("   Probe output: %s", probeOutput)
	}
	return rc.mapFFmpegInputError(probeOutput, err)
}

func (rc *RealCamera) mapFFmpegInputError(stderr string, cause error) error {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "vidioc_streamon"):
		return fmt.Errorf("camera startup failed for %s: V4L2 STREAMON failed (device is not a usable capture node). For CSI cameras inside containers, install rpicam-vid/libcamera-vid and avoid ffmpeg fallback: %w", rc.devicePath, cause)
	case strings.Contains(lower, "error opening input"),
		strings.Contains(lower, "cannot open video device"),
		strings.Contains(lower, "not a video capture device"),
		strings.Contains(lower, "no such file or directory"),
		strings.Contains(lower, "permission denied"):
		return fmt.Errorf("camera startup failed for %s: ffmpeg could not open the V4L2 input. For CSI cameras inside containers, ensure rpicam-vid/libcamera-vid is installed and accessible: %w", rc.devicePath, cause)
	default:
		return fmt.Errorf("camera startup failed for %s: V4L2 probe failed before ffmpeg fallback. For CSI cameras inside containers, install rpicam-vid/libcamera-vid. probe details: %s: %w", rc.devicePath, stderr, cause)
	}
}

func (rc *RealCamera) startCommand(cmd *exec.Cmd, backendName string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("❌ %s: failed creating stdout pipe: %v", backendName, err)
		return nil, nil, nil, nil, fmt.Errorf("failed creating stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("❌ %s: failed creating stderr pipe: %v", backendName, err)
		return nil, nil, nil, nil, fmt.Errorf("failed creating stderr pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("❌ %s: failed creating stdin pipe: %v", backendName, err)
		return nil, nil, nil, nil, fmt.Errorf("failed creating stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("❌ %s: failed to start capture process: %v", backendName, err)
		return nil, nil, nil, nil, fmt.Errorf("failed to start capture process: %w", err)
	}
	log.Printf("✓ %s process started successfully (PID: %d)", backendName, cmd.Process.Pid)
	return cmd, stdin, stdout, stderr, nil
}

func (rc *RealCamera) readMJPEGStream() {
	defer close(rc.readerDone)

	buf := make([]byte, readChunkSize)
	readAttempts := 0
	framesExtracted := 0

	for {
		// Check if stopping before attempting read
		if rc.isStopping.Load() {
			log.Printf("📹 Frame reader: stopping (graceful shutdown)")
			return
		}

		rc.captureMutex.Lock()
		stdout := rc.procStdout
		isStopping := rc.isStopping.Load()
		rc.captureMutex.Unlock()

		if stdout == nil || isStopping {
			return
		}

		n, err := stdout.Read(buf)
		readAttempts++
		if n > 0 {
			if readAttempts <= 5 {
				log.Printf("📹 Frame reader: read %d bytes (attempt %d)", n, readAttempts)
			}

			rc.frameMutex.Lock()
			rc.readBuffer = append(rc.readBuffer, buf[:n]...)
			if len(rc.readBuffer) > maxReadBufferSize {
				rc.readerErr = fmt.Errorf("read buffer exceeded maximum size")
				rc.frameMutex.Unlock()
				log.Printf("❌ Frame reader: buffer overflow (%d bytes)", len(rc.readBuffer))
				return
			}
			for {
				frame, remaining, found := extractJPEGFrame(rc.readBuffer)
				if !found {
					break
				}
				framesExtracted++
				if framesExtracted <= 3 {
					log.Printf("✓ Frame extracted: seq=%d size=%d bytes", rc.frameSeq+1, len(frame))
				}
				rc.latestFrame = append([]byte(nil), frame...)
				rc.frameSeq++
				rc.readBuffer = remaining
			}
			rc.frameMutex.Unlock()
		}

		if err != nil {
			if errors.Is(err, io.EOF) && rc.isStopping.Load() {
				log.Printf("📹 Frame reader: EOF (graceful shutdown), extracted %d frames total", framesExtracted)
				return
			}
			rc.frameMutex.Lock()
			if rc.readerErr == nil {
				rc.readerErr = err
				log.Printf("❌ Frame reader: error after %d bytes, %d frames extracted: %v", readAttempts*readChunkSize, framesExtracted, err)
			}
			rc.frameMutex.Unlock()
			return
		}
	}
}

func (rc *RealCamera) drainStderr() {
	defer close(rc.stderrDone)

	// Check if stopping before attempting to access stderr
	if rc.isStopping.Load() {
		return
	}

	rc.captureMutex.Lock()
	stderr := rc.procStderr
	isStopping := rc.isStopping.Load()
	rc.captureMutex.Unlock()

	if stderr == nil || isStopping {
		return
	}

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		// Check if stopping during scan loop
		if rc.isStopping.Load() {
			return
		}
		line := scanner.Text()
		if line != "" {
			log.Printf("[camera stderr] %s", line)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("❌ Error reading camera stderr: %v", err)
	}
}

func extractJPEGFrame(stream []byte) (frame []byte, remaining []byte, found bool) {
	start := bytes.Index(stream, []byte{0xFF, 0xD8})
	if start == -1 {
		if len(stream) > 2 {
			return nil, stream[len(stream)-2:], false
		}
		return nil, stream, false
	}

	endRel := bytes.Index(stream[start+2:], []byte{0xFF, 0xD9})
	if endRel == -1 {
		if start > 0 {
			return nil, stream[start:], false
		}
		return nil, stream, false
	}

	end := start + 2 + endRel + 2
	return stream[start:end], stream[end:], true
}

// encodeFrameToJPEG converts an image.Image to JPEG bytes.
// Used internally if we get raw image frames instead of pre-encoded JPEG.
func encodeFrameToJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
