package zeitwork

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"net/netip"
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
	"github.com/zeitwork/zeitwork/internal/storage"
)

type Config struct {
	IPAdress string

	DB                *database.DB
	DatabaseDirectURL string // Direct PG connection for WAL replication listener (NOT PgBouncer)

	// Docker registry configuration
	DockerRegistryURL      string
	DockerRegistryUsername string
	DockerRegistryPAT      string

	// GitHub App credentials
	GitHubAppID         string
	GitHubAppPrivateKey string // base64-encoded

	// Metadata server configuration
	MetadataServerAddr string // e.g., "0.0.0.0:8111"

	// Multi-node configuration
	ServerID   uuid.UUID // Stable server identity (read from /data/server-id)
	InternalIP string    // This server's VLAN IP for cross-server communication

	// S3/MinIO for shared image storage
	S3 *storage.S3

	// RouteChangeNotify is sent to when routes may have changed.
	// The edge proxy listens on this channel.
	RouteChangeNotify chan struct{}
}

type Service struct {
	cfg Config

	db *database.DB

	// Server identity
	serverID      uuid.UUID
	serverIPRange netip.Prefix // This server's allocated /20 VM IP range

	// S3 for shared images
	s3 *storage.S3

	// Route change notification channel (for edge proxy)
	routeChangeNotify chan struct{}

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
		cfg:               cfg,
		db:                cfg.DB,
		serverID:          cfg.ServerID,
		s3:                cfg.S3,
		routeChangeNotify: cfg.RouteChangeNotify,
		dnsResolver:       dnsresolver.NewResolver(),
		metadataServer:    NewMetadataServer(),
		vmToCmd:           make(map[uuid.UUID]*exec.Cmd),
		imageMu:           sync.Mutex{},
		nextTap:           atomic.Int32{},
		activeBuilds:      make(map[uuid.UUID]bool),
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
	slog.Info("starting Zeitwork reconciler", "server_id", s.serverID)

	// Register this server in the cluster
	server, err := s.registerServer(ctx)
	if err != nil {
		return fmt.Errorf("failed to register server: %w", err)
	}
	s.serverIPRange = server.IpRange
	slog.Info("server registered", "server_id", s.serverID, "ip_range", s.serverIPRange, "internal_ip", s.cfg.InternalIP)

	// Start metadata server
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

	// Start server lifecycle loops
	go s.heartbeatLoop(ctx)
	go s.deadServerDetectionLoop(ctx)
	go s.drainMonitorLoop(ctx)
	go s.hostRouteSyncLoop(ctx)

	// Bootstrap: schedule all existing entities once on startup
	if err := s.bootstrap(ctx); err != nil {
		return fmt.Errorf("failed to bootstrap: %w", err)
	}

	// Create WAL listener with callbacks to schedulers
	walListener := listener.New(listener.Config{
		DatabaseURL: s.cfg.DatabaseDirectURL,

		OnDeployment: func(ctx context.Context, id uuid.UUID) {
			s.deploymentScheduler.Schedule(id, time.Now())
			s.notifyRouteChange()
		},

		OnBuild: func(ctx context.Context, id uuid.UUID) {
			s.buildScheduler.Schedule(id, time.Now())

			if deployments, err := s.db.DeploymentFindByBuildID(ctx, id); err != nil {
				slog.Error("failed to find deployments by build_id", "build_id", id, "error", err)
			} else {
				for _, d := range deployments {
					s.deploymentScheduler.Schedule(d.ID, time.Now())
				}
			}
		},

		OnImage: func(ctx context.Context, id uuid.UUID) {
			s.imageScheduler.Schedule(id, time.Now())

			if vms, err := s.db.VMFindByImageID(ctx, id); err != nil {
				slog.Error("failed to find VMs by image_id", "image_id", id, "error", err)
			} else {
				for _, vm := range vms {
					s.vmScheduler.Schedule(vm.ID, time.Now())
				}
			}

			if builds, err := s.db.BuildFindWaitingForBuildImage(ctx); err != nil {
				slog.Error("failed to find builds waiting for build image", "error", err)
			} else {
				for _, b := range builds {
					s.buildScheduler.Schedule(b.ID, time.Now())
				}
			}
		},

		OnVM: func(ctx context.Context, id uuid.UUID) {
			s.vmScheduler.Schedule(id, time.Now())
			s.notifyRouteChange()

			if builds, err := s.db.BuildFindByVMID(ctx, id); err != nil {
				slog.Error("failed to find builds by vm_id", "vm_id", id, "error", err)
			} else {
				for _, b := range builds {
					s.buildScheduler.Schedule(b.ID, time.Now())
				}
			}

			if deployments, err := s.db.DeploymentFindByVMID(ctx, id); err != nil {
				slog.Error("failed to find deployments by vm_id", "vm_id", id, "error", err)
			} else {
				for _, d := range deployments {
					s.deploymentScheduler.Schedule(d.ID, time.Now())
				}
			}
		},

		OnDomain: func(ctx context.Context, id uuid.UUID) {
			s.domainScheduler.Schedule(id, time.Now())
			s.notifyRouteChange()
		},

		OnServer: func(ctx context.Context, id uuid.UUID) {
			s.notifyRouteChange()

			// Re-sync host routes when any server changes
			if err := s.syncHostRoutes(ctx); err != nil {
				slog.Error("failed to sync host routes on server change", "err", err)
			}
		},
	})

	// Start WAL listener (blocks until context is cancelled)
	slog.Info("starting WAL listener for database changes")
	return walListener.Start(ctx)
}

// bootstrap schedules all existing entities for reconciliation on startup
func (s *Service) bootstrap(ctx context.Context) error {
	slog.Info("bootstrapping: scheduling all existing entities")

	// Deployments (all — stateless reconcilers run on every server)
	deployments, err := s.db.DeploymentFind(ctx)
	if err != nil {
		return fmt.Errorf("failed to find deployments: %w", err)
	}
	for _, deployment := range deployments {
		s.deploymentScheduler.Schedule(deployment.ID, time.Now())
	}
	slog.Info("bootstrapped deployments", "count", len(deployments))

	// Builds (all)
	builds, err := s.db.BuildFind(ctx)
	if err != nil {
		return fmt.Errorf("failed to find builds: %w", err)
	}
	for _, build := range builds {
		s.buildScheduler.Schedule(build.ID, time.Now())
	}
	slog.Info("bootstrapped builds", "count", len(builds))

	// Images (all)
	images, err := s.db.ImageFind(ctx)
	if err != nil {
		return fmt.Errorf("failed to find images: %w", err)
	}
	for _, image := range images {
		s.imageScheduler.Schedule(image.ID, time.Now())
	}
	slog.Info("bootstrapped images", "count", len(images))

	// VMs — only bootstrap VMs belonging to this server
	vms, err := s.db.VMFindByServerID(ctx, s.serverID)
	if err != nil {
		return fmt.Errorf("failed to find vms for this server: %w", err)
	}
	for _, vm := range vms {
		s.vmScheduler.Schedule(vm.ID, time.Now())
	}
	slog.Info("bootstrapped vms", "count", len(vms), "server_id", s.serverID)

	// Domains (all)
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
