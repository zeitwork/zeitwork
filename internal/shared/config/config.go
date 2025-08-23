package config

import (
	"fmt"
	"os"
	"strconv"
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
	OperatorURL       string
	NodeID            string
	FirecrackerBin    string
	FirecrackerSocket string
	VMWorkDir         string
}

// LoadBalancerConfig contains configuration for the load balancer service
type LoadBalancerConfig struct {
	BaseConfig
	OperatorURL string
	Algorithm   string // round-robin, least-connections, ip-hash
}

// EdgeProxyConfig contains configuration for the edge proxy service
type EdgeProxyConfig struct {
	BaseConfig
	LoadBalancerURL string
	SSLCertPath     string
	SSLKeyPath      string
	RateLimitRPS    int
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
		BaseConfig:        loadBaseConfig("node-agent"),
		OperatorURL:       getEnvOrDefault("OPERATOR_URL", "http://localhost:8080"),
		NodeID:            os.Getenv("NODE_ID"),
		FirecrackerBin:    getEnvOrDefault("FIRECRACKER_BIN", "/usr/bin/firecracker"),
		FirecrackerSocket: getEnvOrDefault("FIRECRACKER_SOCKET", "/tmp/firecracker.socket"),
		VMWorkDir:         getEnvOrDefault("VM_WORK_DIR", "/var/lib/firecracker/vms"),
	}

	return config, nil
}

// LoadLoadBalancerConfig loads configuration for the load balancer service
func LoadLoadBalancerConfig() (*LoadBalancerConfig, error) {
	config := &LoadBalancerConfig{
		BaseConfig:  loadBaseConfig("load-balancer"),
		OperatorURL: getEnvOrDefault("OPERATOR_URL", "http://localhost:8080"),
		Algorithm:   getEnvOrDefault("LB_ALGORITHM", "round-robin"),
	}

	return config, nil
}

// LoadEdgeProxyConfig loads configuration for the edge proxy service
func LoadEdgeProxyConfig() (*EdgeProxyConfig, error) {
	rateLimitStr := getEnvOrDefault("RATE_LIMIT_RPS", "100")
	rateLimit, err := strconv.Atoi(rateLimitStr)
	if err != nil {
		rateLimit = 100
	}

	config := &EdgeProxyConfig{
		BaseConfig:      loadBaseConfig("edge-proxy"),
		LoadBalancerURL: getEnvOrDefault("LOAD_BALANCER_URL", "http://localhost:8082"),
		SSLCertPath:     os.Getenv("SSL_CERT_PATH"),
		SSLKeyPath:      os.Getenv("SSL_KEY_PATH"),
		RateLimitRPS:    rateLimit,
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
	case "load-balancer":
		return "8082"
	case "edge-proxy":
		return "8083"
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
