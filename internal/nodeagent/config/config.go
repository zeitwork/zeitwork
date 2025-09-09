package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the configuration for the node agent service
type Config struct {
	Port         string
	NodeID       string
	RegionID     string
	DatabaseURL  string
	PollInterval time.Duration
	Runtime      *RuntimeConfig
}

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
	NetworkNamespace string // Network namespace for containers

	// Timeouts
	StartTimeout time.Duration // Timeout for container startup
	StopTimeout  time.Duration // Timeout for container shutdown

	// Image configuration
	DefaultKernelPath string // Default kernel image path
	DefaultRootfsPath string // Default VM base rootfs image path (with agent)
	ImageRegistry     string // Container image registry

	// Auto-setup configuration
	EnableAutoSetup bool   // Enable automatic setup and validation on startup
	EnableJailer    bool   // Enable jailer (recommended: true)
	BuildMethod     string // Kernel/rootfs build method: "firecracker-devtool", "manual", "skip"
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	nodeID := os.Getenv("NODEAGENT_NODE_ID")
	if nodeID == "" {
		return nil, fmt.Errorf("NODEAGENT_NODE_ID environment variable is required")
	}

	regionID := os.Getenv("NODEAGENT_REGION_ID")
	if regionID == "" {
		return nil, fmt.Errorf("NODEAGENT_REGION_ID environment variable is required")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is required")
	}

	pollInterval := getEnvDuration("POLL_INTERVAL", 10*time.Second)

	runtimeConfig, err := loadRuntimeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load runtime config: %w", err)
	}

	return &Config{
		Port:         getEnvOrDefault("PORT", "8081"),
		NodeID:       nodeID,
		RegionID:     regionID,
		DatabaseURL:  databaseURL,
		PollInterval: pollInterval,
		Runtime:      runtimeConfig,
	}, nil
}

// loadRuntimeConfig loads runtime-specific configuration
func loadRuntimeConfig() (*RuntimeConfig, error) {
	mode := getEnvOrDefault("RUNTIME_MODE", "development")

	config := &RuntimeConfig{
		Mode: mode,
	}

	switch mode {
	case "development":
		config.DockerConfig = &DockerRuntimeConfig{
			Endpoint:          getEnvOrDefault("DOCKER_ENDPOINT", "unix:///var/run/docker.sock"),
			NetworkName:       getEnvOrDefault("DOCKER_NETWORK", "zeitwork"),
			ImageRegistry:     getEnvOrDefault("IMAGE_REGISTRY", "localhost:5001"),
			PullTimeout:       getEnvDuration("DOCKER_PULL_TIMEOUT", 5*time.Minute),
			StartTimeout:      getEnvDuration("DOCKER_START_TIMEOUT", 30*time.Second),
			StopTimeout:       getEnvDuration("DOCKER_STOP_TIMEOUT", 10*time.Second),
			EnableAutoCleanup: getEnvBool("DOCKER_AUTO_CLEANUP", true),
		}
	case "production":
		config.FirecrackerConfig = &FirecrackerRuntimeConfig{
			// firecracker-containerd configuration
			ContainerdSocket:    getEnvOrDefault("FIRECRACKER_CONTAINERD_SOCKET", "/run/firecracker-containerd/containerd.sock"),
			ContainerdNamespace: getEnvOrDefault("FIRECRACKER_CONTAINERD_NAMESPACE", "zeitwork"),
			RuntimeConfigPath:   getEnvOrDefault("FIRECRACKER_RUNTIME_CONFIG", "/etc/containerd/firecracker-runtime.json"),

			// Resource defaults
			DefaultVCpus:    int32(getEnvInt("DEFAULT_VCPUS", 1)),
			DefaultMemoryMB: int32(getEnvInt("DEFAULT_MEMORY_MB", 128)),

			// CNI networking
			CNIConfDir:       getEnvOrDefault("CNI_CONF_DIR", "/etc/cni/net.d"),
			CNIBinDir:        getEnvOrDefault("CNI_BIN_DIR", "/opt/cni/bin"),
			NetworkNamespace: getEnvOrDefault("NETWORK_NAMESPACE", "zeitwork"),

			// Timeouts
			StartTimeout: getEnvDuration("FC_START_TIMEOUT", 60*time.Second),
			StopTimeout:  getEnvDuration("FC_STOP_TIMEOUT", 30*time.Second),

			// Image configuration
			DefaultKernelPath: getEnvOrDefault("DEFAULT_KERNEL_PATH", "/var/lib/firecracker-containerd/runtime/default-vmlinux.bin"),
			DefaultRootfsPath: getEnvOrDefault("DEFAULT_ROOTFS_PATH", "/var/lib/firecracker-containerd/runtime/default-rootfs.ext4"),
			ImageRegistry:     getEnvOrDefault("IMAGE_REGISTRY", "localhost:5001"),

			// Auto-setup and Security
			EnableAutoSetup: getEnvBool("ENABLE_AUTO_SETUP", true),
			EnableJailer:    getEnvBool("ENABLE_JAILER", true),
			BuildMethod:     getEnvOrDefault("BUILD_METHOD", "firecracker-devtool"),
		}
	default:
		return nil, fmt.Errorf("unsupported runtime mode: %s", mode)
	}

	return config, nil
}

// Helper functions for environment variable parsing
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
