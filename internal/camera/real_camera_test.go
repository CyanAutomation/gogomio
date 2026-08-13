package camera

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestRealCameraInitialization(t *testing.T) {
	const (
		defaultWidth       = 640
		defaultHeight      = 480
		defaultFPS         = 24
		defaultJPEGQuality = 80
	)
	backendStartErr := errors.New("backend start intercepted")

	rc := NewRealCamera()
	rc.statFn = func(path string) (os.FileInfo, error) {
		if path != DefaultDevicePath {
			t.Fatalf("camera device = %q, want default %q", path, DefaultDevicePath)
		}
		return nil, nil
	}
	rc.lookPath = func(name string) (string, error) {
		if name == BackendFFmpeg {
			return "/usr/bin/ffmpeg", nil
		}
		return "", exec.ErrNotFound
	}
	var backendCmd *exec.Cmd
	rc.startCommandFn = func(cmd *exec.Cmd, backend string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		if backend != BackendFFmpeg {
			t.Fatalf("backend = %q, want %q", backend, BackendFFmpeg)
		}
		backendCmd = cmd
		return nil, nil, nil, nil, backendStartErr
	}

	err := rc.Start(defaultWidth, defaultHeight, defaultFPS, defaultJPEGQuality)
	if !errors.Is(err, backendStartErr) {
		t.Fatalf("Start() error = %v, want injected backend error", err)
	}
	if backendCmd == nil {
		t.Fatal("Start() did not invoke the camera backend")
	}
	if got := findCommandArgValue(backendCmd.Args, "-video_size"); got != "640x480" {
		t.Errorf("backend resolution = %q, want 640x480 (args=%v)", got, backendCmd.Args)
	}
	if got := findCommandArgValue(backendCmd.Args, "-framerate"); got != "24" {
		t.Errorf("backend FPS = %q, want 24 (args=%v)", got, backendCmd.Args)
	}
	if got := findCommandArgValue(backendCmd.Args, "-i"); got != DefaultDevicePath {
		t.Errorf("backend device = %q, want %q (args=%v)", got, DefaultDevicePath, backendCmd.Args)
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

func TestRealCameraConcurrentStartLaunchesOnce(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	rc.stopWaitTimeout = time.Second

	launchStarted := make(chan struct{})
	releaseLaunch := make(chan struct{})
	var launchCalls atomic.Int32
	var launched *exec.Cmd
	rc.launchFn = func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		launchCalls.Add(1)
		close(launchStarted)
		<-releaseLaunch

		stdoutR, stdoutW := io.Pipe()
		stderrR, stderrW := io.Pipe()
		cmd := exec.Command("bash", "-c", "sleep 30")
		if err := cmd.Start(); err != nil {
			return nil, nil, nil, nil, err
		}
		launched = cmd
		go func() {
			defer func() { _ = stdoutW.Close() }()
			jpegData, _ := encodeFrameToJPEG(createTestImage(8, 8), 80)
			_, _ = stdoutW.Write(jpegData)
		}()
		go func() { _ = stderrW.Close() }()
		return cmd, nopWriteCloser{}, stdoutR, stderrR, nil
	}

	firstDone := make(chan error, 1)
	go func() { firstDone <- rc.Start(640, 480, 24, 80) }()
	<-launchStarted

	if err := rc.Start(640, 480, 24, 80); !errors.Is(err, ErrCameraAlreadyStarted) {
		t.Fatalf("second Start() error = %v, want %v", err, ErrCameraAlreadyStarted)
	}
	if got := launchCalls.Load(); got != 1 {
		t.Fatalf("launchFn call count = %d, want 1", got)
	}

	close(releaseLaunch)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if err := rc.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if launched == nil {
		t.Fatal("launchFn did not launch a process")
	}
	if err := launched.Process.Signal(syscall.Signal(0)); err == nil {
		t.Fatal("sole camera process is still running after Stop")
	}
}

func TestRealCameraStopCancelsStartBlockedInStat(t *testing.T) {
	rc := NewRealCamera()
	statStarted := make(chan struct{})
	releaseStat := make(chan struct{})
	rc.statFn = func(string) (os.FileInfo, error) {
		close(statStarted)
		<-releaseStat
		return nil, nil
	}
	rc.launchFn = func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		t.Fatal("launchFn called after startup was canceled")
		return nil, nil, nil, nil, nil
	}

	startDone := make(chan error, 1)
	go func() { startDone <- rc.Start(640, 480, 24, 80) }()
	<-statStarted
	stopDone := make(chan error, 1)
	go func() { stopDone <- rc.Stop() }()
	waitForLifecycleState(t, rc, lifecycleStopping)

	close(releaseStat)
	if err := <-startDone; !errors.Is(err, errStartupCanceled) {
		t.Fatalf("Start() error = %v, want startup cancellation", err)
	}
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if rc.IsReady() {
		t.Fatal("camera should not be ready after canceled startup")
	}
}

