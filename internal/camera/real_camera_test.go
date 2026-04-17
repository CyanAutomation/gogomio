package camera

import (
	"errors"
	"image"
	"io"
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
			_, _ = stdoutW.Write([]byte{0xFF, 0xD8, 0x00, 0x01, 0xFF, 0xD9})
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
			frame1 := []byte{0xFF, 0xD8, 0x01, 0xFF, 0xD9}
			frame2 := []byte{0xFF, 0xD8, 0x02, 0xFF, 0xD9}
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

	want := []byte{0xFF, 0xD8, 0x02, 0xFF, 0xD9}
	if string(frame) != string(want) {
		t.Fatalf("CaptureFrame() got %v, want %v", frame, want)
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

	if err := rc.Start(640, 480, 24, 80); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = rc.Stop() }()

	_, err := rc.CaptureFrame()
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for frame") {
		t.Fatalf("expected timeout error, got %v", err)
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
	if err == nil || !strings.Contains(err.Error(), "neither libcamera-vid nor ffmpeg") {
		t.Fatalf("expected missing binary error, got: %v", err)
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
