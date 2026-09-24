package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CyanAutomation/gogomio/internal/config"
)

type discardResponseWriter struct{}

func (w *discardResponseWriter) Header() http.Header {
	return http.Header{}
}

func (w *discardResponseWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (w *discardResponseWriter) WriteHeader(statusCode int) {}

func writeMultipartFrameLegacy(w io.Writer, frame []byte) error {
	boundary := []byte("--frame\r\n")
	headers := []byte("Content-Type: image/jpeg\r\nContent-Length: " + fmt.Sprintf("%d", len(frame)) + "\r\n\r\n")
	trailer := []byte("\r\n")

	if _, err := w.Write(boundary); err != nil {
		return err
	}
	if _, err := w.Write(headers); err != nil {
		return err
	}
	if _, err := w.Write(frame); err != nil {
		return err
	}
	_, err := w.Write(trailer)
	return err
}

func BenchmarkWriteMultipartFrame(b *testing.B) {
	b.ReportAllocs()

	frame := bytes.Repeat([]byte{0xFF, 0xD8, 0xFF, 0xD9}, 16*1024/4)
	writer := &discardResponseWriter{}
	contentLengthScratch := make([]byte, 0, 20)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := writeMultipartFrame(writer, &contentLengthScratch, frame); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteMultipartFrameLegacy(b *testing.B) {
	b.ReportAllocs()

	frame := bytes.Repeat([]byte{0xFF, 0xD8, 0xFF, 0xD9}, 16*1024/4)
	writer := &discardResponseWriter{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := writeMultipartFrameLegacy(writer, frame); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStreamFixedFrames(b *testing.B) {
	originalLogWriter := log.Writer()
	log.SetOutput(io.Discard)
	b.Cleanup(func() { log.SetOutput(originalLogWriter) })

	const framesPerStream = 8

	frame := bytes.Repeat([]byte{0xFF, 0xD8, 0xFF, 0xD9}, 16*1024/4)
	encodedFrame := httptest.NewRecorder()
	contentLengthScratch := make([]byte, 0, 20)
	if err := writeMultipartFrame(encodedFrame, &contentLengthScratch, frame); err != nil {
		b.Fatal(err)
	}
	streamBytes := int64(framesPerStream * encodedFrame.Body.Len())

	cfg := &config.Config{MaxStreamConnections: 1}
	fm := NewFrameManager(newStableFrameCamera(frame), cfg)
	b.Cleanup(func() { fm.Stop() })

	// Benchmark the stream endpoint without API-wide middleware. The production
	// per-IP rate limit is intended for client requests, and benchmark iterations
	// exceed it, which would measure 429 responses instead of streamed frames.
	router := http.NewServeMux()
	var streamErr error
	router.HandleFunc("GET /stream.mjpg", func(w http.ResponseWriter, r *http.Request) {
		streamErr = fm.StreamFrame(w, r, cfg.MaxStreamConnections)
	})

	b.SetBytes(streamBytes)
	b.ReportMetric(framesPerStream, "frames/op")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// The byte limit admits exactly framesPerStream complete frames. The next
		// multipart boundary returns io.EOF, ending the handler deterministically.
		writer := newStreamCapturingWriter(streamBytes)
		req := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil)
		streamErr = nil
		router.ServeHTTP(writer, req)
		if streamErr != nil && !errors.Is(streamErr, io.EOF) {
			b.Fatalf("stream handler failed: %v", streamErr)
		}

		if got := writer.GetBytesWritten(); got != streamBytes {
			b.Fatalf("delivered %d bytes, want %d (%d complete frames)", got, streamBytes, framesPerStream)
		}
	}
}
