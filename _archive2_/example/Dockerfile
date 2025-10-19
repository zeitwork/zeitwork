# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod files if they exist (optional for this simple example)
COPY go.* ./
RUN go mod download || true

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o main .

# Runtime stage
FROM alpine:latest

# Add ca-certificates for HTTPS calls (best practice)
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/main .

# Expose the port the app runs on
EXPOSE 8080

# Run the binary
CMD ["./main"]

