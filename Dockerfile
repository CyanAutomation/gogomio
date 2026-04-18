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
# Note on CSI Camera Support:
# - For native Raspberry Pi CSI camera support, libcamera-apps with libcamera-vid binary is required
# - libcamera-apps is specific to Raspberry Pi OS and not available in standard Debian repositories
# - Current base: Debian Trixie (generic arm64 support)
# - Alternative: Use Raspberry Pi OS base image for full libcamera support
#   (requires removing multi-architecture support, build only for arm64)
# - Workaround: FFmpeg V4L2 fallback (limited compatibility with libcamera devices)
# - When libcamera-vid unavailable: Application gracefully falls back to mock camera with diagnostics
FROM debian:trixie

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
# Attempts to install libcamera-apps for CSI camera support (not in standard Debian repos)
# Falls back gracefully if unavailable; FFmpeg serves as V4L2 fallback
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates tzdata curl ffmpeg; \
    apt-get install -y --no-install-recommends libcamera-apps 2>/dev/null || echo "ℹ️  libcamera-apps not available in Debian repos; native CSI camera tools not installed"; \
    rm -rf /var/lib/apt/lists/*

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
