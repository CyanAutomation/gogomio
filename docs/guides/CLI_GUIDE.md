# GoGoMio CLI Guide

The GoGoMio binary supports two modes:

1. **Server Mode** (default) - Starts the HTTP streaming server
2. **CLI Mode** - Executes commands to interact with a running server

## Quick Start

### Start the server
```bash
./gogomio                    # Default: server mode
./gogomio server            # Explicit: server mode
MOCK_CAMERA=true ./gogomio  # Development: mock camera
```

### Run CLI commands
```bash
# Basic status and info
./gogomio status                    # Show streaming status
./gogomio config                    # Display all configuration
./gogomio diagnostics               # Show system diagnostics
./gogomio version                   # Show version info

# Health monitoring
./gogomio health check              # Quick health check
./gogomio health detailed           # Detailed health status

# Stream management
./gogomio stream info               # Show stream metrics
./gogomio stream stop               # Stop active streams

# Snapshots
./gogomio snapshot capture          # Capture and output to stdout
./gogomio snapshot save /tmp/frame.jpg

# Settings
./gogomio settings get              # List all settings
./gogomio settings get key          # Get specific setting
./gogomio settings set key=value    # Update setting

# Help
./gogomio --help                    # Show all available commands
./gogomio status --help             # Show help for specific command
```

## Docker Usage

### Start server
```bash
docker-compose up -d
docker-compose logs gogomio
```

### Run CLI commands
```bash
# Status commands
docker-compose exec gogomio gogomio status
docker-compose exec gogomio gogomio health check
docker-compose exec gogomio gogomio config

# Specific values
docker-compose exec gogomio gogomio config get fps
docker-compose exec gogomio gogomio config get resolution

# Diagnostics
docker-compose exec gogomio gogomio diagnostics
docker-compose exec gogomio gogomio stream info

# Snapshots
docker-compose exec gogomio gogomio snapshot save /tmp/frame.jpg

# Inside container
docker-compose exec gogomio bash
gogomio status
gogomio config get fps
```

## Environment Configuration

Configure the CLI target server via `GOGOMIO_URL` environment variable:

```bash
# Default: localhost:8000
GOGOMIO_URL=http://localhost:8000 ./gogomio status

# Remote server
GOGOMIO_URL=http://192.168.1.100:8000 ./gogomio status

# Custom port
GOGOMIO_URL=http://localhost:9000 ./gogomio status
```

## Command Reference

### Status Command
```bash
./gogomio status
```
Shows current streaming status, FPS, resolution, uptime, and JPEG quality.

### Config Commands
```bash
./gogomio config                    # Show all configuration
./gogomio config get                # Show all configuration
./gogomio config get fps            # Show specific value (fps, resolution, etc.)
```

### Health Commands
```bash
./gogomio health check              # Quick health status
./gogomio health detailed           # Comprehensive health report
```

### Stream Commands
```bash
./gogomio stream info               # Current stream metrics
./gogomio stream stop               # Stop all active streams
```

### Diagnostics Command
```bash
./gogomio diagnostics               # System diagnostics including version, uptime, goroutines, memory
```

### Snapshot Commands
```bash
./gogomio snapshot capture          # Capture frame and write to stdout
./gogomio snapshot save <path>      # Capture frame and save to file
```

### Settings Commands
```bash
./gogomio settings get              # List all persistent settings
./gogomio settings get <key>        # Get specific setting value
./gogomio settings set <key>=<value>  # Update setting value
```

### Version Command
```bash
./gogomio version                   # Display version and build information
```

## Error Handling

The CLI provides helpful error messages when issues occur:

```bash
# Server not running
$ ./gogomio status
Error: failed to connect to server at http://localhost:8000: connection refused

# Invalid configuration value
$ ./gogomio config get invalid_key
Error: unknown config key: invalid_key
```

## Integration Examples

### Health Monitoring Script
```bash
#!/bin/bash
# Monitor health every 10 seconds
while true; do
    ./gogomio health check
    sleep 10
done
```

### Automated Snapshots
```bash
#!/bin/bash
# Capture snapshots every minute
while true; do
    timestamp=$(date +%Y%m%d_%H%M%S)
    ./gogomio snapshot save "/tmp/snapshots/frame_$timestamp.jpg"
    sleep 60
done
```

### Performance Monitoring
```bash
#!/bin/bash
# Monitor streaming metrics
./gogomio stream info
./gogomio diagnostics
```

### Docker Compose Health Check
```bash
#!/bin/bash
# Run health check in Docker Compose
docker-compose exec gogomio gogomio health check && echo "✓ Healthy" || echo "✗ Unhealthy"
```

## Development

### Building from Source
```bash
# Build CLI-enabled binary
go build -o gogomio ./cmd/gogomio

# Test server mode
./gogomio

# In another terminal, test CLI
./gogomio status
```

### Running Tests
```bash
# Run all CLI tests
go test ./internal/cli -v

# Run specific test
go test ./internal/cli -v -run TestStatusCommand
```

## Architecture

The CLI uses HTTP to communicate with the running server:

- **Default endpoint:** `http://localhost:8000`
- **Configurable via:** `GOGOMIO_URL` environment variable
- **Communication:** Plain HTTP (no authentication required for local use)
- **Timeout:** 5 seconds per request

## Troubleshooting

### "Server not running" error
```bash
# Make sure server is started
./gogomio server &

# Check if listening
netstat -tlnp | grep 8000
curl http://localhost:8000/health
```

### Slow responses
- Check server load: `./gogomio diagnostics`
- Check goroutines: `./gogomio diagnostics`
- Review memory usage: `./gogomio diagnostics`

### Docker permission issues
```bash
# Ensure gogomio user has video device access
docker-compose exec gogomio ls -l /dev/video0
docker-compose exec gogomio groups gogomio
```

## See Also

- [README.md](../../README.md) - Main project documentation
- [Dockerfile](../Dockerfile) - Docker build configuration
- [docker-compose.yml](../docker-compose.yml) - Compose configuration with CLI examples
