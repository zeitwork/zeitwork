package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// BaseConfig contains common configuration for all services
type BaseConfig struct {
	ServiceName string
	LogLevel    string
	Environment string // development, staging, production
}

// NodeAgentConfig contains configuration for the node agent service
type NodeAgentConfig struct {
	BaseConfig
	NodeID      string
	DatabaseURL string
}

// EdgeProxyConfig contains configuration for the edge proxy service
type EdgeProxyConfig struct {
	BaseConfig
	DatabaseURL string
}

type BuilderConfig struct {
	BaseConfig
	DatabaseURL         string
	BuildPollInterval   time.Duration // How often to check for pending builds
	BuildTimeout        time.Duration // Maximum time for a single build
	MaxConcurrentBuilds int           // Maximum number of concurrent builds

	// Image builder configuration
	BuilderType       string // Type of builder to use (docker, firecracker, etc.)
	BuildWorkDir      string // Directory where builds are performed
	ContainerRegistry string // Container registry to push images to
}

// NATSConfig contains configuration for NATS messaging
type NATSConfig struct {
	URLs          []string      // NATS server URLs
	MaxReconnects int           // Maximum number of reconnect attempts (-1 for unlimited)
	ReconnectWait time.Duration // Time to wait between reconnect attempts
	Timeout       time.Duration // Connection timeout
}

// LoadNodeAgentConfig loads configuration for the node agent service
func LoadNodeAgentConfig() (*NodeAgentConfig, error) {
	config := &NodeAgentConfig{
		BaseConfig:  loadBaseConfigWithPrefix("NODEAGENT", "node-agent"),
		DatabaseURL: getEnvWithPrefix("NODEAGENT", "DATABASE_URL", "postgres://localhost/zeitwork"),
		NodeID:      getEnvWithPrefix("NODEAGENT", "NODE_ID", ""),
	}

	return config, nil
}

// LoadEdgeProxyConfig loads configuration for the edge proxy service
func LoadEdgeProxyConfig() (*EdgeProxyConfig, error) {
	config := &EdgeProxyConfig{
		BaseConfig:  loadBaseConfigWithPrefix("EDGEPROXY", "edge-proxy"),
		DatabaseURL: getEnvWithPrefix("EDGEPROXY", "DATABASE_URL", "postgres://localhost/zeitwork"),
	}

	if config.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return config, nil
}

// LoadBuilderConfig loads configuration for the builder service
func LoadBuilderConfig() (*BuilderConfig, error) {
	buildPollIntervalMs, _ := strconv.Atoi(getEnvWithPrefix("BUILDER", "BUILD_POLL_INTERVAL_MS", "5000"))
	buildTimeoutMs, _ := strconv.Atoi(getEnvWithPrefix("BUILDER", "BUILD_TIMEOUT_MS", "1800000")) // 30 minutes default
	maxConcurrentBuilds, _ := strconv.Atoi(getEnvWithPrefix("BUILDER", "MAX_CONCURRENT_BUILDS", "3"))

	config := &BuilderConfig{
		BaseConfig:          loadBaseConfigWithPrefix("BUILDER", "builder"),
		DatabaseURL:         getEnvWithPrefix("BUILDER", "DATABASE_URL", "postgres://localhost/zeitwork"),
		BuildPollInterval:   time.Duration(buildPollIntervalMs) * time.Millisecond,
		BuildTimeout:        time.Duration(buildTimeoutMs) * time.Millisecond,
		MaxConcurrentBuilds: maxConcurrentBuilds,

		// Image builder configuration
		BuilderType:       getEnvWithPrefix("BUILDER", "TYPE", "docker"),
		BuildWorkDir:      getEnvWithPrefix("BUILDER", "WORK_DIR", "/tmp/zeitwork-builds"),
		ContainerRegistry: getEnvWithPrefix("BUILDER", "CONTAINER_REGISTRY", ""),
	}

	if config.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return config, nil
}

// LoadCertsConfig loads configuration for the certs service
func LoadCertsConfig() (*BaseConfig, error) {
	config := loadBaseConfigWithPrefix("CERTS", "certs")
	return &config, nil
}

// LoadListenerConfig loads configuration for the listener service
func LoadListenerConfig() (*BaseConfig, error) {
	config := loadBaseConfigWithPrefix("LISTENER", "listener")
	return &config, nil
}

// loadBaseConfigWithPrefix loads common configuration for all services with service prefix support
func loadBaseConfigWithPrefix(servicePrefix, serviceName string) BaseConfig {
	return BaseConfig{
		ServiceName: serviceName,
		LogLevel:    getEnvWithPrefix(servicePrefix, "LOG_LEVEL", "info"),
		Environment: getEnvWithPrefix(servicePrefix, "ENVIRONMENT", "development"),
	}
}

// LoadNATSConfig loads NATS configuration with dev-local defaults
func LoadNATSConfig() (*NATSConfig, error) {
	return LoadNATSConfigWithPrefix("")
}

// LoadNATSConfigWithPrefix loads NATS configuration with service prefix support
func LoadNATSConfigWithPrefix(servicePrefix string) (*NATSConfig, error) {
	urls := getEnvSliceWithPrefix(servicePrefix, "NATS_URLS", []string{"nats://localhost:4222"})

	maxReconnects, _ := strconv.Atoi(getEnvWithPrefix(servicePrefix, "NATS_MAX_RECONNECTS", "-1"))
	reconnectWaitMs, _ := strconv.Atoi(getEnvWithPrefix(servicePrefix, "NATS_RECONNECT_WAIT_MS", "2000"))
	timeoutMs, _ := strconv.Atoi(getEnvWithPrefix(servicePrefix, "NATS_TIMEOUT_MS", "5000"))

	config := &NATSConfig{
		URLs:          urls,
		MaxReconnects: maxReconnects,
		ReconnectWait: time.Duration(reconnectWaitMs) * time.Millisecond,
		Timeout:       time.Duration(timeoutMs) * time.Millisecond,
	}

	return config, nil
}

// getEnvSlice gets an environment variable as a slice, splitting by comma
func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Simple comma split - for production, consider using a proper CSV parser
		result := make([]string, 0)
		for _, v := range strings.Split(value, ",") {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

// getEnvSliceWithPrefix gets a service-prefixed environment variable as a slice, with fallback
func getEnvSliceWithPrefix(servicePrefix, key string, defaultValue []string) []string {
	// Try service-prefixed version first
	if servicePrefix != "" {
		prefixedKey := servicePrefix + "_" + key
		if result := getEnvSlice(prefixedKey, nil); result != nil {
			return result
		}
	}

	// Fall back to unprefixed version
	return getEnvSlice(key, defaultValue)
}

// getEnvOrDefault gets an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvWithPrefix tries to get a service-prefixed environment variable first, then falls back to the unprefixed version
func getEnvWithPrefix(servicePrefix, key, defaultValue string) string {
	// Try service-prefixed version first (e.g., BUILDER_DATABASE_URL)
	if servicePrefix != "" {
		prefixedKey := servicePrefix + "_" + key
		if value := os.Getenv(prefixedKey); value != "" {
			return value
		}
	}

	// Fall back to unprefixed version (e.g., DATABASE_URL)
	return getEnvOrDefault(key, defaultValue)
}
