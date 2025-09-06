package runtime

import (
	"fmt"
	"log/slog"

	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
	"github.com/zeitwork/zeitwork/internal/nodeagent/runtime/docker"
	"github.com/zeitwork/zeitwork/internal/nodeagent/runtime/firecracker"
	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// NewRuntime creates a new runtime based on configuration
func NewRuntime(cfg *config.RuntimeConfig, logger *slog.Logger) (types.Runtime, error) {
	switch cfg.Mode {
	case "development":
		if cfg.DockerConfig == nil {
			return nil, fmt.Errorf("Docker configuration is required for development mode")
		}
		return docker.NewDockerRuntime(cfg.DockerConfig, logger)

	case "production":
		if cfg.FirecrackerConfig == nil {
			return nil, fmt.Errorf("Firecracker configuration is required for production mode")
		}
		return firecracker.NewFirecrackerRuntime(cfg.FirecrackerConfig, logger)

	default:
		return nil, fmt.Errorf("unsupported runtime mode: %s", cfg.Mode)
	}
}
