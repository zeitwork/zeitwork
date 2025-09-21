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

// NodeAgentRuntimeConfig defines the runtime configuration
type NodeAgentRuntimeConfig struct {
	Mode              string `env:"NODEAGENT_RUNTIME_MODE" envDefault:"development"`
	DockerConfig      *NodeAgentDockerRuntimeConfig
	FirecrackerConfig *NodeAgentFirecrackerRuntimeConfig
}

// NodeAgentDockerRuntimeConfig contains Docker-specific configuration
type NodeAgentDockerRuntimeConfig struct {
	Endpoint          string        `env:"NODEAGENT_DOCKER_ENDPOINT" envDefault:"unix:///var/run/docker.sock"`
	NetworkName       string        `env:"NODEAGENT_DOCKER_NETWORK_NAME" envDefault:"zeitwork"`
	ImageRegistry     string        `env:"NODEAGENT_DOCKER_IMAGE_REGISTRY"`
	PullTimeout       time.Duration `env:"NODEAGENT_DOCKER_PULL_TIMEOUT" envDefault:"5m"`
	StartTimeout      time.Duration `env:"NODEAGENT_DOCKER_START_TIMEOUT" envDefault:"30s"`
	StopTimeout       time.Duration `env:"NODEAGENT_DOCKER_STOP_TIMEOUT" envDefault:"10s"`
	EnableAutoCleanup bool          `env:"NODEAGENT_DOCKER_ENABLE_AUTO_CLEANUP" envDefault:"true"`
}

// NodeAgentFirecrackerRuntimeConfig contains Firecracker-specific configuration
type NodeAgentFirecrackerRuntimeConfig struct {
	// Standalone Firecracker runtime image paths (preferred)
	DefaultKernelPath string `env:"NODEAGENT_FIRECRACKER_DEFAULT_KERNEL_PATH" envDefault:"/var/lib/zeitwork/firecracker/zeitwork-vmlinux.bin"`
	DefaultRootfsPath string `env:"NODEAGENT_FIRECRACKER_DEFAULT_ROOTFS_PATH" envDefault:"/var/lib/zeitwork/firecracker/zeitwork-rootfs.img"`

	// Optional: firecracker-containerd integration (not used by default)
	ContainerdSocket    string `env:"NODEAGENT_FIRECRACKER_CONTAINERD_SOCKET" envDefault:""`
	ContainerdNamespace string `env:"NODEAGENT_FIRECRACKER_CONTAINERD_NAMESPACE" envDefault:""`
	RuntimeConfigPath   string `env:"NODEAGENT_FIRECRACKER_RUNTIME_CONFIG_PATH" envDefault:""`
}

// NodeAgentConfig contains configuration for the node agent service
type NodeAgentConfig struct {
	BaseConfig   `envPrefix:"NODEAGENT_"`
	Port         string        `env:"NODEAGENT_PORT" envDefault:"8081"`
	NodeID       string        `env:"NODEAGENT_NODE_ID"`
	RegionID     string        `env:"NODEAGENT_REGION_ID"`
	DatabaseURL  string        `env:"NODEAGENT_DATABASE_URL" required:"true"`
	PollInterval time.Duration `env:"NODEAGENT_POLL_INTERVAL" envDefault:"5s"`
	NATS         *NATSConfig   `envPrefix:"NODEAGENT_"`
	Runtime      *NodeAgentRuntimeConfig
}

// EdgeProxyConfig contains configuration for the edge proxy service
type EdgeProxyConfig struct {
	BaseConfig  `envPrefix:"EDGEPROXY_"`
	DatabaseURL string      `env:"EDGEPROXY_DATABASE_URL" required:"true"`
	PortHttp    int         `env:"EDGEPROXY_HTTP_PORT" envDefault:"8080"`
	PortHttps   int         `env:"EDGEPROXY_HTTPS_PORT" envDefault:"8443"`
	NATS        *NATSConfig `envPrefix:"EDGEPROXY_"`
}

