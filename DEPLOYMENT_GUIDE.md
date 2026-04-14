# Deployment Guide

## Quick Start

Motion In Ocean is now deployed to Docker Hub and ready for production use on any platform.

### Docker Hub

**Image**: `docker.io/cyanautomation/gogomio:latest`

**Platforms Supported**:
- ✅ linux/amd64 (Intel/AMD processors)
- ✅ linux/arm64 (Raspberry Pi, ARM64 systems)

**Tags Available**:
- `latest` - Latest version (recommended)
- `phase3` - Phase 3 version (current version)

---

## 1. Docker Run (Quick Test)

### With Mock Camera (Development)
```bash
docker run -p 8000:8000 \
  -e MOCK_CAMERA=true \
  docker.io/cyanautomation/gogomio:latest
```

**Access**: http://localhost:8000/

### With Real Camera (Raspberry Pi)
```bash
docker run -p 8000:8000 \
  --device /dev/video0:/dev/video0 \
  docker.io/cyanautomation/gogomio:latest
```

**Access**: http://raspberrypi.local:8000/

---

## 2. Docker Compose (Recommended)

### For Development (Mock Camera)

```bash
# Start
docker-compose -f docker-compose.mock.yml up

# Detached mode
docker-compose -f docker-compose.mock.yml up -d

# Stop
docker-compose -f docker-compose.mock.yml down

# Logs
docker-compose -f docker-compose.mock.yml logs -f
```

**Resolution**: 1280x720 @ 30 FPS  
**Access**: http://localhost:8000/

### For Raspberry Pi (Real Camera)

```bash
# Start
docker-compose up

# Detached mode
docker-compose up -d

# Stop
docker-compose down

# Logs
docker-compose logs -f
```

**Resolution**: 640x480 @ 24 FPS  
**Access**: http://raspberrypi.local:8000/

### For Docker Hub Registry Pull

Both `docker-compose.yml` and `docker-compose.mock.yml` now pull from Docker Hub:
- No local build required
- Automatic multi-architecture selection
- Ensures consistency across deployments

---

## 3. Environment Configuration

All docker-compose files use environment variables. To customize, edit `docker-compose.yml`:

```yaml
environment:
  MIO_RESOLUTION: "640x480"        # Camera resolution
  MIO_FPS: 24                       # Target FPS
  MIO_JPEG_QUALITY: 90             # JPEG quality 1-100
  MIO_MAX_STREAM_CONNECTIONS: 10   # Max simultaneous streams
  MIO_PORT: 8000                   # HTTP port
  MOCK_CAMERA: "false"             # false = real camera, true = mock
```

---

## 4. Raspberry Pi Setup

### Prerequisites
- Raspberry Pi 3/4/5 with 64-bit OS
- Docker and Docker Compose installed
- Raspberry Pi Camera Module (CSI or USB)

### Installation

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker pi

# Install Docker Compose
sudo apt install -y docker-compose

# Clone repository (or just copy docker-compose.yml)
git clone https://github.com/CyanAutomation/gogomio
cd gogomio
```

### Running on Raspberry Pi

```bash
# Pull latest image (auto-selects arm64)
docker pull docker.io/cyanautomation/gogomio:latest

# Start with Docker Compose
docker-compose up -d

# View logs
docker-compose logs -f

# Stop
docker-compose down
```

### Access Web UI

From another device on the network:
```bash
# Replace raspberrypi with your Pi's hostname or IP
curl http://raspberrypi:8000/
firefox http://raspberrypi:8000/
```

---

## 5. Testing

### API Endpoints

```bash
# Health check
curl http://localhost:8000/health | jq

# Configuration
curl http://localhost:8000/api/config | jq

# Get snapshot
curl http://localhost:8000/snapshot.jpg -o frame.jpg

# Save settings
curl -X POST http://localhost:8000/api/settings \
  -H "Content-Type: application/json" \
  -d '{"brightness": 150, "contrast": 120}'

# Get settings
curl http://localhost:8000/api/settings | jq
```

### Web UI Features

1. **Live Stream Viewer**  
   - Real-time MJPEG display
   - Auto-reconnect on disconnect
   - Connection status indicator

2. **Settings Controls**  
   - Brightness, Contrast, Saturation sliders
   - Save/Reset buttons
   - Persistent across restarts

3. **Statistics Dashboard**  
   - FPS counter (updates every 2 seconds)
   - Camera resolution
   - JPEG quality
   - Active connection count

---

## 6. Network Access

### Local Network

```bash
# Device on same network
firefox http://raspberrypi.local:8000/
curl http://raspberrypi.local:8000/api/config
```

### Remote Access (SSH Tunnel)

```bash
# From your machine, tunnel to Pi
ssh -L 8000:localhost:8000 pi@raspberrypi.local

