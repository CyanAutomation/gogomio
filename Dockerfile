# Multi-stage builder for gogomio - Motion In Ocean Go implementation
# Builds for both arm64 (Raspberry Pi) and amd64 (development/testing)

# Stage 1: Build
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# Use go build with optimization flags
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o /build/gogomio \
    ./cmd/gogomio

# Stage 2: Runtime
FROM alpine:3.19

# Install runtime dependencies (keep minimal for fast ARM64 builds)
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -u 1000 gogomio

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/gogomio /app/gogomio

# Copy static assets (when available)
# COPY static/ /app/static/

# Set ownership
RUN chown -R gogomio:gogomio /app

# Switch to non-root user
USER gogomio

# NOTE: For real camera support, ffmpeg must be installed on the host.
# On Raspberry Pi: apk add ffmpeg
# For development/testing: Use MOCK_CAMERA=true environment variable

# Default port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=10s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8000/ready || exit 1

# Run the application
ENTRYPOINT ["/app/gogomio"]
