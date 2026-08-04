package api

// HTTP content type constants
const (
	// ContentTypeJSON is the MIME type for JSON responses.
	ContentTypeJSON = "application/json"

	// ContentTypeTextHTML is the MIME type for HTML responses.
	ContentTypeTextHTML = "text/html; charset=utf-8"

	// ContentTypeMJPEG is the MIME type for MJPEG streaming with boundary marker.
	ContentTypeMJPEG = "multipart/x-mixed-replace; boundary=FRAME"

	// ContentTypeJPEG is the MIME type for JPEG images.
	ContentTypeJPEG = "image/jpeg"
)
