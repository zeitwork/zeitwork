package firecracker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NetworkAgent handles automatic IPv6 configuration inside Firecracker VMs
type NetworkAgent struct {
	logger interface{} // Will use slog.Logger when available
}

// enhanceVMRootfs modifies the VM rootfs to include our network configuration agent
func (r *Runtime) enhanceVMRootfs(ctx context.Context) error {
	rootfsPath := r.cfg.DefaultRootfsPath
	if rootfsPath == "" {
		rootfsPath = "/var/lib/firecracker-containerd/runtime/default-rootfs.img"
	}

	r.logger.Info("Enhancing VM rootfs with network agent", "rootfs", rootfsPath)

	// Create enhanced rootfs with network agent
	enhancedPath := strings.Replace(rootfsPath, ".img", "-enhanced.img", 1)

	// Check if enhanced rootfs already exists and is recent
	if stat, err := os.Stat(enhancedPath); err == nil {
		if originalStat, err2 := os.Stat(rootfsPath); err2 == nil {
			if stat.ModTime().After(originalStat.ModTime()) {
				r.logger.Debug("Enhanced rootfs already exists and is up to date")
				return r.updateRuntimeConfig(enhancedPath)
			}
		}
	}

	// Create the enhanced rootfs
	if err := r.createEnhancedRootfs(ctx, rootfsPath, enhancedPath); err != nil {
		return fmt.Errorf("failed to create enhanced rootfs: %w", err)
	}

	// Update runtime config to use enhanced rootfs
	return r.updateRuntimeConfig(enhancedPath)
}

// createEnhancedRootfs creates a new rootfs with network agent included
func (r *Runtime) createEnhancedRootfs(ctx context.Context, originalPath, enhancedPath string) error {
	tmpDir, err := os.MkdirTemp("", "fc-rootfs-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// Extract original squashfs
	extractDir := filepath.Join(tmpDir, "rootfs")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return err
	}

	r.logger.Info("Extracting original rootfs", "path", originalPath)
	extractCmd := exec.CommandContext(ctx, "unsquashfs", "-f", "-d", extractDir, originalPath)
	if out, err := extractCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract rootfs: %v: %s", err, string(out))
	}

	// Add network configuration agent
	agentDir := filepath.Join(extractDir, "usr", "local", "bin")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return err
	}

	// Create the network agent script
	agentScript := r.createNetworkAgentScript()
	agentPath := filepath.Join(agentDir, "zeitwork-network-agent")
	if err := os.WriteFile(agentPath, []byte(agentScript), 0755); err != nil {
		return err
	}

	// Create systemd service to run agent on boot
	serviceDir := filepath.Join(extractDir, "etc", "systemd", "system")
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return err
	}

	serviceContent := `[Unit]
Description=Zeitwork Network Configuration Agent
After=network.target
Wants=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/zeitwork-network-agent
RemainAfterExit=yes
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`

	servicePath := filepath.Join(serviceDir, "zeitwork-network.service")
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return err
	}

	// Enable the service by creating symlink
	symlinkDir := filepath.Join(extractDir, "etc", "systemd", "system", "multi-user.target.wants")
	if err := os.MkdirAll(symlinkDir, 0755); err != nil {
		return err
	}

	symlinkPath := filepath.Join(symlinkDir, "zeitwork-network.service")
	if err := os.Symlink("../zeitwork-network.service", symlinkPath); err != nil {
		return err
	}

	// Rebuild squashfs
	r.logger.Info("Building enhanced rootfs", "path", enhancedPath)
	buildCmd := exec.CommandContext(ctx, "mksquashfs", extractDir, enhancedPath, "-comp", "gzip", "-noappend")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build enhanced rootfs: %v: %s", err, string(out))
	}

	r.logger.Info("Enhanced rootfs created successfully", "path", enhancedPath)
	return nil
}

