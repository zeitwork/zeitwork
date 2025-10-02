#!/bin/bash
set -e

echo "=== Firecracker Runtime - Host Setup ==="

# Check if running on Ubuntu x86_64
if [ "$(uname -m)" != "x86_64" ]; then
    echo "Error: This script requires x86_64 architecture"
    exit 1
fi

if ! grep -q "Ubuntu" /etc/os-release 2>/dev/null; then
    echo "Warning: This script is designed for Ubuntu. Your OS may not be fully supported."
fi

echo "Step 1: Installing system dependencies..."

# Fix any broken packages first
echo "Checking for broken packages..."
dpkg --configure -a || true
apt-get update || true

# Try to fix broken dependencies
apt-get install -f -y || true

# Clean apt cache to avoid stale state
apt-get clean

# Install packages one by one to avoid conflicts
echo "Installing required packages..."
PACKAGES=(
    "iptables"
    "iproute2"
    "curl"
    "jq"
    "wget"
    "acl"
    "build-essential"
)

for pkg in "${PACKAGES[@]}"; do
    if ! dpkg -l | grep -q "^ii  $pkg "; then
        echo "Installing $pkg..."
        if apt-get install -y "$pkg"; then
            echo "$pkg installed successfully"
        else
            echo "ERROR: Failed to install $pkg"
            exit 1
        fi
    else
        echo "$pkg already installed"
    fi
done

# Handle Docker installation separately
echo "Checking Docker installation..."
if ! command -v docker &> /dev/null; then
    echo "Docker not found, installing..."
    
    # Check if Docker CE is already partially installed
    if dpkg -l | grep -q "^ii  docker-ce"; then
        echo "Docker CE already installed but docker command not found"
        # Try to fix the installation
        apt-get install --reinstall -y docker-ce docker-ce-cli containerd.io || {
            echo "ERROR: Failed to reinstall Docker CE"
            exit 1
        }
    else
        # Try docker.io first (Ubuntu's version)
        echo "Attempting to install docker.io..."
        if apt-get install -y docker.io; then
            echo "Docker.io installed successfully"
        else
            echo "Failed to install docker.io, trying Docker CE..."
            # Install prerequisites
            apt-get install -y ca-certificates gnupg lsb-release
            mkdir -p /etc/apt/keyrings
            curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg 2>/dev/null || true
            chmod a+r /etc/apt/keyrings/docker.gpg
            
            echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
            apt-get update
            
            if apt-get install -y docker-ce docker-ce-cli containerd.io; then
                echo "Docker CE installed successfully"
            else
                echo "ERROR: Failed to install Docker"
                exit 1
            fi
        fi
    fi
else
    echo "Docker already installed: $(docker --version 2>/dev/null || echo 'version unknown')"
    
    # Verify docker is actually working
    if ! docker ps &> /dev/null; then
        echo "Docker command exists but not responding, checking service..."
    fi
fi

# Ensure Docker service is running
echo "Starting Docker service..."
systemctl enable docker 2>/dev/null || true
systemctl start docker 2>/dev/null || true

# Verify Docker is working
if docker ps &> /dev/null; then
    echo "Docker is working correctly"
else
    echo "WARNING: Docker service may not be running properly"
fi

echo "Step 2: Setting up KVM access..."
# Check if KVM is available
if [ ! -e /dev/kvm ]; then
    echo "Error: /dev/kvm not found. KVM support is required."
    exit 1
fi

# Grant access to /dev/kvm
setfacl -m u:${USER}:rw /dev/kvm 2>/dev/null || {
    echo "Warning: Could not set ACL on /dev/kvm. Trying group-based access..."
    if getent group kvm > /dev/null; then
        usermod -aG kvm ${USER} 2>/dev/null || true
        echo "Added user to kvm group. You may need to log out and back in."
    fi
}

# Verify KVM access
[ -r /dev/kvm ] && [ -w /dev/kvm ] && echo "KVM access: OK" || echo "KVM access: FAIL"

