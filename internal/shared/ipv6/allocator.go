package ipv6

import (
	"fmt"
	"net"
	"sync"
)

// Allocator manages IPv6 address allocation for VMs
type Allocator struct {
	mu sync.RWMutex

	// Base prefix for the region (e.g., 2001:db8::/48)
	regionPrefix *net.IPNet

	// Node-specific prefix (e.g., 2001:db8:1::/64)
	nodePrefix *net.IPNet

	// Track allocated addresses
	allocated map[string]bool

	// Next allocation index
	nextIndex uint32
}

// NewAllocator creates a new IPv6 allocator
func NewAllocator(regionPrefix, nodeID string) (*Allocator, error) {
	// Parse region prefix (should be /48)
	_, regionNet, err := net.ParseCIDR(regionPrefix)
	if err != nil {
		return nil, fmt.Errorf("invalid region prefix: %w", err)
	}

	// Generate node-specific /64 from the region /48
	nodePrefix := generateNodePrefix(regionNet, nodeID)

	return &Allocator{
		regionPrefix: regionNet,
		nodePrefix:   nodePrefix,
		allocated:    make(map[string]bool),
		nextIndex:    1, // Start from ::1
	}, nil
}

// AllocateAddress allocates a new IPv6 address for a VM
func (a *Allocator) AllocateAddress() (net.IP, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Find next available address
	for i := uint32(0); i < 65536; i++ {
		candidate := a.generateAddress(a.nextIndex + i)
		if !a.allocated[candidate.String()] {
			a.allocated[candidate.String()] = true
			a.nextIndex = a.nextIndex + i + 1
			return candidate, nil
		}
	}

	return nil, fmt.Errorf("no available IPv6 addresses")
}

// ReleaseAddress releases an allocated IPv6 address
func (a *Allocator) ReleaseAddress(addr net.IP) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.allocated, addr.String())
}

// GetNodePrefix returns the node's /64 prefix
func (a *Allocator) GetNodePrefix() *net.IPNet {
	return a.nodePrefix
}

// generateAddress generates an IPv6 address from an index
func (a *Allocator) generateAddress(index uint32) net.IP {
	ip := make(net.IP, 16)
	copy(ip, a.nodePrefix.IP)

	// Set the last 4 bytes to the index
	ip[12] = byte(index >> 24)
	ip[13] = byte(index >> 16)
	ip[14] = byte(index >> 8)
	ip[15] = byte(index)

	return ip
}

// generateNodePrefix generates a /64 prefix for a node from a region /48
func generateNodePrefix(regionNet *net.IPNet, nodeID string) *net.IPNet {
	// Simple hash-based allocation for demo
	// In production, this would be more sophisticated

	ip := make(net.IP, 16)
	copy(ip, regionNet.IP)

	// Use node ID to determine the subnet within the /48
	// This gives us 2^16 possible /64 networks
	nodeIndex := hashNodeID(nodeID)
	ip[6] = byte(nodeIndex >> 8)
	ip[7] = byte(nodeIndex)

	return &net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(64, 128),
	}
}

// hashNodeID generates a consistent index from a node ID
func hashNodeID(nodeID string) uint16 {
	var hash uint32
	for _, c := range nodeID {
		hash = hash*31 + uint32(c)
	}
	return uint16(hash)
}

// GetRegionPrefixes returns the standard region prefixes
func GetRegionPrefixes() map[string]string {
	return map[string]string{
		"eu-central-1":     "2001:db8:1::/48",
		"us-east-1":        "2001:db8:2::/48",
		"asia-southeast-1": "2001:db8:3::/48",
	}
}
