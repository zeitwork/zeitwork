package builder

import (
	"context"
	"log/slog"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/zeitwork/zeitwork/internal/database"
)

type Config struct {
	DatabaseURL string

	GitHubAppID  string
	GitHubAppKey string

	// Docker registry
	RegistryURL      string
	RegistryUsername string
	RegistryPassword string
}

// Service is the builder service
type Service struct {
	cfg           Config
	db            *database.DB
	logger        *slog.Logger
	hcloudClient  *hcloud.Client
	sshPublicKey  string
	sshPrivateKey []byte
	cancel        context.CancelFunc
}

const BUILD_INTERVAL = 10 * time.Second
const BUILD_TIMEOUT = 30 * time.Minute

// NewService creates a new builder service
func NewService(cfg Config, logger *slog.Logger) (*Service, error) {
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	return &Service{
		cfg:    cfg,
		db:     db,
		logger: logger,
	}, nil
}

func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.logger.Info("starting builder service")

	ticker := time.NewTicker(BUILD_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.processPendingBuilds(ctx)
		}
	}
}

func (s *Service) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}

	if s.db != nil {
		s.db.Close()
	}

	return nil
}

func (s *Service) processPendingBuilds(ctx context.Context) {
	panic("not implemented")
}
