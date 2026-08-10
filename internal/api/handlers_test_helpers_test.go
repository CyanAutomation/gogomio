package api

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

// stableFrameCamera generates consistent JPEG frames for streaming tests and benchmarks.
type stableFrameCamera struct {
	frame      []byte
	captureErr error
}

func newStableFrameCamera(jpegData []byte) *stableFrameCamera {
	if jpegData == nil {
		// Default: valid minimal JPEG frame (SOI + EOI)
		jpegData = []byte{0xFF, 0xD8, 0xFF, 0xD9}
	}
	return &stableFrameCamera{frame: jpegData}
}

func (c *stableFrameCamera) Start(_, _, _, _ int) error { return nil }
func (c *stableFrameCamera) Stop() error                { return nil }
func (c *stableFrameCamera) IsReady() bool              { return true }
func (c *stableFrameCamera) CaptureFrame() ([]byte, error) {
	return c.CaptureFrameWithContext(context.Background())
}
func (c *stableFrameCamera) CaptureFrameWithContext(ctx context.Context) ([]byte, error) {
	timer := time.NewTimer(1 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}
	return c.frame, c.captureErr
}

// streamCapturingWriter captures the full stream response for validation.
type streamCapturingWriter struct {
	header       http.Header
	statusCode   int
	buf          []byte
	mu           sync.Mutex
	maxBytes     int64
	bytesWritten int64
}

func newStreamCapturingWriter(maxBytes int64) *streamCapturingWriter {
	return &streamCapturingWriter{
		header:   make(http.Header),
		maxBytes: maxBytes,
	}
}

func (w *streamCapturingWriter) Header() http.Header {
	return w.header
}

func (w *streamCapturingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.maxBytes > 0 && w.bytesWritten+int64(len(p)) > w.maxBytes {
		// Stop writing after maxBytes
		return 0, io.EOF
	}

	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}

	w.buf = append(w.buf, p...)
	w.bytesWritten += int64(len(p))
	return len(p), nil
}

func (w *streamCapturingWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.statusCode = code
}

func (w *streamCapturingWriter) Flush() {}

func (w *streamCapturingWriter) GetContent() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buf...)
}

func (w *streamCapturingWriter) GetStatusCode() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.statusCode
}

func (w *streamCapturingWriter) GetHeader(key string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.header.Get(key)
}

func (w *streamCapturingWriter) GetBytesWritten() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.bytesWritten
}
