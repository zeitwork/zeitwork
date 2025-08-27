package nodeagent

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// NetworkManager manages VM network interfaces
type NetworkManager struct {
	// Track TAP interfaces by instance ID
	interfaces map[string]string // instanceID -> TAP name
}

// NewNetworkManager creates a new network manager
func NewNetworkManager() *NetworkManager {
	return &NetworkManager{
		interfaces: make(map[string]string),
	}
}

// CreateTAPInterface creates a TAP interface for a VM
func (nm *NetworkManager) CreateTAPInterface(instanceID string, ipv6Address net.IP) (string, error) {
	// Generate TAP interface name
	tapName := fmt.Sprintf("tap-%s", instanceID[:8])

	// Create TAP interface
	cmd := exec.Command("ip", "tuntap", "add", tapName, "mode", "tap")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create TAP interface: %w", err)
	}

	// Bring up the interface
	cmd = exec.Command("ip", "link", "set", tapName, "up")
	if err := cmd.Run(); err != nil {
		nm.DeleteTAPInterface(tapName)
		return "", fmt.Errorf("failed to bring up TAP interface: %w", err)
	}

	// Add IPv6 route for the VM
	cmd = exec.Command("ip", "-6", "route", "add", fmt.Sprintf("%s/128", ipv6Address), "dev", tapName)
	if err := cmd.Run(); err != nil {
		nm.DeleteTAPInterface(tapName)
		return "", fmt.Errorf("failed to add IPv6 route: %w", err)
	}

	// Enable proxy NDP for the VM's IPv6 address
	cmd = exec.Command("ip", "-6", "neigh", "add", "proxy", ipv6Address.String(), "dev", "eth0")
	if err := cmd.Run(); err != nil {
		// Non-fatal, continue
	}

	// Store the mapping
	nm.interfaces[instanceID] = tapName

	return tapName, nil
}

// DeleteTAPInterface deletes a TAP interface
func (nm *NetworkManager) DeleteTAPInterface(tapName string) error {
	// Delete the interface
	cmd := exec.Command("ip", "link", "delete", tapName)
	return cmd.Run()
}

// CleanupInstance cleans up network resources for an instance
func (nm *NetworkManager) CleanupInstance(instanceID string, ipv6Address net.IP) error {
	tapName, exists := nm.interfaces[instanceID]
	if !exists {
		return nil
	}

	// Remove IPv6 route
	cmd := exec.Command("ip", "-6", "route", "del", fmt.Sprintf("%s/128", ipv6Address))
	cmd.Run() // Ignore errors

	// Remove proxy NDP entry
	cmd = exec.Command("ip", "-6", "neigh", "del", "proxy", ipv6Address.String(), "dev", "eth0")
	cmd.Run() // Ignore errors

	// Delete TAP interface
	err := nm.DeleteTAPInterface(tapName)

	// Remove from tracking
	delete(nm.interfaces, instanceID)

	return err
}

// EnableIPv6Forwarding enables IPv6 forwarding on the host
func EnableIPv6Forwarding() error {
	// Enable IPv6 forwarding
	cmd := exec.Command("sysctl", "-w", "net.ipv6.conf.all.forwarding=1")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable IPv6 forwarding: %w", err)
	}

	// Enable proxy NDP
	cmd = exec.Command("sysctl", "-w", "net.ipv6.conf.all.proxy_ndp=1")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable proxy NDP: %w", err)
	}

	return nil
}

// ConfigureNodeIPv6 configures the node's IPv6 networking
func ConfigureNodeIPv6(nodePrefix string) error {
	// Parse the prefix
	_, ipNet, err := net.ParseCIDR(nodePrefix)
	if err != nil {
		return fmt.Errorf("invalid node prefix: %w", err)
	}

	// Add the prefix to the loopback interface
	// This allows the node to handle packets for the entire /64
	cmd := exec.Command("ip", "-6", "addr", "add", ipNet.String(), "dev", "lo")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if address already exists
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("failed to add IPv6 prefix: %w", err)
		}
	}

	// Ensure IPv6 forwarding is enabled
	if err := EnableIPv6Forwarding(); err != nil {
		return err
	}

	return nil
}

// SetupFirecrackerNetworking prepares network configuration for Firecracker
func SetupFirecrackerNetworking(tapName string, vmMAC string, vmIPv6 net.IP, gatewayIPv6 net.IP) map[string]interface{} {
	return map[string]interface{}{
		"network-interfaces": []map[string]interface{}{
			{
				"iface_id":      "eth0",
				"guest_mac":     vmMAC,
				"host_dev_name": tapName,
			},
		},
		"boot-args": fmt.Sprintf(
			"console=ttyS0 reboot=k panic=1 pci=off init=/init ip=%s::%s:64::eth0:off",
			vmIPv6.String(),
			gatewayIPv6.String(),
		),
	}
}

// GenerateMAC generates a MAC address for a VM
func GenerateMAC(instanceID string) string {
	// Use locally administered MAC address range (02:xx:xx:xx:xx:xx)
	// Hash the instance ID to generate consistent MACs
	hash := 0
	for _, c := range instanceID {
		hash = hash*31 + int(c)
	}

	return fmt.Sprintf("02:%02x:%02x:%02x:%02x:%02x",
		(hash>>24)&0xff,
		(hash>>16)&0xff,
		(hash>>8)&0xff,
		hash&0xff,
		0x01,
	)
}
