#!/bin/bash

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK_DIR="${SCRIPT_DIR}/zeitwork-build"
KERNEL_VERSION="6.1"
ALPINE_VERSION="3.19"
IPV6_PREFIX="fd00:42::"
VM_COUNT=3

log() {
    echo -e "${BLUE}[$(date '+%Y-%m-%d %H:%M:%S')] $1${NC}"
}

log_success() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')] ✓ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] ⚠ $1${NC}"
}

log_error() {
    echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] ✗ $1${NC}"
}

cleanup_existing_instances() {
    log "Cleaning up existing Firecracker instances..."
    
    # Kill any running firecracker processes
    sudo pkill -f firecracker || true
    sudo pkill -f jailer || true
    
    # Remove existing tap devices
    for tap in $(ip link show | grep tap | awk -F: '{print $2}' | tr -d ' '); do
        sudo ip link del "$tap" 2>/dev/null || true
    done
    
    # Clean up any existing unix sockets
    sudo rm -f /tmp/firecracker*.socket
    sudo rm -f /run/firecracker*.socket
    
    # Clean up any existing jail directories
    sudo rm -rf /srv/jailer/firecracker/* 2>/dev/null || true
    
    log_success "Cleanup completed"
}

setup_directories() {
    log "Setting up work directories..."
    
    mkdir -p "${WORK_DIR}"
    mkdir -p "${WORK_DIR}/kernel"
    mkdir -p "${WORK_DIR}/rootfs"
    mkdir -p "${WORK_DIR}/vm-configs"
    mkdir -p "${WORK_DIR}/binaries"
    mkdir -p "${WORK_DIR}/logs"
    
    log_success "Directories created"
}

install_dependencies() {
    log "Installing required dependencies for Ubuntu..."
    
    # Update package list
    sudo apt-get update
    
    # Install required packages
    sudo apt-get install -y \
        build-essential \
        libssl-dev \
        pkg-config \
        curl \
        git \
        bc \
        flex \
        bison \
        libelf-dev \
        squashfs-tools \
        docker.io \
        socat \
        bridge-utils \
        iptables \
        iproute2 \
        jq \
        wget \
        file \
        busybox-static
    
    # Start and enable Docker
    sudo systemctl start docker
    sudo systemctl enable docker
    
    # Add current user to docker group if not already
    if ! groups | grep -q docker; then
        sudo usermod -aG docker ${USER}
        log_warning "Added ${USER} to docker group. You may need to log out and back in."
    fi
    
    log_success "Ubuntu dependencies installed"
}

download_firecracker() {
    log "Downloading Firecracker binary..."
    
    ARCH="$(uname -m)"
    RELEASE_URL="https://github.com/firecracker-microvm/firecracker/releases"
    LATEST=$(basename $(curl -fsSLI -o /dev/null -w %{url_effective} ${RELEASE_URL}/latest))
    
    cd "${WORK_DIR}/binaries"
    
    if [[ ! -f "firecracker" ]]; then
        curl -L ${RELEASE_URL}/download/${LATEST}/firecracker-${LATEST}-${ARCH}.tgz | tar -xz
        mv release-${LATEST}-${ARCH}/firecracker-${LATEST}-${ARCH} firecracker
        mv release-${LATEST}-${ARCH}/jailer-${LATEST}-${ARCH} jailer
        chmod +x firecracker jailer
        rm -rf release-${LATEST}-${ARCH}
    fi
    
    log_success "Firecracker binary downloaded"
}

build_zeitwork_kernel() {
    log "Building Zeitwork kernel based on Alpine Linux..."
    
    cd "${WORK_DIR}/kernel"
    
    if [[ ! -d "linux" ]]; then
        git clone --depth 1 --branch v${KERNEL_VERSION} https://github.com/torvalds/linux.git
    fi
    
    cd linux
    
    # Create custom kernel config based on Alpine and Firecracker requirements
    cat > .config << 'EOF'
# Zeitwork Kernel Configuration for Firecracker
CONFIG_64BIT=y
CONFIG_X86_64=y
CONFIG_LOCALVERSION="-zeitwork"

# Basic system requirements
CONFIG_PRINTK=y
CONFIG_BUG=y
CONFIG_ELF_CORE=y
CONFIG_BASE_FULL=y
CONFIG_FUTEX=y
CONFIG_EPOLL=y
CONFIG_SIGNALFD=y
CONFIG_TIMERFD=y
CONFIG_EVENTFD=y
CONFIG_SHMEM=y
CONFIG_AIO=y
CONFIG_ADVISE_SYSCALLS=y
CONFIG_MEMBARRIER=y
CONFIG_KALLSYMS=y

# KVM Guest support
CONFIG_HYPERVISOR_GUEST=y
CONFIG_KVM_GUEST=y
CONFIG_PARAVIRT=y
CONFIG_PARAVIRT_SPINLOCKS=y

# CPU features
CONFIG_SMP=y
CONFIG_X86_LOCAL_APIC=y
CONFIG_X86_IO_APIC=y
CONFIG_X86_REROUTE_FOR_BROKEN_BOOT_IRQS=y
CONFIG_X86_TSC=y

# Memory management
CONFIG_FLATMEM=y
CONFIG_HAVE_MEMBLOCK=y
CONFIG_NO_BOOTMEM=y
CONFIG_MEMORY_BALLOON=y

# PCI support (required for ACPI)
CONFIG_PCI=y
CONFIG_PCI_DIRECT=y
CONFIG_PCI_MMCONFIG=y
CONFIG_PCI_MSI=y

# ACPI support
CONFIG_ACPI=y
CONFIG_ACPI_LEGACY_TABLES_LOOKUP=y
CONFIG_ACPI_SYSTEM_POWER_STATES_SUPPORT=y

# Virtio support
CONFIG_VIRTIO=y
CONFIG_VIRTIO_PCI=y
CONFIG_VIRTIO_PCI_LEGACY=y
CONFIG_VIRTIO_MMIO=y
CONFIG_VIRTIO_MMIO_CMDLINE_DEVICES=y

# Block devices
CONFIG_VIRTIO_BLK=y
CONFIG_BLK_DEV=y
CONFIG_BLK_DEV_INITRD=y

# Network support
CONFIG_NET=y
CONFIG_INET=y
CONFIG_IPV6=y
CONFIG_NETDEVICES=y
CONFIG_NET_CORE=y
CONFIG_VIRTIO_NET=y

# Vsock support
CONFIG_VHOST_VSOCK=y
CONFIG_VIRTIO_VSOCKETS=y

# Entropy device
CONFIG_HW_RANDOM=y
CONFIG_HW_RANDOM_VIRTIO=y

# Serial console
CONFIG_SERIAL_8250=y
CONFIG_SERIAL_8250_CONSOLE=y
CONFIG_PRINTK=y

# File systems
CONFIG_EXT4_FS=y
CONFIG_EXT4_FS_POSIX_ACL=y
CONFIG_EXT4_FS_SECURITY=y
CONFIG_TMPFS=y
CONFIG_TMPFS_POSIX_ACL=y
CONFIG_PROC_FS=y
CONFIG_SYSFS=y
CONFIG_DEVTMPFS=y
CONFIG_DEVTMPFS_MOUNT=y

# Security
CONFIG_SECURITY=y
CONFIG_SECCOMP=y
CONFIG_SECCOMP_FILTER=y

# Networking extras for IPv6
CONFIG_IPV6_ROUTER_PREF=y
CONFIG_IPV6_ROUTE_INFO=y
CONFIG_IPV6_OPTIMISTIC_DAD=y
CONFIG_INET6_AH=y
CONFIG_INET6_ESP=y
CONFIG_IPV6_MIP6=y
CONFIG_IPV6_SIT=y
CONFIG_IPV6_TUNNEL=y
CONFIG_IPV6_MULTIPLE_TABLES=y
CONFIG_IPV6_SUBTREES=y

# Essential kernel features
CONFIG_UNIX=y
CONFIG_PACKET=y
CONFIG_NETLINK_DIAG=y
CONFIG_UNIX_DIAG=y
CONFIG_INET_DIAG=y
CONFIG_INET_TCP_DIAG=y
CONFIG_INET_UDP_DIAG=y

# Disable unnecessary features for microVM
# CONFIG_MODULES is not set
# CONFIG_SUSPEND is not set
# CONFIG_HIBERNATION is not set
# CONFIG_USB is not set
# CONFIG_SOUND is not set
# CONFIG_WIRELESS is not set
# CONFIG_WLAN is not set
# CONFIG_BLUETOOTH is not set
EOF

    # Build the kernel
    make olddefconfig
    make -j$(nproc) vmlinux
    
    log_success "Zeitwork kernel built successfully"
}

create_go_test_server() {
    log "Creating Go test server Docker image..."
    
    cd "${WORK_DIR}"
    
    # Create Go server source
    cat > main.go << 'EOF'
package main

import (
    "fmt"
    "log"
    "net/http"
    "os"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        hostname, _ := os.Hostname()
        fmt.Fprintf(w, "Hello, Zeitwork! from %s\n", hostname)
    })
    
    log.Println("Starting server on :3000")
    log.Fatal(http.ListenAndServe(":3000", nil))
}
EOF

    # Create Dockerfile
    cat > Dockerfile << 'EOF'
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY main.go .
RUN go mod init zeitwork-server && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE 3000
CMD ["./server"]
EOF

    # Build Docker image
    sudo docker build -t zeitwork-server .
    
    log_success "Go test server Docker image created"
}

create_alpine_rootfs() {
    log "Creating Alpine-based rootfs with Go server..."
    
    cd "${WORK_DIR}/rootfs"
    
    if [[ ! -f "zeitwork-rootfs.ext4" ]]; then
        # Create empty rootfs file
        dd if=/dev/zero of=zeitwork-rootfs.ext4 bs=1M count=512
        mkfs.ext4 zeitwork-rootfs.ext4
        
        # Mount rootfs
        mkdir -p rootfs-mount
        sudo mount zeitwork-rootfs.ext4 rootfs-mount
        
        # Use Alpine Docker container to create proper Alpine rootfs following official docs
        log "Setting up Alpine Linux base system..."
        
        # First, set up Alpine in the container and copy to our rootfs
        sudo docker run --rm -v "$(pwd)/rootfs-mount:/my-rootfs" alpine:${ALPINE_VERSION} sh -c '
            # Install required packages (following official Firecracker docs)
            apk add --no-cache openrc util-linux busybox-extras iproute2 iputils wget curl
            
            # Set up OpenRC services (following official docs)
            ln -s agetty /etc/init.d/agetty.ttyS0
            echo ttyS0 > /etc/securetty
            rc-update add agetty.ttyS0 default
            
            # Make sure special file systems are mounted on boot
            rc-update add devfs boot
            rc-update add procfs boot  
            rc-update add sysfs boot
            
            # Copy the newly configured system to the rootfs image (from official docs)
            for d in bin etc lib root sbin usr; do tar c "/$d" | tar x -C /my-rootfs; done
            
            # Create necessary directories (from official docs)
            for dir in dev proc run sys var tmp; do mkdir -p /my-rootfs/${dir}; done
            
            # Ensure init is executable and properly linked
            chmod +x /my-rootfs/sbin/init
            ls -la /my-rootfs/sbin/init
        '
        
        # Extract Go server binary from Docker image
        sudo docker create --name zeitwork-temp zeitwork-server
        sudo docker cp zeitwork-temp:/root/server rootfs-mount/usr/bin/
        sudo docker rm zeitwork-temp
        
        # Create a custom service for our Go server (Alpine/OpenRC way)
        sudo tee rootfs-mount/etc/init.d/zeitwork-server > /dev/null << 'EOF'
#!/sbin/openrc-run

name="zeitwork-server"
description="Zeitwork HTTP Server"
command="/usr/bin/server"
command_background="yes"
pidfile="/run/zeitwork-server.pid"
output_log="/var/log/zeitwork-server.log"
error_log="/var/log/zeitwork-server.log"

depend() {
    need net
    after firewall
}

start_pre() {
    # Configure IPv6 address if available
    if [ -f /etc/ipv6-addr ]; then
        IPV6_ADDR=$(cat /etc/ipv6-addr)
        einfo "Configuring IPv6 address: ${IPV6_ADDR}"
        ip -6 addr add ${IPV6_ADDR}/64 dev eth0 2>/dev/null || true
        ip -6 route add default via fd00:42::1 dev eth0 2>/dev/null || true
        einfo "IPv6 configuration complete"
    fi
    
    # Create log directory
    mkdir -p /var/log
    
    # Show network status for debugging
    einfo "Network configuration:"
    ip -6 addr show eth0 || true
    ip -6 route show || true
}

start() {
    ebegin "Starting ${name}"
    start-stop-daemon --start --exec "${command}" \
        --background --make-pidfile --pidfile "${pidfile}" \
        --stdout "${output_log}" --stderr "${error_log}"
    eend $?
}

stop() {
    ebegin "Stopping ${name}"
    start-stop-daemon --stop --pidfile "${pidfile}"
    eend $?
}
EOF

        sudo chmod +x rootfs-mount/etc/init.d/zeitwork-server
        
        # Create a network configuration service
        sudo tee rootfs-mount/etc/init.d/zeitwork-network > /dev/null << 'EOF'
#!/sbin/openrc-run

name="zeitwork-network"
description="Zeitwork Network Configuration"

depend() {
    need localmount
    before net
}

start() {
    ebegin "Configuring Zeitwork network"
    
    # Bring up loopback
    ip link set lo up
    
    # Bring up eth0
    ip link set eth0 up
    
    # Configure IPv6 if address file exists
    if [ -f /etc/ipv6-addr ]; then
        IPV6_ADDR=$(cat /etc/ipv6-addr)
        einfo "Setting IPv6 address: ${IPV6_ADDR}"
        ip -6 addr add ${IPV6_ADDR}/64 dev eth0
        ip -6 route add default via fd00:42::1 dev eth0
    fi
    
    eend $?
}
EOF

        sudo chmod +x rootfs-mount/etc/init.d/zeitwork-network
        
        # Add our services to the appropriate runlevels (inside the Alpine container setup)
        sudo docker run -it --rm -v "$(pwd)/rootfs-mount:/my-rootfs" alpine:${ALPINE_VERSION} sh -c '
            chroot /my-rootfs rc-update add zeitwork-network boot
            chroot /my-rootfs rc-update add zeitwork-server default
        '
        
        # Create basic /etc files
        sudo tee rootfs-mount/etc/passwd > /dev/null << 'EOF'
root:x:0:0:root:/root:/bin/sh
EOF

        sudo tee rootfs-mount/etc/group > /dev/null << 'EOF'
root:x:0:
EOF

        sudo tee rootfs-mount/etc/hosts > /dev/null << 'EOF'
127.0.0.1 localhost
::1 localhost
EOF

        # Create network interfaces file for Alpine Linux
        sudo tee rootfs-mount/etc/network/interfaces > /dev/null << 'EOF'
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet manual
EOF

        # Create log directory in rootfs
        sudo mkdir -p rootfs-mount/var/log
        
        # Debug: Check what we have in the rootfs
        log "Debugging rootfs contents:"
        echo "Contents of /sbin:"
        sudo ls -la rootfs-mount/sbin/ || echo "No /sbin directory"
        echo "Contents of /etc:"
        sudo ls -la rootfs-mount/etc/ | head -10 || echo "No /etc directory"
        echo "Init file details:"
        sudo ls -la rootfs-mount/sbin/init || echo "No init file found"
        echo "File type of init:"
        sudo file rootfs-mount/sbin/init || echo "Cannot determine file type"

        # Unmount
        sudo umount rootfs-mount
        rmdir rootfs-mount
    fi
    
    log_success "Alpine rootfs with Go server created"
}

setup_ipv6_networking() {
    log "Setting up IPv6 networking..."
    
    # Enable IPv6 forwarding
    echo 1 | sudo tee /proc/sys/net/ipv6/conf/all/forwarding
    
    # Create bridge for IPv6
    if ! ip link show br-zeitwork >/dev/null 2>&1; then
        sudo ip link add name br-zeitwork type bridge
        sudo ip link set br-zeitwork up
        sudo ip -6 addr add ${IPV6_PREFIX}1/64 dev br-zeitwork
    fi
    
    # Configure ip6tables for NAT (if available)
    if command -v ip6tables >/dev/null 2>&1; then
        sudo ip6tables -t nat -A POSTROUTING -s ${IPV6_PREFIX}/64 -j MASQUERADE || true
        sudo ip6tables -A FORWARD -i br-zeitwork -j ACCEPT || true
        sudo ip6tables -A FORWARD -o br-zeitwork -j ACCEPT || true
    fi
    
    log_success "IPv6 networking configured"
}

create_vm_config() {
    local vm_id=$1
    local ipv6_suffix=$2
    
    log "Creating VM configuration for VM ${vm_id}..."
    
    local config_dir="${WORK_DIR}/vm-configs/vm${vm_id}"
    mkdir -p "${config_dir}"
    mkdir -p "${config_dir}/logs"
    
    # Create unique IPv6 address
    local vm_ipv6="${IPV6_PREFIX}${ipv6_suffix}"
    
    # Copy rootfs and configure IPv6
    cp "${WORK_DIR}/rootfs/zeitwork-rootfs.ext4" "${config_dir}/rootfs.ext4"
    
    # Mount rootfs temporarily to set IPv6 address
    mkdir -p "${config_dir}/rootfs-temp"
    sudo mount "${config_dir}/rootfs.ext4" "${config_dir}/rootfs-temp"
    echo "${vm_ipv6}" | sudo tee "${config_dir}/rootfs-temp/etc/ipv6-addr" > /dev/null
    sudo umount "${config_dir}/rootfs-temp"
    rmdir "${config_dir}/rootfs-temp"
    
    # Create TAP device
    local tap_name="tap-zeitwork${vm_id}"
    if ! ip link show "${tap_name}" >/dev/null 2>&1; then
        sudo ip tuntap add dev "${tap_name}" mode tap
        sudo ip link set "${tap_name}" up
        sudo ip link set "${tap_name}" master br-zeitwork
    fi
    
    # Create log files before Firecracker starts (required by logger config)
    touch "${config_dir}/logs/firecracker.log"
    mkfifo "${config_dir}/logs/metrics.fifo" || true
    
    # Create VM configuration file with logging
    cat > "${config_dir}/vm-config.json" << EOF
{
  "boot-source": {
    "kernel_image_path": "${WORK_DIR}/kernel/linux/vmlinux",
    "boot_args": "console=ttyS0 reboot=k panic=1 pci=off"
  },
  "drives": [
    {
      "drive_id": "rootfs",
      "path_on_host": "${config_dir}/rootfs.ext4",
      "is_root_device": true,
      "is_read_only": false
    }
  ],
  "network-interfaces": [
    {
      "iface_id": "eth0",
      "host_dev_name": "${tap_name}"
    }
  ],
  "machine-config": {
    "vcpu_count": 1,
    "mem_size_mib": 256
  },
  "logger": {
    "log_path": "${config_dir}/logs/firecracker.log",
    "level": "Debug",
    "show_level": true,
    "show_log_origin": true
  },
  "metrics": {
    "metrics_path": "${config_dir}/logs/metrics.fifo"
  }
}
EOF
    
    echo "${vm_ipv6}" > "${config_dir}/ipv6-addr"
    echo "${tap_name}" > "${config_dir}/tap-name"
    
    log_success "VM ${vm_id} configuration created with IPv6: ${vm_ipv6}"
}

start_vm() {
    local vm_id=$1
    
    log "Starting VM ${vm_id}..."
    
    local config_dir="${WORK_DIR}/vm-configs/vm${vm_id}"
    local socket_path="/tmp/firecracker-vm${vm_id}.socket"
    local console_log="${config_dir}/logs/console.log"
    
    # Remove existing socket
    sudo rm -f "${socket_path}"
    
    # Start metrics collector in background
    (cat "${config_dir}/logs/metrics.fifo" > "${config_dir}/logs/metrics.log" 2>/dev/null &)
    
    # Start Firecracker with console output capture
    sudo "${WORK_DIR}/binaries/firecracker" \
        --api-sock "${socket_path}" \
        --config-file "${config_dir}/vm-config.json" \
        > "${console_log}" 2>&1 &
    
    local fc_pid=$!
    echo $fc_pid > "${config_dir}/firecracker.pid"
    echo "${socket_path}" > "${config_dir}/api-socket"
    echo "${console_log}" > "${config_dir}/console-log"
    
    # Wait for VM to start and check if it's running
    sleep 3
    
    if kill -0 $fc_pid 2>/dev/null; then
        log_success "VM ${vm_id} started (PID: ${fc_pid})"
        log "Console log: ${console_log}"
        log "Firecracker log: ${config_dir}/logs/firecracker.log"
        log "API socket: ${socket_path}"
    else
        log_error "VM ${vm_id} failed to start"
        log "Check console log: ${console_log}"
        return 1
    fi
}

test_vm_connectivity() {
    local vm_id=$1
    
    log "Testing connectivity to VM ${vm_id}..."
    
    local config_dir="${WORK_DIR}/vm-configs/vm${vm_id}"
    local vm_ipv6=$(cat "${config_dir}/ipv6-addr")
    
    # Wait for server to be ready
    sleep 10
    
    # Test HTTP connectivity
    local max_attempts=10
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        if curl -6 --connect-timeout 2 "http://[${vm_ipv6}]:3000" 2>/dev/null; then
            log_success "VM ${vm_id} HTTP server is responding!"
            return 0
        fi
        
        log "Attempt ${attempt}/${max_attempts} failed, retrying in 2 seconds..."
        sleep 2
        ((attempt++))
    done
    
    log_error "Failed to connect to VM ${vm_id} HTTP server after ${max_attempts} attempts"
    return 1
}

main() {
    log "Starting Zeitwork Firecracker setup..."
    
    # Check if running as root for some operations
    if [[ $EUID -eq 0 ]]; then
        log_error "Please don't run this script as root. Use sudo when needed."
        exit 1
    fi
    
    cleanup_existing_instances
    setup_directories
    install_dependencies
    download_firecracker
    build_zeitwork_kernel
    create_go_test_server
    create_alpine_rootfs
    setup_ipv6_networking
    
    # Create and start VMs
    for i in $(seq 1 $VM_COUNT); do
        local ipv6_suffix=$((10 + i))
        create_vm_config $i $ipv6_suffix
        start_vm $i
        test_vm_connectivity $i
    done
    
    log_success "Zeitwork Firecracker setup completed!"
    
    # Display connection information
    echo
    echo "=== Zeitwork VMs Status ==="
    for i in $(seq 1 $VM_COUNT); do
        local config_dir="${WORK_DIR}/vm-configs/vm${i}"
        local vm_ipv6=$(cat "${config_dir}/ipv6-addr")
        echo "VM ${i}: IPv6 [${vm_ipv6}]:3000"
        echo "  Test: curl -6 'http://[${vm_ipv6}]:3000'"
    done
    echo
    
    log "To stop VMs, run: sudo pkill -f firecracker"
}

# Run main function
main "$@"
