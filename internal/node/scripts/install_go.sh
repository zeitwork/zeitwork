#!/bin/bash
# Install Go programming language

set -euo pipefail

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

# Arguments
VERSION="${1:-1.23.0}"

# Check for root/sudo
SUDO=$(check_root)

# Main installation
main() {
    log_info "Checking Go..."
    
    # Check if already installed
    if command_exists go; then
        local current_version
        current_version=$(go version 2>&1 || echo "unknown")
        log_success "Go already installed: ${current_version}"
        exit 0
    fi
    
    log_info "Installing Go..."
    
    # Normalize version (remove 'v' prefix if present)
    VERSION=$(normalize_version "$VERSION" false)
    
    # Special handling for "latest"
    if [[ "$VERSION" == "latest" ]]; then
        VERSION="1.23.0"
        log_info "Using default Go version: ${VERSION}"
    else
        log_info "Using Go version: ${VERSION}"
    fi
    
    # Download and install
    cd /tmp
    
    local download_url="https://go.dev/dl/go${VERSION}.linux-amd64.tar.gz"
    log_info "Downloading from: ${download_url}"
    
    if ! curl -fsSL "$download_url" | $SUDO tar -xz -C /usr/local; then
        log_error "Failed to download Go ${VERSION}"
        exit 1
    fi
    
    # Add to PATH
    if ! grep -q "/usr/local/go/bin" ~/.bashrc 2>/dev/null; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
        log_info "Added Go to PATH in ~/.bashrc"
    fi
    
    # Export for current session
    export PATH=$PATH:/usr/local/go/bin
    
    log_success "Go ${VERSION} installed successfully"
    
    # Verify installation
    /usr/local/go/bin/go version || true
}

main "$@"
