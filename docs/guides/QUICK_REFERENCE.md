# 📋 Quick Reference Card

## Docker Hub Deployment Commands

### Pull & Deploy

```bash
# Pull latest image
docker pull docker.io/cyanautomation/gogomio:latest

# Start with docker-compose (mock camera)
docker-compose -f docker-compose.mock.yml up -d

# Start with docker-compose (real camera on Raspberry Pi)
docker-compose up -d

# Start with docker run (mock camera)
docker run -p 8000:8000 -e MOCK_CAMERA=true \
  docker.io/cyanautomation/gogomio:latest

# Start with docker run (real camera)
docker run -p 8000:8000 --device /dev/video0 \
  docker.io/cyanautomation/gogomio:latest
```

## Access Application

| Use Case | URL |
|----------|-----|
| Local dev | <http://localhost:8000> |
| Raspberry Pi | <http://raspberrypi.local:8000> |
| IP address | <http://192.168.1.50:8000> |
| Remote (tunnel) | <http://localhost:8000> |

## Docker Compose Commands

```bash
# Start (detached)
docker-compose up -d

# View logs
docker-compose logs -f

# Stop
docker-compose down

# Restart
docker-compose restart

# Check status
docker-compose ps
```

## API Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/` | GET | Web UI |
| `/health` | GET | Health check |
| `/ready` | GET | Readiness probe |
| `/stream.mjpg` | GET | MJPEG stream |
| `/snapshot.jpg` | GET | Latest frame |
| `/api/config` | GET | Configuration & stats |
| `/api/status` | GET | Server status |
| `/api/settings` | GET/POST/PUT | Manage settings |
| `/docs/index.html` | GET | **Swagger API documentation** |

## Example API Calls

```bash
# Get server health
curl http://localhost:8000/health | jq

# Get configuration
curl http://localhost:8000/api/config | jq

# Save settings
curl -X POST http://localhost:8000/api/settings \
  -H "Content-Type: application/json" \
  -d '{"brightness": 150, "contrast": 120}'

# Get settings
curl http://localhost:8000/api/settings | jq

# Download snapshot
curl http://localhost:8000/snapshot.jpg -o frame.jpg
```

## Environment Variables

```yaml
# Resolution
MIO_RESOLUTION: "640x480"          # WIDTHxHEIGHT

# Performance
MIO_FPS: 24                         # Frames per second
MIO_JPEG_QUALITY: 90               # JPEG quality (1-100)

# Limits
MIO_MAX_STREAM_CONNECTIONS: 10     # Max simultaneous streams

# Network
MIO_PORT: 8000                      # HTTP port
MIO_BIND_HOST: "0.0.0.0"           # Bind address

# Camera
MOCK_CAMERA: "false"                # true=mock, false=real
```

## Configuration Examples

### Development (High Speed)

```yaml
MIO_RESOLUTION: "1280x720"
MIO_FPS: 30
MIO_JPEG_QUALITY: 85
MOCK_CAMERA: "true"
```

### Raspberry Pi (Balanced)

```yaml
MIO_RESOLUTION: "640x480"
MIO_FPS: 24
MIO_JPEG_QUALITY: 90
MOCK_CAMERA: "false"
```

### Raspberry Pi Zero (Low Res)

```yaml
MIO_RESOLUTION: "320x240"
MIO_FPS: 12
MIO_JPEG_QUALITY: 75
```

### Server (High Quality)

```yaml
MIO_RESOLUTION: "1920x1080"
MIO_FPS: 30
MIO_JPEG_QUALITY: 95
MIO_MAX_STREAM_CONNECTIONS: 20
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Container won't start | Check logs: `docker-compose logs -f` |
| Port already in use | Change port in docker-compose.yml |
| Camera not detected | Verify `/dev/video0` exists |
| Slow performance | Lower resolution/FPS in environment |
| Memory issues | Reduce max connections / resolution |
| No web UI | Check firewall, verify port exposed |

## Docker Commands

```bash
# See running containers
docker ps

# View all images
docker images | grep gogomio

# Remove image
docker rmi cyanautomation/gogomio:latest

# Stop specific container
docker stop gogomio-gogomio-1

# View resource usage
docker stats
```

## File Locations

- **Web UI**: <http://localhost:8000/>
- **Settings file**: /tmp/gogomio/settings.json (in container)
- **Logs**: Check `docker-compose logs`
- **Docker compose**: docker-compose.yml, docker-compose.mock.yml
- **MIO sprite migration reference**: ../architecture/MIO_SPRITE_MIGRATION.md

## Repository Information

| Item | Value |
|------|-------|
| **Repository** | docker.io/cyanautomation/gogomio |
| **GitHub** | <https://github.com/CyanAutomation/gogomio> |
| **Docker Hub** | <https://hub.docker.com/r/cyanautomation/gogomio> |
| **Platforms** | amd64, arm64 |
| **Status** | Production Ready |

## One-Liners

```bash
# Quick test with mock camera
docker run -p 8000:8000 -e MOCK_CAMERA=true docker.io/cyanautomation/gogomio:latest

# Deploy to Raspberry Pi
ssh pi@raspberrypi 'cd gogomio && docker-compose up -d'

# View live stream
curl http://localhost:8000/stream.mjpg | ffplay -

# Monitor performance
docker stats gogomio-gogomio-1 --no-stream

# Backup settings
docker exec gogomio-gogomio-1 cat /tmp/gogomio/settings.json > settings.json
```

## Performance Metrics

| Metric | Value |
|--------|-------|
| **Image pull** | 30-60 seconds |
| **Container start** | <2 seconds |
| **App startup** | <1 second |
| **Memory (idle)** | 20-30MB |
| **Memory (streaming)** | 30-50MB |
| **CPU (streaming)** | 2-5% per stream |
| **Frame latency** | 40-100ms |

## Web UI Features

| Feature | Description |
|---------|-------------|
| **Live Stream** | Real-time MJPEG display with auto-reconnect |
| **Brightness** | 0-200% slider with real-time display |
| **Contrast** | 0-200% slider with real-time display |
| **Saturation** | 0-200% slider with real-time display |
| **Save Settings** | Persist preferences via API |
| **Statistics** | Live FPS, resolution, quality, connections |
| **Status** | Connection indicator (green/red) |
| **Mobile** | Fully responsive design |

## Security Notes

- ⚠️ No authentication enabled by default
- ⚠️ Use behind firewall for public networks
- 💡 Consider reverse proxy for HTTPS
- 💡 Use VPN or SSH tunnel for remote access

---

**Last Updated**: April 14, 2026  
**Version**: Phase 3 (Production Ready)  
**Status**: ✅ All systems operational
