#!/bin/bash
set -euo pipefail

# Build a Firecracker VM image that can build Docker containers
# This creates a rootfs with Docker and build tools installed

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="/tmp/builder-vm"
OUTPUT_DIR="${SCRIPT_DIR}/../build/vms"
ROOTFS_SIZE="4G"
ROOTFS_IMAGE="builder-rootfs.ext4"
KERNEL_IMAGE="vmlinux"

echo "🔨 Building Firecracker Builder VM..."

# Cleanup and setup
cleanup() {
    echo "Cleaning up..."
    sudo umount "${BUILD_DIR}/mnt" 2>/dev/null || true
    sudo losetup -d /dev/loop99 2>/dev/null || true
    rm -rf "${BUILD_DIR}"
}
trap cleanup EXIT

# Create build directory
mkdir -p "${BUILD_DIR}/mnt" "${OUTPUT_DIR}"

# Create rootfs image
echo "Creating ${ROOTFS_SIZE} rootfs image..."
dd if=/dev/zero of="${BUILD_DIR}/${ROOTFS_IMAGE}" bs=1M count=4096 2>/dev/null
mkfs.ext4 -F "${BUILD_DIR}/${ROOTFS_IMAGE}"

# Mount rootfs
echo "Mounting rootfs..."
sudo losetup /dev/loop99 "${BUILD_DIR}/${ROOTFS_IMAGE}"
sudo mount /dev/loop99 "${BUILD_DIR}/mnt"

# Install base system using debootstrap
echo "Installing base Ubuntu 22.04 system..."
sudo debootstrap --arch=amd64 --variant=minbase jammy "${BUILD_DIR}/mnt" http://archive.ubuntu.com/ubuntu/

# Configure base system
echo "Configuring base system..."
sudo tee "${BUILD_DIR}/mnt/etc/fstab" > /dev/null <<EOF
/dev/vda / ext4 defaults,noatime 0 1
EOF

# Set hostname
echo "builder" | sudo tee "${BUILD_DIR}/mnt/etc/hostname" > /dev/null

# Configure networking
sudo tee "${BUILD_DIR}/mnt/etc/systemd/network/10-eth0.network" > /dev/null <<EOF
[Match]
Name=eth0

[Network]
DHCP=ipv4
IPv6AcceptRA=yes

[DHCP]
UseMTU=true
UseHostname=false
EOF

# Create init script for Docker builds
sudo tee "${BUILD_DIR}/mnt/usr/local/bin/build-docker.sh" > /dev/null <<'BUILDSCRIPT'
#!/bin/bash
set -euo pipefail

# This script runs inside the VM to build Docker images

BUILD_DIR="/build"
OUTPUT_DIR="/output"
LOG_FILE="/var/log/build.log"

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

# Parse build arguments from kernel command line
REPO_URL=""
COMMIT_SHA=""
DOCKERFILE="Dockerfile"
S3_BUCKET=""
S3_KEY=""
NOTIFY_URL=""

for arg in $(cat /proc/cmdline); do
    case "$arg" in
        repo_url=*) REPO_URL="${arg#*=}" ;;
        commit_sha=*) COMMIT_SHA="${arg#*=}" ;;
        dockerfile=*) DOCKERFILE="${arg#*=}" ;;
        s3_bucket=*) S3_BUCKET="${arg#*=}" ;;
        s3_key=*) S3_KEY="${arg#*=}" ;;
        notify_url=*) NOTIFY_URL="${arg#*=}" ;;
    esac
done

log "Starting Docker build process"
log "Repository: $REPO_URL"
log "Commit: $COMMIT_SHA"
log "Dockerfile: $DOCKERFILE"

# Start Docker daemon
log "Starting Docker daemon..."
dockerd --storage-driver=vfs > /var/log/docker.log 2>&1 &
DOCKER_PID=$!

# Wait for Docker to start
for i in {1..30}; do
    if docker version > /dev/null 2>&1; then
        log "Docker daemon started"
        break
    fi
    sleep 1
done

# Clone repository
log "Cloning repository..."
cd "$BUILD_DIR"
git clone "$REPO_URL" app
cd app
git checkout "$COMMIT_SHA"

# Build Docker image
log "Building Docker image..."
IMAGE_TAG="build-$(date +%s)"
if docker build -f "$DOCKERFILE" -t "$IMAGE_TAG" .; then
    log "Docker build successful"
    BUILD_STATUS="success"
else
    log "Docker build failed"
    BUILD_STATUS="failed"
    exit 1
fi

# Export Docker image
log "Exporting Docker image..."
docker save "$IMAGE_TAG" -o "${OUTPUT_DIR}/image.tar"

