package zeitwork

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/docker/client"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/listener"
	"github.com/zeitwork/zeitwork/internal/reconciler"
	dnsresolver "github.com/zeitwork/zeitwork/internal/shared/dns"
	"github.com/zeitwork/zeitwork/internal/shared/github"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type Config struct {
	IPAdress string

	DB          *database.DB
	DatabaseURL string // Required for WAL replication listener

	// Docker registry configuration
	// For GHCR, URL is "ghcr.io" and Username is the org/user (e.g., "zeitwork")
	DockerRegistryURL      string
	DockerRegistryUsername string
	DockerRegistryPAT      string // GitHub PAT with write:packages scope for pushing images

	// GitHub App credentials for fetching source code
	GitHubAppID         string
	GitHubAppPrivateKey string // base64-encoded

	// Metadata server configuration
	// Serves env variables to VMs via HTTP (avoids kernel cmdline size limits)
	MetadataServerAddr string // e.g., "0.0.0.0:8111"
}

type Service struct {
	cfg Config

	db *database.DB

	// Docker client
	dockerClient *client.Client

	// DNS resolution
	dnsResolver dnsresolver.Resolver

	// GitHub token service for fetching source code
	githubTokenService *github.Service

	// Metadata server for serving env vars to VMs
	metadataServer *MetadataServer

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

	// Build execution tracking (prevents concurrent execution of the same build)
	activeBuildsMu sync.Mutex
	activeBuilds   map[uuid.UUID]bool
}

