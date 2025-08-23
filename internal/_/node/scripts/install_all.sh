#!/bin/bash
# Master script to install all components

set -euo pipefail

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

# Arguments (versions)
FIRECRACKER_VERSION="${1:-latest}"
CONTAINERD_VERSION="${2:-latest}"
RUNC_VERSION="${3:-latest}"
CNI_VERSION="${4:-latest}"
GO_VERSION="${5:-1.23.0}"

# Main installation
main() {
    log_info "Starting complete installation..."
    echo ""
    
    # Install basic dependencies
    log_info "Step 1/7: Installing basic dependencies..."
    "${SCRIPT_DIR}/install_dependencies.sh"
    echo ""
    
    # Install Firecracker
    log_info "Step 2/7: Installing Firecracker..."
    "${SCRIPT_DIR}/install_firecracker.sh" "$FIRECRACKER_VERSION"
    echo ""
    
    # Install containerd
    log_info "Step 3/7: Installing containerd..."
    "${SCRIPT_DIR}/install_containerd.sh" "$CONTAINERD_VERSION"
    echo ""
    
    # Install runc
    log_info "Step 4/7: Installing runc..."
    "${SCRIPT_DIR}/install_runc.sh" "$RUNC_VERSION"
    echo ""
    
    # Install CNI plugins
    log_info "Step 5/7: Installing CNI plugins..."
    "${SCRIPT_DIR}/install_cni_plugins.sh" "$CNI_VERSION"
    echo ""
    
    # Install Go (for building firecracker-containerd)
    log_info "Step 6/7: Installing Go..."
    "${SCRIPT_DIR}/install_go.sh" "$GO_VERSION"
    echo ""
    
    # Build firecracker-containerd (optional)
    log_info "Step 7/7: Setting up firecracker-containerd..."
    if ! command_exists firecracker-containerd; then
        log_info "Building firecracker-containerd (this may take a while)..."
        
        # Export Go path
        export PATH=$PATH:/usr/local/go/bin
        
        # Clone and build
        cd /tmp
        if [[ ! -d "firecracker-containerd" ]]; then
            git clone https://github.com/firecracker-microvm/firecracker-containerd.git
        fi
        
        cd firecracker-containerd
        if make all 2>/dev/null; then
            sudo make install 2>/dev/null || true
            log_success "firecracker-containerd built and installed"
        else
            log_warning "firecracker-containerd build failed (optional component)"
        fi
    else
        log_success "firecracker-containerd already installed"
    fi
    
    echo ""
    log_success "All components installed successfully!"
    echo ""
    
    # Summary
    log_info "Installation Summary:"
    echo "  - Firecracker: $(firecracker --version 2>&1 | head -1 || echo 'not found')"
    echo "  - containerd:  $(containerd --version 2>&1 | head -1 || echo 'not found')"
    echo "  - runc:        $(runc --version 2>&1 | head -1 || echo 'not found')"
    echo "  - CNI plugins: $(ls /opt/cni/bin 2>/dev/null | wc -l || echo '0') plugins installed"
    echo "  - Go:          $(go version 2>&1 || echo 'not found')"
}

main "$@"
