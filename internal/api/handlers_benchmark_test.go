package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/CyanAutomation/gogomio/internal/config"
	"github.com/go-chi/chi/v5"
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
	const framesPerStream = 8

	frame := bytes.Repeat([]byte{0xFF, 0xD8, 0xFF, 0xD9}, 16*1024/4)
	frameBytes := len(mjpegBoundaryBytes) + len(mjpegContentTypeBytes) +
		len(mjpegContentLengthBytes) + len(strconv.Itoa(len(frame))) +
		len(mjpegHeaderEndBytes) + len(frame) + len(mjpegTrailerBytes)
	streamBytes := int64(framesPerStream * frameBytes)

	cfg := &config.Config{MaxStreamConnections: 1}
	fm := NewFrameManager(newStableFrameCamera(frame), cfg)
	b.Cleanup(func() { fm.Stop() })

	router := chi.NewRouter()
	RegisterHandlers(router, fm, cfg)

	b.SetBytes(streamBytes)
	b.ReportMetric(framesPerStream, "frames/op")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// The byte limit admits exactly framesPerStream complete frames. The next
		// multipart boundary returns io.EOF, ending the handler deterministically.
		writer := newStreamCapturingWriter(streamBytes)
		req := httptest.NewRequest(http.MethodGet, "/stream.mjpg", nil)
		router.ServeHTTP(writer, req)

		if got := writer.GetBytesWritten(); got != streamBytes {
			b.Fatalf("delivered %d bytes, want %d (%d complete frames)", got, streamBytes, framesPerStream)
		}
	}
}