type BuilderConfig struct {
	BaseConfig          `envPrefix:"BUILDER_"`
	DatabaseURL         string        `env:"BUILDER_DATABASE_URL" required:"true"`
	BuildPollInterval   time.Duration `env:"BUILDER_BUILD_POLL_INTERVAL" envDefault:"5s"`    // How often to check for pending builds
	BuildTimeout        time.Duration `env:"BUILDER_BUILD_TIMEOUT" envDefault:"30m"`         // Maximum time for a single build
	MaxConcurrentBuilds int           `env:"BUILDER_MAX_CONCURRENT_BUILDS" envDefault:"3"`   // Maximum number of concurrent builds
	CleanupInterval     time.Duration `env:"BUILDER_CLEANUP_INTERVAL" envDefault:"5m"`       // How often to check for orphaned builds
	ShutdownGracePeriod time.Duration `env:"BUILDER_SHUTDOWN_GRACE_PERIOD" envDefault:"30s"` // How long to wait for in-flight builds on shutdown

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
	ReconnectWait time.Duration `env:"NATS_RECONNECT_WAIT" envDefault:"2s"`        // Time to wait between reconnect attempts
	Timeout       time.Duration `env:"NATS_TIMEOUT" envDefault:"5s"`               // Connection timeout
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

	// Initialize NATS config if not already set
	if config.NATS == nil {
		natsConfig, err := env.ParseAsWithOptions[NATSConfig](env.Options{
			Prefix: "NODEAGENT_",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse NodeAgent NATS config: %w", err)
		}
		config.NATS = &natsConfig
	}

	// Initialize Runtime config if not already set
	if config.Runtime == nil {
		runtimeConfig, err := env.ParseAs[NodeAgentRuntimeConfig]()
		if err != nil {
			return nil, fmt.Errorf("failed to parse NodeAgent runtime config: %w", err)
		}
		config.Runtime = &runtimeConfig
	}

	// Initialize Docker config
	if config.Runtime.DockerConfig == nil {
		dockerConfig, err := env.ParseAs[NodeAgentDockerRuntimeConfig]()
		if err != nil {
			return nil, fmt.Errorf("failed to parse Docker config: %w", err)
		}
		config.Runtime.DockerConfig = &dockerConfig
	}

	// Initialize Firecracker config
	if config.Runtime.FirecrackerConfig == nil {
		firecrackerConfig, err := env.ParseAs[NodeAgentFirecrackerRuntimeConfig]()
		if err != nil {
			return nil, fmt.Errorf("failed to parse Firecracker config: %w", err)
		}
		config.Runtime.FirecrackerConfig = &firecrackerConfig
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

	// Initialize NATS config if not already set
	if config.NATS == nil {
		natsConfig, err := env.ParseAsWithOptions[NATSConfig](env.Options{
			Prefix: "EDGEPROXY_",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse EdgeProxy NATS config: %w", err)
		}
		config.NATS = &natsConfig
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
		natsConfig, err := env.ParseAsWithOptions[NATSConfig](env.Options{
			Prefix: "BUILDER_",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse Builder NATS config: %w", err)
		}
		config.NATS = &natsConfig
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
		natsConfig, err := env.ParseAsWithOptions[NATSConfig](env.Options{
			Prefix: "MANAGER_",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse Manager NATS config: %w", err)
		}
		config.NATS = &natsConfig
	}

	return &config, nil
}

// CertManagerConfig contains configuration for the cert manager service
type CertManagerConfig struct {
	BaseConfig     `envPrefix:"CERTMANAGER_"`
	DatabaseURL    string        `env:"CERTMANAGER_DATABASE_URL" required:"true"`
	PollInterval   time.Duration `env:"CERTMANAGER_POLL_INTERVAL" envDefault:"15m"`                 // How often to reconcile/renew certs
	RenewBefore    time.Duration `env:"CERTMANAGER_RENEW_BEFORE" envDefault:"720h"`                 // Renew certificates before this remaining validity (30 days)
	Provider       string        `env:"CERTMANAGER_PROVIDER" envDefault:"local"`                    // local | acme (future)
	DevBaseDomain  string        `env:"CERTMANAGER_DEV_BASE_DOMAIN" envDefault:"zeitwork.internal"` // e.g. zeitwork.internal
	ProdBaseDomain string        `env:"CERTMANAGER_PROD_BASE_DOMAIN" envDefault:"zeitwork.app"`     // e.g. zeitwork.app
	LockTimeout    time.Duration `env:"CERTMANAGER_LOCK_TIMEOUT" envDefault:"10s"`                  // Storage lock timeout/backoff
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
		natsConfig, err := env.ParseAsWithOptions[NATSConfig](env.Options{
			Prefix: "CERTMANAGER_",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse CertManager NATS config: %w", err)
		}
		config.NATS = &natsConfig
	}

	return &config, nil
}

// ListenerConfig contains configuration for the listener service
type ListenerConfig struct {
	BaseConfig          `envPrefix:"LISTENER_"`
	DatabaseURL         string        `env:"LISTENER_DATABASE_URL" required:"true"`
	NATS                *NATSConfig   `envPrefix:"LISTENER_"`
	ReplicationSlotName string        `env:"LISTENER_REPLICATION_SLOT" envDefault:"zeitwork_listener"`
	PublicationName     string        `env:"LISTENER_PUBLICATION_NAME" envDefault:"zeitwork_changes"`
	StandbyTimeout      time.Duration `env:"LISTENER_STANDBY_TIMEOUT" envDefault:"30s"`
	PluginArgs          []string      `env:"LISTENER_PLUGIN_ARGS" envSeparator:","`
}

// LoadListenerConfig loads configuration for the listener service
func LoadListenerConfig() (*ListenerConfig, error) {
	config, err := env.ParseAs[ListenerConfig]()
	if err != nil {
		return nil, fmt.Errorf("failed to parse Listener config: %w", err)
	}

	// Set service name if not provided
	if config.ServiceName == "" {
		config.ServiceName = "listener"
	}

	// Initialize NATS config if not already set
	if config.NATS == nil {
		natsConfig, err := env.ParseAsWithOptions[NATSConfig](env.Options{
			Prefix: "LISTENER_",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse Listener NATS config: %w", err)
		}
		config.NATS = &natsConfig
	}

	return &config, nil
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
