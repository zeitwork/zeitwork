package types

import (
	"context"
	"time"
)

// Runtime defines the interface for managing VM instances
// This abstraction allows switching between Docker (development) and Firecracker (production)
type Runtime interface {
	// Lifecycle operations
	CreateInstance(ctx context.Context, spec *InstanceSpec) (*Instance, error)
	StartInstance(ctx context.Context, instance *Instance) error
	StopInstance(ctx context.Context, instance *Instance) error
	DeleteInstance(ctx context.Context, instance *Instance) error

	// State queries
	GetInstanceState(ctx context.Context, instance *Instance) (InstanceState, error)
	ListInstances(ctx context.Context) ([]*Instance, error)
	IsInstanceRunning(ctx context.Context, instance *Instance) (bool, error)

	// Resource management
	GetStats(ctx context.Context, instance *Instance) (*InstanceStats, error)

	// Operations
	ExecuteCommand(ctx context.Context, instance *Instance, cmd []string) (string, error)
	GetLogs(ctx context.Context, instance *Instance, lines int) ([]string, error)

	// Cleanup and maintenance
	CleanupOrphanedInstances(ctx context.Context, desiredInstances []*Instance) error

	// Runtime information
	GetRuntimeInfo() *RuntimeInfo
}

// InstanceSpec defines the specification for creating an instance
type InstanceSpec struct {
	ID                   string
	ImageID              string
	ImageTag             string
	Resources            *ResourceSpec
	EnvironmentVariables map[string]string
	NetworkConfig        *NetworkConfig
	VolumeConfig         *VolumeConfig
}

// Instance represents a running VM instance
type Instance struct {
	ID          string
	ImageID     string
	ImageTag    string
	State       InstanceState
	Resources   *ResourceSpec
	EnvVars     map[string]string
	NetworkInfo *NetworkInfo
	CreatedAt   time.Time
	StartedAt   *time.Time
	RuntimeID   string // Docker container ID or Firecracker VM ID
}

// InstanceState represents the state of an instance
type InstanceState string

const (
	InstanceStatePending    InstanceState = "pending"
	InstanceStateCreating   InstanceState = "creating"
	InstanceStateStarting   InstanceState = "starting"
	InstanceStateRunning    InstanceState = "running"
	InstanceStateStopping   InstanceState = "stopping"
	InstanceStateStopped    InstanceState = "stopped"
	InstanceStateFailed     InstanceState = "failed"
	InstanceStateTerminated InstanceState = "terminated"
)

// ResourceSpec defines resource constraints for an instance
type ResourceSpec struct {
	VCPUs       int32   // Number of virtual CPUs
	Memory      int32   // Memory in MB
	DiskSize    int64   // Disk size in bytes (optional)
	CPULimit    float64 // CPU limit as percentage (0.0-1.0)
	MemoryLimit int64   // Memory limit in bytes
}

// NetworkConfig defines network configuration for an instance
type NetworkConfig struct {
	DefaultPort  int32             // Default port to expose
	PortMappings map[int32]int32   // Internal port -> External port mapping (not used in development)
	NetworkName  string            // Network to connect to
	DNSServers   []string          // Custom DNS servers
	ExtraHosts   map[string]string // Extra host entries
}

// VolumeConfig defines volume configuration for an instance
type VolumeConfig struct {
	Mounts []VolumeMount // Volume mounts
}

// VolumeMount represents a volume mount
type VolumeMount struct {
	Source      string // Source path on host
	Destination string // Destination path in container
	ReadOnly    bool   // Whether mount is read-only
}

// NetworkInfo contains runtime network information
type NetworkInfo struct {
	IPAddress    string          // Internal IP address (IPv4 in development, IPv6 in production)
	DefaultPort  int32           // Default port to expose
	PortMappings map[int32]int32 // Actual port mappings (not used in development)
	NetworkID    string          // Network ID
}

// InstanceStats represents resource usage statistics for an instance
type InstanceStats struct {
	InstanceID string    `json:"instance_id"`
	Timestamp  time.Time `json:"timestamp"`

	// CPU usage
	CPUPercent float64 `json:"cpu_percent"`
	CPUUsage   uint64  `json:"cpu_usage_ns"`

	// Memory usage
	MemoryUsed    uint64  `json:"memory_used"`
	MemoryLimit   uint64  `json:"memory_limit"`
	MemoryPercent float64 `json:"memory_percent"`

	// Network usage
	NetworkRxBytes uint64 `json:"network_rx_bytes"`
	NetworkTxBytes uint64 `json:"network_tx_bytes"`

	// Disk usage
	DiskReadBytes  uint64 `json:"disk_read_bytes"`
	DiskWriteBytes uint64 `json:"disk_write_bytes"`
	DiskUsed       uint64 `json:"disk_used"`
}

// RuntimeInfo provides information about the runtime
type RuntimeInfo struct {
	Type    string `json:"type"`    // "docker" or "firecracker"
	Version string `json:"version"` // Runtime version
	Status  string `json:"status"`  // Runtime status
}

// InstanceHealth represents health check results for an instance
type InstanceHealth struct {
	InstanceID string    `json:"instance_id"`
	Healthy    bool      `json:"healthy"`
	Message    string    `json:"message"`
	LastCheck  time.Time `json:"last_check"`
	CheckType  string    `json:"check_type"` // "tcp", "http", "exec"
}
