#!/bin/bash
# Install basic system dependencies

set -euo pipefail

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

# Check for root/sudo
SUDO=$(check_root)

# Update package list
update_packages() {
    log_info "Updating package list..."
    if $SUDO apt-get update > /dev/null 2>&1; then
        log_success "Package list updated"
    else
        log_warning "Failed to update package list"
    fi
}

# Install a package if not present
install_if_missing() {
    local package=$1
    local check_cmd=$2
    local install_cmd=$3
    
    log_info "Checking ${package}..."
    if eval "$check_cmd" > /dev/null 2>&1; then
        log_success "${package} already installed"
    else
        log_info "Installing ${package}..."
        if eval "$install_cmd" > /dev/null 2>&1; then
            log_success "${package} installed"
        else
            log_error "Failed to install ${package}"
            return 1
        fi
    fi
}

# Main installation
main() {
    log_info "Installing basic dependencies..."
    
    # Update package list first
    update_packages
    
    # Install basic dependencies
    install_if_missing "curl" \
        "command -v curl" \
        "$SUDO apt-get install -y curl"
    
    install_if_missing "git" \
        "command -v git" \
        "$SUDO apt-get install -y git"
    
    install_if_missing "build-essential" \
        "dpkg -l | grep build-essential" \
        "$SUDO apt-get install -y build-essential"
    
    install_if_missing "CPU checker" \
        "command -v kvm-ok" \
        "$SUDO apt-get install -y cpu-checker"
    
    install_if_missing "QEMU/KVM" \
        "command -v qemu-system-x86_64" \
        "$SUDO apt-get install -y qemu-kvm"
    
    install_if_missing "libvirt" \
        "dpkg -l | grep libvirt-daemon" \
        "$SUDO apt-get install -y libvirt-daemon-system libvirt-clients"
    
    # Check and enable KVM support
    log_info "Checking and configuring KVM support..."
    
    # First check CPU support - but don't fail if flags aren't visible
    if grep -E -q '(vmx|svm)' /proc/cpuinfo 2>/dev/null; then
        log_success "CPU virtualization flags detected"
    else
        log_warning "CPU virtualization flags not visible in /proc/cpuinfo"
        log_info "This might be normal on some systems - will attempt to load KVM anyway"
    fi
    
    # Always try to setup KVM on bare metal or when uncertain
    if true; then
        
        # Install KVM and related packages
        log_info "Installing KVM packages..."
        $SUDO apt-get install -y qemu-kvm libvirt-daemon-system libvirt-clients bridge-utils virt-manager cpu-checker > /dev/null 2>&1 || true
        
        # Load KVM modules
        log_info "Loading KVM kernel modules..."
        if grep -q "Intel" /proc/cpuinfo; then
            $SUDO modprobe kvm_intel 2>/dev/null || true
            # Enable nested virtualization for Intel
            echo "options kvm_intel nested=1" | $SUDO tee /etc/modprobe.d/kvm.conf > /dev/null
        else
            $SUDO modprobe kvm_amd 2>/dev/null || true
            # Enable nested virtualization for AMD
            echo "options kvm_amd nested=1" | $SUDO tee /etc/modprobe.d/kvm.conf > /dev/null
        fi
        $SUDO modprobe kvm 2>/dev/null || true
        
        # Ensure /dev/kvm exists with proper permissions
        if [[ ! -e /dev/kvm ]]; then
            log_warning "/dev/kvm doesn't exist, trying to create it..."
            $SUDO mknod /dev/kvm c 10 232 2>/dev/null || true
        fi
        
        # Set permissions for /dev/kvm
        $SUDO chmod 666 /dev/kvm 2>/dev/null || true
        
        # Add current user to kvm and libvirt groups
        if [[ -n "${USER:-}" ]]; then
            $SUDO usermod -aG kvm ${USER} 2>/dev/null || true
            $SUDO usermod -aG libvirt ${USER} 2>/dev/null || true
        fi
        
        # Verify KVM is working
        if [[ -e /dev/kvm ]]; then
            log_success "KVM device /dev/kvm is available"
            
            # Double check with kvm-ok if available
            if command_exists kvm-ok; then
                if kvm-ok 2>&1 | grep -q "KVM acceleration can be used"; then
                    log_success "KVM acceleration is enabled and working"
                else
                    log_warning "KVM device exists but kvm-ok reports issues"
                    kvm-ok 2>&1 || true
                fi
            fi
        else
            log_error "Failed to enable KVM - /dev/kvm not available"
            log_info "You may need to:"
            log_info "  1. Enable virtualization in BIOS/UEFI"
            log_info "  2. Reboot the system after KVM installation"
            log_info "  3. Check dmesg for KVM-related errors"
        fi
    fi
    
    log_success "Basic dependencies installed"
}

main "$@"
