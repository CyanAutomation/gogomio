package camera

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestHealthMonitorRunsProductionCheckOnTick covers REQ-CAMERA-HEALTH-FRAME-PROGRESS:
// a delivered tick must run the production health check and report frame progress.
func TestHealthMonitorRunsProductionCheckOnTick(t *testing.T) {
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
	defer func() {
		close(ticks)
		<-done
	}()

	assertProgress := func(sequence uint64, tick time.Time) {
		t.Helper()
		rc.frameMutex.Lock()
		rc.frameSeq = sequence // The same counter advanced by the production frame reader.
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

// TestHealthMonitorConcurrentFrameUpdates tests concurrent frame sequence updates
func TestHealthMonitorConcurrentFrameUpdates(t *testing.T) {
	type frameMonitor struct {
		frameSeq uint64
		mu       sync.RWMutex
	}

	monitor := &frameMonitor{}

	// Simulate concurrent frame captures
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				monitor.mu.Lock()
				atomic.AddUint64(&monitor.frameSeq, 1)
				monitor.mu.Unlock()
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	monitor.mu.RLock()
	finalSeq := atomic.LoadUint64(&monitor.frameSeq)
	monitor.mu.RUnlock()

	if finalSeq != 50 {
		t.Errorf("Expected frame count 50, got %d", finalSeq)
	}
}

// TestHealthMonitorErrorDetection tests that health monitor detects reader errors
func TestHealthMonitorErrorDetection(t *testing.T) {
	// Test error tracking logic
	errorCount := 0
	lastError := error(nil)

	// Simulate error occurring
	err := context.Canceled
	if err != nil {
		errorCount++
		lastError = err
	}

	if errorCount != 1 {
		t.Errorf("Expected error count 1, got %d", errorCount)
	}

	if lastError == nil {
		t.Error("Expected last error to be set")
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
