#!/bin/bash
# Install runc OCI runtime

set -euo pipefail

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

# Arguments
VERSION="${1:-latest}"
REPO="opencontainers/runc"
FALLBACK_VERSION="v1.1.14"

# Check for root/sudo
SUDO=$(check_root)

# Main installation
main() {
    log_info "Checking runc..."
    
    # Check if already installed
    if command_exists runc; then
        local current_version
        current_version=$(runc --version 2>&1 | head -1 || echo "unknown")
        log_success "runc already installed: ${current_version}"
        exit 0
    fi
    
    log_info "Installing runc..."
    
    # Resolve version
    if [[ "$VERSION" == "latest" ]]; then
        log_info "Fetching latest version for ${REPO}..."
        VERSION=$(get_latest_version "$REPO" "$FALLBACK_VERSION")
    fi
    
    VERSION=$(normalize_version "$VERSION" true)
    log_info "Using runc version: ${VERSION}"
    
    # Download and install
    cd /tmp
    
    local download_url="https://github.com/${REPO}/releases/download/${VERSION}/runc.amd64"
    log_info "Downloading from: ${download_url}"
    
    if ! curl -fsSL "$download_url" -o runc; then
        log_error "Failed to download runc ${VERSION}"
        exit 1
    fi
    
    # Install binary
    $SUDO chmod +x runc
    
    if ! $SUDO mv runc /usr/local/bin/; then
        log_error "Failed to install runc binary"
        exit 1
    fi
    
    log_success "runc ${VERSION} installed successfully"
    
    # Verify installation
    if command_exists runc; then
        runc --version 2>&1 | head -1 || true
    fi
}

main "$@"