func TestRealCameraStopCleansProcessFromBlockedLaunch(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	launchStarted := make(chan struct{})
	releaseLaunch := make(chan struct{})
	var launched *exec.Cmd
	rc.launchFn = func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		close(launchStarted)
		<-releaseLaunch
		cmd := exec.Command("bash", "-c", "sleep 30")
		if err := cmd.Start(); err != nil {
			return nil, nil, nil, nil, err
		}
		launched = cmd
		return cmd, nopWriteCloser{}, io.NopCloser(strings.NewReader("")), io.NopCloser(strings.NewReader("")), nil
	}

	startDone := make(chan error, 1)
	go func() { startDone <- rc.Start(640, 480, 24, 80) }()
	<-launchStarted
	stopDone := make(chan error, 1)
	go func() { stopDone <- rc.Stop() }()
	waitForLifecycleState(t, rc, lifecycleStopping)
	close(releaseLaunch)

	if err := <-startDone; !errors.Is(err, errStartupCanceled) {
		t.Fatalf("Start() error = %v, want startup cancellation", err)
	}
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if rc.IsReady() {
		t.Fatal("camera should not be ready after canceled startup")
	}
	if launched == nil {
		t.Fatal("launchFn did not launch a process")
	}
	if err := launched.Process.Signal(syscall.Signal(0)); err == nil {
		t.Fatal("process from canceled startup is still running")
	}
}

func waitForLifecycleState(t *testing.T, rc *RealCamera, want lifecycleState) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		rc.captureMutex.Lock()
		got := rc.lifecycle
		rc.captureMutex.Unlock()
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("lifecycle state = %d, want %d", got, want)
		}
		time.Sleep(time.Millisecond)
	}
}

type fakeCommandProcess struct {
	started chan struct{}
	killed  chan struct{}
	wait    chan error

	stdinR  *io.PipeReader
	stdinW  *io.PipeWriter
	stdoutR *io.PipeReader
	stdoutW *io.PipeWriter
	stderrR *io.PipeReader
	stderrW *io.PipeWriter
}

type waitCommandProcess struct {
	realCommandProcess
	waitFn func(*exec.Cmd) error
}

func (p waitCommandProcess) Wait(cmd *exec.Cmd) error { return p.waitFn(cmd) }

