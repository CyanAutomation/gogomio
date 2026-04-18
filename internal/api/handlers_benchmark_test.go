package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"
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
