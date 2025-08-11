# Multi-stage Docker build for secure, optimized Go applications
# Stage 1: Build stage
FROM golang:1.23-alpine AS builder

# Security: Create non-root user for build
RUN adduser -D -s /bin/sh -u 1001 appuser

# Install security tools and build dependencies
RUN apk add --no-cache \
    ca-certificates \
    git \
    make \
    gcc \
    musl-dev \
    && update-ca-certificates

# Set working directory
WORKDIR /build

# Copy dependency files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the applications with security flags
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o ad-api ./cmd/ad-api

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o ad-processor ./cmd/ad-processor

# Stage 2: Runtime stage for ad-api
FROM alpine:3.18 AS ad-api

# Security: Install security updates and minimal runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && update-ca-certificates \
    && rm -rf /var/cache/apk/*

# Security: Create non-root user
RUN adduser -D -s /bin/sh -u 1001 appuser

# Create necessary directories with proper permissions
RUN mkdir -p /app/configs /app/logs /app/tmp \
    && chown -R appuser:appuser /app

# Copy configuration files
COPY configs/ /app/configs/

# Copy binary from builder stage
COPY --from=builder /build/ad-api /app/ad-api

# Security: Set file permissions
RUN chmod +x /app/ad-api \
    && chown appuser:appuser /app/ad-api

# Switch to non-root user
USER appuser

# Set working directory
WORKDIR /app

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD /app/ad-api -health-check || exit 1

# Expose port (custom port for security)
EXPOSE 8443

# Set runtime labels
LABEL maintainer="Reforged Labs <team@reforgedlabs.com>"
LABEL version="1.0.0"
LABEL description="Ad Processing Queue API Service"
LABEL security.scan="enabled"

# Run the application
ENTRYPOINT ["/app/ad-api"]

# Stage 3: Runtime stage for ad-processor
FROM alpine:3.18 AS ad-processor

# Security: Install security updates and minimal runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && update-ca-certificates \
    && rm -rf /var/cache/apk/*

# Security: Create non-root user
RUN adduser -D -s /bin/sh -u 1001 appuser

# Create necessary directories with proper permissions
RUN mkdir -p /app/configs /app/logs /app/tmp \
    && chown -R appuser:appuser /app

# Copy configuration files
COPY configs/ /app/configs/

# Copy binary from builder stage
COPY --from=builder /build/ad-processor /app/ad-processor

# Security: Set file permissions
RUN chmod +x /app/ad-processor \
    && chown appuser:appuser /app/ad-processor

# Switch to non-root user
USER appuser

# Set working directory
WORKDIR /app

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD /app/ad-processor -health-check || exit 1

# Set runtime labels
LABEL maintainer="Reforged Labs <team@reforgedlabs.com>"
LABEL version="1.0.0"
LABEL description="Ad Processing Queue Worker Service"
LABEL security.scan="enabled"

# Run the application
ENTRYPOINT ["/app/ad-processor"]