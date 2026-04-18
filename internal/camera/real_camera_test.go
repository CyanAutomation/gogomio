package camera

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"log"
	"math"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRealCameraInitialization(t *testing.T) {
	rc := NewRealCamera()

	if rc.width != 640 || rc.height != 480 {
		t.Errorf("default resolution incorrect: %dx%d", rc.width, rc.height)
	}
	if rc.fps != 24 {
		t.Errorf("default FPS: got %d, want 24", rc.fps)
	}
	if rc.devicePath == "" {
		t.Error("device path not set")
	}
}

func TestRealCameraStartNoDevice(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/video999"

	err := rc.Start(640, 480, 24, 80)
	if err == nil {
		t.Error("Start should return error for non-existent device")
	}
}

func TestRealCameraProcessLifecycle(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	rc.captureWaitTimeout = 200 * time.Millisecond

	var startedCmd *exec.Cmd
	rc.launchFn = func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		stdoutR, stdoutW := io.Pipe()
		stderrR, stderrW := io.Pipe()

		cmd := exec.Command("bash", "-c", "sleep 30")
		if err := cmd.Start(); err != nil {
			return nil, nil, nil, nil, err
		}
		startedCmd = cmd

		go func() {
			defer func() { _ = stdoutW.Close() }()
			jpegData, _ := encodeFrameToJPEG(createTestImage(8, 8), 80)
			_, _ = stdoutW.Write(jpegData)
		}()
		go func() {
			defer func() { _ = stderrW.Close() }()
		}()

		return cmd, nopWriteCloser{}, stdoutR, stderrR, nil
	}

	if err := rc.Start(640, 480, 24, 80); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !rc.IsReady() {
		t.Fatal("camera should be ready after Start")
	}
	if startedCmd == nil || rc.proc == nil {
		t.Fatal("expected process to be started")
	}

	if err := rc.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if rc.IsReady() {
		t.Fatal("camera should not be ready after Stop")
	}

	if err := startedCmd.Process.Signal(syscall.Signal(0)); err == nil {
		t.Fatal("expected process to be terminated after Stop")
	}
}

func TestRealCameraCaptureFrameReturnsBufferedLatest(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	rc.captureWaitTimeout = 500 * time.Millisecond

	rc.launchFn = func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		stdoutR, stdoutW := io.Pipe()
		stderrR, stderrW := io.Pipe()
		cmd := exec.Command("bash", "-c", "sleep 30")
		if err := cmd.Start(); err != nil {
			return nil, nil, nil, nil, err
		}

		go func() {
			defer func() { _ = stdoutW.Close() }()
			frame1, _ := encodeFrameToJPEG(createTestImage(8, 8), 75)
			frame2, _ := encodeFrameToJPEG(createTestImage(10, 10), 80)
			_, _ = stdoutW.Write(append(append([]byte("noise"), frame1...), frame2...))
		}()
		go func() {
			defer func() { _ = stderrW.Close() }()
		}()

		return cmd, nopWriteCloser{}, stdoutR, stderrR, nil
	}

	if err := rc.Start(640, 480, 24, 80); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = rc.Stop() }()

	frame, err := rc.CaptureFrame()
	if err != nil {
		t.Fatalf("CaptureFrame() error = %v", err)
	}

	if _, err := jpeg.DecodeConfig(bytes.NewReader(frame)); err != nil {
		t.Fatalf("CaptureFrame() should return valid JPEG, decode error: %v", err)
	}
}

func TestRealCameraCaptureFrameTimeout(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	rc.captureWaitTimeout = 75 * time.Millisecond

	rc.launchFn = func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		stdoutR, stdoutW := io.Pipe()
		stderrR, stderrW := io.Pipe()
		cmd := exec.Command("bash", "-c", "sleep 30")
		if err := cmd.Start(); err != nil {
			return nil, nil, nil, nil, err
		}
		go func() {
			defer func() { _ = stderrW.Close() }()
			defer func() { _ = stdoutW.Close() }()
			time.Sleep(500 * time.Millisecond)
		}()
		return cmd, nopWriteCloser{}, stdoutR, stderrR, nil
	}

	err := rc.Start(640, 480, 24, 80)
	if err == nil {
		t.Fatal("expected Start() timeout error")
	}
	if !strings.Contains(err.Error(), "timed out waiting") {
		t.Fatalf("expected startup timeout, got %v", err)
	}
	if rc.IsReady() {
		t.Fatal("camera should not be ready when startup times out")
	}
}

