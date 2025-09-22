package runtime

import (
	"fmt"
	"log/slog"

	"github.com/zeitwork/zeitwork/internal/builder/config"
	"github.com/zeitwork/zeitwork/internal/builder/runtime/docker"
	"github.com/zeitwork/zeitwork/internal/builder/runtime/firecracker"
	"github.com/zeitwork/zeitwork/internal/builder/types"
	sharedConfig "github.com/zeitwork/zeitwork/internal/shared/config"
)

// NewBuildRuntime creates a new build runtime based on configuration
func NewBuildRuntime(cfg *sharedConfig.BuilderConfig, logger *slog.Logger) (types.BuildRuntime, error) {
	switch cfg.BuilderType {
	case "docker":
		dockerConfig := config.DockerRuntimeConfig{
			WorkDir:          cfg.BuildWorkDir,
			Registry:         cfg.ContainerRegistry,
			InsecureRegistry: cfg.InsecureRegistry,
		}
		return docker.NewDockerBuildRuntime(dockerConfig, logger.With("runtime", "docker"))

	case "firecracker":
		firecrackerConfig := config.FirecrackerRuntimeConfig{
			WorkDir:  cfg.BuildWorkDir,
			Registry: cfg.ContainerRegistry,
			// TODO: Add firecracker-specific configuration fields
		}
		return firecracker.NewFirecrackerBuildRuntime(firecrackerConfig, logger.With("runtime", "firecracker"))

	default:
		return nil, fmt.Errorf("unsupported build runtime: %s", cfg.BuilderType)
	}
}
