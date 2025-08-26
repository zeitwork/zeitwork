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
	KernelImagePath   string
	BuilderRootfsPath string
	S3Endpoint        string
	S3Bucket          string
	S3AccessKey       string
	S3SecretKey       string
	S3Region          string
}

// LoadBalancerConfig contains configuration for the load balancer service
type LoadBalancerConfig struct {
	BaseConfig
	OperatorURL string
	Algorithm   string // round-robin, least-connections, ip-hash
	HealthPort  string // Port for health check HTTP endpoint
}

// EdgeProxyConfig contains configuration for the edge proxy service
type EdgeProxyConfig struct {
	BaseConfig
	LoadBalancerURL string
	SSLCertPath     string
	SSLKeyPath      string
	RateLimitRPS    int
}

// APIConfig contains configuration for the public API service
type APIConfig struct {
	BaseConfig
	DatabaseURL    string
	GitHubClientID string
	GitHubSecret   string
	JWTSecret      string
	BaseURL        string
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
		KernelImagePath:   getEnvOrDefault("KERNEL_IMAGE_PATH", "/var/lib/zeitwork/kernel/vmlinux.bin"),
		BuilderRootfsPath: getEnvOrDefault("BUILDER_ROOTFS_PATH", "/var/lib/zeitwork/builder/rootfs.ext4"),
		S3Endpoint:        os.Getenv("S3_ENDPOINT"), // Empty for AWS S3
		S3Bucket:          os.Getenv("S3_BUCKET"),
		S3AccessKey:       os.Getenv("S3_ACCESS_KEY_ID"),
		S3SecretKey:       os.Getenv("S3_SECRET_ACCESS_KEY"),
		S3Region:          getEnvOrDefault("S3_REGION", "us-east-1"),
	}

	return config, nil
}

// LoadLoadBalancerConfig loads configuration for the load balancer service
func LoadLoadBalancerConfig() (*LoadBalancerConfig, error) {
	config := &LoadBalancerConfig{
		BaseConfig:  loadBaseConfig("load-balancer"),
		OperatorURL: getEnvOrDefault("OPERATOR_URL", "http://localhost:8080"),
		Algorithm:   getEnvOrDefault("LB_ALGORITHM", "round-robin"),
		HealthPort:  getEnvOrDefault("HEALTH_PORT", "8083"),
	}

	return config, nil
}

// LoadEdgeProxyConfig loads configuration for the edge proxy service
func LoadEdgeProxyConfig() (*EdgeProxyConfig, error) {
	ratelimitStr := getEnvOrDefault("RATE_LIMIT_RPS", "100")
	ratelimit, err := strconv.Atoi(ratelimitStr)
	if err != nil {
		ratelimit = 100
	}

	config := &EdgeProxyConfig{
		BaseConfig:      loadBaseConfig("edge-proxy"),
		LoadBalancerURL: getEnvOrDefault("LOAD_BALANCER_URL", "http://localhost:8082"),
		SSLCertPath:     os.Getenv("SSL_CERT_PATH"),
		SSLKeyPath:      os.Getenv("SSL_KEY_PATH"),
		RateLimitRPS:    ratelimit,
	}

	return config, nil
}

// LoadAPIConfig loads configuration for the public API service
func LoadAPIConfig() (*APIConfig, error) {
	config := &APIConfig{
		BaseConfig:     loadBaseConfig("api"),
		DatabaseURL:    getEnvOrDefault("DATABASE_URL", "postgres://localhost/zeitwork"),
		GitHubClientID: os.Getenv("GITHUB_CLIENT_ID"),
		GitHubSecret:   os.Getenv("GITHUB_CLIENT_SECRET"),
		JWTSecret:      getEnvOrDefault("JWT_SECRET", "change-me-in-production"),
		BaseURL:        getEnvOrDefault("BASE_URL", "https://api.zeitwork.com"),
	}

	if config.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	if config.GitHubClientID == "" || config.GitHubSecret == "" {
		return nil, fmt.Errorf("GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET are required")
	}

	if config.JWTSecret == "change-me-in-production" && config.Environment == "production" {
		return nil, fmt.Errorf("JWT_SECRET must be changed in production")
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