echo "Step 3: Downloading Firecracker binary..."
ARCH="x86_64"
release_url="https://github.com/firecracker-microvm/firecracker/releases"
latest=$(basename $(curl -fsSLI -o /dev/null -w %{url_effective} ${release_url}/latest))

echo "Latest Firecracker version: $latest"

# Download Firecracker and jailer
mkdir -p /opt/firecracker
cd /opt/firecracker

if [ ! -f "firecracker" ] || [ ! -f "jailer" ]; then
    echo "Downloading Firecracker $latest..."
    curl -L ${release_url}/download/${latest}/firecracker-${latest}-${ARCH}.tgz | tar -xz
    
    # Move binaries to /opt/firecracker
    mv release-${latest}-${ARCH}/firecracker-${latest}-${ARCH} firecracker
    mv release-${latest}-${ARCH}/jailer-${latest}-${ARCH} jailer
    rm -rf release-${latest}-${ARCH}
    
    chmod +x firecracker jailer
else
    echo "Firecracker and jailer already exist, skipping download"
fi

echo "Step 4: Downloading kernel..."
CI_VERSION="v1.9"
KERNEL_VERSION="5.10.225"

if [ ! -f "vmlinux" ]; then
    echo "Downloading kernel ${KERNEL_VERSION}..."
    wget -q -O vmlinux "https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/${CI_VERSION}/${ARCH}/vmlinux-${KERNEL_VERSION}"
    
    # Verify it's an ELF file
    if ! head -c 4 vmlinux | grep -q $'\x7fELF'; then
        echo "ERROR: Downloaded kernel is not a valid ELF file"
        rm vmlinux
        exit 1
    fi
    echo "Kernel downloaded and verified"
else
    echo "Kernel already exists, skipping download"
fi

echo "Step 5: Creating jailer user..."
# Create dedicated user for running Firecracker
if ! id "fcuser" &>/dev/null; then
    useradd -r -s /bin/false fcuser
    echo "Created fcuser"
else
    echo "fcuser already exists"
fi

# Add fcuser to kvm group for /dev/kvm access
if getent group kvm > /dev/null; then
    usermod -aG kvm fcuser
    echo "Added fcuser to kvm group"
else
    echo "Warning: kvm group does not exist"
fi

echo "Step 6: Setting up directories..."
# Create required directories for jailer
mkdir -p /srv/jailer
chmod 755 /srv/jailer

# Create working directory
mkdir -p /var/lib/firecracker-runtime
chmod 755 /var/lib/firecracker-runtime

echo "Step 7: Enabling IP forwarding..."
echo 1 > /proc/sys/net/ipv4/ip_forward

# Make it persistent
if ! grep -q "net.ipv4.ip_forward=1" /etc/sysctl.conf; then
    echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf
fi

echo "Step 8: Setting up bridge network..."
# Create bridge br0 if it doesn't exist
if ! ip link show br0 &> /dev/null; then
    echo "Creating bridge br0..."
    ip link add br0 type bridge
    ip addr add 172.16.0.1/16 dev br0
    ip link set br0 up
    echo "Bridge br0 created and configured"
else
    echo "Bridge br0 already exists"
    # Ensure it has the correct IP
    if ! ip addr show br0 | grep -q "172.16.0.1/16"; then
        ip addr add 172.16.0.1/16 dev br0 2>/dev/null || echo "IP already assigned to br0"
    fi
    ip link set br0 up
fi

echo "Step 9: Setting up NAT for VM internet access..."
# Check if NAT rule already exists
if ! iptables -t nat -C POSTROUTING -s 172.16.0.0/16 -j MASQUERADE 2>/dev/null; then
    echo "Adding NAT rule for 172.16.0.0/16..."
    iptables -t nat -A POSTROUTING -s 172.16.0.0/16 -j MASQUERADE
    echo "NAT rule added"
else
    echo "NAT rule already exists"
fi

echo ""
echo "=== Setup Complete ==="
echo "Firecracker binary: /opt/firecracker/firecracker"
echo "Jailer binary: /opt/firecracker/jailer"
echo "Kernel: /opt/firecracker/vmlinux"
echo "Bridge: br0 (172.16.0.1/16)"
echo ""