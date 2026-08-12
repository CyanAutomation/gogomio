package api

import (
	"sync"
	"testing"
)

func TestStreamCapturingWriterClosesFirstBoundaryOnce(t *testing.T) {
	const writers = 32

	writer := newStreamCapturingWriter(0)
	start := make(chan struct{})
	writeErrors := make(chan error, writers)
	var wg sync.WaitGroup
	wg.Add(writers)

	for range writers {
		go func() {
			defer wg.Done()
			<-start
			if _, err := writer.Write([]byte("--frame\r\n")); err != nil {
				writeErrors <- err
			}
		}()
	}

	close(start)
	wg.Wait()
	close(writeErrors)

	for err := range writeErrors {
		t.Errorf("Write() error = %v", err)
	}

	select {
	case <-writer.FirstBoundary():
	default:
		t.Fatal("FirstBoundary() was not closed")
	}
}
