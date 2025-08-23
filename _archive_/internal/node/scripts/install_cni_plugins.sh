#!/bin/bash
# Install CNI plugins for container networking

set -euo pipefail

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

# Arguments
VERSION="${1:-latest}"
REPO="containernetworking/plugins"
FALLBACK_VERSION="v1.5.1"

# Check for root/sudo
SUDO=$(check_root)

# Main installation
main() {
    log_info "Checking CNI plugins..."
    
    # Check if already installed
    if [[ -d "/opt/cni/bin" ]] && [[ -n "$(ls -A /opt/cni/bin 2>/dev/null)" ]]; then
        log_success "CNI plugins already installed in /opt/cni/bin"
        exit 0
    fi
    
    log_info "Installing CNI plugins..."
    
    # Resolve version
    if [[ "$VERSION" == "latest" ]]; then
        log_info "Fetching latest version for ${REPO}..."
        VERSION=$(get_latest_version "$REPO" "$FALLBACK_VERSION")
    fi
    
    VERSION=$(normalize_version "$VERSION" true)
    log_info "Using CNI plugins version: ${VERSION}"
    
    # Create directory
    $SUDO mkdir -p /opt/cni/bin
    
    # Download and install
    cd /tmp
    
    local download_url="https://github.com/${REPO}/releases/download/${VERSION}/cni-plugins-linux-amd64-${VERSION}.tgz"
    log_info "Downloading from: ${download_url}"
    
    if ! curl -fsSL "$download_url" | $SUDO tar -xz -C /opt/cni/bin; then
        log_error "Failed to download CNI plugins ${VERSION}"
        exit 1
    fi
    
    log_success "CNI plugins ${VERSION} installed successfully"
    
    # List installed plugins
    log_info "Installed CNI plugins:"
    ls -la /opt/cni/bin/ | head -10 || true
}

main "$@"
