# Multi-stage builder for gogomio - Motion In Ocean Go implementation
# Builds for both arm64 (Raspberry Pi) and amd64 (development/testing)

# Build arguments
ARG VERSION=0.1.0-dev
ARG PORT=8000
ARG INSTALL_FFMPEG=false
ARG BUILDER_BASE_IMAGE=golang:1.22-alpine3.19

# Stage 1: Build
FROM ${BUILDER_BASE_IMAGE} AS builder

# Build arguments are available in this stage
ARG VERSION
ARG INSTALL_FFMPEG

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies (cached until go.mod/go.sum change)
RUN go mod download

# Copy remaining source code
COPY . .

# Build the binary with version information
# Use go build with optimization flags
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /build/gogomio \
    ./cmd/gogomio

# Stage 2: Runtime
FROM ubuntu:24.04

# Build arguments for runtime stage
ARG VERSION
ARG INSTALL_FFMPEG
ARG PORT

# Image metadata labels
LABEL org.opencontainers.image.title="Motion In Ocean" \
      org.opencontainers.image.description="Motion detection and MJPEG streaming service for cameras" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.authors="CyanAutomation" \
      org.opencontainers.image.source="https://github.com/CyanAutomation/gogomio"

# Update package lists and install runtime dependencies
# Camera package selection is architecture-aware:
# - arm64: Add Raspberry Pi repository and install rpicam-apps (provides rpicam-vid/libcamera-vid)
# - non-arm64: install ffmpeg-only fallback
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates tzdata curl ffmpeg; \
    arch="$(dpkg --print-architecture)"; \
    if [ "${arch}" = "arm64" ]; then \
      echo "Adding Raspberry Pi repository for arm64 camera tools..."; \
      apt-get install -y --no-install-recommends wget gnupg; \
      mkdir -p /etc/apt/keyrings; \
      wget -qO - https://archive.raspberrypi.org/debian/raspberrypi.gpg.key | gpg --dearmor -o /etc/apt/keyrings/raspberrypi-archive-keyring.gpg 2>/dev/null || true; \
      echo "deb [signed-by=/etc/apt/keyrings/raspberrypi-archive-keyring.gpg] http://archive.raspberrypi.org/debian $(grep VERSION_CODENAME= /etc/os-release | cut -d= -f2) main" | tee /etc/apt/sources.list.d/raspberry.list; \
      apt-get update || true; \
      if apt-cache show rpicam-apps >/dev/null 2>&1; then \
        apt-get install -y --no-install-recommends rpicam-apps; \
      else \
        if apt-cache show libcamera-apps >/dev/null 2>&1; then \
          apt-get install -y --no-install-recommends libcamera-apps; \
        fi; \
      fi; \
      apt-get purge -y wget gnupg || true; \
    fi; \
    rm -rf /var/lib/apt/lists/* /etc/apt/sources.list.d/raspberry.list

# Create non-root user with explicit umask
RUN useradd -m -u 1001 -s /bin/bash gogomio && \
    usermod -a -G video gogomio && \
    mkdir -p /etc/profile.d && \
    echo "umask 0077" >> /etc/profile.d/gogomio.sh && \
    chmod 755 /etc/profile.d/gogomio.sh

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/gogomio /app/gogomio
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# Copy static assets (when available)
# COPY static/ /app/static/

# Set ownership
RUN chown -R gogomio:gogomio /app

# Set environment variables
ENV PORT=${PORT}

# Expose configured port
EXPOSE ${PORT}

# Configure graceful shutdown signal
STOPSIGNAL SIGTERM

# Health check endpoint
HEALTHCHECK --interval=10s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -fsS http://localhost:${PORT}/ready || exit 1

# Switch to non-root user
USER gogomio

# Run the application
ENTRYPOINT ["/app/docker-entrypoint.sh"]
