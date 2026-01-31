package reconciler

import (
	"context"
	_ "embed"
	"log/slog"
	"slices"
	"time"

	"github.com/docker/docker/client"
	"github.com/zeitwork/zeitwork/internal/database"
	dnsresolver "github.com/zeitwork/zeitwork/internal/shared/dns"
)

type Config struct {
	IPAdress string

	DB *database.DB

	DockerRegistryURL      string
	DockerRegistryUsername string
	DockerRegistryPassword string
}

type Service struct {
	cfg Config

	db     *database.DB
	logger *slog.Logger
	cancel context.CancelFunc

	// Docker client
	dockerClient *client.Client

	// DNS resolution
	dnsResolver dnsresolver.Resolver
}

// NewService creates a new reconciler service
func New(cfg Config, logger *slog.Logger) (*Service, error) {
	s := &Service{
		cfg:         cfg,
		db:          cfg.DB,
		logger:      logger,
		dnsResolver: dnsresolver.NewResolver(),
	}

	return s, nil
}

// Start starts the reconciler service
func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Debug("reconciler stopped")
			return nil
		case <-ticker.C:
			start := time.Now()
			s.logger.Debug("starting reconciliation cycle")

			s.reconcileDomain(ctx)
			s.reconcileBuild(ctx)
			s.reconcileDeployment(ctx)
			s.reconcileVM(ctx)

			// ** deployments **
			// deployment status = `pending`
			// -> create build with `pending` status AND deployment => `building`
			// deployment status = `building`
			// -> if build status `pending` or `building` for more than 10 minutes then set deployment status to `failed`
			// -> if build status `failed` then mark deployment `failed`
			// -> if build status `succesful` then create vm with `pending` status and update deployment to `starting`
			// deployment status = `starting`
			// -> if vm status `pending` or `starting` for more than 10 minutes set deployment status to `failed`
			// deployment status = `running`
			// -> if there is a newer deployment with status `running` then mark this one as `stopping`
			// deployment status = `stopping`
			// -> if vm status is `running` then mark it as stopping
			// -> if vm status is `stopped` then mark the deployment as `stopped`

			// ** builds **
			// build status = `pending`
			// -> create a `vm` with status `pending` with the `zeitwork-build` image and update build to status `building`
			// build status = `building
			// -> if build status is `building` for more than 30 minutes mark it as failed
			// -> if vm status `pending`, `starting`, `running` or `stopping` for more than 10 minutes then set build status to `failed`
			// -> if vm status `failed` then set build status to `failed`
			// -> if vm status `stopped` then check build image
			// |-> if it exists then mark build as `successful`
			// |-> if it does not exist then mark build as `failed`

			// ** vms **
			// vm status = `pending`
			// -> mark vm as starting
			// vm status = `starting`
			// -> start the vm with cloud hypervisor and either mark it as `running` or `failed`
			// vm status = `running`
			// -> check status and ensure the vm is running
			// vm status = `stopping`
			// -> stop the vm
			// vm status = `stopped`
			// -> nothing
			// vm status = `failed`
			// -> nothing

			// ** domains **
			// TODO

			s.logger.Debug("reconciliation cycle completed", "duration", time.Since(start))
		}
	}
}

// Stop gracefully stops the reconciler service
func (s *Service) Stop() error {
	s.logger.Info("stopping reconciler")

	if s.cancel != nil {
		s.cancel()
	}

	if s.db != nil {
		s.db.Close()
	}

	return nil
}

const domainResolveTimeout = 10 * time.Second

func (s *Service) reconcileDomain(ctx context.Context) error {
	// TODO: query one domain
	domains, err := s.db.Queries().DomainListUnverified(ctx)
	if err != nil {
		return err
	}

	if len(domains) == 0 {
		return nil
	}

	for _, domain := range domains {
		domainName := domain.Name

		resolveCtx, cancel := context.WithTimeout(ctx, domainResolveTimeout)
		resolution, err := s.dnsResolver.Resolve(resolveCtx, domainName)
		cancel()
		if err != nil {
			// domain resolution failed
			continue
		}

		matchedIP := matchesAllowedIP(resolution, s.cfg.IPAdress)

		if !matchedIP {
			// domain does not point to allowed targets
			continue
		}

		s.db.Queries().DomainMarkVerified(ctx, domain.ID)
	}

	return nil
}

func matchesAllowedIP(resolution *dnsresolver.Resolution, allowedIP string) bool {
	return slices.Contains(resolution.IPv4, allowedIP)
}

func (s *Service) reconcileVM(ctx context.Context) {
	panic("unimplemented")
}

func (s *Service) reconcileDeployment(ctx context.Context) {
	panic("unimplemented")
}

func (s *Service) reconcileBuild(ctx context.Context) {
	panic("unimplemented")
}