func TestRealCameraStopAfterUnstartedCommandLaunchFailure(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	startupFailure := errors.New("backend failed before command start")
	var createdCommand *exec.Cmd
	rc.launchFn = func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		createdCommand = exec.Command("unused")
		return createdCommand, nil, nil, nil, startupFailure
	}

	startErr := rc.Start(640, 480, 24, 80)
	if !errors.Is(startErr, startupFailure) {
		t.Fatalf("Start() error = %v, want startup failure %v", startErr, startupFailure)
	}
	if createdCommand == nil {
		t.Fatal("launch did not create a command")
	}
	if createdCommand.Process != nil {
		t.Fatal("launch failure command was unexpectedly started")
	}
	if rc.IsReady() {
		t.Fatal("camera should not be ready after startup failure")
	}

	for call := 1; call <= 2; call++ {
		stopDone := make(chan error, 1)
		go func() { stopDone <- rc.Stop() }()
		select {
		case err := <-stopDone:
			if err != nil {
				t.Fatalf("Stop() call %d error = %v, want nil", call, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("Stop() call %d did not return promptly", call)
		}
		if rc.IsReady() {
			t.Fatalf("camera should not be ready after Stop() call %d", call)
		}
	}

	if !errors.Is(startErr, startupFailure) {
		t.Fatalf("startup error after Stop() = %v, want original failure %v", startErr, startupFailure)
	}
}

func newFakeCommandProcess() *fakeCommandProcess {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	return &fakeCommandProcess{
		started: make(chan struct{}, 1),
		killed:  make(chan struct{}, 1),
		wait:    make(chan error, 1),
		stdinR:  stdinR, stdinW: stdinW,
		stdoutR: stdoutR, stdoutW: stdoutW,
		stderrR: stderrR, stderrW: stderrW,
	}
}

func (p *fakeCommandProcess) Start(*exec.Cmd) error {
	p.started <- struct{}{}
	return nil
}
func (p *fakeCommandProcess) Wait(*exec.Cmd) error              { return <-p.wait }
func (p *fakeCommandProcess) Signal(*exec.Cmd, os.Signal) error { return nil }
func (p *fakeCommandProcess) Kill(*exec.Cmd) error {
	select {
	case p.killed <- struct{}{}:
	default:
	}
	_ = p.stdinR.Close()
	_ = p.stdinW.Close()
	_ = p.stdoutW.Close()
	_ = p.stderrW.Close()
	return nil
}
func (p *fakeCommandProcess) close() {
	_ = p.stdinR.Close()
	_ = p.stdinW.Close()
	_ = p.stdoutR.Close()
	_ = p.stdoutW.Close()
	_ = p.stderrR.Close()
	_ = p.stderrW.Close()
	select {
	case p.wait <- nil:
	default:
	}
}

func TestRealCameraProcessLifecycle(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	rc.captureWaitTimeout = 200 * time.Millisecond
	rc.stopWaitTimeout = time.Second

	process := newFakeCommandProcess()
	t.Cleanup(process.close)
	rc.process = process
	rc.launchFn = func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		cmd := &exec.Cmd{}
		if err := process.Start(cmd); err != nil {
			return nil, nil, nil, nil, err
		}
		return cmd, process.stdinW, process.stdoutR, process.stderrR, nil
	}

	if rc.IsReady() {
		t.Fatal("camera should not be ready before Start")
	}
	startDone := make(chan error, 1)
	go func() { startDone <- rc.Start(640, 480, 24, 80) }()
	<-process.started
	if rc.IsReady() {
		t.Fatal("camera should not be ready before the first valid frame")
	}

	frame, err := encodeFrameToJPEG(createTestImage(8, 8), 80)
	if err != nil {
		t.Fatalf("encode startup frame: %v", err)
	}
	if _, err := process.stdoutW.Write(frame); err != nil {
		t.Fatalf("write startup frame: %v", err)
	}
	if err := <-startDone; err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !rc.IsReady() {
		t.Fatal("camera should be ready after the first valid frame")
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- rc.Stop() }()
	<-process.killed
	if rc.IsReady() {
		t.Fatal("camera should not be ready once shutdown begins")
	}
	select {
	case err := <-stopDone:
		t.Fatalf("Stop() completed before process wait was released: %v", err)
	default:
	}
	process.wait <- nil
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestRealCameraStopReturnsWhenBackendWaitExceedsDeadline(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	rc.captureWaitTimeout = 200 * time.Millisecond

	waitStarted := make(chan struct{})
	releaseWait := make(chan struct{})
	waitExited := make(chan struct{})
	rc.process = waitCommandProcess{waitFn: func(*exec.Cmd) error {
		defer close(waitExited)
		close(waitStarted)
		<-releaseWait
		return nil
	}}
	timeoutRequested := make(chan time.Duration, 1)
	expireTimeout := make(chan time.Time, 1)
	alreadyExpired := make(chan time.Time)
	close(alreadyExpired)
	var timeoutCalls atomic.Int32
	rc.stopWaitAfter = func(timeout time.Duration) <-chan time.Time {
		if timeoutCalls.Add(1) == 1 {
			timeoutRequested <- timeout
			return expireTimeout
		}
		return alreadyExpired
	}

	rc.launchFn = func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		stdoutR, stdoutW := io.Pipe()
		stderrR, stderrW := io.Pipe()
		go func() {
			defer func() { _ = stdoutW.Close() }()
			jpegData, _ := encodeFrameToJPEG(createTestImage(8, 8), 80)
			_, _ = stdoutW.Write(jpegData)
		}()
		go func() {
			defer func() { _ = stderrW.Close() }()
		}()
		return &exec.Cmd{}, nopWriteCloser{}, stdoutR, stderrR, nil
	}

	if err := rc.Start(640, 480, 24, 80); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	<-waitStarted
	t.Cleanup(func() {
		close(releaseWait)
		<-waitExited
	})

	stopDone := make(chan error, 1)
	go func() {
		stopDone <- rc.Stop()
	}()

	if got := <-timeoutRequested; got != rc.stopWaitTimeout {
		t.Fatalf("Stop() timeout = %v, want %v", got, rc.stopWaitTimeout)
	}
	expireTimeout <- time.Time{}
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if rc.IsReady() {
		t.Fatal("camera should not be ready after Stop")
	}
}

