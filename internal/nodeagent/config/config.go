package config

import (
	"time"
)

// NOTE: This package contains legacy config types used by the runtime system.
// The actual configuration loading is now handled by the shared config package.

// RuntimeConfig defines the runtime configuration
type RuntimeConfig struct {
	Mode              string // "development" or "production"
	DockerConfig      *DockerRuntimeConfig
	FirecrackerConfig *FirecrackerRuntimeConfig
}

// DockerRuntimeConfig contains Docker-specific configuration
type DockerRuntimeConfig struct {
	Endpoint          string        // Docker daemon endpoint
	NetworkName       string        // Docker network for containers
	ImageRegistry     string        // Container image registry
	PullTimeout       time.Duration // Timeout for image pulls
	StartTimeout      time.Duration // Timeout for container starts
	StopTimeout       time.Duration // Timeout for graceful stops
	EnableAutoCleanup bool          // Auto-cleanup stopped containers
}

// FirecrackerRuntimeConfig contains configuration for firecracker-containerd
type FirecrackerRuntimeConfig struct {
	// firecracker-containerd daemon configuration
	ContainerdSocket    string // Path to firecracker-containerd socket
	ContainerdNamespace string // Containerd namespace to use
	RuntimeConfigPath   string // Path to firecracker runtime config JSON

	// Container resource defaults
	DefaultVCpus    int32 // Default vCPUs per container
	DefaultMemoryMB int32 // Default memory per container in MB

	// Networking (CNI-based)
	CNIConfDir       string // CNI configuration directory
	CNIBinDir        string // CNI plugin binaries directory
	CNIStateDir      string // CNI state/cache directory (for host-local IPAM leases)
	NetworkNamespace string // Network namespace for containers
	NetworkName      string // CNI network name used by firecracker-containerd (matches runtime JSON)

	// Timeouts
	StartTimeout time.Duration // Timeout for container startup
	StopTimeout  time.Duration // Timeout for container shutdown
	PullTimeout  time.Duration // Timeout for image pulls

	// Image configuration
	DefaultKernelPath string // Default kernel image path
	DefaultRootfsPath string // Default VM base rootfs image path (standalone Zeitwork VM)
	ImageRegistry     string // Container image registry

	// Auto-setup configuration
	EnableAutoSetup bool   // Enable automatic setup and validation on startup
	EnableJailer    bool   // Enable jailer (recommended: true)
	BuildMethod     string // Kernel/rootfs build method: "firecracker-devtool", "manual", "skip"
}
