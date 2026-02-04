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
		cfg:          cfg,
		db:           cfg.DB,
		dnsResolver:  dnsresolver.NewResolver(),
		vmToCmd:      make(map[uuid.UUID]*exec.Cmd),
		imageMu:      sync.Mutex{},
		nextTap:      atomic.Int32{},
		activeBuilds: make(map[uuid.UUID]bool),
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
	walListener := listener.New(listener.Config{
		DatabaseURL: s.cfg.DatabaseURL,
		OnDeployment: func(id uuid.UUID) {
			s.deploymentScheduler.Schedule(id, time.Now())
		},
		OnBuild: func(id uuid.UUID) {
			s.buildScheduler.Schedule(id, time.Now())
		},
		OnImage: func(id uuid.UUID) {
			s.imageScheduler.Schedule(id, time.Now())
		},
		OnVM: func(id uuid.UUID) {
			s.vmScheduler.Schedule(id, time.Now())
		},
		OnDomain: func(id uuid.UUID) {
			s.domainScheduler.Schedule(id, time.Now())
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
