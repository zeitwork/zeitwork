#!/bin/bash

# Diagnostic script to test Firecracker directly on a node
# This helps identify why Firecracker might be failing to start

set -e

echo "=== Firecracker Diagnostic Test ==="
echo "Date: $(date)"
echo ""

# Check system requirements
echo "1. Checking system requirements..."

# Check KVM
if [ -e /dev/kvm ]; then
    echo "✓ /dev/kvm exists"
    ls -l /dev/kvm
    if [ -r /dev/kvm ] && [ -w /dev/kvm ]; then
        echo "✓ /dev/kvm is readable and writable"
    else
        echo "✗ /dev/kvm permissions issue"
        echo "  Try: sudo chmod 666 /dev/kvm"
    fi
else
    echo "✗ /dev/kvm not found"
    echo "  KVM is required for Firecracker"
    echo "  Check if virtualization is enabled in BIOS"
    echo "  Check if KVM modules are loaded: lsmod | grep kvm"
fi

echo ""

# Check Firecracker installation
echo "2. Checking Firecracker installation..."
if command -v firecracker &> /dev/null; then
    echo "✓ Firecracker is installed"
    firecracker --version
    which firecracker
else
    echo "✗ Firecracker not found in PATH"
fi

echo ""

# Check kernel
echo "3. Checking kernel..."
KERNEL_PATH="/var/lib/firecracker/kernels/vmlinux"
if [ -f "$KERNEL_PATH" ]; then
    echo "✓ Kernel found at $KERNEL_PATH"
    ls -lh "$KERNEL_PATH"
    file "$KERNEL_PATH" | head -1
else
    echo "✗ Kernel not found at $KERNEL_PATH"
fi

echo ""

# Create a minimal test configuration
echo "4. Creating minimal test configuration..."

TEST_DIR="/tmp/firecracker-test-$$"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Create minimal rootfs
echo "Creating minimal rootfs..."
ROOTFS="$TEST_DIR/rootfs.ext4"
dd if=/dev/zero of="$ROOTFS" bs=1M count=50 status=none
mkfs.ext4 -F "$ROOTFS" &>/dev/null

# Mount and add minimal files
MOUNT_DIR="$TEST_DIR/mnt"
mkdir -p "$MOUNT_DIR"
sudo mount "$ROOTFS" "$MOUNT_DIR"

# Create minimal filesystem structure
sudo mkdir -p "$MOUNT_DIR"/{bin,dev,etc,proc,sys,tmp}
sudo mknod -m 666 "$MOUNT_DIR/dev/null" c 1 3 2>/dev/null || true
sudo mknod -m 666 "$MOUNT_DIR/dev/console" c 5 1 2>/dev/null || true

# Create init
sudo tee "$MOUNT_DIR/init" > /dev/null << 'EOF'
#!/bin/sh
echo "Test VM booted successfully!"
poweroff -f
EOF
sudo chmod +x "$MOUNT_DIR/init"

# Copy busybox if available
if command -v busybox &>/dev/null; then
    sudo cp $(which busybox) "$MOUNT_DIR/bin/busybox"
    sudo ln -s /bin/busybox "$MOUNT_DIR/bin/sh" 2>/dev/null || true
    sudo ln -s /bin/busybox "$MOUNT_DIR/bin/poweroff" 2>/dev/null || true
fi

sudo umount "$MOUNT_DIR"

# Create Firecracker config
CONFIG="$TEST_DIR/config.json"
cat > "$CONFIG" << EOF
{
  "boot-source": {
    "kernel_image_path": "$KERNEL_PATH",
    "boot_args": "console=ttyS0 reboot=k panic=1 pci=off init=/init"
  },
  "drives": [
    {
      "drive_id": "rootfs",
      "path_on_host": "$ROOTFS",
      "is_root_device": true,
      "is_read_only": false
    }
  ],
  "machine-config": {
    "vcpu_count": 1,
    "mem_size_mib": 128,
    "smt": false
  }
}
EOF

echo "Test configuration created in $TEST_DIR"
echo ""

# Try to start Firecracker
echo "5. Testing Firecracker startup..."

SOCKET="$TEST_DIR/firecracker.sock"
LOG="$TEST_DIR/firecracker.log"

echo "Starting Firecracker with:"
echo "  Config: $CONFIG"
echo "  Socket: $SOCKET"
echo "  Log: $LOG"
echo ""

# Start Firecracker
timeout 10 firecracker --api-sock "$SOCKET" --config-file "$CONFIG" > "$LOG" 2>&1 &
FC_PID=$!

echo "Started with PID: $FC_PID"

# Wait a moment
sleep 2

# Check if running
if ps -p $FC_PID > /dev/null 2>&1; then
    echo "✓ Firecracker process is running"
    
    # Wait for socket
    TIMEOUT=5
    ELAPSED=0
    while [ $ELAPSED -lt $TIMEOUT ]; do
        if [ -e "$SOCKET" ]; then
            echo "✓ API socket created"
            break
        fi
        sleep 1
        ELAPSED=$((ELAPSED + 1))
    done
    
    if [ ! -e "$SOCKET" ]; then
        echo "✗ API socket not created after $TIMEOUT seconds"
    fi
    
    # Kill the test process
    kill $FC_PID 2>/dev/null || true
    wait $FC_PID 2>/dev/null || true
else
    echo "✗ Firecracker process died immediately"
fi

echo ""
echo "Log contents:"
echo "============"
if [ -f "$LOG" ]; then
    cat "$LOG"
else
    echo "No log file created"
fi
echo "============"

# Cleanup
echo ""
echo "6. Cleaning up..."
cd /
rm -rf "$TEST_DIR"

echo ""
echo "=== Diagnostic test complete ==="

# Summary
echo ""
echo "Summary:"
echo "--------"
if [ -e /dev/kvm ] && command -v firecracker &>/dev/null && [ -f "$KERNEL_PATH" ]; then
    echo "All basic requirements are met."
    echo "If Firecracker still fails to start, check:"
    echo "  - SELinux/AppArmor policies"
    echo "  - System resource limits (ulimit)"
    echo "  - Kernel compatibility"
    echo "  - Log files for specific errors"
else
    echo "Some requirements are missing. Please fix the issues above."
fi