// New creates a new reconciler service
func New(cfg Config) (*Service, error) {
	s := &Service{
		cfg:            cfg,
		db:             cfg.DB,
		dnsResolver:    dnsresolver.NewResolver(),
		metadataServer: NewMetadataServer(),
		vmToCmd:        make(map[uuid.UUID]*exec.Cmd),
		imageMu:        sync.Mutex{},
		nextTap:        atomic.Int32{},
		activeBuilds:   make(map[uuid.UUID]bool),
	}

	// Initialize GitHub token service if credentials are provided
	if cfg.GitHubAppID != "" && cfg.GitHubAppPrivateKey != "" {
		githubSvc, err := github.NewTokenService(github.Config{
			DB:              *cfg.DB,
			AppID:           cfg.GitHubAppID,
			PrivatKeyBase64: cfg.GitHubAppPrivateKey,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create github token service: %w", err)
		}
		s.githubTokenService = githubSvc
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

	// Start metadata server (firewall rules are configured via nftables in ansible)
	if s.cfg.MetadataServerAddr != "" {
		go func() {
			if err := s.metadataServer.Start(ctx, s.cfg.MetadataServerAddr); err != nil {
				slog.Error("metadata server error", "err", err)
			}
		}()
	}

	// Start all schedulers
	s.deploymentScheduler.Start()
	s.buildScheduler.Start()
	s.imageScheduler.Start()
	s.vmScheduler.Start()
	s.domainScheduler.Start()

	// Bootstrap: schedule all existing entities once on startup
	// This ensures we don't miss any changes that happened while we were down
	if err := s.bootstrap(ctx); err != nil {
		return fmt.Errorf("failed to bootstrap: %w", err)
	}

	// Create WAL listener with callbacks to schedulers
	// Following K8s pattern: when an entity changes, schedule self + notify parents via reverse lookups
	walListener := listener.New(listener.Config{
		DatabaseURL: s.cfg.DatabaseURL,

		OnDeployment: func(ctx context.Context, id uuid.UUID) {
			s.deploymentScheduler.Schedule(id, time.Now())
			// Nothing depends on deployments
		},

		OnBuild: func(ctx context.Context, id uuid.UUID) {
			s.buildScheduler.Schedule(id, time.Now())

			// Notify deployments that reference this build
			if deployments, err := s.db.DeploymentFindByBuildID(ctx, id); err != nil {
				slog.Error("failed to find deployments by build_id", "build_id", id, "error", err)
			} else {
				for _, d := range deployments {
					slog.Debug("notifying deployment of build change", "deployment_id", d.ID, "build_id", id)
					s.deploymentScheduler.Schedule(d.ID, time.Now())
				}
			}
		},

		OnImage: func(ctx context.Context, id uuid.UUID) {
			s.imageScheduler.Schedule(id, time.Now())

			// Notify VMs that use this image
			if vms, err := s.db.VMFindByImageID(ctx, id); err != nil {
				slog.Error("failed to find VMs by image_id", "image_id", id, "error", err)
			} else {
				for _, vm := range vms {
					slog.Debug("notifying VM of image change", "vm_id", vm.ID, "image_id", id)
					s.vmScheduler.Schedule(vm.ID, time.Now())
				}
			}

			// Notify builds waiting for build image (dind case)
			// When any image becomes ready, check if pending/building builds can proceed
			if builds, err := s.db.BuildFindWaitingForBuildImage(ctx); err != nil {
				slog.Error("failed to find builds waiting for build image", "error", err)
			} else {
				for _, b := range builds {
					slog.Debug("notifying build of image change", "build_id", b.ID, "image_id", id)
					s.buildScheduler.Schedule(b.ID, time.Now())
				}
			}
		},

		OnVM: func(ctx context.Context, id uuid.UUID) {
			s.vmScheduler.Schedule(id, time.Now())

			// Notify builds that use this VM
			if builds, err := s.db.BuildFindByVMID(ctx, id); err != nil {
				slog.Error("failed to find builds by vm_id", "vm_id", id, "error", err)
			} else {
				for _, b := range builds {
					slog.Debug("notifying build of VM change", "build_id", b.ID, "vm_id", id)
					s.buildScheduler.Schedule(b.ID, time.Now())
				}
			}

			// Notify deployments that use this VM
			if deployments, err := s.db.DeploymentFindByVMID(ctx, id); err != nil {
				slog.Error("failed to find deployments by vm_id", "vm_id", id, "error", err)
			} else {
				for _, d := range deployments {
					slog.Debug("notifying deployment of VM change", "deployment_id", d.ID, "vm_id", id)
					s.deploymentScheduler.Schedule(d.ID, time.Now())
				}
			}
		},

		OnDomain: func(ctx context.Context, id uuid.UUID) {
			s.domainScheduler.Schedule(id, time.Now())
			// Nothing depends on domains
		},
	})

	// Start WAL listener (blocks until context is cancelled)
	slog.Info("starting WAL listener for database changes")
	return walListener.Start(ctx)
}

// bootstrap schedules all existing entities for reconciliation on startup
func (s *Service) bootstrap(ctx context.Context) error {
	slog.Info("bootstrapping: scheduling all existing entities")

	// Deployments
	deployments, err := s.db.DeploymentFind(ctx)
	if err != nil {
		return fmt.Errorf("failed to find deployments: %w", err)
	}
	for _, deployment := range deployments {
		s.deploymentScheduler.Schedule(deployment.ID, time.Now())
	}
	slog.Info("bootstrapped deployments", "count", len(deployments))

	// Builds
	builds, err := s.db.BuildFind(ctx)
	if err != nil {
		return fmt.Errorf("failed to find builds: %w", err)
	}
	for _, build := range builds {
		s.buildScheduler.Schedule(build.ID, time.Now())
	}
	slog.Info("bootstrapped builds", "count", len(builds))

	// Images
	images, err := s.db.ImageFind(ctx)
	if err != nil {
		return fmt.Errorf("failed to find images: %w", err)
	}
	for _, image := range images {
		s.imageScheduler.Schedule(image.ID, time.Now())
	}
	slog.Info("bootstrapped images", "count", len(images))

	// VMs
	vms, err := s.db.VMFind(ctx)
	if err != nil {
		return fmt.Errorf("failed to find vms: %w", err)
	}
	for _, vm := range vms {
		s.vmScheduler.Schedule(vm.ID, time.Now())
	}
	slog.Info("bootstrapped vms", "count", len(vms))

	// Domains
	domains, err := s.db.DomainFind(ctx)
	if err != nil {
		return fmt.Errorf("failed to find domains: %w", err)
	}
	for _, domain := range domains {
		s.domainScheduler.Schedule(domain.ID, time.Now())
	}
	slog.Info("bootstrapped domains", "count", len(domains))

	slog.Info("bootstrap complete")
	return nil
}

// Stop gracefully stops the reconciler service
func (s *Service) Stop() error {
	slog.Info("stopping reconciler")

	return nil
}
