package config

// Network and server constants
const (
	// DefaultBindHost is the default IP address the server binds to.
	// 0.0.0.0 means bind to all available network interfaces.
	DefaultBindHost = "0.0.0.0"

	// DefaultPort is the default HTTP port the server listens on.
	DefaultPort = 8000

	// DefaultServerURL is the default base URL for CLI commands to connect to a running server.
	DefaultServerURL = "http://localhost:8000"
)
