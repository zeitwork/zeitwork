#!/bin/bash

# Build script for Docker images from GitHub repositories
# Usage: build_image.sh <image_id> <github_repo> <ref> <build_dir> <image_file>

set -e

# Arguments
IMAGE_ID="$1"
GITHUB_REPO="$2"
REF="$3"
BUILD_DIR="$4"
IMAGE_FILE="$5"

# Parse GitHub repo (format: owner/repo)
IFS='/' read -r OWNER REPO <<< "$GITHUB_REPO"

# Logging functions
log_info() {
    echo "[INFO] $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_error() {
    echo "[ERROR] $(date '+%Y-%m-%d %H:%M:%S') - $1" >&2
}

log_step() {
    echo ""
    echo "============================================"
    echo "[STEP] $1"
    echo "============================================"
}

# Main build process
main() {
    log_info "Starting build for image $IMAGE_ID"
    log_info "Repository: https://github.com/$OWNER/$REPO"
    log_info "Reference: $REF"
    log_info "Build directory: $BUILD_DIR"
    log_info "Output file: $IMAGE_FILE"
    
    # Step 1: Cleanup and prepare
    log_step "1/6: Preparing build environment"
    log_info "Cleaning up previous build directory..."
    rm -rf "$BUILD_DIR"
    mkdir -p "$BUILD_DIR"
    mkdir -p "$(dirname "$IMAGE_FILE")"
    
    # Step 2: Clone repository
    log_step "2/6: Cloning repository"
    log_info "Cloning from https://github.com/$OWNER/$REPO.git"
    
    # Clone directly into the build directory
    if ! git clone "https://github.com/$OWNER/$REPO.git" "$BUILD_DIR" 2>&1; then
        log_error "Failed to clone repository"
        log_error "Please verify:"
        log_error "  - Repository exists: https://github.com/$OWNER/$REPO"
        log_error "  - Repository is public or credentials are configured"
        log_error "  - Network connectivity to GitHub"
        exit 128
    fi
    
    log_info "Repository cloned successfully to $BUILD_DIR"
    
    # Change to build directory
    cd "$BUILD_DIR"
    log_info "Changed to build directory: $(pwd)"
    
    # Checkout specific ref if not main
    if [ "$REF" != "main" ] && [ "$REF" != "master" ]; then
        log_info "Checking out ref: $REF"
        if ! git checkout "$REF" 2>&1; then
            log_info "Warning: Could not checkout $REF, using default branch"
        fi
    fi
    
    # Step 3: Check for Dockerfile
    log_step "3/6: Checking for Dockerfile"
    
    log_info "Current directory: $(pwd)"
    log_info "Repository contents:"
    ls -la | head -20
    
    # Check for Dockerfile (case-insensitive)
    if [ -f Dockerfile ]; then
        log_info "Found Dockerfile"
    elif [ -f dockerfile ]; then
        log_info "Found dockerfile (lowercase), renaming to Dockerfile"
        mv dockerfile Dockerfile
    else
        log_error "No Dockerfile found in repository root"
        log_error "Current directory: $(pwd)"
        log_error "Repository contents:"
        ls -la
        log_error "A Dockerfile is required to build the image"
        exit 1
    fi
    
    # Show first few lines of Dockerfile for debugging
    log_info "Dockerfile preview:"
    head -n 10 Dockerfile | sed 's/^/  > /'
    
    # Step 4: Verify Docker installation
    log_step "4/6: Verifying Docker installation"
    
    if ! command -v docker &> /dev/null; then
        log_info "Docker not found, installing..."
        if ! curl -fsSL https://get.docker.com | sh; then
            log_error "Failed to install Docker"
            exit 1
        fi
        systemctl start docker || true
    fi
    
    # Check Docker daemon
    log_info "Checking Docker daemon status..."
    if ! docker info &> /dev/null; then
        log_info "Starting Docker daemon..."
        systemctl start docker || service docker start || dockerd &
        sleep 5
        
        if ! docker info &> /dev/null; then
            log_error "Docker daemon is not running"
            log_error "Please ensure Docker is properly installed and can be started"
            exit 1
        fi
    fi
    
    log_info "Docker is ready"
    docker version | head -n 1
    
    # Step 5: Build Docker image
    log_step "5/6: Building Docker image"
    
    IMAGE_TAG="${IMAGE_ID}:latest"
    log_info "Building image with tag: $IMAGE_TAG"
    
    # Build with detailed output
    if ! docker build -t "$IMAGE_TAG" . 2>&1; then
        log_error "Docker build failed"
        log_error "Please check the Dockerfile and build logs above"
        exit 1
    fi
    
    log_info "Docker image built successfully"
    
    # Show image info
    docker images "$IMAGE_TAG" --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
    
    # Step 6: Export image
    log_step "6/6: Exporting Docker image"
    
    log_info "Exporting to: $IMAGE_FILE"
    if ! docker save "$IMAGE_TAG" -o "$IMAGE_FILE" 2>&1; then
        log_error "Failed to export Docker image"
        exit 1
    fi
    
    # Cleanup Docker image
    log_info "Cleaning up Docker image..."
    docker rmi "$IMAGE_TAG" 2>/dev/null || true
    
    # Verify export
    if [ ! -f "$IMAGE_FILE" ]; then
        log_error "Image file was not created"
        exit 1
    fi
    
    # Get file size
    SIZE=$(stat -c%s "$IMAGE_FILE" 2>/dev/null || stat -f%z "$IMAGE_FILE" 2>/dev/null || echo "0")
    SIZE_MB=$((SIZE / 1024 / 1024))
    
    log_info "Export completed successfully"
    log_info "Image file: $IMAGE_FILE"
    log_info "Image size: ${SIZE_MB}MB ($SIZE bytes)"
    
    log_step "Build completed successfully!"
}

# Run main function
main "$@"
