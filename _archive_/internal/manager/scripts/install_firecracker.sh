#!/bin/bash

# Script to install Firecracker on a node
# Usage: install_firecracker.sh

set -e

# Logging functions
log_info() {
    echo "[INFO] $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_error() {
    echo "[ERROR] $(date '+%Y-%m-%d %H:%M:%S') - $1" >&2
}

# Main installation function
install_firecracker() {
    log_info "Installing Firecracker..."
    
    # Check if Firecracker is already installed
    if command -v firecracker &> /dev/null; then
        CURRENT_VERSION=$(firecracker --version 2>&1 | head -1)
        log_info "Firecracker is already installed: $CURRENT_VERSION"
        return 0
    fi
    
    # Detect architecture
    ARCH=$(uname -m)
    if [ "$ARCH" != "x86_64" ] && [ "$ARCH" != "aarch64" ]; then
        log_error "Unsupported architecture: $ARCH"
        exit 1
    fi
    
    # Use the latest stable version
    FC_VERSION="v1.12.1"
    log_info "Installing Firecracker $FC_VERSION for $ARCH"
    
    # Create temporary directory
    TEMP_DIR=$(mktemp -d)
    cd "$TEMP_DIR"
    
    # Download Firecracker release
    DOWNLOAD_URL="https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-${ARCH}.tgz"
    log_info "Downloading from: $DOWNLOAD_URL"
    
    if ! curl -fsSL -o firecracker.tgz "$DOWNLOAD_URL"; then
        log_error "Failed to download Firecracker"
        cd /
        rm -rf "$TEMP_DIR"
        exit 1
    fi
    
    # Extract the archive
    log_info "Extracting Firecracker..."
    tar -xzf firecracker.tgz
    
    # Find the release directory
    RELEASE_DIR="release-${FC_VERSION}-${ARCH}"
    if [ ! -d "$RELEASE_DIR" ]; then
        log_error "Release directory not found: $RELEASE_DIR"
        ls -la
        cd /
        rm -rf "$TEMP_DIR"
        exit 1
    fi
    
    # Install Firecracker binaries
    log_info "Installing Firecracker binaries..."
    
    # Create installation directory
    INSTALL_DIR="/usr/local/bin"
    mkdir -p "$INSTALL_DIR"
    
    # Copy firecracker binary
    if [ -f "$RELEASE_DIR/firecracker-${FC_VERSION}-${ARCH}" ]; then
        cp "$RELEASE_DIR/firecracker-${FC_VERSION}-${ARCH}" "$INSTALL_DIR/firecracker"
        chmod +x "$INSTALL_DIR/firecracker"
        log_info "Installed firecracker to $INSTALL_DIR/firecracker"
    else
        log_error "Firecracker binary not found"
        ls -la "$RELEASE_DIR"
        cd /
        rm -rf "$TEMP_DIR"
        exit 1
    fi
    
    # Copy jailer binary (optional but recommended)
    if [ -f "$RELEASE_DIR/jailer-${FC_VERSION}-${ARCH}" ]; then
        cp "$RELEASE_DIR/jailer-${FC_VERSION}-${ARCH}" "$INSTALL_DIR/jailer"
        chmod +x "$INSTALL_DIR/jailer"
        log_info "Installed jailer to $INSTALL_DIR/jailer"
    fi
    
    # Clean up
    cd /
    rm -rf "$TEMP_DIR"
    
    # Verify installation
    if command -v firecracker &> /dev/null; then
        VERSION=$(firecracker --version 2>&1 | head -1)
        log_info "Firecracker installed successfully: $VERSION"
    else
        log_error "Firecracker installation verification failed"
        exit 1
    fi
    
    # Check KVM support
    log_info "Checking KVM support..."
    if [ -e /dev/kvm ]; then
        log_info "KVM is available"
        # Ensure proper permissions
        if [ -w /dev/kvm ]; then
            log_info "KVM is accessible"
        else
            log_info "Setting KVM permissions..."
            chmod 666 /dev/kvm 2>/dev/null || true
        fi
    else
        log_error "KVM is not available. Firecracker requires KVM support."
        log_error "Please ensure:"
        log_error "  1. Your CPU supports virtualization (Intel VT-x or AMD-V)"
        log_error "  2. Virtualization is enabled in BIOS"
        log_error "  3. KVM kernel modules are loaded (kvm_intel or kvm_amd)"
        exit 1
    fi
}

# Run installation
install_firecracker
