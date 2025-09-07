package runtime

import (
	"fmt"
	"log/slog"

	"github.com/zeitwork/zeitwork/internal/certmanager/runtime/local"
	"github.com/zeitwork/zeitwork/internal/certmanager/runtime/stubacme"
	"github.com/zeitwork/zeitwork/internal/certmanager/types"
	"github.com/zeitwork/zeitwork/internal/shared/config"
)

// NewRuntime creates a certificate runtime based on config
func NewRuntime(cfg *config.CertManagerConfig, logger *slog.Logger) (types.Runtime, error) {
	switch cfg.Provider {
	case "local":
		return local.NewLocalRuntime(cfg, logger.With("runtime", "local"))
	case "acme":
		return stubacme.NewStubACMERuntime(cfg, logger.With("runtime", "acme"))
	default:
		return nil, fmt.Errorf("unsupported cert runtime: %s", cfg.Provider)
	}
}
