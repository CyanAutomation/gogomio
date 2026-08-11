package camera

import (
	"fmt"
	"io"
	"log"
	"sync/atomic"
	"testing"
	"time"
)

// TestHealthMonitorReportsFrameProgress covers REQ-CAMERA-HEALTH-FRAME-PROGRESS:
// a delivered tick must run the production health check and report frame progress
// published concurrently by the production camera state.
func TestHealthMonitorReportsFrameProgress(t *testing.T) {
	ticks := make(chan time.Time)
	diagnostics := make(chan string, 4)
	rc := NewRealCamera()
	rc.healthTicks = ticks
	rc.logger = log.New(logWriterFunc(func(message string) {
		diagnostics <- message
	}), "", 0)

	done := make(chan struct{})
	go func() {
		rc.healthMonitor()
		close(done)
	}()

	frameUpdates := make(chan uint64)
	framePublished := make(chan struct{})
	publisherDone := make(chan struct{})
	go func() {
		defer close(publisherDone)
		for sequence := range frameUpdates {
			rc.frameMutex.Lock()
			rc.frameSeq = sequence // The same counter advanced by the production frame reader.
			rc.frameMutex.Unlock()
			framePublished <- struct{}{}
		}
	}()

	defer func() {
		close(frameUpdates)
		<-publisherDone
		close(ticks)
		<-done
	}()

	assertProgress := func(sequence uint64, tick time.Time) {
		t.Helper()
		frameUpdates <- sequence
		<-framePublished
		// Ensure health monitor can observe the updated frameSeq before tick
		rc.frameMutex.Lock()
		rc.frameMutex.Unlock()
		ticks <- tick

		want := fmt.Sprintf("🏥 Health check: frames flowing normally (seq: %d)\n", sequence)
		for {
			select {
			case diagnostic := <-diagnostics:
				if diagnostic == want {
					return
				}
			case <-time.After(time.Second):
				t.Fatalf("health monitor did not report frame progress: want %q", want)
			}
		}
	}

	firstTick := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	assertProgress(7, firstTick)
	assertProgress(8, firstTick.Add(10*time.Second))

	rc.isStopping.Store(true)
	ticks <- firstTick.Add(20 * time.Second)
}

type logWriterFunc func(string)

func (write logWriterFunc) Write(p []byte) (int, error) {
	write(string(p))
	return len(p), nil
}

// TestHealthMonitorStallDetection verifies that RealCamera's health monitor
// reports increasingly severe diagnostics while the frame sequence is stalled.
func TestHealthMonitorStallDetection(t *testing.T) {
	ticks := make(chan time.Time)
	diagnostics := make(chan string, 4)
	rc := NewRealCamera()
	rc.healthTicks = ticks
	rc.logger = log.New(logWriterFunc(func(message string) {
		diagnostics <- message
	}), "", 0)
	rc.frameSeq = 5

	done := make(chan struct{})
	go func() {
		rc.healthMonitor()
		close(done)
	}()
	defer func() {
		close(ticks)
		<-done
	}()

	assertDiagnostic := func(tick time.Time, want string) {
		t.Helper()
		ticks <- tick
		for {
			select {
			case diagnostic := <-diagnostics:
				if diagnostic == want {
					return
				}
			case <-time.After(time.Second):
				t.Fatalf("health monitor did not report stall: want %q", want)
			}
		}
	}

	firstTick := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	assertDiagnostic(firstTick, "🏥 Health check: frames flowing normally (seq: 5)\n")
	assertDiagnostic(firstTick.Add(11*time.Second), "ℹ️  Health check: no recent frames for 11s (seq: 5)\n")
	assertDiagnostic(firstTick.Add(31*time.Second), "⚠️  Health check: frame capture stalled for 31s (seq: 5)\n")
}

// TestHealthMonitorReportsReaderError verifies that an error published by the
// production MJPEG reader is reported by RealCamera's health monitor.
func TestHealthMonitorReportsReaderError(t *testing.T) {
	ticks := make(chan time.Time)
	diagnostics := make(chan string, 4)
	rc := NewRealCamera()
	rc.healthTicks = ticks
	rc.logger = log.New(logWriterFunc(func(message string) {
		diagnostics <- message
	}), "", 0)
	rc.readerDone = make(chan struct{})
	rc.frameUpdateCh = make(chan struct{})

	stdout, backend := io.Pipe()
	rc.procStdout = stdout
	go rc.readMJPEGStream()

	readerErr := fmt.Errorf("camera stream disconnected")
	if err := backend.CloseWithError(readerErr); err != nil {
		t.Fatalf("close camera stream: %v", err)
	}
	<-rc.readerDone

	done := make(chan struct{})
	go func() {
		rc.healthMonitor()
		close(done)
	}()
	defer func() {
		rc.isStopping.Store(true)
		close(ticks)
		<-done
	}()

	ticks <- time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	want := "⚠️  Health check: reader error detected: camera stream disconnected\n"
	for {
		select {
		case diagnostic := <-diagnostics:
			if diagnostic == want {
				return
			}
		case <-time.After(time.Second):
			t.Fatalf("health monitor did not report reader error: want %q", want)
		}
	}
}

// BenchmarkHealthMonitorFrameProgressCheck benchmarks frame progress detection
func BenchmarkHealthMonitorFrameProgressCheck(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var frameSeq uint64
		atomic.StoreUint64(&frameSeq, uint64(i))

		_ = uint64(i - 1)
	}
}

// BenchmarkHealthMonitorStallCalculation benchmarks stall duration calculation
func BenchmarkHealthMonitorStallCalculation(b *testing.B) {
	lastTime := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stallDuration := time.Since(lastTime)
		if stallDuration > 10*time.Second {
			_ = stallDuration
		}
	}
}

// BenchmarkHealthMonitorTickerCreation benchmarks ticker creation
func BenchmarkHealthMonitorTickerCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ticker := time.NewTicker(10 * time.Second)
		<-ticker.C // Simulate one tick
		ticker.Stop()
	}
}
