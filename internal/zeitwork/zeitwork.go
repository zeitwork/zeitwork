package zeitwork

import (
	"context"
	_ "embed"
	"errors"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/docker/client"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/reconciler"
	dnsresolver "github.com/zeitwork/zeitwork/internal/shared/dns"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
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

	db *database.DB

	// Docker client
	dockerClient *client.Client

	// DNS resolution
	dnsResolver dnsresolver.Resolver

	// Schedulers
	deploymentScheduler *reconciler.Scheduler
	buildScheduler      *reconciler.Scheduler
	imageScheduler      *reconciler.Scheduler
	vmScheduler         *reconciler.Scheduler
	domainScheduler     *reconciler.Scheduler

	// VM Stuff
	imageMu sync.Mutex
	vmToCmd map[uuid.UUID]*exec.Cmd
	nextTap atomic.Int32
}

// New creates a new reconciler service
func New(cfg Config) (*Service, error) {
	s := &Service{
		cfg:         cfg,
		db:          cfg.DB,
		dnsResolver: dnsresolver.NewResolver(),
		vmToCmd:     make(map[uuid.UUID]*exec.Cmd),
		imageMu:     sync.Mutex{},
		nextTap:     atomic.Int32{},
	}

	s.deploymentScheduler = reconciler.NewScheduler(s.reconcileDeployment)
	s.buildScheduler = reconciler.NewScheduler(s.reconcileBuild)
	s.imageScheduler = reconciler.NewScheduler(s.reconcileImage)
	s.vmScheduler = reconciler.NewScheduler(s.reconcileVM)
	s.domainScheduler = reconciler.NewScheduler(s.reconcileDomain)

	return s, nil
}

// Start starts the reconciler service
func (s *Service) Start(ctx context.Context) error {
	slog.Info("Starting Zeitwork Reconciler")

	// // Deployments
	// deployments, err := s.db.Queries().DeploymentFind(ctx)
	// if err != nil {
	// 	panic(err)
	// }
	// for _, deployment := range deployments {
	// 	s.reconcileDeployment(ctx, deployment.ID)
	// }

	// // Builds
	// builds, err := s.db.Queries().BuildFind(ctx)
	// if err != nil {
	// 	panic(err)
	// }
	// for _, build := range builds {
	// 	s.reconcileBuild(ctx, build.ID)
	// }

	// // Domains
	// domains, err := s.db.Queries().DomainFind(ctx)
	// if err != nil {
	// 	panic(err)
	// }
	// for _, domain := range domains {
	// 	s.reconcileDomain(ctx, domain.ID)
	// }

	// VMs
	s.vmScheduler.SetupPGXListener(ctx, s.db.Pool, "vms")
	vms, err := s.db.Queries.VMFind(ctx)
	if err != nil {
		return err
	}

	for _, vm := range vms {
		s.vmScheduler.Schedule(vm.ID, time.Now())
	}

	s.vmScheduler.Start()

	return nil
}

// Stop gracefully stops the reconciler service
func (s *Service) Stop() error {
	slog.Info("stopping reconciler")

	return errors.New("unimplemented")
}