func TestRealCameraRestartAfterStopTimeoutIsolatesBlockedReader(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	rc.captureWaitTimeout = time.Second
	expired := make(chan time.Time)
	close(expired)
	rc.stopWaitAfter = func(time.Duration) <-chan time.Time { return expired }
	rc.process = waitCommandProcess{waitFn: func(*exec.Cmd) error { return nil }}

	type generationStream struct {
		reader *io.PipeReader
		writer *io.PipeWriter
	}
	streams := make([]generationStream, 0, 2)
	launched := make(chan generationStream, 2)
	rc.launchFn = func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		reader, writer := io.Pipe()
		stream := generationStream{reader: reader, writer: writer}
		launched <- stream
		return &exec.Cmd{}, nopWriteCloser{}, reader, io.NopCloser(strings.NewReader("")), nil
	}

	startWithFrame := func(size int) error {
		done := make(chan error, 1)
		go func() { done <- rc.Start(640, 480, 24, 80) }()
		stream := <-launched
		streams = append(streams, stream)
		frame, _ := encodeFrameToJPEG(createTestImage(size, size), 80)
		if _, err := streams[len(streams)-1].writer.Write(frame); err != nil {
			t.Fatalf("write startup frame: %v", err)
		}
		return <-done
	}

	if err := startWithFrame(8); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	oldReaderDone := rc.readerDone
	if err := rc.Stop(); err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	select {
	case <-oldReaderDone:
		t.Fatal("old reader unexpectedly exited before its blocked pipe was released")
	default:
	}

	if err := startWithFrame(10); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	newReaderDone := rc.readerDone
	if err := streams[0].writer.CloseWithError(errors.New("old generation released")); err != nil {
		t.Fatalf("release old reader: %v", err)
	}
	select {
	case <-oldReaderDone:
	case <-time.After(time.Second):
		t.Fatal("old reader did not exit after release")
	}
	select {
	case <-newReaderDone:
		t.Fatal("old reader closed the new generation's completion channel")
	default:
	}
	rc.frameMutex.Lock()
	readerErr := rc.readerErr
	rc.frameMutex.Unlock()
	if readerErr != nil {
		t.Fatalf("old reader changed new generation stream state: %v", readerErr)
	}
	if _, err := rc.CaptureFrame(); err != nil {
		t.Fatalf("new generation stream unavailable after old reader exit: %v", err)
	}
	_ = streams[1].writer.Close()
	if err := rc.Stop(); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestRealCameraStopStuckWaitNoGoroutineGrowth(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	rc.captureWaitTimeout = 200 * time.Millisecond

	var releases []chan struct{}
	var activeWaiters atomic.Int32
	var completedWaiters atomic.Int32
	rc.process = waitCommandProcess{waitFn: func(*exec.Cmd) error {
		activeWaiters.Add(1)
		release := releases[len(releases)-1]
		<-release
		activeWaiters.Add(-1)
		completedWaiters.Add(1)
		return nil
	}}
	rc.launchFn = func() (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
		release := make(chan struct{})
		releases = append(releases, release)
		stdoutR, stdoutW := io.Pipe()
		frame, _ := encodeFrameToJPEG(createTestImage(8, 8), 80)
		go func() {
			defer func() { _ = stdoutW.Close() }()
			_, _ = stdoutW.Write(frame)
		}()
		return &exec.Cmd{}, nopWriteCloser{}, stdoutR, io.NopCloser(strings.NewReader("")), nil
	}

	const cycles = 5
	for cycle := 1; cycle <= cycles; cycle++ {
		if err := rc.Start(640, 480, 24, 80); err != nil {
			t.Fatalf("Start() cycle %d error = %v", cycle, err)
		}
		if !rc.IsReady() {
			t.Fatalf("camera should be ready during cycle %d", cycle)
		}
		close(releases[cycle-1])
		if err := rc.Stop(); err != nil {
			t.Fatalf("Stop() cycle %d error = %v", cycle, err)
		}
		if rc.IsReady() {
			t.Fatalf("camera should not be ready after cycle %d", cycle)
		}
		if got := activeWaiters.Load(); got != 0 {
			t.Fatalf("active backend waiters after cycle %d = %d, want 0", cycle, got)
		}
		if got := completedWaiters.Load(); got != int32(cycle) {
			t.Fatalf("completed backend waiters after cycle %d = %d, want %d", cycle, got, cycle)
		}
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

	if rc.IsReady() {
		t.Fatal("camera should not be ready before Start")
	}

	releaseStream := make(chan struct{})
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
			<-releaseStream
		}()
		return cmd, nopWriteCloser{}, stdoutR, stderrR, nil
	}

	err := rc.Start(640, 480, 24, 80)
	close(releaseStream)
	if err == nil {
		t.Fatal("expected Start() timeout error")
	}
	if !errors.Is(err, ErrFirstFrameTimeout) {
		t.Fatalf("expected ErrFirstFrameTimeout, got %v", err)
	}
	if rc.IsReady() {
		t.Fatal("camera should not be ready after startup timeout")
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

func TestFirstFrameTimeoutRpiCamStartupMinimumIgnoresCaptureWaitTimeout(t *testing.T) {
	rc := NewRealCamera()
	rc.fps = 24
	rc.captureWaitTimeout = 75 * time.Millisecond
	rc.backendAttempted = "rpicam-vid"

	if got, want := rc.firstFrameTimeout(), 4*time.Second; got != want {
		t.Fatalf("firstFrameTimeout() = %v, want %v", got, want)
	}
}

func TestFirstFrameTimeoutFFmpegStillUsesGenericBounds(t *testing.T) {
	rc := NewRealCamera()
	rc.captureWaitTimeout = 50 * time.Millisecond
	rc.backendAttempted = "ffmpeg"

	rc.fps = 120
	if got, want := rc.firstFrameTimeout(), 500*time.Millisecond; got != want {
		t.Fatalf("high-fps timeout = %v, want %v", got, want)
	}

	rc.fps = 1
	if got, want := rc.firstFrameTimeout(), 3*time.Second; got != want {
		t.Fatalf("low-fps timeout = %v, want %v", got, want)
	}
}

func TestRealCameraStartTimeoutMessageUsesStartupTimeout(t *testing.T) {
	rc := NewRealCamera()
	rc.devicePath = "/dev/null"
	rc.captureWaitTimeout = 50 * time.Millisecond
	rc.fps = 120
	rc.backendAttempted = "ffmpeg"

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
			time.Sleep(2 * time.Second)
		}()
		return cmd, nopWriteCloser{}, stdoutR, stderrR, nil
	}

	start := time.Now()
	err := rc.Start(640, 480, 120, 80)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected Start() timeout error")
	}
	if !strings.Contains(err.Error(), "waiting 500ms") {
		t.Fatalf("expected timeout message to include 500ms startup timeout, got %v", err)
	}
	if elapsed < 450*time.Millisecond {
		t.Fatalf("startup wait elapsed too quickly (%v), captureWaitTimeout likely capped startup timeout", elapsed)
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

func TestRealCameraEncodesDecodableJPEGWithInputDimensions(t *testing.T) {
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

	decoded, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		t.Fatalf("jpeg.Decode failed: %v", err)
	}
	if got, want := decoded.Bounds().Size(), img.Bounds().Size(); got != want {
		t.Errorf("decoded image dimensions = %v, want %v", got, want)
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
	if got := countCommandArgOccurrences(cmd.Args, "-q:v"); got != 1 {
		t.Fatalf("expected exactly one -q:v argument, got %d (args=%v)", got, cmd.Args)
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
			if got := countCommandArgOccurrences(cmd.Args, "-q:v"); got != 1 {
				t.Fatalf("expected exactly one -q:v argument, got %d (args=%v)", got, cmd.Args)
			}
		})
	}
}

