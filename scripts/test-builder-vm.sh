#!/bin/bash
set -euo pipefail

# Test script for the Firecracker Builder VM
# This simulates what the node-agent does when building an image

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VM_DIR="${SCRIPT_DIR}/../build/vms"
TEST_DIR="/tmp/builder-test-$$"
SOCKET="/tmp/firecracker-builder-$$.sock"

# Test parameters
TEST_REPO="${1:-https://github.com/docker/getting-started.git}"
TEST_COMMIT="${2:-main}"
TEST_DOCKERFILE="${3:-Dockerfile}"

echo "🧪 Testing Firecracker Builder VM..."
echo "   Repository: $TEST_REPO"
echo "   Commit: $TEST_COMMIT"
echo "   Dockerfile: $TEST_DOCKERFILE"

# Check if builder VM exists
if [ ! -f "${VM_DIR}/builder-rootfs.ext4" ] || [ ! -f "${VM_DIR}/vmlinux" ]; then
    echo "❌ Builder VM not found. Run 'make builder-vm' first."
    exit 1
fi

# Cleanup function
cleanup() {
    echo "Cleaning up..."
    # Kill Firecracker if running
    if [ -S "$SOCKET" ]; then
        curl --unix-socket "$SOCKET" -X PUT 'http://localhost/actions' \
            -H 'Content-Type: application/json' \
            -d '{"action_type": "SendCtrlAltDel"}' 2>/dev/null || true
        sleep 2
    fi
    pkill -f "firecracker.*$SOCKET" 2>/dev/null || true
    rm -f "$SOCKET"
    rm -rf "$TEST_DIR"
}
trap cleanup EXIT

# Create test directory
mkdir -p "$TEST_DIR"

# Create Firecracker configuration
cat > "${TEST_DIR}/config.json" <<EOF
{
  "boot-source": {
    "kernel_image_path": "${VM_DIR}/vmlinux",
    "boot_args": "console=ttyS0 reboot=k panic=1 pci=off repo_url=${TEST_REPO} commit_sha=${TEST_COMMIT} dockerfile=${TEST_DOCKERFILE}",
    "initrd_path": null
  },
  "drives": [
    {
      "drive_id": "rootfs",
      "path_on_host": "${VM_DIR}/builder-rootfs.ext4",
      "is_root_device": true,
      "is_read_only": false
    }
  ],
  "machine-config": {
    "vcpu_count": 2,
    "mem_size_mib": 2048,
    "track_dirty_pages": false
  },
  "network-interfaces": [
    {
      "iface_id": "eth0",
      "guest_mac": "02:FC:00:00:00:01",
      "host_dev_name": "tap0"
    }
  ],
  "logger": {
    "level": "Debug",
    "log_path": "${TEST_DIR}/firecracker.log",
    "show_level": true,
    "show_log_origin": true
  },
  "metrics": {
    "metrics_path": "${TEST_DIR}/metrics.json"
  }
}
EOF

# Create TAP interface (requires sudo)
echo "Creating network interface..."
sudo ip tuntap add tap0 mode tap 2>/dev/null || true
sudo ip addr add 172.16.0.1/24 dev tap0
sudo ip link set tap0 up

# Enable IP forwarding and NAT
sudo sysctl -w net.ipv4.ip_forward=1 > /dev/null
sudo iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE 2>/dev/null || true
sudo iptables -A FORWARD -i tap0 -j ACCEPT 2>/dev/null || true

# Start Firecracker
echo "Starting Firecracker VM..."
rm -f "$SOCKET"
firecracker --api-sock "$SOCKET" --config-file "${TEST_DIR}/config.json" &
FC_PID=$!

# Wait for VM to start
echo "Waiting for VM to boot..."
sleep 5

# Monitor VM output (it will automatically shutdown after build)
echo "VM is building Docker image..."
echo "Check ${TEST_DIR}/firecracker.log for details"

# Wait for VM to complete (max 10 minutes)
TIMEOUT=600
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
    if ! kill -0 $FC_PID 2>/dev/null; then
        echo "VM has shut down"
        break
    fi
    sleep 5
    ELAPSED=$((ELAPSED + 5))
    echo -n "."
done
echo ""

# Check if build was successful
if [ -f "${TEST_DIR}/metrics.json" ]; then
    echo "✅ Builder VM test completed"
    echo "   Metrics: ${TEST_DIR}/metrics.json"
    echo "   Logs: ${TEST_DIR}/firecracker.log"
else
    echo "❌ Builder VM test may have failed"
fi

# Cleanup TAP interface
sudo ip link set tap0 down
sudo ip tuntap del tap0 mode tap 2>/dev/null || true