# Access locally
firefox http://localhost:8000/
```

### With Domain/Public IP

Use reverse proxy (nginx/Caddy) or expose via:
- Cloudflare Tunnel
- Tailscale
- ngrok

Example with ngrok:
```bash
# On Pi terminal
ssh raspberrypi 'docker exec gogomio-gogomio-1 ngrok http 8000'
```

---

## 7. Resource Allocation

### Default Limits (docker-compose.yml)
- **CPU**: 1 core (limit), 0.5 core (reservation)
- **Memory**: 256MB (limit), 128MB (reservation)

### Adjust for Your Pi

Edit `docker-compose.yml`:
```yaml
deploy:
  resources:
    limits:
      cpus: '1'           # Raspberry Pi 3: use '0.5'
      memory: 256M        # Reduce to 128M if needed
```

---

## 8. Troubleshooting

### Container won't start

```bash
# Check logs
docker-compose logs -f

# Check image pulled correctly
docker images | grep gogomio

# Verify Docker is running
docker ps
```

### Camera not detected

```bash
# List available camera devices
ls -la /dev/video*

# Check device permissions
getfacl /dev/video0

# Grant permissions if needed
sudo usermod -aG video docker
```

### Port already in use

```bash
# Kill process on port 8000
sudo fuser -k 8000/tcp

# Or use different port in docker-compose.yml
# Change "8000:8000" to "8001:8000"
```

### No connection to web UI

```bash
# Verify container is running
docker ps

# Check network connectivity
docker exec gogomio-gogomio-1 ping 8.8.8.8

# Verify exposed port
docker port gogomio-gogomio-1
```

---

## 9. Monitoring

### Check Status

```bash
# Container status
docker ps -a

# Resource usage
docker stats gogomio-gogomio-1

# System logs
journalctl -u docker -f
```

### Health Checks

```bash
# Health endpoint
curl -s http://localhost:8000/health | jq

# Readiness endpoint
curl -s http://localhost:8000/ready | jq

# Expected: {"status":"ok"} or {status:"ready"}
```

---

## 10. Production Deployment

### Using Docker Stack (Swarm)

```bash
docker stack deploy -c docker-compose.yml gogomio
```

### Using Kubernetes

Convert docker-compose to Kubernetes manifests:
```bash
kompose convert --volumes hostPath docker-compose.yml
kubectl apply -f *.yaml
```

### With Reverse Proxy (nginx)

```nginx
server {
    listen 80;
    server_name camera.example.com;

    location / {
        proxy_pass http://localhost:8000;
        proxy_http_version 1.1;
        proxy_set_header Connection "upgrade";
        proxy_set_header Upgrade $http_upgrade;
        proxy_buffering off;
    }
}
```

---

## 11. Logs and Debugging

### View Application Logs

```bash
# Live logs
docker-compose logs -f

# Last 100 lines
docker-compose logs --tail=100

# Export logs
docker-compose logs > gogomio.log
```

### Debug Mode

Add to docker-compose.yml:
```yaml
environment:
  # ... existing vars ...
  DEBUG: "true"  # (if implemented)
```

---

## 12. Updates

### Pull Latest Version

```bash
# Stop current container
docker-compose down

# Pull latest image
docker pull docker.io/cyanautomation/gogomio:latest

# Start updated version
docker-compose up -d
```

### Pin Specific Version

Edit `docker-compose.yml`:
```yaml
image: docker.io/cyanautomation/gogomio:phase3
```

---

## Quick Commands Reference

```bash
# Start (development)
docker-compose -f docker-compose.mock.yml up -d

# Start (Raspberry Pi)
docker-compose up -d

# Status
docker-compose ps

# Logs
docker-compose logs -f

# Stop
docker-compose down

# Restart
docker-compose restart

# Test API
curl http://localhost:8000/api/config | jq

# Access web UI
firefox http://localhost:8000/
```

---

## Support

- **GitHub**: https://github.com/CyanAutomation/gogomio
- **Docker Hub**: https://hub.docker.com/r/cyanautomation/gogomio
- **Issues**: https://github.com/CyanAutomation/gogomio/issues

---

**Status**: Production Ready ✅
