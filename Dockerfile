# Multi-stage builder for gogomio - Motion In Ocean Go implementation
# Builds for both arm64 (Raspberry Pi) and amd64 (development/testing)

# Build arguments
ARG VERSION=0.1.0-dev
ARG PORT=8000
ARG INSTALL_FFMPEG=false
ARG BUILDER_BASE_IMAGE=golang:1.23-alpine3.19

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

# Install C toolchain required for -race (CGO) test builds
RUN apk add --no-cache gcc musl-dev

# Copy remaining source code
COPY . .

# Run tests to validate the build with structured logs and concise failure summaries
RUN set -eu;     mkdir -p /tmp/test-logs;     status=0;     for pkg in $(go list ./...); do       safe_pkg=$(echo "$pkg" | tr '/.' '__');       log_file="/tmp/test-logs/${safe_pkg}.jsonl";       echo "=== Testing package: ${pkg} ===";       if ! CGO_ENABLED=1 go test -race -json "$pkg" | tee "$log_file"; then         status=1;         echo "--- Failure summary for ${pkg} ---";         awk '
          BEGIN { pkg = ""; test = ""; msg = "" }
          /"Action":"fail"/ {
            if (match($0, /"Package":"[^"]+"/)) {
              pkg = substr($0, RSTART + 11, RLENGTH - 12)
            }
            if (match($0, /"Test":"[^"]+"/)) {
              test = substr($0, RSTART + 8, RLENGTH - 9)
            } else {
              test = "(package)"
            }
          }
          /"Action":"output"/ && /--- FAIL:/ {
            if (msg == "") {
              tmp = $0
              sub(/^.*"Output":"/, "", tmp)
              sub(/"[[:space:]]*}$/, "", tmp)
              gsub(/\\n/, "", tmp)
              msg = tmp
            }
          }
          END {
            if (pkg == "") pkg = "(unknown package)"
            if (msg == "") msg = "(failure message not captured; inspect full JSON log)"
            printf("package=%s test=%s message=%s\n", pkg, test, msg)
          }
        ' "$log_file";       fi;     done;     test "$status" -eq 0

# Install swag CLI and generate Swagger docs
RUN go install github.com/swaggo/swag/cmd/swag@latest && \
    /go/bin/swag init -g cmd/gogomio/main.go

# Build the binary with version information
# Use go build with optimization flags
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /build/gogomio \
    ./cmd/gogomio

# Stage 2: Runtime
# Raspberry Pi OS base for native CSI camera support
# Note: This image is optimized for arm64 Raspberry Pi deployment
# - Includes libcamera-apps with libcamera-vid for native CSI camera access
# - Includes Raspberry Pi-specific camera middleware
# - For multi-architecture builds, use a separate generic Dockerfile
FROM debian:bookworm

# Build arguments for runtime stage
ARG VERSION
ARG INSTALL_FFMPEG
ARG PORT

# Image metadata labels
LABEL org.opencontainers.image.title="Motion In Ocean" \
      org.opencontainers.image.description="Motion detection and MJPEG streaming service for cameras" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.authors="CyanAutomation" \
      org.opencontainers.image.source="https://github.com/CyanAutomation/gogomio" \
      org.opencontainers.image.arch="arm64"

# Setup Raspberry Pi repository and install libcamera-apps
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl gnupg wget tzdata; \
    \
    echo "Setting up Raspberry Pi repository for libcamera-apps..."; \
    mkdir -p /etc/apt/keyrings; \
    \
    wget -qO - https://archive.raspberrypi.org/debian/raspberrypi.gpg.key 2>/dev/null | \
    gpg --dearmor -o /etc/apt/keyrings/raspberrypi-archive-keyring.gpg 2>/dev/null || \
    (echo "⚠️  Failed to download Raspberry Pi GPG key; attempting fallback..."; \
     curl -fsSL https://archive.raspberrypi.org/debian/raspberrypi.gpg.key 2>/dev/null | \
     gpg --dearmor -o /etc/apt/keyrings/raspberrypi-archive-keyring.gpg 2>/dev/null || \
     echo "⚠️  GPG key fetch failed, continuing without signature verification"); \
    \
    printf "Types: deb deb-src\nURIs: http://archive.raspberrypi.org/debian\nSuites: bookworm\nComponents: main\nSigned-By: /etc/apt/keyrings/raspberrypi-archive-keyring.gpg\n" > /etc/apt/sources.list.d/raspi.sources; \
    \
    apt-get update 2>/dev/null || echo "⚠️  Raspberry Pi repository update had issues, continuing..."; \
    \
    echo "Installing camera support packages..."; \
    apt-get install -y --no-install-recommends ffmpeg || echo "⚠️  ffmpeg install failed"; \
    apt-get install -y --no-install-recommends libcamera-apps 2>/dev/null || \
    apt-get install -y --no-install-recommends rpicam-apps 2>/dev/null || \
    echo "⚠️  libcamera/rpicam packages not found in repository"; \
    \
    apt-get purge -y gnupg wget || true; \
    rm -rf /var/lib/apt/lists/* /etc/apt/sources.list.d/raspi.sources

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

# Two-Mode Execution:
# - Default (server mode): /app/gogomio → Starts HTTP server
# - CLI mode: /app/gogomio <command> → Executes CLI command
#
# Examples:
#   docker run gogomio:latest              # Starts HTTP server
#   docker run gogomio:latest gogomio status
#   docker-compose exec gogomio gogomio config get fps
#
# Run the application
ENTRYPOINT ["/app/docker-entrypoint.sh"]
