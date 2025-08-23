#!/bin/bash

# Script to download and setup Firecracker kernel
# Usage: download_kernel.sh <kernel_dir>

set -e

KERNEL_DIR="${1:-/var/lib/firecracker/kernels}"
KERNEL_VERSION="5.10.186"

# Logging functions
log_info() {
    echo "[INFO] $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_error() {
    echo "[ERROR] $(date '+%Y-%m-%d %H:%M:%S') - $1" >&2
}

# Main function
main() {
    log_info "Setting up Firecracker kernel in $KERNEL_DIR"
    
    # Create kernel directory
    mkdir -p "$KERNEL_DIR"
    
    # Check if kernel already exists
    if [ -f "$KERNEL_DIR/vmlinux" ]; then
        log_info "Kernel already exists at $KERNEL_DIR/vmlinux"
        exit 0
    fi
    
    # Try multiple sources for the kernel
    KERNEL_URLS=(
        "https://github.com/firecracker-microvm/firecracker/releases/download/v1.4.0/firecracker-v1.4.0-x86_64.tgz"
        "https://s3.amazonaws.com/spec.ccfc.min/firecracker-kernels/vmlinux-${KERNEL_VERSION}.bin"
        "https://github.com/firecracker-microvm/firecracker/releases/download/v1.3.3/firecracker-v1.3.3-x86_64.tgz"
    )
    
    DOWNLOAD_SUCCESS=false
    
    # Method 1: Try downloading from Firecracker releases (includes kernel)
    log_info "Attempting to download Firecracker release with kernel..."
    
    TEMP_DIR=$(mktemp -d)
    cd "$TEMP_DIR"
    
    # Method 1: Try the official Firecracker quickstart kernel (most reliable)
    log_info "Downloading official Firecracker quickstart kernel..."
    QUICKSTART_KERNEL="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
    
    if curl -fsSL -o "$KERNEL_DIR/vmlinux" "$QUICKSTART_KERNEL" 2>/dev/null; then
        # Verify the download
        if [ -f "$KERNEL_DIR/vmlinux" ] && [ -s "$KERNEL_DIR/vmlinux" ]; then
            DOWNLOAD_SUCCESS=true
            log_info "Successfully downloaded Firecracker quickstart kernel"
        else
            log_error "Downloaded file is empty or invalid"
            rm -f "$KERNEL_DIR/vmlinux"
        fi
    else
        log_error "Failed to download quickstart kernel from S3"
    fi
    
    # Method 2: Try alternative quickstart kernels (fallback)
    if [ "$DOWNLOAD_SUCCESS" = false ]; then
        log_info "Trying alternative kernel sources..."
        
        # Alternative S3 locations for quickstart kernels
        ALTERNATIVE_KERNELS=(
            "https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
            "https://s3.amazonaws.com/spec.ccfc.min/ci-artifacts/kernels/x86_64/vmlinux-5.10.bin"
            "https://github.com/firecracker-microvm/firecracker/releases/download/v1.0.0/vmlinux.bin"
        )
        
        for KERNEL_URL in "${ALTERNATIVE_KERNELS[@]}"; do
            log_info "Trying: $KERNEL_URL"
            
            if curl -fsSL -o "$KERNEL_DIR/vmlinux" "$KERNEL_URL" 2>/dev/null; then
                if [ -f "$KERNEL_DIR/vmlinux" ] && [ -s "$KERNEL_DIR/vmlinux" ]; then
                    DOWNLOAD_SUCCESS=true
                    log_info "Successfully downloaded kernel from alternative source"
                    break
                else
                    rm -f "$KERNEL_DIR/vmlinux"
                fi
            fi
        done
    fi
    
    # Clean up temp directory
    cd /
    rm -rf "$TEMP_DIR"
    
    if [ "$DOWNLOAD_SUCCESS" = true ]; then
        # Verify the kernel file
        if [ -f "$KERNEL_DIR/vmlinux" ]; then
            SIZE=$(stat -c%s "$KERNEL_DIR/vmlinux" 2>/dev/null || stat -f%z "$KERNEL_DIR/vmlinux" 2>/dev/null || echo "0")
            SIZE_MB=$((SIZE / 1024 / 1024))
            
            if [ "$SIZE" -gt 0 ]; then
                log_info "Kernel successfully installed at $KERNEL_DIR/vmlinux"
                log_info "Kernel size: ${SIZE_MB}MB"
                
                # Set proper permissions
                chmod 644 "$KERNEL_DIR/vmlinux"
                
                exit 0
            else
                log_error "Downloaded kernel file is empty"
                rm -f "$KERNEL_DIR/vmlinux"
                exit 1
            fi
        fi
    fi
    
    log_error "Failed to download kernel from any source"
    log_error "Please manually download a Firecracker-compatible kernel and place it at $KERNEL_DIR/vmlinux"
    log_error "You can find kernels at: https://github.com/firecracker-microvm/firecracker/releases"
    exit 1
}

# Run main function
main "$@"
