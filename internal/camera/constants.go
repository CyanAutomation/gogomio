package camera

// Camera device and backend constants
const (
	// DefaultDevicePath is the default V4L2 device path for Raspberry Pi CSI cameras.
	DefaultDevicePath = "/dev/video0"

	// Backend names for camera implementations
	BackendRpicam    = "rpicam-vid"
	BackendLibcamera = "libcamera-vid"
	BackendFFmpeg    = "ffmpeg"

	// FFmpeg MJPEG quantizer constants (lower = higher quality)
	FFmpegQMax = 31 // lowest visual quality
	FFmpegQMin = 2  // highest visual quality
)
