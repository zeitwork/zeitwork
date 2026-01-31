package zeitwork

import (
	"context"
	_ "embed"
	"log/slog"
	"slices"
	"time"

	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/database/queries"
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
			s.logger.Info("starting reconciliation cycle")

			// Deployments
			deployments, err := s.db.Queries().DeploymentFind(ctx)
			if err != nil {
				panic(err)
			}
			for _, deployment := range deployments {
				s.reconcileDeployment(ctx, deployment.ID)
			}

			// Builds
			builds, err := s.db.Queries().BuildFind(ctx)
			if err != nil {
				panic(err)
			}
			for _, build := range builds {
				s.reconcileBuild(ctx, build.ID)
			}

			// VMs
			vms, err := s.db.Queries().VMFind(ctx)
			if err != nil {
				panic(err)
			}
			for _, vm := range vms {
				s.reconcileVM(ctx, vm.ID)
			}

			// Domains
			domains, err := s.db.Queries().DomainFind(ctx)
			if err != nil {
				panic(err)
			}
			for _, domain := range domains {
				s.reconcileDomain(ctx, domain.ID)
			}

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

func matchesAllowedIP(resolution *dnsresolver.Resolution, allowedIP string) bool {
	return slices.Contains(resolution.IPv4, allowedIP)
}

func (s *Service) reconcileDeployment(ctx context.Context, objectID uuid.UUID) error {
	deployment, err := s.db.Queries().DeploymentFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	switch deployment.Status {
	case queries.DeploymentStatusPending:
		panic("unimplemented")
		// TODO: create build with `pending` status AND deployment => `building`
	case queries.DeploymentStatusBuilding:
		panic("unimplemented")
		// -> if build status `pending` or `building` for more than 10 minutes then set deployment status to `failed`
		// -> if build status `failed` then mark deployment `failed`
		// -> if build status `succesful` then create vm with `pending` status and update deployment to `starting`
	case queries.DeploymentStatusStarting:
		panic("unimplemented")
		// -> if vm status `pending` or `starting` for more than 10 minutes set deployment status to `failed`
	case queries.DeploymentStatusRunning:
		panic("unimplemented")
		// -> if there is a newer deployment with status `running` then mark this one as `stopping`
	case queries.DeploymentStatusStopping:
		panic("unimplemented")
	case queries.DeploymentStatusStopped:
		panic("unimplemented")
		// -> if vm status is `running` then mark it as stopping
		// -> if vm status is `stopped` then mark the deployment as `stopped`
	case queries.DeploymentStatusFailed:
		panic("unimplemented")
	}

	return nil
}

func (s *Service) reconcileBuild(ctx context.Context, objectID uuid.UUID) error {
	build, err := s.db.Queries().BuildFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	switch build.Status {
	case queries.BuildStatusPending:
		panic("unimplemented")
		// -> create a `vm` with status `pending` with the `zeitwork-build` image and update build to status `building`
	case queries.BuildStatusBuilding:
		panic("unimplemented")
		// -> if build status is `building` for more than 30 minutes mark it as failed
		// -> if vm status `pending`, `starting`, `running` or `stopping` for more than 10 minutes then set build status to `failed`
		// -> if vm status `failed` then set build status to `failed`
		// -> if vm status `stopped` then check build image
		// |-> if it exists then mark build as `successful`
		// |-> if it does not exist then mark build as `failed`
	case queries.BuildStatusSuccesful:
		panic("unimplemented")
	case queries.BuildStatusFailed:
		panic("unimplemented")

	}

	return nil
}

func (s *Service) reconcileVM(ctx context.Context, objectID uuid.UUID) error {
	vm, err := s.db.Queries().VMFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	switch vm.Status {
	case queries.VmStatusPending:
		panic("unimplemented")
		// TODO: mark vm as starting
	case queries.VmStatusStarting:
		panic("unimplemented")
		// TODO: start the vm with cloud hypervisor and either mark it as `running` or `failed`
	case queries.VmStatusRunning:
		panic("unimplemented")
		// TODO: check status and ensure the vm is running
	case queries.VmStatusStopping:
		panic("unimplemented")
		// TODO: stop the vm
	case queries.VmStatusStopped:
		panic("unimplemented")
	case queries.VmStatusFailed:
		panic("unimplemented")
	}

	return nil
}

func (s *Service) reconcileDomain(ctx context.Context, objectID uuid.UUID) error {
	domain, err := s.db.Queries().DomainFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	domainName := domain.Name

	resolveCtx, cancel := context.WithTimeout(ctx, domainResolveTimeout)
	resolution, err := s.dnsResolver.Resolve(resolveCtx, domainName)
	cancel()
	if err != nil {
		// domain resolution failed
		return nil
	}

	matchedIP := matchesAllowedIP(resolution, s.cfg.IPAdress)

	if !matchedIP {
		// domain does not point to allowed targets
		return nil
	}

	s.db.Queries().DomainMarkVerified(ctx, domain.ID)

	return nil
}
