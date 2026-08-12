package api

import (
	"sync"
	"testing"
)

func TestStreamCapturingWriterClosesFirstBoundaryOnce(t *testing.T) {
	const writers = 32

	writer := newStreamCapturingWriter(0)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(writers)

	for range writers {
		go func() {
			defer wg.Done()
			<-start
			_, _ = writer.Write([]byte("--frame\r\n"))
		}()
	}

	close(start)
	wg.Wait()

	select {
	case <-writer.FirstBoundary():
	default:
		t.Fatal("FirstBoundary() was not closed")
	}
}
