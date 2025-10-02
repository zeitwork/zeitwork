package network

import (
	"fmt"
	"net"
	"strings"
)

// FirecrackerInternalSubnet is the internal IP range used by Firecracker VMs
// All VMs across all regions use this shared internal address space
// VMs are assigned IPs from 172.16.0.2 onwards, with 172.16.0.1 as the bridge gateway
const FirecrackerInternalSubnet = "172.16.0.0/16"

// RegionalSubnets maps region UUIDs to their subnet ranges
// Note: Currently using Firecracker internal subnet (172.16.0.0/16) for all VMs
// These regional subnets are reserved for future use if needed
var RegionalSubnets = map[string]string{
	"01996e48-2c25-7a67-9507-2126d85bb007": "10.1.0.0/16", // us-west-1 (reserved)
	"01996e48-1d69-742a-af33-631631b2c91d": "10.2.0.0/16", // eu-west-1 (reserved)
}

// SubnetConfig represents a network subnet configuration
type SubnetConfig struct {
	Base string // e.g., "10.1.0.0"
	Mask int    // e.g., 16
	CIDR string // e.g., "10.1.0.0/16"
}

// GetRegionSubnet returns the subnet configuration for a region
func GetRegionSubnet(regionID string) (*SubnetConfig, error) {
	subnet, exists := RegionalSubnets[regionID]
	if !exists {
		return nil, fmt.Errorf("unknown region ID: %s", regionID)
	}

	parts := strings.Split(subnet, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid subnet format: %s", subnet)
	}

	var mask int
	if _, err := fmt.Sscanf(parts[1], "%d", &mask); err != nil {
		return nil, fmt.Errorf("invalid subnet mask: %s", parts[1])
	}

	return &SubnetConfig{
		Base: parts[0],
		Mask: mask,
		CIDR: subnet,
	}, nil
}

// IsIPInRegionalSubnet checks if an IP address belongs to a region's subnet
func IsIPInRegionalSubnet(ipAddr, regionID string) (bool, error) {
	subnetConfig, err := GetRegionSubnet(regionID)
	if err != nil {
		return false, err
	}

	// Parse the subnet
	_, subnet, err := net.ParseCIDR(subnetConfig.CIDR)
	if err != nil {
		return false, fmt.Errorf("failed to parse subnet %s: %w", subnetConfig.CIDR, err)
	}

	// Parse the IP address
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return false, fmt.Errorf("invalid IP address: %s", ipAddr)
	}

	return subnet.Contains(ip), nil
}

// GetBridgeSubnet returns the Docker bridge subnet for a region
// The bridge uses the .0.x subnet within each region
func GetBridgeSubnet(regionID string) (string, error) {
	subnetConfig, err := GetRegionSubnet(regionID)
	if err != nil {
		return "", err
	}

	// Extract base octets (e.g., "10.1" from "10.1.0.0")
	parts := strings.Split(subnetConfig.Base, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid base IP format: %s", subnetConfig.Base)
	}

	// Return .0.x/24 subnet for bridge
	return fmt.Sprintf("%s.%s.0.0/24", parts[0], parts[1]), nil
}

// ValidateRegionalIP validates that an IP address is properly assigned for a region
// Accepts both Firecracker internal IPs (10.77.0.0/16) and regional subnet IPs
func ValidateRegionalIP(ipAddr, regionID string) error {
	// First check if it's a Firecracker internal IP
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return fmt.Errorf("invalid IP address format: %s", ipAddr)
	}

	// Check if IP is in Firecracker internal subnet
	_, firecrackerSubnet, err := net.ParseCIDR(FirecrackerInternalSubnet)
	if err != nil {
		return fmt.Errorf("failed to parse Firecracker subnet: %w", err)
	}

	if firecrackerSubnet.Contains(ip) {
		// Valid Firecracker internal IP
		return nil
	}

	// Fall back to regional subnet validation
	valid, err := IsIPInRegionalSubnet(ipAddr, regionID)
	if err != nil {
		return err
	}

	if !valid {
		subnetConfig, _ := GetRegionSubnet(regionID)
		return fmt.Errorf("IP %s is not in Firecracker subnet (%s) or region %s subnet (%s)",
			ipAddr, FirecrackerInternalSubnet, regionID, subnetConfig.CIDR)
	}

	return nil
}

// GetNetworkBaseOctets returns the first two octets of a regional subnet
// e.g., for "10.1.0.0/16" returns "10.1"
func GetNetworkBaseOctets(regionID string) (string, error) {
	subnetConfig, err := GetRegionSubnet(regionID)
	if err != nil {
		return "", err
	}

	parts := strings.Split(subnetConfig.Base, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid base IP format: %s", subnetConfig.Base)
	}

	return fmt.Sprintf("%s.%s", parts[0], parts[1]), nil
}
