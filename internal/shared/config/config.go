package config

import (
	"fmt"
	"os"
)

// BaseConfig contains common configuration for all services
type BaseConfig struct {
	ServiceName string
	Port        string
	LogLevel    string
	Environment string // development, staging, production
}

// OperatorConfig contains configuration for the operator service
type OperatorConfig struct {
	BaseConfig
	DatabaseURL   string
	NodeAgentPort string
}

// NodeAgentConfig contains configuration for the node agent service
type NodeAgentConfig struct {
	BaseConfig
	OperatorURL string
	NodeID      string
}

// EdgeProxyConfig contains configuration for the edge proxy service
type EdgeProxyConfig struct {
	BaseConfig
	OperatorURL string
}

// APIConfig contains configuration for the public API service
type APIConfig struct {
	BaseConfig
	DatabaseURL string
}

// LoadOperatorConfig loads configuration for the operator service
func LoadOperatorConfig() (*OperatorConfig, error) {
	config := &OperatorConfig{
		BaseConfig:    loadBaseConfig("operator"),
		DatabaseURL:   getEnvOrDefault("DATABASE_URL", "postgres://localhost/zeitwork"),
		NodeAgentPort: getEnvOrDefault("NODE_AGENT_PORT", "8081"),
	}

	if config.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return config, nil
}

// LoadNodeAgentConfig loads configuration for the node agent service
func LoadNodeAgentConfig() (*NodeAgentConfig, error) {
	config := &NodeAgentConfig{
		BaseConfig:  loadBaseConfig("node-agent"),
		OperatorURL: getEnvOrDefault("OPERATOR_URL", "http://localhost:8080"),
		NodeID:      os.Getenv("NODE_ID"),
	}

	return config, nil
}

// LoadEdgeProxyConfig loads configuration for the edge proxy service
func LoadEdgeProxyConfig() (*EdgeProxyConfig, error) {
	config := &EdgeProxyConfig{
		BaseConfig:  loadBaseConfig("edge-proxy"),
		OperatorURL: getEnvOrDefault("OPERATOR_URL", "http://localhost:8080"),
	}

	return config, nil
}

// LoadAPIConfig loads configuration for the public API service
func LoadAPIConfig() (*APIConfig, error) {
	config := &APIConfig{
		BaseConfig:  loadBaseConfig("api"),
		DatabaseURL: getEnvOrDefault("DATABASE_URL", "postgres://localhost/zeitwork"),
	}

	if config.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return config, nil
}

// loadBaseConfig loads common configuration for all services
func loadBaseConfig(serviceName string) BaseConfig {
	return BaseConfig{
		ServiceName: serviceName,
		Port:        getEnvOrDefault("PORT", getDefaultPort(serviceName)),
		LogLevel:    getEnvOrDefault("LOG_LEVEL", "info"),
		Environment: getEnvOrDefault("ENVIRONMENT", "development"),
	}
}

// getDefaultPort returns the default port for a service
func getDefaultPort(serviceName string) string {
	switch serviceName {
	case "operator":
		return "8080"
	case "node-agent":
		return "8081"
	case "edge-proxy":
		return "8083"
	case "api":
		return "8090"
	default:
		return "8080"
	}
}

// getEnvOrDefault gets an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
