package certmanager

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	certRuntime "github.com/zeitwork/zeitwork/internal/certmanager/runtime"
	"github.com/zeitwork/zeitwork/internal/certmanager/types"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	natsClient "github.com/zeitwork/zeitwork/internal/shared/nats"
	shareduuid "github.com/zeitwork/zeitwork/internal/shared/uuid"
	pb "github.com/zeitwork/zeitwork/proto"
)

// Service implements certificate management using local or ACME providers.
// MVP: local provider generates self-signed certs for development.
type Service struct {
	logger     *slog.Logger
	config     *config.CertManagerConfig
	db         *pgxpool.Pool
	queries    *database.Queries
	runtime    types.Runtime
	natsClient *natsClient.Client
}

func NewService(cfg *config.CertManagerConfig, logger *slog.Logger) (*Service, error) {
	// Init DB
	dbConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	db, err := pgxpool.NewWithConfig(context.Background(), dbConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	if err := db.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	rt, err := certRuntime.NewRuntime(cfg, logger.With("component", "cert-runtime"))
	if err != nil {
		return nil, err
	}

	// NATS client
	nc, err := natsClient.NewClient(cfg.NATS, "certmanager")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &Service{
		logger:     logger,
		config:     cfg,
		db:         db,
		queries:    database.New(db),
		runtime:    rt,
		natsClient: nc,
	}, nil
}

func (s *Service) Close() error {
	if s.db != nil {
		s.db.Close()
	}
	if s.runtime != nil {
		_ = s.runtime.Cleanup()
	}
	if s.natsClient != nil {
		_ = s.natsClient.Close()
	}
	return nil
}

// Start begins the reconciliation loop
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting certmanager service",
		"environment", s.config.Environment,
		"provider", s.config.Provider,
	)

	// Initial reconcile
	if err := s.reconcileOnce(ctx); err != nil {
		s.logger.Error("initial reconcile failed", "error", err)
	}

	// Subscribe to domain events via NATS
	go s.subscribeToDomainEvents(ctx)

	// Hourly drift poll (unless configured otherwise)
	drift := time.Hour
	if s.config.PollInterval > 0 {
		drift = s.config.PollInterval
	}
	ticker := time.NewTicker(drift)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Shutting down certmanager service")
			return nil
		case <-ticker.C:
			if err := s.reconcileOnce(ctx); err != nil {
				s.logger.Error("reconcile failed", "error", err)
			}
		}
	}
}

func (s *Service) subscribeToDomainEvents(ctx context.Context) {
	s.logger.Info("Subscribing to domain events")
	queueGroup := "certmanager-workers"
	ctxNats := s.natsClient.WithContext(ctx)

	// domain.created
	_, err := ctxNats.QueueSubscribe("domain.created", queueGroup, func(msg *nats.Msg) {
		var event pb.DomainCreated
		if err := proto.Unmarshal(msg.Data, &event); err != nil {
			s.logger.Error("failed to unmarshal DomainCreated", "error", err)
			return
		}
		s.handleDomainByID(ctx, event.GetId())
	})
	if err != nil {
		s.logger.Error("Failed to subscribe to domain.created", "error", err)
		return
	}

	// domain.updated
	_, err = ctxNats.QueueSubscribe("domain.updated", queueGroup, func(msg *nats.Msg) {
		var event pb.DomainUpdated
		if err := proto.Unmarshal(msg.Data, &event); err != nil {
			s.logger.Error("failed to unmarshal DomainUpdated", "error", err)
			return
		}
		s.handleDomainByID(ctx, event.GetId())
	})
	if err != nil {
		s.logger.Error("Failed to subscribe to domain.updated", "error", err)
		return
	}

	<-ctx.Done()
}

func (s *Service) handleDomainByID(ctx context.Context, id string) {
	pgID := shareduuid.MustParseUUID(id)
	row, err := s.queries.DomainsGetById(ctx, pgID)
	if err != nil || row == nil {
		s.logger.Error("failed to load domain", "id", id, "error", err)
		return
	}
	base := s.config.DevBaseDomain
	if s.config.Environment == "production" {
		base = s.config.ProdBaseDomain
	}
	name := strings.ToLower(row.Name)
	if strings.HasSuffix(name, "."+base) {
		// Covered by wildcard; nothing to do
		return
	}
	if err := s.runtime.EnsureCertificate(ctx, name, false); err != nil {
		s.logger.Error("ensure custom domain cert failed", "domain", name, "error", err)
	}
}

func (s *Service) reconcileOnce(ctx context.Context) error {
	// Ensure wildcard for base domain
	base := s.config.DevBaseDomain
	if s.config.Environment == "production" {
		base = s.config.ProdBaseDomain
	}
	wildcard := "*." + base
	if err := s.runtime.EnsureCertificate(ctx, wildcard, true); err != nil {
		return fmt.Errorf("ensure wildcard %s: %w", wildcard, err)
	}

	// Gather domains
	rows, err := s.queries.DomainsListAll(ctx)
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}

	for _, d := range rows {
		name := strings.ToLower(d.Name)
		// Skip base-domain subdomains covered by wildcard
		if strings.HasSuffix(name, "."+base) {
			continue
		}
		// For custom domains, ensure individual cert
		if err := s.runtime.EnsureCertificate(ctx, name, false); err != nil {
			s.logger.Error("ensure domain cert failed", "domain", name, "error", err)
			continue
		}
	}
	return nil
}
