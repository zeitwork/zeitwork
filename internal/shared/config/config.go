package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// BaseConfig contains common configuration for all services
type BaseConfig struct {
	ServiceName string `env:"SERVICE_NAME"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	Environment string `env:"ENVIRONMENT" envDefault:"development"` // development, staging, production
}

// NodeAgentConfig contains configuration for the node agent service
type NodeAgentConfig struct {
	BaseConfig  `envPrefix:"NODEAGENT_"`
	NodeID      string `env:"NODEAGENT_NODE_ID"`
	DatabaseURL string `env:"NODEAGENT_DATABASE_URL" required:"true"`
}

// EdgeProxyConfig contains configuration for the edge proxy service
type EdgeProxyConfig struct {
	BaseConfig  `envPrefix:"EDGEPROXY_"`
	DatabaseURL string `env:"EDGEPROXY_DATABASE_URL" required:"true"`
	PortHttp    int    `env:"EDGEPROXY_HTTP_PORT" envDefault:"8080"`
	PortHttps   int    `env:"EDGEPROXY_HTTPS_PORT" envDefault:"8443"`
}

type BuilderConfig struct {
	BaseConfig          `envPrefix:"BUILDER_"`
	DatabaseURL         string        `env:"BUILDER_DATABASE_URL" required:"true"`
	BuildPollInterval   time.Duration `env:"BUILDER_BUILD_POLL_INTERVAL_MS" envDefault:"5s"` // How often to check for pending builds
	BuildTimeout        time.Duration `env:"BUILDER_BUILD_TIMEOUT_MS" envDefault:"30m"`      // Maximum time for a single build
	MaxConcurrentBuilds int           `env:"BUILDER_MAX_CONCURRENT_BUILDS" envDefault:"3"`   // Maximum number of concurrent builds
	CleanupInterval     time.Duration `env:"BUILDER_CLEANUP_INTERVAL_MS" envDefault:"5m"`    // How often to check for orphaned builds
	ShutdownGracePeriod time.Duration `env:"BUILDER_SHUTDOWN_GRACE_MS" envDefault:"30s"`     // How long to wait for in-flight builds on shutdown

	// Image builder configuration
	BuilderType       string      `env:"BUILDER_TYPE" envDefault:"docker"`                   // Type of builder to use (docker, firecracker, etc.)
	BuildWorkDir      string      `env:"BUILDER_WORK_DIR" envDefault:"/tmp/zeitwork-builds"` // Directory where builds are performed
	ContainerRegistry string      `env:"BUILDER_CONTAINER_REGISTRY"`                         // Container registry to push images to
	NATS              *NATSConfig `envPrefix:"BUILDER_"`
}

// ManagerConfig contains configuration for the manager service
type ManagerConfig struct {
	BaseConfig  `envPrefix:"MANAGER_"`
	DatabaseURL string      `env:"MANAGER_DATABASE_URL" required:"true"`
	NATS        *NATSConfig `envPrefix:"MANAGER_"`
}

// NATSConfig contains configuration for NATS messaging
type NATSConfig struct {
	URLs          []string      `env:"NATS_URLS" envSeparator:"," required:"true"` // NATS server URLs
	MaxReconnects int           `env:"NATS_MAX_RECONNECTS" envDefault:"-1"`        // Maximum number of reconnect attempts (-1 for unlimited)
	ReconnectWait time.Duration `env:"NATS_RECONNECT_WAIT_MS" envDefault:"2s"`     // Time to wait between reconnect attempts
	Timeout       time.Duration `env:"NATS_TIMEOUT_MS" envDefault:"5s"`            // Connection timeout
}

// LoadNodeAgentConfig loads configuration for the node agent service
func LoadNodeAgentConfig() (*NodeAgentConfig, error) {
	config, err := env.ParseAs[NodeAgentConfig]()
	if err != nil {
		return nil, fmt.Errorf("failed to parse NodeAgent config: %w", err)
	}

	// Set service name if not provided
	if config.ServiceName == "" {
		config.ServiceName = "node-agent"
	}

	return &config, nil
}

// LoadEdgeProxyConfig loads configuration for the edge proxy service
func LoadEdgeProxyConfig() (*EdgeProxyConfig, error) {
	config, err := env.ParseAs[EdgeProxyConfig]()
	if err != nil {
		return nil, fmt.Errorf("failed to parse EdgeProxy config: %w", err)
	}

	// Set service name if not provided
	if config.ServiceName == "" {
		config.ServiceName = "edge-proxy"
	}

	return &config, nil
}

// LoadBuilderConfig loads configuration for the builder service
func LoadBuilderConfig() (*BuilderConfig, error) {
	config, err := env.ParseAs[BuilderConfig]()
	if err != nil {
		return nil, fmt.Errorf("failed to parse Builder config: %w", err)
	}

	// Set service name if not provided
	if config.ServiceName == "" {
		config.ServiceName = "builder"
	}

	// Initialize NATS config if not already set
	if config.NATS == nil {
		config.NATS = &NATSConfig{}
	}

	return &config, nil
}

// LoadManagerConfig loads configuration for the manager service
func LoadManagerConfig() (*ManagerConfig, error) {
	config, err := env.ParseAs[ManagerConfig]()
	if err != nil {
		return nil, fmt.Errorf("failed to parse Manager config: %w", err)
	}

	// Set service name if not provided
	if config.ServiceName == "" {
		config.ServiceName = "manager"
	}

	// Initialize NATS config if not already set
	if config.NATS == nil {
		config.NATS = &NATSConfig{}
	}

	return &config, nil
}

// CertManagerConfig contains configuration for the cert manager service
type CertManagerConfig struct {
	BaseConfig     `envPrefix:"CERTMANAGER_"`
	DatabaseURL    string        `env:"CERTMANAGER_DATABASE_URL" required:"true"`
	PollInterval   time.Duration `env:"CERTMANAGER_POLL_INTERVAL_MS" envDefault:"15m"`              // How often to reconcile/renew certs
	RenewBefore    time.Duration `env:"CERTMANAGER_RENEW_BEFORE_DAYS" envDefault:"720h"`            // Renew certificates before this remaining validity (30 days)
	Provider       string        `env:"CERTMANAGER_PROVIDER" envDefault:"local"`                    // local | acme (future)
	DevBaseDomain  string        `env:"CERTMANAGER_DEV_BASE_DOMAIN" envDefault:"zeitwork.internal"` // e.g. zeitwork.internal
	ProdBaseDomain string        `env:"CERTMANAGER_PROD_BASE_DOMAIN" envDefault:"zeitwork.app"`     // e.g. zeitwork.app
	LockTimeout    time.Duration `env:"CERTMANAGER_LOCK_TIMEOUT_MS" envDefault:"10s"`               // Storage lock timeout/backoff
	NATS           *NATSConfig   `envPrefix:"CERTMANAGER_"`
}

// LoadCertManagerConfig loads configuration for the certmanager service
func LoadCertManagerConfig() (*CertManagerConfig, error) {
	config, err := env.ParseAs[CertManagerConfig]()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CertManager config: %w", err)
	}

	// Set service name if not provided
	if config.ServiceName == "" {
		config.ServiceName = "certmanager"
	}

	// Initialize NATS config if not already set
	if config.NATS == nil {
		config.NATS = &NATSConfig{}
	}

	return &config, nil
}

// LoadListenerConfig loads configuration for the listener service
func LoadListenerConfig() (*BaseConfig, error) {
	config, err := env.ParseAsWithOptions[BaseConfig](env.Options{
		Prefix: "LISTENER_",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse Listener config: %w", err)
	}

	// Set service name if not provided
	if config.ServiceName == "" {
		config.ServiceName = "listener"
	}

	return &config, nil
}

// LoadNATSConfig loads NATS configuration with dev-local defaults
func LoadNATSConfig() (*NATSConfig, error) {
	return LoadNATSConfigWithPrefix("")
}

// LoadNATSConfigWithPrefix loads NATS configuration with service prefix support
func LoadNATSConfigWithPrefix(servicePrefix string) (*NATSConfig, error) {
	var config NATSConfig
	var err error

	if servicePrefix != "" {
		config, err = env.ParseAsWithOptions[NATSConfig](env.Options{
			Prefix: servicePrefix + "_",
		})
	} else {
		config, err = env.ParseAs[NATSConfig]()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse NATS config: %w", err)
	}

	return &config, nil
}