func TestRealCameraBuildRpiCamVidCommandIncludesQualityFlag(t *testing.T) {
	tests := []struct {
		name        string
		jpegQuality int
		wantQuality string
	}{
		{name: "low quality", jpegQuality: 1, wantQuality: "1"},
		{name: "mid quality", jpegQuality: 50, wantQuality: "50"},
		{name: "high quality", jpegQuality: 100, wantQuality: "100"},
		{name: "below range clamps", jpegQuality: -5, wantQuality: "1"},
		{name: "above range clamps", jpegQuality: 160, wantQuality: "100"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rc := NewRealCamera()
			rc.width, rc.height, rc.fps = 1280, 720, 30
			rc.jpegQuality = tc.jpegQuality

			cmd := rc.buildRpiCamVidCommand()
			if got := findCommandArgValue(cmd.Args, "--quality"); got != tc.wantQuality {
				t.Fatalf("jpegQuality=%d => --quality %q, want %q (args=%v)", tc.jpegQuality, got, tc.wantQuality, cmd.Args)
			}
		})
	}
}

func TestRealCameraBuildLibcameraVidCommandIncludesQualityFlag(t *testing.T) {
	tests := []struct {
		name        string
		jpegQuality int
		wantQuality string
	}{
		{name: "low quality", jpegQuality: 1, wantQuality: "1"},
		{name: "mid quality", jpegQuality: 50, wantQuality: "50"},
		{name: "high quality", jpegQuality: 100, wantQuality: "100"},
		{name: "below range clamps", jpegQuality: -5, wantQuality: "1"},
		{name: "above range clamps", jpegQuality: 160, wantQuality: "100"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rc := NewRealCamera()
			rc.width, rc.height, rc.fps = 1280, 720, 30
			rc.jpegQuality = tc.jpegQuality

			cmd := rc.buildLibcameraVidCommand()
			if got := findCommandArgValue(cmd.Args, "--quality"); got != tc.wantQuality {
				t.Fatalf("jpegQuality=%d => --quality %q, want %q (args=%v)", tc.jpegQuality, got, tc.wantQuality, cmd.Args)
			}
		})
	}
}

