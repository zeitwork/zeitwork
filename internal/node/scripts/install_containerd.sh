#!/bin/bash
# Install containerd container runtime

set -euo pipefail

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

# Arguments
VERSION="${1:-latest}"
REPO="containerd/containerd"
FALLBACK_VERSION="v2.0.0"

# Check for root/sudo
SUDO=$(check_root)

# Main installation
main() {
    log_info "Checking containerd..."
    
    # Check if already installed
    if command_exists containerd; then
        local current_version
        current_version=$(containerd --version 2>&1 | head -1 || echo "unknown")
        log_success "containerd already installed: ${current_version}"
        exit 0
    fi
    
    log_info "Installing containerd..."
    
    # Resolve version
    if [[ "$VERSION" == "latest" ]]; then
        log_info "Fetching latest version for ${REPO}..."
        VERSION=$(get_latest_version "$REPO" "$FALLBACK_VERSION")
    fi
    
    VERSION=$(normalize_version "$VERSION" true)
    local VERSION_NO_V=$(normalize_version "$VERSION" false)
    log_info "Using containerd version: ${VERSION}"
    
    # Download and install
    cd /tmp
    
    local download_url="https://github.com/${REPO}/releases/download/${VERSION}/containerd-${VERSION_NO_V}-linux-amd64.tar.gz"
    log_info "Downloading from: ${download_url}"
    
    if ! curl -fsSL "$download_url" | $SUDO tar -xz -C /usr/local; then
        log_error "Failed to download containerd ${VERSION}"
        exit 1
    fi
    
    # Setup systemd service if systemd is available
    if command_exists systemctl; then
        log_info "Setting up containerd systemd service..."
        $SUDO mkdir -p /usr/local/lib/systemd/system
        
        if curl -fsSL https://raw.githubusercontent.com/containerd/containerd/main/containerd.service | \
           $SUDO tee /usr/local/lib/systemd/system/containerd.service > /dev/null; then
            $SUDO systemctl daemon-reload
            $SUDO systemctl enable --now containerd || true
            log_success "containerd systemd service configured"
        else
            log_warning "Failed to setup systemd service, containerd installed but not started"
        fi
    else
        log_info "systemd not available, containerd installed but not started automatically"
    fi
    
    log_success "containerd ${VERSION} installed successfully"
    
    # Verify installation
    if command_exists containerd; then
        containerd --version 2>&1 | head -1 || true
    fi
}

main "$@"
