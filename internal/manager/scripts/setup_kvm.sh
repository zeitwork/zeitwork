#!/bin/bash
# Setup KVM for Firecracker

set -euo pipefail

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

# Check for root/sudo
SUDO=$(check_root)

# Check CPU virtualization support
check_cpu_support() {
    log_info "Checking CPU virtualization support..."
    
    # Check for vmx or svm flags (just need to find one occurrence)
    if grep -E -q '(vmx|svm)' /proc/cpuinfo; then
        if grep -q "vmx" /proc/cpuinfo; then
            log_success "Intel VT-x virtualization supported"
            echo "intel"
        elif grep -q "svm" /proc/cpuinfo; then
            log_success "AMD-V virtualization supported"
            echo "amd"
        fi
    else
        # Flags not visible, but let's check CPU model to determine type
        log_warning "CPU virtualization flags not visible in /proc/cpuinfo"
        log_info "Attempting to detect CPU type from model..."
        
        if grep -qi "intel" /proc/cpuinfo; then
            log_info "Detected Intel CPU - will attempt to load kvm_intel"
            echo "intel"
        elif grep -qi "amd" /proc/cpuinfo; then
            log_info "Detected AMD CPU - will attempt to load kvm_amd"
            echo "amd"
        else
            log_warning "Could not detect CPU type, will try both Intel and AMD modules"
            echo "unknown"
        fi
    fi
}

# Install KVM packages
install_kvm_packages() {
    log_info "Installing KVM and virtualization packages..."
    
    local packages=(
        qemu-kvm
        libvirt-daemon-system
        libvirt-clients
        bridge-utils
        cpu-checker
        qemu-utils
    )
    
    for package in "${packages[@]}"; do
        if ! dpkg -l | grep -q "^ii.*$package"; then
            log_info "Installing $package..."
            $SUDO apt-get install -y "$package" || {
                log_error "Failed to install $package"
                return 1
            }
        else
            log_success "$package already installed"
        fi
    done
}

# Load KVM kernel modules
load_kvm_modules() {
    local cpu_type=$1
    
    log_info "Loading KVM kernel modules..."
    
    # First, ensure the modules are available
    log_info "Checking for KVM module availability..."
    if ! find /lib/modules/$(uname -r) -name 'kvm*.ko*' | grep -q kvm; then
        log_error "KVM modules not found for kernel $(uname -r)"
        log_info "Installing linux-modules-extra package..."
        $SUDO apt-get install -y linux-modules-extra-$(uname -r) || true
    fi
    
    # Load base KVM module
    if ! lsmod | grep -q "^kvm "; then
        $SUDO modprobe kvm || {
            log_error "Failed to load kvm module"
            log_info "Checking dmesg for errors..."
            dmesg | tail -20 | grep -i kvm || true
            return 1
        }
        log_success "Loaded kvm module"
    else
        log_success "kvm module already loaded"
    fi
    
    # Load CPU-specific module
    if [[ "$cpu_type" == "intel" ]]; then
        if ! lsmod | grep -q "^kvm_intel "; then
            $SUDO modprobe kvm_intel || {
                log_error "Failed to load kvm_intel module"
                log_info "You may need to enable VT-x in BIOS/UEFI"
                return 1
            }
            log_success "Loaded kvm_intel module"
        else
            log_success "kvm_intel module already loaded"
        fi
        
        # Enable nested virtualization for Intel
        echo "options kvm_intel nested=1" | $SUDO tee /etc/modprobe.d/kvm-nested.conf > /dev/null
        
    elif [[ "$cpu_type" == "amd" ]]; then
        if ! lsmod | grep -q "^kvm_amd "; then
            $SUDO modprobe kvm_amd || {
                log_error "Failed to load kvm_amd module"
                log_info "You may need to enable AMD-V in BIOS/UEFI"
                return 1
            }
            log_success "Loaded kvm_amd module"
        else
            log_success "kvm_amd module already loaded"
        fi
        
        # Enable nested virtualization for AMD
        echo "options kvm_amd nested=1" | $SUDO tee /etc/modprobe.d/kvm-nested.conf > /dev/null
        
    elif [[ "$cpu_type" == "unknown" ]]; then
        # Try both modules
        log_info "Trying to load Intel KVM module..."
        if $SUDO modprobe kvm_intel 2>/dev/null; then
            log_success "Loaded kvm_intel module"
            echo "options kvm_intel nested=1" | $SUDO tee /etc/modprobe.d/kvm-nested.conf > /dev/null
        else
            log_info "Intel module failed, trying AMD..."
            if $SUDO modprobe kvm_amd 2>/dev/null; then
                log_success "Loaded kvm_amd module"
                echo "options kvm_amd nested=1" | $SUDO tee /etc/modprobe.d/kvm-nested.conf > /dev/null
            else
                log_error "Failed to load both kvm_intel and kvm_amd modules"
                log_info "Please check BIOS/UEFI virtualization settings"
                return 1
            fi
        fi
    fi
}

