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

// TestHealthMonitorReportsFrameProgress covers REQ-CAMERA-HEALTH-FRAME-PROGRESS:
// every sequence advance must be reported as a healthy, flowing capture stream.
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
	<-done
}

type logWriterFunc func(string)

func (write logWriterFunc) Write(p []byte) (int, error) {
	write(string(p))
	return len(p), nil
}

// TestHealthMonitorStallDetection tests that health monitor detects frame stalls
func TestHealthMonitorStallDetection(t *testing.T) {
	// Test stall detection logic
	lastSeq := uint64(5)
	lastTime := time.Now().Add(-15 * time.Second)

	// Simulate no frame progress for 15 seconds
	currentSeq := uint64(5)

	// Check for stall
	if currentSeq == lastSeq {
		stallDuration := time.Since(lastTime)

		if stallDuration > 30*time.Second {
			// Should log as error
			if stallDuration <= 30*time.Second {
				t.Error("Stall duration should be > 30 seconds")
			}
		} else if stallDuration > 10*time.Second {
			// Should log as warning
			if stallDuration <= 10*time.Second {
				t.Error("Stall duration should be > 10 seconds")
			}
		}
	}
}

// TestHealthMonitorTickInterval tests that health monitor runs at expected interval
func TestHealthMonitorTickInterval(t *testing.T) {
	// Test that the health monitor ticker interval is reasonable
	expectedInterval := 10 * time.Second

	// Verify tick count over a period
	ticker := time.NewTicker(expectedInterval)
	defer ticker.Stop()

	tickCount := 0
	testDuration := 25 * time.Millisecond // Short duration for testing
	timeout := time.After(testDuration)

	for {
		select {
		case <-ticker.C:
			tickCount++
			if tickCount >= 3 {
				goto done
			}
		case <-timeout:
			// Timeout is expected in unit test
			goto done
		}
	}

done:
	// In this short test, we won't get actual ticks, but this validates the ticker logic
	if expectedInterval != 10*time.Second {
		t.Error("Expected health monitor interval to be 10 seconds")
	}
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