# Convert to rootfs if successful
if [ "$BUILD_STATUS" = "success" ]; then
    log "Converting to Firecracker rootfs..."
    
    # Create container from image
    CONTAINER_ID=$(docker create "$IMAGE_TAG")
    
    # Export container filesystem
    docker export "$CONTAINER_ID" > "${OUTPUT_DIR}/rootfs.tar"
    
    # Create ext4 filesystem
    dd if=/dev/zero of="${OUTPUT_DIR}/rootfs.ext4" bs=1M count=2048
    mkfs.ext4 -F "${OUTPUT_DIR}/rootfs.ext4"
    
    # Mount and extract
    mkdir -p /mnt/rootfs
    mount -o loop "${OUTPUT_DIR}/rootfs.ext4" /mnt/rootfs
    tar -xf "${OUTPUT_DIR}/rootfs.tar" -C /mnt/rootfs
    
    # Configure for Firecracker
    echo '#!/bin/sh' > /mnt/rootfs/sbin/init
    echo 'exec /usr/local/bin/app-start.sh' >> /mnt/rootfs/sbin/init
    chmod +x /mnt/rootfs/sbin/init
    
    # Cleanup
    umount /mnt/rootfs
    docker rm "$CONTAINER_ID"
    
    # Upload to S3 if configured
    if [ -n "$S3_BUCKET" ] && [ -n "$S3_KEY" ]; then
        log "Uploading to S3..."
        aws s3 cp "${OUTPUT_DIR}/rootfs.ext4" "s3://${S3_BUCKET}/${S3_KEY}"
    fi
fi

# Notify build completion
if [ -n "$NOTIFY_URL" ]; then
    log "Notifying build completion..."
    curl -X POST "$NOTIFY_URL" \
        -H "Content-Type: application/json" \
        -d "{\"status\":\"$BUILD_STATUS\",\"s3_bucket\":\"$S3_BUCKET\",\"s3_key\":\"$S3_KEY\"}"
fi

# Shutdown
log "Build complete, shutting down..."
kill $DOCKER_PID 2>/dev/null || true
poweroff
BUILDSCRIPT

sudo chmod +x "${BUILD_DIR}/mnt/usr/local/bin/build-docker.sh"

# Create systemd service for builder
sudo tee "${BUILD_DIR}/mnt/etc/systemd/system/docker-builder.service" > /dev/null <<EOF
[Unit]
Description=Docker Image Builder
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/build-docker.sh
RemainAfterExit=yes
StandardOutput=journal+console
StandardError=journal+console

[Install]
WantedBy=multi-user.target
EOF

# Install packages in chroot
echo "Installing Docker and build tools..."
sudo chroot "${BUILD_DIR}/mnt" /bin/bash <<'CHROOT_CMDS'
export DEBIAN_FRONTEND=noninteractive

# Update package lists
apt-get update

# Install essential packages
apt-get install -y --no-install-recommends \
    systemd \
    systemd-sysv \
    udev \
    iproute2 \
    iputils-ping \
    ca-certificates \
    curl \
    git \
    build-essential \
    python3 \
    python3-pip \
    nodejs \
    npm

# Install Docker
curl -fsSL https://get.docker.com | sh

# Install AWS CLI for S3 uploads
pip3 install awscli

# Enable services
systemctl enable systemd-networkd
systemctl enable systemd-resolved
systemctl enable docker
systemctl enable docker-builder

# Clean up
apt-get clean
rm -rf /var/lib/apt/lists/*
rm -rf /tmp/*
CHROOT_CMDS

# Configure init for Firecracker
sudo tee "${BUILD_DIR}/mnt/sbin/init" > /dev/null <<'INIT'
#!/bin/bash
# Firecracker init script

# Mount essential filesystems
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
mount -t tmpfs tmpfs /run

# Start systemd
exec /lib/systemd/systemd
INIT

sudo chmod +x "${BUILD_DIR}/mnt/sbin/init"

# Configure serial console
sudo tee -a "${BUILD_DIR}/mnt/etc/systemd/system/serial-getty@ttyS0.service" > /dev/null <<EOF
[Service]
ExecStart=-/sbin/agetty -a root --keep-baud 115200,57600,38400,9600 ttyS0 \$TERM
EOF

# Unmount rootfs
echo "Finalizing rootfs..."
sudo umount "${BUILD_DIR}/mnt"
sudo losetup -d /dev/loop99

# Copy to output directory
echo "Copying builder rootfs to output directory..."
cp "${BUILD_DIR}/${ROOTFS_IMAGE}" "${OUTPUT_DIR}/"

# Download Firecracker kernel if not present
if [ ! -f "${OUTPUT_DIR}/${KERNEL_IMAGE}" ]; then
    echo "Downloading Firecracker kernel..."
    KERNEL_URL="https://github.com/firecracker-microvm/firecracker/releases/download/v1.4.0/vmlinux-5.10.186"
    curl -L -o "${OUTPUT_DIR}/${KERNEL_IMAGE}" "$KERNEL_URL"
fi

# Create metadata file
cat > "${OUTPUT_DIR}/builder-vm.json" <<EOF
{
  "version": "1.0.0",
  "type": "builder",
  "kernel": "${KERNEL_IMAGE}",
  "rootfs": "${ROOTFS_IMAGE}",
  "vcpus": 4,
  "memory_mb": 2048,
  "created": "$(date -Iseconds)",
  "features": [
    "docker",
    "git", 
    "aws-cli",
    "build-tools"
  ]
}
EOF

echo "✅ Firecracker Builder VM created successfully!"
echo "   Rootfs: ${OUTPUT_DIR}/${ROOTFS_IMAGE}"
echo "   Kernel: ${OUTPUT_DIR}/${KERNEL_IMAGE}"
echo "   Metadata: ${OUTPUT_DIR}/builder-vm.json"
echo ""
echo "To test the builder VM:"
echo "  firecracker --api-sock /tmp/firecracker.sock --config-file test-config.json"
