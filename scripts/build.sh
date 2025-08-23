#!/bin/bash
set -e

# Build script for Zeitwork platform services
# Builds all services as static binaries for deployment

echo "Building Zeitwork platform services..."

# Set build output directory
BUILD_DIR="build"
mkdir -p $BUILD_DIR

# Build flags for static binaries
BUILD_FLAGS="-a -installsuffix cgo"
LDFLAGS="-s -w"

# Build operator
echo "Building operator..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    $BUILD_FLAGS \
    -ldflags "$LDFLAGS" \
    -o $BUILD_DIR/zeitwork-operator \
    ./cmd/operator

# Build node-agent
echo "Building node-agent..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    $BUILD_FLAGS \
    -ldflags "$LDFLAGS" \
    -o $BUILD_DIR/zeitwork-node-agent \
    ./cmd/node-agent

# Build load-balancer
echo "Building load-balancer..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    $BUILD_FLAGS \
    -ldflags "$LDFLAGS" \
    -o $BUILD_DIR/zeitwork-load-balancer \
    ./cmd/load-balancer

# Build edge-proxy
echo "Building edge-proxy..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    $BUILD_FLAGS \
    -ldflags "$LDFLAGS" \
    -o $BUILD_DIR/zeitwork-edge-proxy \
    ./cmd/edge-proxy

echo "Build complete! Binaries are in $BUILD_DIR/"
ls -lh $BUILD_DIR/
