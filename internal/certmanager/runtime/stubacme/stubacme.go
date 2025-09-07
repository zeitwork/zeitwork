package stubacme

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zeitwork/zeitwork/internal/certmanager/types"
	"github.com/zeitwork/zeitwork/internal/shared/config"
)

// StubACMERuntime is a placeholder for a future CertMagic/ACME implementation
type StubACMERuntime struct {
	logger *slog.Logger
	cfg    *config.CertManagerConfig
}

func NewStubACMERuntime(cfg *config.CertManagerConfig, logger *slog.Logger) (types.Runtime, error) {
	return &StubACMERuntime{logger: logger, cfg: cfg}, nil
}

func (s *StubACMERuntime) Name() string { return "acme" }

func (s *StubACMERuntime) Cleanup() error { return nil }

func (s *StubACMERuntime) EnsureCertificate(ctx context.Context, name string, isWildcard bool) error {
	// This will be replaced by CertMagic storage+lock backed by DB
	// For now, just return a TODO error so prod is clearly not implemented
	return fmt.Errorf("acme runtime not implemented yet - TODO CertMagic integration")
}
