#!/bin/bash
set -e

# Accept OPERATOR_URL as first argument
OPERATOR_URL="$1"
if [ -z "$OPERATOR_URL" ]; then
    echo "Error: OPERATOR_URL is required as first argument"
    exit 1
fi

echo "Configuring node agent to connect to operator at: $OPERATOR_URL"

# Enable KVM
sudo modprobe kvm
sudo modprobe kvm_intel 2>/dev/null || sudo modprobe kvm_amd 2>/dev/null || true
echo "kvm" | sudo tee -a /etc/modules
echo "kvm_intel" | sudo tee -a /etc/modules 2>/dev/null || echo "kvm_amd" | sudo tee -a /etc/modules

# Install Firecracker if not present
if [ ! -f /usr/bin/firecracker ]; then
    FC_VERSION="v1.12.1"
    ARCH=$(uname -m)
    cd /tmp
    wget -q https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-${ARCH}.tgz
    tar -xzf firecracker-${FC_VERSION}-${ARCH}.tgz
    sudo cp release-${FC_VERSION}-${ARCH}/firecracker-${FC_VERSION}-${ARCH} /usr/bin/firecracker
    sudo chmod +x /usr/bin/firecracker
fi

# Download kernel if not present
sudo mkdir -p /var/lib/firecracker/kernels
if [ ! -f /var/lib/firecracker/kernels/vmlinux.bin ]; then
    cd /var/lib/firecracker/kernels
    ARCH=$(uname -m)
    sudo wget -q https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/${ARCH}/kernels/vmlinux.bin
fi

# Stop existing service if it's running (ignore errors if it doesn't exist)
echo "Stopping existing node agent service..."
sudo systemctl stop zeitwork-node-agent 2>/dev/null || true

# Give service time to fully stop
sleep 2

# Extract and install node agent (suppress extended header warnings from macOS tar)
cd /tmp
tar -xzf zeitwork-binaries.tar.gz 2>&1 | grep -v "Ignoring unknown extended header" || true
sudo cp -f build/zeitwork-node-agent /usr/local/bin/
sudo chmod +x /usr/local/bin/zeitwork-node-agent

# Create directories
sudo mkdir -p /etc/zeitwork /var/lib/zeitwork /var/log/zeitwork /var/lib/firecracker/vms

# Create service user with KVM access
sudo useradd -r -s /bin/false zeitwork 2>/dev/null || true
sudo usermod -aG kvm zeitwork
sudo chown -R zeitwork:zeitwork /var/lib/zeitwork /var/log/zeitwork /var/lib/firecracker

# Create configuration
cat << EOF | sudo tee /etc/zeitwork/node-agent.env
SERVICE_NAME=zeitwork-node-agent
PORT=8081
OPERATOR_URL=${OPERATOR_URL}
FIRECRACKER_BIN=/usr/bin/firecracker
KERNEL_PATH=/var/lib/firecracker/kernels/vmlinux.bin
VM_DIR=/var/lib/firecracker/vms
LOG_LEVEL=info
ENVIRONMENT=production
EOF

# Create systemd service
sudo tee /etc/systemd/system/zeitwork-node-agent.service << SERVICE
[Unit]
Description=Zeitwork Node Agent Service
After=network.target

[Service]
Type=simple
User=root
EnvironmentFile=/etc/zeitwork/node-agent.env
ExecStart=/usr/local/bin/zeitwork-node-agent
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
SERVICE

# Start service
sudo systemctl daemon-reload
sudo systemctl enable zeitwork-node-agent

# Start service with error handling
echo "Starting node agent service..."
if sudo systemctl start zeitwork-node-agent; then
    echo "Node agent service started successfully"
else
    echo "Failed to start node agent service, checking logs..."
    sudo journalctl -u zeitwork-node-agent --no-pager -n 20
    exit 1
fi

echo "Worker setup complete"
