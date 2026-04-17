package camera

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultCaptureWaitTimeout = 2 * time.Second
	readChunkSize             = 32 * 1024
	maxReadBufferSize         = 10 * 1024 * 1024
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

	// test hooks
	lookPath func(string) (string, error)
	statFn   func(string) (os.FileInfo, error)
	launchFn func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error)
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
	return rc
}

// Start initializes camera configuration and starts the long-lived capture process.
func (rc *RealCamera) Start(width, height, fps, jpegQuality int) error {
	rc.captureMutex.Lock()
	defer rc.captureMutex.Unlock()

	if rc.isReady.Load() {
		return fmt.Errorf("camera already started")
	}

	rc.width = width
	rc.height = height
	rc.fps = fps
	if jpegQuality < 1 || jpegQuality > 100 {
		jpegQuality = 80
	}
	rc.jpegQuality = jpegQuality

	// Check if camera device exists
	if _, err := rc.statFn(rc.devicePath); err != nil {
		log.Printf("❌ Camera device not found at %s: %v", rc.devicePath, err)
		log.Printf("   Please ensure:")
		log.Printf("   - CSI camera is physically connected")
		log.Printf("   - Camera is enabled in raspi-config")
		log.Printf("   - Device permissions allow access to %s", rc.devicePath)
		return fmt.Errorf("camera device not found at %s: %w", rc.devicePath, err)
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
		return err
	}

	rc.proc = cmd
	rc.procStdin = stdin
	rc.procStdout = stdout
	rc.procStderr = stderr
	rc.readerDone = make(chan struct{})
	rc.stderrDone = make(chan struct{})

	rc.isStopping.Store(false)
	rc.isReady.Store(true)

	go rc.readMJPEGStream()
	go rc.drainStderr()

	return nil
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
		_ = proc.Wait()
	}

	if readerDone != nil {
		<-readerDone
	}
	if stderrDone != nil {
		<-stderrDone
	}

	return nil
}

// IsReady returns true if camera is initialized and ready to capture.
func (rc *RealCamera) IsReady() bool {
	return rc.isReady.Load() && !rc.isStopping.Load()
}

func (rc *RealCamera) launchContinuousProducer() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	if _, err := rc.lookPath("libcamera-vid"); err == nil {
		log.Printf("✓ Using libcamera-vid for native CSI camera support")
		log.Printf("  Resolution: %dx%d | FPS: %d | Quality: %d%%", rc.width, rc.height, rc.fps, rc.jpegQuality)
		cmd := exec.Command(
			"libcamera-vid",
			"--codec", "mjpeg",
			"--nopreview",
			"--timeout", "0",
			"--width", fmt.Sprintf("%d", rc.width),
			"--height", fmt.Sprintf("%d", rc.height),
			"--framerate", fmt.Sprintf("%d", rc.fps),
			"-o", "-",
		)
		return rc.startCommand(cmd, "libcamera-vid")
	}

	if _, err := rc.lookPath("ffmpeg"); err != nil {
		log.Printf("❌ Neither libcamera-vid nor ffmpeg found in PATH")
		log.Printf("   libcamera-vid: Check if libcamera-apps package is installed in container")
		log.Printf("   ffmpeg: Check if ffmpeg package is installed in container")
		return nil, nil, nil, nil, fmt.Errorf("neither libcamera-vid nor ffmpeg found in PATH")
	}

	log.Printf("⚠️  libcamera-vid not available, falling back to ffmpeg (V4L2 mode)")
	log.Printf("  Note: libcamera-apps may not be installed or available in container")
	log.Printf("  Using device: %s | Resolution: %dx%d | FPS: %d | Quality: %d%%", rc.devicePath, rc.width, rc.height, rc.fps, rc.jpegQuality)
	
	// Use YUV420P format which is more universally supported by V4L2 devices
	// FFmpeg will encode this to MJPEG for streaming
	cmd := exec.Command(
		"ffmpeg",
		"-f", "video4linux2",
		"-pixel_format", "yuyv422",          // Common V4L2 format
		"-video_size", fmt.Sprintf("%dx%d", rc.width, rc.height),
		"-framerate", fmt.Sprintf("%d", rc.fps),
		"-i", rc.devicePath,
		"-c:v", "mjpeg",
		"-q:v", fmt.Sprintf("%d", rc.jpegQuality),
		"-f", "mjpeg",
		"pipe:1",
	)
	return rc.startCommand(cmd, "ffmpeg")
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
	for {
		rc.captureMutex.Lock()
		stdout := rc.procStdout
		rc.captureMutex.Unlock()

		if stdout == nil {
			return
		}

		n, err := stdout.Read(buf)
		if n > 0 {
			rc.frameMutex.Lock()
			rc.readBuffer = append(rc.readBuffer, buf[:n]...)
			if len(rc.readBuffer) > maxReadBufferSize {
				rc.readerErr = fmt.Errorf("read buffer exceeded maximum size")
				rc.frameMutex.Unlock()
				return
			}
			for {
				frame, remaining, found := extractJPEGFrame(rc.readBuffer)
				if !found {
					break
				}
				rc.latestFrame = append([]byte(nil), frame...)
				rc.frameSeq++
				rc.readBuffer = remaining
			}
			rc.frameMutex.Unlock()
		}

		if err != nil {
			if errors.Is(err, io.EOF) && rc.isStopping.Load() {
				return
			}
			rc.frameMutex.Lock()
			if rc.readerErr == nil {
				rc.readerErr = err
			}
			rc.frameMutex.Unlock()
			return
		}
	}
}

func (rc *RealCamera) drainStderr() {
	defer close(rc.stderrDone)

	rc.captureMutex.Lock()
	stderr := rc.procStderr
	rc.captureMutex.Unlock()
	if stderr == nil {
		return
	}

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
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