# Setup KVM device permissions
setup_kvm_device() {
    log_info "Setting up /dev/kvm device..."
    
    if [[ ! -e /dev/kvm ]]; then
        log_warning "/dev/kvm does not exist, attempting to create..."
        
        # Try to reload modules
        $SUDO rmmod kvm_intel 2>/dev/null || true
        $SUDO rmmod kvm_amd 2>/dev/null || true
        $SUDO rmmod kvm 2>/dev/null || true
        
        sleep 1
        
        # Reload based on CPU type
        local cpu_type=$(check_cpu_support)
        load_kvm_modules "$cpu_type"
        
        # If still no /dev/kvm, try to create it manually
        if [[ ! -e /dev/kvm ]]; then
            $SUDO mknod /dev/kvm c 10 232 || {
                log_error "Failed to create /dev/kvm device"
                return 1
            }
        fi
    fi
    
    # Set permissions
    $SUDO chmod 666 /dev/kvm || {
        log_error "Failed to set permissions on /dev/kvm"
        return 1
    }
    
    log_success "/dev/kvm is available with proper permissions"
}

# Add user to virtualization groups
setup_user_groups() {
    log_info "Setting up user groups for virtualization..."
    
    # Add root to groups (for when running as root via SSH)
    $SUDO usermod -aG kvm root 2>/dev/null || true
    $SUDO usermod -aG libvirt root 2>/dev/null || true
    
    # If USER is set, add that user too
    if [[ -n "${USER:-}" ]] && [[ "${USER}" != "root" ]]; then
        $SUDO usermod -aG kvm "${USER}" 2>/dev/null || true
        $SUDO usermod -aG libvirt "${USER}" 2>/dev/null || true
        log_success "Added ${USER} to kvm and libvirt groups"
    fi
}

# Verify KVM is working
verify_kvm() {
    log_info "Verifying KVM setup..."
    
    # Check /dev/kvm exists
    if [[ ! -e /dev/kvm ]]; then
        log_error "/dev/kvm does not exist"
        return 1
    fi
    
    # Check permissions
    if [[ ! -r /dev/kvm ]] || [[ ! -w /dev/kvm ]]; then
        log_warning "/dev/kvm permissions may be incorrect"
        ls -la /dev/kvm
    fi
    
    # Use kvm-ok if available
    if command_exists kvm-ok; then
        if kvm-ok 2>&1 | grep -q "KVM acceleration can be used"; then
            log_success "KVM acceleration is available and working"
        else
            log_warning "kvm-ok reports issues:"
            kvm-ok 2>&1 || true
        fi
    fi
    
    # Check loaded modules
    log_info "Loaded KVM modules:"
    lsmod | grep kvm || true
    
    return 0
}

# Main
main() {
    log_info "Setting up KVM for Firecracker..."
    
    # Check CPU support
    cpu_type=$(check_cpu_support)
    
    # Update package list
    log_info "Updating package list..."
    $SUDO apt-get update || true
    
    # Install packages
    install_kvm_packages
    
    # Load kernel modules
    load_kvm_modules "$cpu_type"
    
    # Setup device
    setup_kvm_device
    
    # Setup groups
    setup_user_groups
    
    # Verify
    if verify_kvm; then
        log_success "KVM is properly configured for Firecracker!"
    else
        log_error "KVM setup completed but verification failed"
        log_info "You may need to:"
        log_info "  1. Reboot the system"
        log_info "  2. Check BIOS/UEFI virtualization settings"
        log_info "  3. Check dmesg for KVM-related errors: dmesg | grep -i kvm"
        exit 1
    fi
}

main "$@"
