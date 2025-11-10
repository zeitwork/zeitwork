# Reconciler Service Dockerfile
# This service manages VM lifecycle and deployments

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
RUN CGO_ENABLED=0 GOOS=linux go build -o reconciler ./cmd/reconciler

# Final stage
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    openssh-client

# Create non-root user
RUN addgroup -S zeitwork && adduser -S zeitwork -G zeitwork

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/reconciler .

# Copy cloud-init template
COPY --from=builder /app/internal/reconciler/templates/ ./templates/

# Switch to non-root user
USER zeitwork

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ps aux | grep -v grep | grep reconciler || exit 1

# Run the service
CMD ["./reconciler"]

