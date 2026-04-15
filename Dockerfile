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
FROM alpine:3.19

# Build arguments for runtime stage
ARG INSTALL_FFMPEG
ARG PORT

# Image metadata labels
LABEL org.opencontainers.image.title="Motion In Ocean" \
      org.opencontainers.image.description="Motion detection and MJPEG streaming service for cameras" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.authors="CyanAutomation" \
      org.opencontainers.image.source="https://github.com/CyanAutomation/gogomio" \
      org.opencontainers.image.created="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata curl

# Conditionally install ffmpeg for real camera support
RUN if [ "${INSTALL_FFMPEG}" = "true" ]; then \
      echo "Installing ffmpeg for real camera support..."; \
      apk add --no-cache ffmpeg; \
    fi

# Create non-root user with explicit umask
RUN adduser -D -u 1000 gogomio && \
    echo "umask 0077" >> /etc/profile.d/gogomio.sh && \
    chmod 755 /etc/profile.d/gogomio.sh

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/gogomio /app/gogomio

# Copy static assets (when available)
# COPY static/ /app/static/

# Set ownership
RUN chown -R gogomio:gogomio /app

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
ENTRYPOINT ["/app/gogomio"]
