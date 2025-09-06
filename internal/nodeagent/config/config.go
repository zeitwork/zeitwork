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

// FirecrackerRuntimeConfig contains Firecracker-specific configuration
type FirecrackerRuntimeConfig struct {
	// TODO: Implement Firecracker configuration
	// ContainerdEndpoint string
	// SnapshotterName    string
	// NetworkNamespace   string
	// KernelImagePath    string
	// RootfsImagePath    string
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
			ImageRegistry:     getEnvOrDefault("IMAGE_REGISTRY", "localhost:5000"),
			PullTimeout:       getEnvDuration("DOCKER_PULL_TIMEOUT", 5*time.Minute),
			StartTimeout:      getEnvDuration("DOCKER_START_TIMEOUT", 30*time.Second),
			StopTimeout:       getEnvDuration("DOCKER_STOP_TIMEOUT", 10*time.Second),
			EnableAutoCleanup: getEnvBool("DOCKER_AUTO_CLEANUP", true),
		}
	case "production":
		config.FirecrackerConfig = &FirecrackerRuntimeConfig{
			// TODO: Implement Firecracker configuration loading
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
