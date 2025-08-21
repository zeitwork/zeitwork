#!/bin/bash
# Setup and run Firecracker VM with container

set -euo pipefail

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

# Arguments
VCPU_COUNT="${1:-1}"
MEMORY_MIB="${2:-128}"
KERNEL_URL="${3:-}"

# Check for root/sudo
SUDO=$(check_root)

# Default kernel URL if not provided
DEFAULT_KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.5/x86_64/vmlinux-5.10.186"

# Create container rootfs
create_rootfs() {
    log_info "Creating container rootfs..."
    
    $SUDO mkdir -p /tmp/firecracker-vm
    cd /tmp/firecracker-vm
    $SUDO mkdir -p rootfs
    
    # Try with Docker first, then containerd
    if command_exists docker; then
        log_info "Using Docker to create rootfs..."
        $SUDO docker pull alpine:latest 2>/dev/null || true
        local container_id=$($SUDO docker create alpine:latest 2>/dev/null || echo "")
        if [[ -n "$container_id" ]]; then
            $SUDO docker export "$container_id" | $SUDO tar -xC rootfs
            $SUDO docker rm "$container_id" > /dev/null 2>&1 || true
        fi
    elif command_exists ctr; then
        log_info "Using containerd to create rootfs..."
        # Pull the image
        $SUDO ctr image pull docker.io/library/alpine:latest || true
        
        # Export the image to a tar file and extract it
        $SUDO ctr image export /tmp/alpine.tar docker.io/library/alpine:latest 2>/dev/null || true
        if [[ -f /tmp/alpine.tar ]]; then
            # Extract the OCI image layers to get the rootfs
            $SUDO mkdir -p /tmp/alpine-oci
            $SUDO tar -xf /tmp/alpine.tar -C /tmp/alpine-oci 2>/dev/null || true
            
            # Find and extract all layer tars
            for layer in /tmp/alpine-oci/blobs/sha256/*; do
                if file "$layer" 2>/dev/null | grep -q "gzip"; then
                    $SUDO tar -xzf "$layer" -C rootfs 2>/dev/null || true
                elif file "$layer" 2>/dev/null | grep -q "tar archive"; then
                    $SUDO tar -xf "$layer" -C rootfs 2>/dev/null || true
                fi
            done
            
            # Cleanup
            $SUDO rm -rf /tmp/alpine.tar /tmp/alpine-oci
        else
            # Fallback: Try to create a snapshot and mount it
            $SUDO ctr snapshot prepare alpine-snapshot docker.io/library/alpine:latest 2>/dev/null || true
            $SUDO ctr snapshot mount rootfs alpine-snapshot 2>/dev/null || true
        fi
    else
        log_info "Creating minimal rootfs..."
        $SUDO mkdir -p rootfs/{bin,dev,etc,proc,sys,tmp}
    fi
    
    # Create hello world script as init
    cat << 'EOF' | $SUDO tee rootfs/hello.sh > /dev/null
#!/bin/sh
# Mount essential filesystems
mount -t proc none /proc 2>/dev/null || true
mount -t sysfs none /sys 2>/dev/null || true

echo ""
echo "===================================="
echo "Hello World from Firecracker VM!"
echo "===================================="
echo ""
echo "System info:"
uname -a
echo ""
echo "Memory:"
free -h 2>/dev/null || cat /proc/meminfo | head -3
echo ""
echo "CPU info:"
cat /proc/cpuinfo | grep "model name" | head -1 2>/dev/null || echo "CPU info not available"
echo ""
echo "===================================="
echo ""

# Halt the system after displaying info
halt -f
EOF
    
    $SUDO chmod +x rootfs/hello.sh
    
    # Also create a simpler init if hello.sh doesn't work
    cat << 'EOF' | $SUDO tee rootfs/init > /dev/null
#!/bin/sh
exec /hello.sh
EOF
    $SUDO chmod +x rootfs/init
    log_success "Rootfs created"
}

# Create filesystem image
create_filesystem_image() {
    log_info "Creating filesystem image..."
    
    cd /tmp/firecracker-vm
    
    # Create ext4 image
    $SUDO dd if=/dev/zero of=rootfs.ext4 bs=1M count=512 2>/dev/null
    $SUDO mkfs.ext4 -q rootfs.ext4 2>/dev/null
    
    # Mount and copy rootfs
    $SUDO mkdir -p /mnt/rootfs
    $SUDO mount -o loop rootfs.ext4 /mnt/rootfs
    $SUDO cp -r rootfs/* /mnt/rootfs/ 2>/dev/null || true
    $SUDO umount /mnt/rootfs
    
    log_success "Filesystem image created"
}

# Download kernel
download_kernel() {
    log_info "Downloading kernel..."
    
    cd /tmp/firecracker-vm
    
    local kernel_url="${KERNEL_URL:-$DEFAULT_KERNEL_URL}"
    log_info "Kernel URL: ${kernel_url}"
    
    if curl -fsSL -o vmlinux.bin "$kernel_url"; then
        log_success "Kernel downloaded"
    else
        log_error "Failed to download kernel from ${kernel_url}"
        exit 1
    fi
}

# Create Firecracker configuration
create_vm_config() {
    log_info "Creating Firecracker configuration..."
    
    cd /tmp/firecracker-vm
    
    # Create log file first
    $SUDO touch /tmp/firecracker-vm/firecracker.log
    $SUDO chmod 666 /tmp/firecracker-vm/firecracker.log
    
    cat << EOF | $SUDO tee config.json > /dev/null
{
    "boot-source": {
        "kernel_image_path": "/tmp/firecracker-vm/vmlinux.bin",
        "boot_args": "console=ttyS0 reboot=k panic=1 pci=off init=/hello.sh root=/dev/vda rw"
    },
    "drives": [
        {
            "drive_id": "rootfs",
            "path_on_host": "/tmp/firecracker-vm/rootfs.ext4",
            "is_root_device": true,
            "is_read_only": false
        }
    ],
    "machine-config": {
        "vcpu_count": ${VCPU_COUNT},
        "mem_size_mib": ${MEMORY_MIB},
        "track_dirty_pages": false
    },
    "logger": {
        "log_path": "/tmp/firecracker-vm/firecracker.log",
        "level": "Debug",
        "show_level": true,
        "show_log_origin": true
    }
}
EOF
    
    log_success "VM configuration created (vCPUs: ${VCPU_COUNT}, Memory: ${MEMORY_MIB}MiB)"
}

# Run Firecracker VM
run_vm() {
    log_info "Starting Firecracker VM..."
    
    cd /tmp/firecracker-vm
    
    # Clean up any existing sockets
    $SUDO rm -f /tmp/firecracker*.socket 2>/dev/null || true
    
    # Try different methods to run Firecracker
    local output=""
    
    # Method 1: Try with --no-api flag (simplest)
    log_info "Running VM with --no-api flag (5 second timeout)..."
    output=$($SUDO timeout 5 firecracker --no-api --config-file config.json 2>&1 || true)
    
    # If that didn't work, try alternative method
    if ! echo "$output" | grep -q "Hello World from Firecracker VM!" && ! echo "$output" | grep -q "Running Firecracker"; then
        log_info "Trying alternative method with stdin config..."
        output=$($SUDO timeout 5 sh -c 'cat config.json | firecracker --no-api' 2>&1 || true)
    fi
    
    # Check output
    if echo "$output" | grep -q "Hello World from Firecracker VM!"; then
        log_success "VM executed successfully!"
        echo ""
        echo "VM Output:"
        echo "----------------------------------------"
        echo "$output"
        echo "----------------------------------------"
    elif echo "$output" | grep -q "Running Firecracker"; then
        log_info "Firecracker started but hello world script may not have executed"
        echo "Output:"
        echo "$output"
    else
        log_warning "VM execution completed with unexpected output"
        if [[ -n "$output" ]]; then
            echo "Output:"
            echo "$output"
        fi
    fi
    
    # Check log file
    if [[ -f "/tmp/firecracker-vm/firecracker.log" ]]; then
        log_info "Checking Firecracker log file..."
        if [[ -s "/tmp/firecracker-vm/firecracker.log" ]]; then
            echo "Log contents:"
            $SUDO tail -20 /tmp/firecracker-vm/firecracker.log 2>/dev/null || true
        else
            echo "Log file is empty"
        fi
    fi
}

# Cleanup
cleanup() {
    log_info "Cleaning up..."
    $SUDO umount /mnt/rootfs 2>/dev/null || true
    # Optionally remove temporary files
    # $SUDO rm -rf /tmp/firecracker-vm
    log_success "Cleanup completed"
}

# Main
main() {
    log_info "Setting up Firecracker VM..."
    
    # Check if Firecracker is installed
    if ! command_exists firecracker; then
        log_error "Firecracker is not installed. Please install it first."
        exit 1
    fi
    
    # Setup VM
    create_rootfs
    create_filesystem_image
    download_kernel
    create_vm_config
    
    # Run VM
    run_vm
    
    # Cleanup
    trap cleanup EXIT
    
    log_success "Firecracker VM setup completed!"
}

main "$@"