// createNetworkAgentScript creates the script that configures IPv6 inside the VM
func (r *Runtime) createNetworkAgentScript() string {
	return `#!/bin/bash
set -e

# Zeitwork Network Configuration Agent
# This script runs inside Firecracker VMs to configure IPv6 networking

LOG_PREFIX="[zeitwork-network]"

log() {
    echo "$LOG_PREFIX $1" | systemd-cat -t zeitwork-network -p info
}

error() {
    echo "$LOG_PREFIX ERROR: $1" | systemd-cat -t zeitwork-network -p err
}

debug() {
    echo "$LOG_PREFIX DEBUG: $1" | systemd-cat -t zeitwork-network -p debug
}

log "Starting network configuration"

# Wait for eth0 to appear
for i in {1..30}; do
    if [ -e /sys/class/net/eth0 ]; then
        log "eth0 interface found"
        break
    fi
    debug "Waiting for eth0 interface... ($i/30)"
    sleep 1
done

if [ ! -e /sys/class/net/eth0 ]; then
    error "eth0 interface not found after 30 seconds"
    exit 1
fi

# Install networking tools if not available
if ! command -v ip >/dev/null 2>&1; then
    log "Installing networking tools"
    export DEBIAN_FRONTEND=noninteractive
    
    # Update package lists
    if apt-get update >/dev/null 2>&1; then
        # Install iproute2 and related tools
        if apt-get install -y iproute2 iputils-ping net-tools >/dev/null 2>&1; then
            log "Networking tools installed successfully"
        else
            error "Failed to install networking tools"
            exit 1
        fi
    else
        error "Failed to update package lists"
        exit 1
    fi
fi

# Try to discover IPv6 address from environment or CNI lease
IPV6_ADDR=""

# Method 1: Check if IPv6 was passed via environment
if [ -n "${ZEITWORK_IPV6:-}" ]; then
    IPV6_ADDR="$ZEITWORK_IPV6"
    log "Using IPv6 from environment: $IPV6_ADDR"
fi

# Method 2: Try to discover from host CNI state (if mounted)
if [ -z "$IPV6_ADDR" ] && [ -d "/host/var/lib/cni/networks/fcnet6" ]; then
    # Look for our VM ID in the lease files
    VMID=$(dmesg | grep -o 'firecracker.*vmm.*id=[a-f0-9-]*' | head -1 | cut -d= -f2 || echo "")
    if [ -n "$VMID" ]; then
        for lease_file in /host/var/lib/cni/networks/fcnet6/fd00:fc::*; do
            if [ -f "$lease_file" ] && grep -q "$VMID" "$lease_file" 2>/dev/null; then
                IPV6_ADDR=$(basename "$lease_file")
                log "Discovered IPv6 from CNI lease: $IPV6_ADDR"
                break
            fi
        done
    fi
fi

# Method 3: Generate fallback IPv6 if still not found
if [ -z "$IPV6_ADDR" ]; then
    # Generate a deterministic IPv6 based on hostname/container ID
    CONTAINER_ID=$(hostname || echo "unknown")
    # Simple hash to generate IPv6 suffix
    HASH=$(echo -n "$CONTAINER_ID" | sha256sum | cut -c1-8)
    IPV6_ADDR="fd00:fc::${HASH:0:4}:${HASH:4:4}"
    log "Generated fallback IPv6: $IPV6_ADDR"
fi

log "Configuring IPv6 address: $IPV6_ADDR"

# Configure networking
log "Bringing up eth0 interface"
ip link set eth0 up || {
    error "Failed to bring up eth0"
    exit 1
}

log "Adding IPv6 address"
ip -6 addr add ${IPV6_ADDR}/64 dev eth0 || {
    error "Failed to add IPv6 address"
    exit 1
}

log "Adding default route"
ip -6 route add default via fd00:fc::1 dev eth0 || {
    error "Failed to add default route"
    exit 1
}

# Verify configuration
log "Network configuration complete"
ip -6 addr show dev eth0 | systemd-cat -t zeitwork-network -p info
ip -6 route show | systemd-cat -t zeitwork-network -p info

# Save the configured IP for other processes
echo "$IPV6_ADDR" > /tmp/zeitwork-ipv6.addr

log "Network agent completed successfully"
exit 0
`
}

// updateRuntimeConfig updates the firecracker runtime config to use the enhanced rootfs
func (r *Runtime) updateRuntimeConfig(enhancedRootfsPath string) error {
	// This would update the runtime JSON to use the enhanced rootfs
	// For now, we'll just log what we would do
	r.logger.Info("Would update runtime config to use enhanced rootfs", "path", enhancedRootfsPath)

	// TODO: Actually update /etc/containerd/firecracker-runtime.json
	// to point to the enhanced rootfs instead of the default one

	return nil
}
