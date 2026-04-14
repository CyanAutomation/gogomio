package camera

// Camera interface defines the contract for camera implementations (real and mock).
type Camera interface {
	// Start initializes and starts the camera capture.
	// width, height: resolution in pixels
	// fps: target frames per second
	// jpegQuality: JPEG quality 1-100
	Start(width, height, fps, jpegQuality int) error

	// CaptureFrame captures and returns a JPEG-encoded frame.
	// Should block until a frame is available.
	CaptureFrame() ([]byte, error)

	// Stop stops the camera capture.
	Stop() error

	// IsReady returns true if the camera is initialized and ready.
	IsReady() bool
}
