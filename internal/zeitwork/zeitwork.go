package zeitwork

import (
	"context"
	_ "embed"
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

	s.deploymentScheduler = reconciler.NewWithName("deployment", s.reconcileDeployment)
	s.buildScheduler = reconciler.NewWithName("build", s.reconcileBuild)
	s.imageScheduler = reconciler.NewWithName("image", s.reconcileImage)
	s.vmScheduler = reconciler.NewWithName("vm", s.reconcileVM)
	s.domainScheduler = reconciler.NewWithName("domain", s.reconcileDomain)

	return s, nil
}

// Start starts the reconciler service
func (s *Service) Start(ctx context.Context) error {
	slog.Info("starting Zeitwork reconciler")

	s.deploymentScheduler.Start()
	s.buildScheduler.Start()
	s.imageScheduler.Start()
	s.vmScheduler.Start()
	// s.domainScheduler.Start()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// TODO: we want to migrate this to listen/notify OR even nicer postgres wal log + nats
	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down Zeitwork reconciler")
			return ctx.Err()
		case <-ticker.C:
			// deployment
			deployments, err := s.db.DeploymentFind(ctx)
			if err != nil {
				panic(err)
			}
			for _, deployment := range deployments {
				s.deploymentScheduler.Schedule(deployment.ID, time.Now())
			}

			// build
			builds, err := s.db.BuildFind(ctx)
			if err != nil {
				panic(err)
			}
			for _, build := range builds {
				s.buildScheduler.Schedule(build.ID, time.Now())
			}

			// image
			images, err := s.db.ImageFind(ctx)
			if err != nil {
				panic(err)
			}
			for _, image := range images {
				s.imageScheduler.Schedule(image.ID, time.Now())
			}

			// vm
			vms, err := s.db.VMFind(ctx)
			if err != nil {
				panic(err)
			}
			for _, vm := range vms {
				s.vmScheduler.Schedule(vm.ID, time.Now())
			}

			// domain
			domains, err := s.db.DomainFind(ctx)
			if err != nil {
				panic(err)
			}
			for _, domain := range domains {
				s.domainScheduler.Schedule(domain.ID, time.Now())
			}
		}
	}

	// // VMs
	// s.vmScheduler.SetupPGXListener(ctx, s.db.Pool, "vms")
	// vms, err := s.db.Queries.VMFind(ctx)
	// if err != nil {
	// 	return err
	// }

	// for _, vm := range vms {
	// 	s.vmScheduler.Schedule(vm.ID, time.Now())
	// }

}

// Stop gracefully stops the reconciler service
func (s *Service) Stop() error {
	slog.Info("stopping reconciler")

	return nil
}