func TestRealCameraNativeCommandsSensorMode(t *testing.T) {
	tests := []struct {
		name  string
		build func(*RealCamera) *exec.Cmd
	}{
		{name: "rpicam", build: func(rc *RealCamera) *exec.Cmd { return rc.buildRpiCamVidCommand() }},
		{name: "libcamera", build: func(rc *RealCamera) *exec.Cmd { return rc.buildLibcameraVidCommand() }},
	}

	for _, tc := range tests {
		t.Run(tc.name+"/automatic", func(t *testing.T) {
			rc := NewRealCamera()
			if got := countCommandArgOccurrences(tc.build(rc).Args, "--mode"); got != 0 {
				t.Fatalf("automatic mode unexpectedly generated --mode")
			}
		})
		t.Run(tc.name+"/explicit", func(t *testing.T) {
			rc := NewRealCamera()
			rc.width, rc.height = 1280, 720
			rc.SetSensorMode(2304, 1296)
			cmd := tc.build(rc)
			if got := findCommandArgValue(cmd.Args, "--mode"); got != "2304:1296" {
				t.Fatalf("--mode = %q, want 2304:1296 (args=%v)", got, cmd.Args)
			}
			if got := findCommandArgValue(cmd.Args, "--width"); got != "1280" {
				t.Fatalf("--width = %q, want independent output width 1280", got)
			}
		})
		t.Run(tc.name+"/invalid", func(t *testing.T) {
			rc := NewRealCamera()
			rc.SetSensorMode(2304, 0)
			if got := countCommandArgOccurrences(tc.build(rc).Args, "--mode"); got != 0 {
				t.Fatalf("invalid mode generated --mode")
			}
		})
	}
}

func TestFFmpegMJPEGQuantizerFromQualityMatchesRoundedLinearMapping(t *testing.T) {
	const (
		ffmpegQMax = 31
		ffmpegQMin = 2
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

func TestNativeMJPEGQualityFromQualityClampsToAppRange(t *testing.T) {
	tests := []struct {
		in   int
		want int
	}{
		{in: -20, want: 1},
		{in: 0, want: 1},
		{in: 1, want: 1},
		{in: 55, want: 55},
		{in: 100, want: 100},
		{in: 101, want: 100},
		{in: 200, want: 100},
	}

	for _, tc := range tests {
		if got := nativeMJPEGQualityFromQuality(tc.in); got != tc.want {
			t.Fatalf("quality=%d => %d, want %d", tc.in, got, tc.want)
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

func countCommandArgOccurrences(args []string, key string) int {
	count := 0
	for _, arg := range args {
		if arg == key {
			count++
		}
	}
	return count
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
	rc.SetLogger(log.New(&buf, "", 0))

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
