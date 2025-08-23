#!/bin/bash
# Install Firecracker microVM

set -euo pipefail

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

# Arguments
VERSION="${1:-latest}"
REPO="firecracker-microvm/firecracker"
FALLBACK_VERSION="v1.6.0"

# Check for root/sudo
SUDO=$(check_root)

# Main installation
main() {
    log_info "Checking Firecracker..."
    
    # Check if already installed
    if command_exists firecracker; then
        local current_version
        current_version=$(firecracker --version 2>&1 | head -1 || echo "unknown")
        log_success "Firecracker already installed: ${current_version}"
        exit 0
    fi
    
    log_info "Installing Firecracker..."
    
    # Resolve version
    if [[ "$VERSION" == "latest" ]]; then
        log_info "Fetching latest version for ${REPO}..."
        VERSION=$(get_latest_version "$REPO" "$FALLBACK_VERSION")
    fi
    
    VERSION=$(normalize_version "$VERSION" true)
    log_info "Using Firecracker version: ${VERSION}"
    
    # Download and install
    cd /tmp
    
    local download_url="https://github.com/${REPO}/releases/download/${VERSION}/firecracker-${VERSION}-x86_64.tgz"
    log_info "Downloading from: ${download_url}"
    
    if ! curl -fsSL "$download_url" | tar -xz; then
        log_error "Failed to download Firecracker ${VERSION}"
        exit 1
    fi
    
    # Install binary
    if ! $SUDO mv "release-${VERSION}-x86_64/firecracker-${VERSION}-x86_64" /usr/local/bin/firecracker; then
        log_error "Failed to install Firecracker binary"
        exit 1
    fi
    
    $SUDO chmod +x /usr/local/bin/firecracker
    
    # Cleanup
    rm -rf "release-${VERSION}-x86_64"
    
    log_success "Firecracker ${VERSION} installed successfully"
    
    # Verify installation
    if command_exists firecracker; then
        firecracker --version 2>&1 | head -1 || true
    fi
}

main "$@"
