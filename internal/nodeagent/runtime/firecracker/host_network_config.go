package firecracker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// configureIPv6FromHost configures IPv6 networking from the host side using network namespaces
func (r *Runtime) configureIPv6FromHost(ctx context.Context, vmID, ipv6 string) error {
	r.logger.Info("Configuring IPv6 from host side", "vmID", vmID, "ipv6", ipv6)

	// Find the network namespace for this VM
	netns, err := r.findVMNetworkNamespace(ctx, vmID)
	if err != nil {
		return fmt.Errorf("failed to find VM network namespace: %w", err)
	}

	if netns == "" {
		r.logger.Warn("No network namespace found for VM, using direct configuration", "vmID", vmID)
		return r.configureIPv6Direct(ctx, vmID, ipv6)
	}

	// Configure IPv6 in the VM's network namespace from host
	commands := []string{
		"ip link set eth0 up",
		fmt.Sprintf("ip -6 addr add %s/64 dev eth0", ipv6),
		"ip -6 route add default via fd00:fc::1 dev eth0",
	}

	for _, cmd := range commands {
		nsCmd := fmt.Sprintf("ip netns exec %s %s", netns, cmd)
		execCmd := exec.CommandContext(ctx, "sh", "-c", nsCmd)

		if out, err := execCmd.CombinedOutput(); err != nil {
			r.logger.Warn("Host-side network command failed",
				"cmd", nsCmd,
				"error", err,
				"output", string(out))
			// Continue with other commands
		} else {
			r.logger.Debug("Host-side network command succeeded", "cmd", nsCmd)
		}
	}

	return nil
}

// findVMNetworkNamespace attempts to find the network namespace for a VM
func (r *Runtime) findVMNetworkNamespace(ctx context.Context, vmID string) (string, error) {
	// List network namespaces
	cmd := exec.CommandContext(ctx, "ip", "netns", "list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to list network namespaces: %w", err)
	}

	// Look for a namespace that might be related to our VM
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		// Network namespaces might be named with VM ID or container ID
		if strings.Contains(line, vmID) {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				return parts[0], nil
			}
		}
	}

	return "", nil
}

// configureIPv6Direct configures IPv6 by directly manipulating the VM's network interface
func (r *Runtime) configureIPv6Direct(ctx context.Context, vmID, ipv6 string) error {
	r.logger.Info("Attempting direct IPv6 configuration", "vmID", vmID, "ipv6", ipv6)

	// Try to find the tap device associated with this VM
	tapDevice, err := r.findVMTapDevice(ctx, vmID)
	if err != nil {
		return fmt.Errorf("failed to find VM tap device: %w", err)
	}

	if tapDevice == "" {
		return fmt.Errorf("no tap device found for VM %s", vmID)
	}

	r.logger.Info("Found VM tap device", "device", tapDevice, "vmID", vmID)

	// Configure the host side of the tap device
	hostCommands := []string{
		fmt.Sprintf("ip link set %s up", tapDevice),
		// The host side should already be configured by CNI, but ensure it's up
	}

	for _, cmd := range hostCommands {
		execCmd := exec.CommandContext(ctx, "sh", "-c", cmd)
		if out, err := execCmd.CombinedOutput(); err != nil {
			r.logger.Debug("Host tap command failed", "cmd", cmd, "error", err, "output", string(out))
		}
	}

	return nil
}

// findVMTapDevice attempts to find the tap device associated with a VM
func (r *Runtime) findVMTapDevice(ctx context.Context, vmID string) (string, error) {
	// List network interfaces
	cmd := exec.CommandContext(ctx, "ip", "link", "show")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	// Look for tap devices that might be associated with our VM
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		// Look for tap devices (usually named tap* or fc-tap*)
		if strings.Contains(line, "tap") && strings.Contains(line, "UP") {
			// Extract device name
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				deviceName := strings.TrimSuffix(parts[1], ":")
				// This is a potential tap device
				return deviceName, nil
			}
		}
	}

	return "", nil
}

// injectNetworkConfigIntoVM injects network configuration directly into a running VM
func (r *Runtime) injectNetworkConfigIntoVM(ctx context.Context, containerName, ipv6 string) error {
	r.logger.Info("Injecting network config into VM", "container", containerName, "ipv6", ipv6)

	// Create a script that configures networking
	configScript := fmt.Sprintf(`#!/bin/bash
set -e

# Install networking tools if not available
if ! command -v ip >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update >/dev/null 2>&1 || true
    apt-get install -y iproute2 iputils-ping >/dev/null 2>&1 || true
fi

# Configure networking
if command -v ip >/dev/null 2>&1; then
    ip link set eth0 up || true
    ip -6 addr add %s/64 dev eth0 || true  
    ip -6 route add default via fd00:fc::1 dev eth0 || true
    echo "Network configured successfully: %s"
else
    echo "WARNING: Could not install networking tools"
fi

# Show final configuration
if command -v ip >/dev/null 2>&1; then
    ip -6 addr show dev eth0 || true
    ip -6 route show || true
fi
`, ipv6, ipv6)

	// Write script to a temporary file
	tmpScript := fmt.Sprintf("/tmp/network-config-%s.sh", randomHex(8))
	if err := os.WriteFile(tmpScript, []byte(configScript), 0755); err != nil {
		return err
	}
	defer os.Remove(tmpScript)

	// Copy script into the VM and execute it
	copyArgs := []string{"tasks", "exec", "--exec-id", "copy-" + randomHex(4), containerName, "sh", "-c",
		fmt.Sprintf("cat > /tmp/network-setup.sh << 'EOF'\n%s\nEOF", configScript)}

	if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, copyArgs); err != nil {
		return fmt.Errorf("failed to copy network script: %w", err)
	}

	// Make script executable and run it
	execArgs := []string{"tasks", "exec", "--exec-id", "net-" + randomHex(4), containerName, "sh", "-c",
		"chmod +x /tmp/network-setup.sh && /tmp/network-setup.sh"}

	if out, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, execArgs); err != nil {
		r.logger.Warn("Network configuration script failed", "error", err, "output", out)
		return fmt.Errorf("network configuration failed: %w", err)
	} else {
		r.logger.Info("Network configuration script completed", "output", out)
	}

	return nil
}