func TestRealCameraStartDetectsEarlyBackendExit(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	rc.captureWaitTimeout = 400 * time.Millisecond

	rc.launchFn = func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		stdoutR, stdoutW := io.Pipe()
		stderrR, stderrW := io.Pipe()
		cmd := exec.Command("bash", "-c", "sleep 0.01")
		if err := cmd.Start(); err != nil {
			return nil, nil, nil, nil, err
		}
		go func() {
			defer func() { _ = stderrW.Close() }()
			defer func() { _ = stdoutW.Close() }()
			time.Sleep(20 * time.Millisecond)
		}()
		return cmd, nopWriteCloser{}, stdoutR, stderrR, nil
	}

	err := rc.Start(640, 480, 24, 80)
	if err == nil {
		t.Fatal("expected Start() early backend exit error")
	}
	if !strings.Contains(err.Error(), "before first JPEG frame") {
		t.Fatalf("expected early exit reason, got %v", err)
	}
}

func TestExtractJPEGFrame(t *testing.T) {
	stream := []byte{0x00, 0xFF, 0xD8, 0x11, 0x22, 0xFF, 0xD9, 0x33}
	frame, rem, found := extractJPEGFrame(stream)
	if !found {
		t.Fatal("expected frame to be found")
	}
	if len(frame) == 0 || frame[0] != 0xFF || frame[1] != 0xD8 {
		t.Fatalf("unexpected frame: %v", frame)
	}
	if len(rem) != 1 || rem[0] != 0x33 {
		t.Fatalf("unexpected remaining bytes: %v", rem)
	}
}

func TestRealCameraIsReady(t *testing.T) {
	rc := NewRealCamera()
	if rc.IsReady() {
		t.Error("camera should not be ready before Start")
	}
	rc.isReady.Store(true)
	if !rc.IsReady() {
		t.Error("camera should be ready after isReady set")
	}
	rc.isStopping.Store(true)
	if rc.IsReady() {
		t.Error("camera should not be ready when stopping")
	}
}

func TestRealCameraEncodeFrame(t *testing.T) {
	img := createTestImage(10, 10)
	jpegData, err := encodeFrameToJPEG(img, 80)
	if err != nil {
		t.Fatalf("encodeFrameToJPEG failed: %v", err)
	}
	if len(jpegData) == 0 {
		t.Error("encoded JPEG data is empty")
	}
	if len(jpegData) >= 2 && (jpegData[0] != 0xFF || jpegData[1] != 0xD8) {
		t.Error("encoded data doesn't start with JPEG SOI marker")
	}
}

func TestRealCameraStartMissingBinaries(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	rc.lookPath = func(string) (string, error) { return "", errors.New("missing") }
	rc.launchFn = rc.launchContinuousProducer

	err := rc.Start(640, 480, 24, 80)
	if err == nil || !strings.Contains(err.Error(), "none of rpicam-vid, libcamera-vid, or ffmpeg found in PATH") {
		t.Fatalf("expected missing binary error, got: %v", err)
	}
}

func TestRealCameraStartFFmpegFallbackProbeFails(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	rc.lookPath = func(bin string) (string, error) {
		if bin == "ffmpeg" {
			return "/usr/bin/ffmpeg", nil
		}
		return "", errors.New("missing")
	}
	rc.runCmdFn = func(*exec.Cmd) ([]byte, error) {
		return []byte("VIDIOC_STREAMON: Invalid argument"), errors.New("probe failed")
	}

	err := rc.Start(640, 480, 24, 80)
	if err == nil {
		t.Fatal("expected probe failure")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "streamon failed") {
		t.Fatalf("expected streamon mapping, got: %v", err)
	}
	if !strings.Contains(err.Error(), "rpicam-vid/libcamera-vid") {
		t.Fatalf("expected CSI guidance, got: %v", err)
	}
}

