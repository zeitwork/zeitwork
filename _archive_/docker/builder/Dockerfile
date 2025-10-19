# Builder Service Dockerfile
# This service builds Docker images from Git repositories

FROM golang:1.25-alpine AS builder

# Set working directory
WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o builder ./cmd/builder

# Final stage
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache \
    git \
    docker-cli \
    ca-certificates

# Create non-root user
RUN addgroup -S zeitwork && adduser -S zeitwork -G zeitwork

# Add zeitwork user to docker group for Docker socket access
# Create docker group with GID 988 to match host's docker socket
RUN addgroup -g 988 docker && adduser zeitwork docker

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/builder .

# Create work directory with proper permissions
RUN mkdir -p /tmp/zeitwork-builder && chown -R zeitwork:zeitwork /tmp/zeitwork-builder

# Switch to non-root user
USER zeitwork

# Expose port (if needed for health checks)
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ps aux | grep -v grep | grep builder || exit 1

# Run the service
CMD ["./builder"]
