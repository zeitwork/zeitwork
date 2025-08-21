#!/bin/bash

# Diagnostic script to test kernel downloads from Firecracker releases
# This script will help identify why kernel downloads are failing

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_section() {
    echo ""
    echo "=========================================="
    echo "$1"
    echo "=========================================="
}

# Create a temporary directory for testing
TEMP_DIR=$(mktemp -d)
cd "$TEMP_DIR"
log_info "Working in temporary directory: $TEMP_DIR"

# Test downloading Firecracker releases and inspect their contents
test_firecracker_release() {
    local VERSION=$1
    log_section "Testing Firecracker $VERSION"
    
    local URL="https://github.com/firecracker-microvm/firecracker/releases/download/${VERSION}/firecracker-${VERSION}-x86_64.tgz"
    log_info "Download URL: $URL"
    
    # Try to download
    if curl -fsSL -o "firecracker-${VERSION}.tgz" "$URL" 2>/dev/null; then
        log_info "✓ Successfully downloaded firecracker-${VERSION}.tgz"
        
        # Check file size
        SIZE=$(ls -lh "firecracker-${VERSION}.tgz" | awk '{print $5}')
        log_info "Archive size: $SIZE"
        
        # Extract and inspect contents
        log_info "Extracting archive..."
        if tar -xzf "firecracker-${VERSION}.tgz" 2>/dev/null; then
            log_info "✓ Successfully extracted archive"
            
            # List all files in the extracted directory
            log_info "Archive contents:"
            find . -type f -name "*" | head -20
            
            # Look specifically for kernel files
            log_warn "Looking for kernel files (vmlinux*, kernel*):"
            KERNEL_FILES=$(find . -type f \( -name "vmlinux*" -o -name "kernel*" -o -name "*kernel*" \) 2>/dev/null || true)
            
            if [ -n "$KERNEL_FILES" ]; then
                log_info "✓ Found kernel files:"
                echo "$KERNEL_FILES"
                
                # Check file details
                for KERNEL in $KERNEL_FILES; do
                    if [ -f "$KERNEL" ]; then
                        FILE_SIZE=$(ls -lh "$KERNEL" | awk '{print $5}')
                        FILE_TYPE=$(file "$KERNEL" | cut -d: -f2)
                        log_info "  - $KERNEL (Size: $FILE_SIZE)"
                        log_info "    Type: $FILE_TYPE"
                    fi
                done
            else
                log_error "✗ No kernel files found in this release"
                
                # Show what directories exist
                log_warn "Directories in archive:"
                find . -type d | head -10
                
                # Show all files to understand structure
                log_warn "All files in archive (first 30):"
                find . -type f | head -30
            fi
        else
            log_error "✗ Failed to extract archive"
        fi
        
        # Clean up this version
        rm -rf release-* firecracker-${VERSION}.tgz
    else
        log_error "✗ Failed to download from $URL"
    fi
    
    echo ""
}

# Test alternative kernel sources
test_alternative_sources() {
    log_section "Testing Alternative Kernel Sources"
    
    # Test Firecracker demo kernel
    log_info "Testing firecracker-demo kernel..."
    if curl -fsSL -o test-kernel-demo "https://github.com/firecracker-microvm/firecracker-demo/raw/main/vmlinux" 2>/dev/null; then
        SIZE=$(ls -lh test-kernel-demo | awk '{print $5}')
        TYPE=$(file test-kernel-demo | cut -d: -f2)
        log_info "✓ Downloaded demo kernel (Size: $SIZE)"
        log_info "  Type: $TYPE"
        rm -f test-kernel-demo
    else
        log_error "✗ Failed to download demo kernel"
    fi
    
    # Test S3 bucket kernels
    log_info "Testing S3 bucket kernels..."
    S3_VERSIONS=("5.10.186" "5.10.176" "5.10.0" "5.4.0")
    for VERSION in "${S3_VERSIONS[@]}"; do
        URL="https://s3.amazonaws.com/spec.ccfc.min/firecracker-kernels/vmlinux-${VERSION}.bin"
        if curl -fsSL --head "$URL" 2>/dev/null | grep -q "200 OK"; then
            log_info "✓ Kernel $VERSION exists at S3"
        else
            log_warn "✗ Kernel $VERSION not found at S3"
        fi
    done
}

# Test downloading actual kernel files from a working source
download_working_kernel() {
    log_section "Attempting to Download a Working Kernel"
    
    # Try to get the Firecracker binary and extract kernel from examples
    log_info "Downloading Firecracker v1.4.0 (known to have examples)..."
    
    if curl -fsSL -o fc.tgz "https://github.com/firecracker-microvm/firecracker/releases/download/v1.4.0/firecracker-v1.4.0-x86_64.tgz"; then
        tar -xzf fc.tgz
        
        log_info "Looking for any bootable files..."
        find . -type f -size +1M -size -50M | while read -r file; do
            FILE_TYPE=$(file "$file" 2>/dev/null | head -1)
            if echo "$FILE_TYPE" | grep -qE "(Linux|kernel|boot|ELF|executable)"; then
                SIZE=$(ls -lh "$file" | awk '{print $5}')
                log_info "Potential kernel: $file (Size: $SIZE)"
                log_info "  Type: $FILE_TYPE"
            fi
        done
    fi
    
    # Try getting a kernel from the quickstart guide
    log_info ""
    log_info "Trying kernel from Firecracker quickstart..."
    QUICKSTART_KERNEL="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
    if curl -fsSL -o quickstart-kernel "$QUICKSTART_KERNEL" 2>/dev/null; then
        SIZE=$(ls -lh quickstart-kernel | awk '{print $5}')
        TYPE=$(file quickstart-kernel | cut -d: -f2)
        log_info "✓ Downloaded quickstart kernel (Size: $SIZE)"
        log_info "  Type: $TYPE"
        
        if [ -f quickstart-kernel ] && [ -s quickstart-kernel ]; then
            log_info "✓ This kernel appears valid and could be used!"
            log_info "  URL: $QUICKSTART_KERNEL"
        fi
    else
        log_error "✗ Failed to download quickstart kernel"
    fi
}

# Main execution
main() {
    log_section "Firecracker Kernel Download Diagnostic"
    log_info "This script will test various kernel download sources"
    
    # Test latest versions
    VERSIONS=("v1.12.1" "v1.12.0" "v1.11.0" "v1.10.1" "v1.9.1" "v1.4.0")
    
    for VERSION in "${VERSIONS[@]}"; do
        test_firecracker_release "$VERSION"
    done
    
    # Test alternative sources
    test_alternative_sources
    
    # Try to find a working kernel
    download_working_kernel
    
    # Summary
    log_section "Summary and Recommendations"
    
    log_info "Based on the tests above:"
    log_info "1. Firecracker releases may not include kernel files directly"
    log_info "2. Kernels need to be downloaded separately"
    log_info "3. The quickstart kernel from S3 appears to be the most reliable source"
    log_info ""
    log_info "Recommended kernel URL:"
    log_info "  https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
    
    # Cleanup
    cd /
    rm -rf "$TEMP_DIR"
    
    log_info ""
    log_info "Diagnostic complete. Temporary files cleaned up."
}

# Run the diagnostic
main "$@"