func TestRealCameraBuildFFmpegCommandIncludesInputNegotiation(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/video0"
	rc.width, rc.height, rc.fps, rc.jpegQuality = 1280, 720, 30, 80

	cmd := rc.buildFFmpegCommand()
	args := strings.Join(cmd.Args, " ")
	// Should include V4L2 format without restrictive input_format to support libcamera devices
	if !strings.Contains(args, "-f video4linux2") {
		t.Fatalf("expected video4linux2 format, args=%q", args)
	}
	if !strings.Contains(args, "-framerate 30") {
		t.Fatalf("expected explicit framerate, args=%q", args)
	}
	if !strings.Contains(args, "-i /dev/video0") {
		t.Fatalf("expected device path, args=%q", args)
	}
	// Should NOT have restrictive input_format that breaks libcamera devices
	if strings.Contains(args, "-input_format mjpeg") {
		t.Fatalf("should not use restrictive input_format for libcamera compatibility, args=%q", args)
	}
}

func TestRealCameraBuildFFmpegCommandMapsJPEGQualityToQuantizer(t *testing.T) {
	tests := []struct {
		name            string
		jpegQuality     int
		wantQuantizerQV string
	}{
		{name: "low quality", jpegQuality: 1, wantQuantizerQV: "31"},
		{name: "mid quality", jpegQuality: 50, wantQuantizerQV: "17"},
		{name: "high quality", jpegQuality: 100, wantQuantizerQV: "2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rc := NewRealCamera()
			rc.devicePath = "/dev/video0"
			rc.width, rc.height, rc.fps = 1280, 720, 30
			rc.jpegQuality = tc.jpegQuality

			cmd := rc.buildFFmpegCommand()
			if got := findCommandArgValue(cmd.Args, "-q:v"); got != tc.wantQuantizerQV {
				t.Fatalf("jpegQuality=%d => -q:v %q, want %q (args=%v)", tc.jpegQuality, got, tc.wantQuantizerQV, cmd.Args)
			}
		})
	}
}

func TestFFmpegMJPEGQuantizerFromQualityMatchesRoundedLinearMapping(t *testing.T) {
	const (
		jpegQualityMin = 1
		jpegQualityMax = 100
		ffmpegQMax     = 31
		ffmpegQMin     = 2
	)

	span := float64(ffmpegQMax - ffmpegQMin)
	for quality := -20; quality <= 140; quality++ {
		clamped := quality
		if clamped < jpegQualityMin {
			clamped = jpegQualityMin
		}
		if clamped > jpegQualityMax {
			clamped = jpegQualityMax
		}

		progress := float64(clamped-jpegQualityMin) / float64(jpegQualityMax-jpegQualityMin)
		want := ffmpegQMax - int(math.Round(progress*span))
		if got := ffmpegMJPEGQuantizerFromQuality(quality); got != want {
			t.Fatalf("quality=%d (clamped=%d) => q:v=%d, want %d", quality, clamped, got, want)
		}
	}
}

func findCommandArgValue(args []string, key string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}

func TestMapFFmpegInputError(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/video0"

	tests := []struct {
		name       string
		stderr     string
		wantSubstr string
	}{
		{name: "streamon", stderr: "VIDIOC_STREAMON: Invalid argument", wantSubstr: "STREAMON failed"},
		{name: "open input", stderr: "Error opening input: Permission denied", wantSubstr: "could not open the V4L2 input"},
		{name: "default", stderr: "some unknown ffmpeg message", wantSubstr: "V4L2 probe failed before ffmpeg fallback"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := rc.mapFFmpegInputError(tc.stderr, errors.New("boom"))
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("expected %q in %v", tc.wantSubstr, err)
			}
		})
	}
}

func TestProbeV4L2CaptureNodeSuccess(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/video0"
	rc.runCmdFn = func(*exec.Cmd) ([]byte, error) {
		return nil, nil
	}
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

	if err := rc.probeV4L2CaptureNode(); err != nil {
		t.Fatalf("expected probe success, got %v", err)
	}
	if !strings.Contains(buf.String(), "V4L2 probe succeeded") {
		t.Fatalf("expected success log, got %q", buf.String())
	}
}

func createTestImage(width, height int) *image.YCbCr {
	img := image.NewYCbCr(image.Rect(0, 0, width, height), image.YCbCrSubsampleRatio420)
	for i := 0; i < len(img.Y); i++ {
		img.Y[i] = uint8(i & 0xFF)
	}
	return img
}

type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }
