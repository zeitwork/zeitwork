package reconciler

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
	dnsresolver "github.com/zeitwork/zeitwork/internal/shared/dns"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
	"golang.org/x/crypto/ssh"
)

//go:embed templates/cloud-init.yaml
var cloudInitTemplate string

// Config holds the reconciler configuration
type Config struct {
	DatabaseURL           string        // Database connection string
	ReconcileInterval     time.Duration // How often to run reconciliation loop
	VMPoolSize            int           // Minimum number of VMs to keep in pool
	BuildTimeout          time.Duration // How long before a build times out
	DeploymentGracePeriod time.Duration // Grace period before marking old deployments inactive
	AllowedIPTarget       string        // Allowed IPv4 target customer domains must point to

	// Hetzner configuration
	HetznerToken           string // Hetzner Cloud API token
	HetznerSSHKeyName      string // SSH key name in Hetzner (optional, defaults to "zeitwork-reconciler-key")
	HetznerServerType      string // Server type (e.g., "cx22")
	HetznerImage           string // OS image (e.g., "ubuntu-24.04")
	DockerRegistryURL      string // Docker registry for pulling images
	DockerRegistryUsername string // Docker registry username
	DockerRegistryPassword string // Docker registry password

	// SSH configuration
	SSHPublicKey  string // SSH public key (e.g., "ssh-ed25519 AAAA...")
	SSHPrivateKey string // SSH private key (base64 encoded)
}

// Service is the reconciler service
type Service struct {
	cfg    Config
	db     *database.DB
	logger *slog.Logger
	cancel context.CancelFunc

	// Hetzner client
	hcloudClient  *hcloud.Client
	sshPublicKey  string
	sshPrivateKey []byte

	// Docker client
	dockerClient *client.Client

	// DNS resolution
	dnsResolver     dnsresolver.Resolver
	allowedIPTarget string
}

// NewService creates a new reconciler service
func NewService(cfg Config, logger *slog.Logger) (*Service, error) {
	// Set defaults
	if cfg.ReconcileInterval == 0 {
		cfg.ReconcileInterval = 5 * time.Second
	}
	if cfg.VMPoolSize == 0 {
		cfg.VMPoolSize = 3
	}
	if cfg.BuildTimeout == 0 {
		cfg.BuildTimeout = 10 * time.Minute
	}
	if cfg.DeploymentGracePeriod == 0 {
		cfg.DeploymentGracePeriod = 5 * time.Minute
	}
	if cfg.HetznerServerType == "" {
		cfg.HetznerServerType = "cx22"
	}
	if cfg.HetznerImage == "" {
		cfg.HetznerImage = "ubuntu-24.04"
	}
	if cfg.HetznerSSHKeyName == "" {
		cfg.HetznerSSHKeyName = "zeitwork-reconciler-key"
	}

	// Initialize database connection
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize Hetzner client
	var hcloudClient *hcloud.Client
	if cfg.HetznerToken != "" {
		hcloudClient = hcloud.NewClient(hcloud.WithToken(cfg.HetznerToken))
		logger.Info("Hetzner client initialized")
	} else {
		logger.Warn("No Hetzner token provided, VM management will not work")
	}

	// Initialize Docker client (for pulling and saving images)
	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		logger.Warn("failed to create Docker client, image operations will not work", "error", err)
		dockerClient = nil
	} else {
		logger.Info("Docker client initialized")
	}

	allowedIP := strings.TrimSpace(cfg.AllowedIPTarget)
	if allowedIP == "" {
		return nil, fmt.Errorf("no allowed IP target configured; set NUXT_PUBLIC_DOMAIN_TARGET")
	}

	s := &Service{
		cfg:             cfg,
		db:              db,
		logger:          logger,
		hcloudClient:    hcloudClient,
		dockerClient:    dockerClient,
		dnsResolver:     dnsresolver.NewResolver(),
		allowedIPTarget: allowedIP,
	}

	// Initialize SSH keys if Hetzner is configured
	if hcloudClient != nil {
		if err := s.initializeSSHKeys(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to initialize SSH keys: %w", err)
		}
	}

	return s, nil
}

// Start starts the reconciler service
func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.logger.Info("starting reconciler service",
		"reconcile_interval", s.cfg.ReconcileInterval,
		"vm_pool_size", s.cfg.VMPoolSize,
		"build_timeout", s.cfg.BuildTimeout,
		"deployment_grace_period", s.cfg.DeploymentGracePeriod,
	)

	// Ensure at least one region exists before starting reconciliation
	if err := s.ensureRegionExists(ctx); err != nil {
		s.logger.Error("failed to ensure region exists", "error", err)
		return fmt.Errorf("failed to ensure region exists: %w", err)
	}

	ticker := time.NewTicker(s.cfg.ReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("reconciler stopped")
			return nil
		case <-ticker.C:
			s.logger.Debug("starting reconciliation cycle")
			start := time.Now()

			s.reconcileDomains(ctx)
			s.reconcileBuilds(ctx)
			s.reconcileDeployments(ctx)
			s.reconcileVMs(ctx)

			duration := time.Since(start)
			s.logger.Debug("reconciliation cycle completed", "duration", duration)
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

// ensureRegionExists checks if at least one region exists, and creates one if not
func (s *Service) ensureRegionExists(ctx context.Context) error {
	regions, err := s.db.Queries().GetAllRegions(ctx)
	if err != nil {
		return fmt.Errorf("failed to get regions: %w", err)
	}

	if len(regions) > 0 {
		s.logger.Info("regions already exist", "count", len(regions))
		return nil
	}

	s.logger.Info("no regions found, creating default region")

	// Use first available Hetzner location from config, or default to "nbg1"
	defaultLocation := "nbg1"

	// Create region with load balancer
	if err := s.createRegionWithLoadBalancer(ctx, defaultLocation); err != nil {
		return fmt.Errorf("failed to create region: %w", err)
	}

	return nil
}

// createRegionWithLoadBalancer creates a new region and its associated load balancer
func (s *Service) createRegionWithLoadBalancer(ctx context.Context, locationName string) error {
	if s.hcloudClient == nil {
		return fmt.Errorf("hetzner client not available, cannot create load balancer")
	}

	s.logger.Info("creating region with load balancer", "location", locationName)

	// Get next region number
	nextNo, err := s.db.Queries().GetNextRegionNumber(ctx)
	if err != nil {
		return fmt.Errorf("failed to get next region number: %w", err)
	}

	// Get Hetzner location
	location, _, err := s.hcloudClient.Location.GetByName(ctx, locationName)
	if err != nil {
		return fmt.Errorf("failed to get location: %w", err)
	}
	if location == nil {
		return fmt.Errorf("location '%s' not found", locationName)
	}

	// Get load balancer type (smallest one for cost efficiency)
	lbType, _, err := s.hcloudClient.LoadBalancerType.GetByName(ctx, "lb11")
	if err != nil {
		return fmt.Errorf("failed to get load balancer type: %w", err)
	}
	if lbType == nil {
		return fmt.Errorf("load balancer type 'lb11' not found")
	}

	// Create load balancer
	lbName := fmt.Sprintf("zeitwork-lb-%d", nextNo)
	s.logger.Info("creating load balancer", "name", lbName, "location", locationName)

	lbResult, _, err := s.hcloudClient.LoadBalancer.Create(ctx, hcloud.LoadBalancerCreateOpts{
		Name:             lbName,
		LoadBalancerType: lbType,
		Location:         location,
		PublicInterface:  hcloud.Ptr(true),
		Labels: map[string]string{
			"managed-by": "zeitwork",
			"region":     locationName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create load balancer: %w", err)
	}

	// Wait for load balancer creation
	if err := s.hcloudClient.Action.WaitFor(ctx, lbResult.Action); err != nil {
		return fmt.Errorf("failed to wait for load balancer creation: %w", err)
	}

	lb := lbResult.LoadBalancer

	// Get IPv4 and IPv6 addresses
	ipv4 := lb.PublicNet.IPv4.IP.String()
	ipv6 := lb.PublicNet.IPv6.IP.String()

	s.logger.Info("load balancer created",
		"load_balancer_id", lb.ID,
		"ipv4", ipv4,
		"ipv6", ipv6,
	)

	// Create region in database
	regionID := uuid.New()
	region, err := s.db.Queries().CreateRegion(ctx, &database.CreateRegionParams{
		ID:               regionID,
		No:               nextNo,
		Name:             locationName,
		LoadBalancerIpv4: ipv4,
		LoadBalancerIpv6: ipv6,
		LoadBalancerNo:   pgtype.Int4{Int32: int32(lb.ID), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to create region in database: %w", err)
	}

	s.logger.Info("region created successfully",
		"region_id", uuid.ToString(region.ID),
		"region_no", region.No,
		"name", region.Name,
		"load_balancer_no", region.LoadBalancerNo.Int32,
		"ipv4", region.LoadBalancerIpv4,
		"ipv6", region.LoadBalancerIpv6,
	)

	return nil
}

const domainResolveTimeout = 10 * time.Second

// reconcileDomains verifies domain ownership by ensuring domains point to approved targets
func (s *Service) reconcileDomains(ctx context.Context) {
	domains, err := s.db.Queries().GetUnverifiedDomains(ctx)
	if err != nil {
		s.logger.Error("failed to get unverified domains", "error", err)
		return
	}

	if len(domains) == 0 {
		return
	}

	if s.allowedIPTarget == "" {
		s.logger.Warn("no allowed IP target configured; skipping domain verification")
		return
	}

	s.logger.Info("reconciling domains",
		"count", len(domains),
		"allowed_ip_target", s.allowedIPTarget,
	)

	for _, domain := range domains {
		domainID := uuid.ToString(domain.ID)
		domainName := domain.Name

		resolveCtx, cancel := context.WithTimeout(ctx, domainResolveTimeout)
		resolution, err := s.dnsResolver.Resolve(resolveCtx, domainName)
		cancel()
		if err != nil {
			s.logger.Debug("domain resolution failed",
				"domain_id", domainID,
				"domain_name", domainName,
				"error", err,
			)
			continue
		}

		matchedIP := matchesAllowedIP(resolution, s.allowedIPTarget)

		s.logger.Debug("domain resolution complete",
			"domain_id", domainID,
			"domain_name", domainName,
			"host_chain", resolution.HostChain,
			"ipv4", resolution.IPv4,
			"ipv6", resolution.IPv6,
			"matched_ip", matchedIP,
		)

		if !matchedIP {
			s.logger.Debug("domain does not point to allowed targets",
				"domain_id", domainID,
				"domain_name", domainName,
			)
			continue
		}

		if err := s.db.Queries().MarkDomainVerified(ctx, domain.ID); err != nil {
			s.logger.Error("failed to mark domain as verified",
				"domain_id", domainID,
				"domain_name", domainName,
				"error", err,
			)
			continue
		}

		s.logger.Info("domain verified",
			"domain_id", domainID,
			"domain_name", domainName,
			"matched_ip", matchedIP,
		)
	}
}

func matchesAllowedIP(resolution *dnsresolver.Resolution, allowedIP string) bool {
	if allowedIP == "" {
		return false
	}

	for _, ip := range resolution.IPv4 {
		if ip == allowedIP {
			return true
		}
	}

	return false
}

// reconcileBuilds monitors build timeouts
func (s *Service) reconcileBuilds(ctx context.Context) {
	builds, err := s.db.Queries().GetTimedOutBuilds(ctx)
	if err != nil {
		s.logger.Error("failed to get timed out builds", "error", err)
		return
	}

	if len(builds) == 0 {
		return
	}

	s.logger.Info("reconciling timed out builds", "count", len(builds))

	for _, build := range builds {
		buildID := uuid.ToString(build.ID)

		s.logger.Warn("marking build as timed out",
			"build_id", buildID,
			"project_id", uuid.ToString(build.ProjectID),
		)

		if err := s.db.Queries().MarkBuildTimedOut(ctx, build.ID); err != nil {
			s.logger.Error("failed to mark build as timed out",
				"build_id", buildID,
				"error", err,
			)
		}
	}
}

// reconcileDeployments manages the deployment lifecycle
func (s *Service) reconcileDeployments(ctx context.Context) {
	s.reconcileQueuedDeployments(ctx)
	s.reconcileBuildingDeploymentsWithoutImage(ctx)
	s.reconcileBuildingDeploymentsWithoutVM(ctx)
	s.reconcileReadyDeployments(ctx)
	s.reconcileInactiveDeployments(ctx)
	s.reconcileFailedDeployments(ctx)
}

// reconcileQueuedDeployments creates builds for queued deployments
func (s *Service) reconcileQueuedDeployments(ctx context.Context) {
	deployments, err := s.db.Queries().GetQueuedDeployments(ctx)
	if err != nil {
		s.logger.Error("failed to get queued deployments", "error", err)
		return
	}

	if len(deployments) == 0 {
		return
	}

	s.logger.Info("reconciling queued deployments", "count", len(deployments))

	for _, deployment := range deployments {
		deploymentID := uuid.ToString(deployment.ID)

		s.logger.Info("creating build for queued deployment",
			"deployment_id", deploymentID,
			"project_id", uuid.ToString(deployment.ProjectID),
		)

		// Get project environment to retrieve branch
		environment, err := s.db.Queries().GetProjectEnvironmentByID(ctx, deployment.EnvironmentID)
		if err != nil {
			s.logger.Error("failed to get project environment",
				"deployment_id", deploymentID,
				"environment_id", uuid.ToString(deployment.EnvironmentID),
				"error", err,
			)
			continue
		}

		// Create a new build
		buildID := uuid.New()
		params := &database.CreateBuildParams{
			ID:             buildID,
			Status:         database.BuildStatusesQueued,
			ProjectID:      deployment.ProjectID,
			GithubCommit:   deployment.GithubCommit,
			GithubBranch:   environment.Branch,
			OrganisationID: deployment.OrganisationID,
		}

		build, err := s.db.Queries().CreateBuild(ctx, params)
		if err != nil {
			s.logger.Error("failed to create build",
				"deployment_id", deploymentID,
				"error", err,
			)
			continue
		}

		// Update deployment with build_id
		if err := s.db.Queries().UpdateDeploymentWithBuild(ctx, &database.UpdateDeploymentWithBuildParams{
			ID:      deployment.ID,
			BuildID: build.ID,
		}); err != nil {
			s.logger.Error("failed to update deployment with build",
				"deployment_id", deploymentID,
				"build_id", uuid.ToString(build.ID),
				"error", err,
			)
			continue
		}

		s.logger.Info("deployment transitioned to building",
			"deployment_id", deploymentID,
			"build_id", uuid.ToString(build.ID),
		)
	}
}

// reconcileBuildingDeploymentsWithoutImage checks build status and copies image_id
func (s *Service) reconcileBuildingDeploymentsWithoutImage(ctx context.Context) {
	deployments, err := s.db.Queries().GetBuildingDeploymentsWithoutImage(ctx)
	if err != nil {
		s.logger.Error("failed to get building deployments without image", "error", err)
		return
	}

	if len(deployments) == 0 {
		return
	}

	s.logger.Info("reconciling building deployments without image", "count", len(deployments))

	for _, row := range deployments {
		deploymentID := uuid.ToString(row.ID)
		buildStatus := row.BuildStatus

		s.logger.Debug("checking build status",
			"deployment_id", deploymentID,
			"build_status", buildStatus,
		)

		switch buildStatus {
		case database.BuildStatusesReady:
			// Build is ready, copy image_id to deployment
			if !row.BuildImageID.Valid {
				s.logger.Warn("build is ready but has no image_id",
					"deployment_id", deploymentID,
					"build_id", uuid.ToString(row.BuildID),
				)
				continue
			}

			s.logger.Info("copying image_id from build to deployment",
				"deployment_id", deploymentID,
				"image_id", uuid.ToString(row.BuildImageID),
			)

			if err := s.db.Queries().UpdateDeploymentWithImage(ctx, &database.UpdateDeploymentWithImageParams{
				ID:      row.ID,
				ImageID: row.BuildImageID,
			}); err != nil {
				s.logger.Error("failed to update deployment with image",
					"deployment_id", deploymentID,
					"error", err,
				)
			}

		case database.BuildStatusesError, database.BuildStatusesCanceled:
			// Build failed, mark deployment as failed
			s.logger.Warn("build failed, marking deployment as failed",
				"deployment_id", deploymentID,
				"build_status", buildStatus,
			)

			if err := s.db.Queries().MarkDeploymentFailed(ctx, row.ID); err != nil {
				s.logger.Error("failed to mark deployment as failed",
					"deployment_id", deploymentID,
					"error", err,
				)
			}

		default:
			// Build is still pending or building, wait
			continue
		}
	}
}

// reconcileBuildingDeploymentsWithoutVM assigns VMs to deployments with images
func (s *Service) reconcileBuildingDeploymentsWithoutVM(ctx context.Context) {
	deployments, err := s.db.Queries().GetBuildingDeploymentsWithoutVM(ctx)
	if err != nil {
		s.logger.Error("failed to get building deployments without VM", "error", err)
		return
	}

	if len(deployments) == 0 {
		return
	}

	s.logger.Info("reconciling building deployments without VM", "count", len(deployments))

	// Get available pool VMs
	poolVMs, err := s.db.Queries().GetPoolVMs(ctx)
	if err != nil {
		s.logger.Error("failed to get pool VMs", "error", err)
		return
	}

	// Assign VMs to deployments
	poolIndex := 0
	for _, deployment := range deployments {
		deploymentID := uuid.ToString(deployment.ID)

		// Check if we have a pool VM available
		if poolIndex < len(poolVMs) {
			vm := poolVMs[poolIndex]
			poolIndex++

			s.logger.Info("assigning pool VM to deployment",
				"deployment_id", deploymentID,
				"vm_id", uuid.ToString(vm.ID),
			)

			// Assign VM to deployment (updates VM status to "starting")
			if err := s.db.Queries().AssignVMToDeployment(ctx, &database.AssignVMToDeploymentParams{
				ID:      vm.ID,
				ImageID: deployment.ImageID,
			}); err != nil {
				s.logger.Error("failed to assign VM",
					"deployment_id", deploymentID,
					"vm_id", uuid.ToString(vm.ID),
					"error", err,
				)
				continue
			}

			// Deploy container to the VM if Hetzner is configured
			if s.hcloudClient != nil {
				if err := s.deployContainerToVM(ctx, vm, deployment.ImageID); err != nil {
					s.logger.Error("failed to deploy container to VM",
						"deployment_id", deploymentID,
						"vm_id", uuid.ToString(vm.ID),
						"error", err,
					)
					// Mark deployment as failed
					s.db.Queries().MarkDeploymentFailed(ctx, deployment.ID)
					// Return VM to pool
					s.db.Queries().ReturnVMToPool(ctx, vm.ID)
					continue
				}
			}

			// Update deployment with VM
			if err := s.db.Queries().UpdateDeploymentWithVM(ctx, &database.UpdateDeploymentWithVMParams{
				ID:   deployment.ID,
				VmID: vm.ID,
			}); err != nil {
				s.logger.Error("failed to update deployment with VM",
					"deployment_id", deploymentID,
					"vm_id", uuid.ToString(vm.ID),
					"error", err,
				)
				continue
			}

			s.logger.Info("deployment transitioned to ready",
				"deployment_id", deploymentID,
				"vm_id", uuid.ToString(vm.ID),
			)
		} else {
			// No pool VM available, log and continue (VM reconciliation will create more)
			s.logger.Warn("no pool VM available for deployment",
				"deployment_id", deploymentID,
			)
		}
	}
}

// reconcileReadyDeployments checks for supersession and marks old deployments inactive
func (s *Service) reconcileReadyDeployments(ctx context.Context) {
	deployments, err := s.db.Queries().GetReadyDeployments(ctx)
	if err != nil {
		s.logger.Error("failed to get ready deployments", "error", err)
		return
	}

	if len(deployments) == 0 {
		return
	}

	// Group deployments by project+environment
	type groupKey struct {
		projectID     string
		environmentID string
	}
	groups := make(map[groupKey][]*database.Deployment)
	for _, d := range deployments {
		key := groupKey{
			projectID:     uuid.ToString(d.ProjectID),
			environmentID: uuid.ToString(d.EnvironmentID),
		}
		groups[key] = append(groups[key], d)
	}

	s.logger.Debug("reconciling ready deployments", "groups", len(groups))

	// Process each group
	for key, group := range groups {
		if len(group) <= 1 {
			// Only one deployment in this project+environment, nothing to do
			continue
		}

		// Newest is at index 0 (query orders by created_at DESC)
		newest := group[0]
		newestID := uuid.ToString(newest.ID)

		// Check if newest has been ready for grace period
		timeSinceReady := time.Since(newest.UpdatedAt.Time)
		if timeSinceReady < s.cfg.DeploymentGracePeriod {
			s.logger.Debug("newest deployment still in grace period",
				"deployment_id", newestID,
				"project_id", key.projectID,
				"environment_id", key.environmentID,
				"time_since_ready", timeSinceReady,
			)
			continue
		}

		// Mark older deployments as inactive
		for i := 1; i < len(group); i++ {
			oldDeployment := group[i]
			oldID := uuid.ToString(oldDeployment.ID)

			s.logger.Info("marking old deployment as inactive (superseded)",
				"deployment_id", oldID,
				"newer_deployment_id", newestID,
				"project_id", key.projectID,
				"environment_id", key.environmentID,
			)

			if err := s.db.Queries().MarkDeploymentInactive(ctx, oldDeployment.ID); err != nil {
				s.logger.Error("failed to mark deployment as inactive",
					"deployment_id", oldID,
					"error", err,
				)
			}
		}
	}
}

// reconcileInactiveDeployments cleans up VMs from inactive deployments
func (s *Service) reconcileInactiveDeployments(ctx context.Context) {
	deployments, err := s.db.Queries().GetInactiveDeployments(ctx)
	if err != nil {
		s.logger.Error("failed to get inactive deployments", "error", err)
		return
	}

	if len(deployments) == 0 {
		return
	}

	s.logger.Info("reconciling inactive deployments", "count", len(deployments))

	for _, deployment := range deployments {
		deploymentID := uuid.ToString(deployment.ID)
		vmID := uuid.ToString(deployment.VmID)

		s.logger.Info("cleaning up VM from inactive deployment",
			"deployment_id", deploymentID,
			"vm_id", vmID,
		)

		// Clear image from VM
		if err := s.db.Queries().ClearVMImage(ctx, deployment.VmID); err != nil {
			s.logger.Error("failed to clear VM image",
				"vm_id", vmID,
				"error", err,
			)
			continue
		}

		// Return VM to pool
		if err := s.db.Queries().ReturnVMToPool(ctx, deployment.VmID); err != nil {
			s.logger.Error("failed to return VM to pool",
				"deployment_id", deploymentID,
				"vm_id", vmID,
				"error", err,
			)
			continue
		}

		// Clear VM from deployment
		if err := s.db.Queries().ClearDeploymentVM(ctx, deployment.ID); err != nil {
			s.logger.Error("failed to clear VM from deployment",
				"deployment_id", deploymentID,
				"error", err,
			)
		}

		s.logger.Info("VM returned to pool",
			"deployment_id", deploymentID,
			"vm_id", vmID,
		)
	}
}

// reconcileFailedDeployments cleans up VMs from failed deployments
func (s *Service) reconcileFailedDeployments(ctx context.Context) {
	deployments, err := s.db.Queries().GetFailedDeployments(ctx)
	if err != nil {
		s.logger.Error("failed to get failed deployments", "error", err)
		return
	}

	if len(deployments) == 0 {
		return
	}

	s.logger.Info("reconciling failed deployments", "count", len(deployments))

	for _, deployment := range deployments {
		deploymentID := uuid.ToString(deployment.ID)
		vmID := uuid.ToString(deployment.VmID)

		s.logger.Info("cleaning up VM from failed deployment",
			"deployment_id", deploymentID,
			"vm_id", vmID,
		)

		// Clear image from VM
		if err := s.db.Queries().ClearVMImage(ctx, deployment.VmID); err != nil {
			s.logger.Error("failed to clear VM image",
				"vm_id", vmID,
				"error", err,
			)
			continue
		}

		// Return VM to pool (or could mark for deletion)
		if err := s.db.Queries().ReturnVMToPool(ctx, deployment.VmID); err != nil {
			s.logger.Error("failed to return VM to pool",
				"deployment_id", deploymentID,
				"vm_id", vmID,
				"error", err,
			)
			continue
		}

		// Clear VM from deployment
		if err := s.db.Queries().ClearDeploymentVM(ctx, deployment.ID); err != nil {
			s.logger.Error("failed to clear VM from deployment",
				"deployment_id", deploymentID,
				"error", err,
			)
		}

		s.logger.Info("VM returned to pool",
			"deployment_id", deploymentID,
			"vm_id", vmID,
		)
	}
}

// reconcileDeletingVMs deletes Hetzner servers for VMs marked for deletion
func (s *Service) reconcileDeletingVMs(ctx context.Context) {
	vms, err := s.db.Queries().GetDeletingVMs(ctx)
	if err != nil {
		s.logger.Error("failed to get deleting VMs", "error", err)
		return
	}

	if len(vms) == 0 {
		return
	}

	s.logger.Info("reconciling deleting VMs", "count", len(vms))

	for _, vm := range vms {
		vmID := uuid.ToString(vm.ID)

		s.logger.Info("deleting Hetzner server",
			"vm_id", vmID,
			"vm_no", vm.No,
			"server_no", vm.ServerNo,
		)

		// Delete Hetzner server if client is available and server_no is set
		if s.hcloudClient != nil && vm.ServerNo.Valid {
			server, _, err := s.hcloudClient.Server.GetByID(ctx, int64(vm.ServerNo.Int32))
			if err != nil {
				s.logger.Error("failed to get Hetzner server for deletion",
					"vm_id", vmID,
					"server_no", vm.ServerNo.Int32,
					"error", err,
				)
				continue
			}

			if server != nil {
				_, _, err := s.hcloudClient.Server.DeleteWithResult(ctx, server)
				if err != nil {
					s.logger.Error("failed to delete Hetzner server",
						"vm_id", vmID,
						"server_no", vm.ServerNo.Int32,
						"server_id", server.ID,
						"error", err,
					)
					continue
				}

				s.logger.Info("Hetzner server deleted",
					"vm_id", vmID,
					"server_no", vm.ServerNo.Int32,
					"server_id", server.ID,
				)
			} else {
				s.logger.Warn("Hetzner server not found, marking VM as deleted anyway",
					"vm_id", vmID,
					"server_no", vm.ServerNo.Int32,
				)
			}
		} else if !vm.ServerNo.Valid {
			s.logger.Info("VM has no Hetzner server (server_no not set), marking as deleted",
				"vm_id", vmID,
			)
		}

		// Mark VM as deleted in database
		if err := s.db.Queries().MarkVMDeleted(ctx, vm.ID); err != nil {
			s.logger.Error("failed to mark VM as deleted",
				"vm_id", vmID,
				"error", err,
			)
			continue
		}

		s.logger.Info("VM marked as deleted",
			"vm_id", vmID,
			"vm_no", vm.No,
		)
	}
}

// reconcileVMs maintains the pool of ready VMs
func (s *Service) reconcileVMs(ctx context.Context) {
	// First, delete VMs marked for deletion
	s.reconcileDeletingVMs(ctx)

	// Get current pool VMs (including initializing ones to avoid creating duplicates)
	poolVMs, err := s.db.Queries().GetPoolAndInitializingVMs(ctx)
	if err != nil {
		s.logger.Error("failed to get pool VMs", "error", err)
		return
	}

	currentPoolSize := len(poolVMs)
	needed := s.cfg.VMPoolSize - currentPoolSize

	if needed <= 0 {
		return
	}

	s.logger.Info("creating VMs to maintain pool",
		"current_pool_size", currentPoolSize,
		"target_pool_size", s.cfg.VMPoolSize,
		"creating", needed,
	)

	// Get all regions
	regions, err := s.db.Queries().GetAllRegions(ctx)
	if err != nil {
		s.logger.Error("failed to get regions", "error", err)
		return
	}

	if len(regions) == 0 {
		s.logger.Warn("no regions available to create VMs")
		return
	}

	// Create needed VMs (distribute across regions round-robin)
	for i := 0; i < needed; i++ {
		region := regions[i%len(regions)]

		// Get next VM number
		nextNo, err := s.db.Queries().GetNextVMNumber(ctx)
		if err != nil {
			s.logger.Error("failed to get next VM number", "error", err)
			continue
		}

		// Generate VM details
		vmID := uuid.New()
		port := int32(3000)

		s.logger.Info("creating pool VM",
			"vm_no", nextNo,
			"region_id", uuid.ToString(region.ID),
		)

		// Create VM in database with "initializing" status
		params := &database.CreateVMParams{
			ID:       vmID,
			No:       nextNo,
			Status:   "initializing",
			RegionID: region.ID,
			Port:     port,
		}

		vm, err := s.db.Queries().CreateVM(ctx, params)
		if err != nil {
			s.logger.Error("failed to create VM in database",
				"vm_no", nextNo,
				"error", err,
			)
			continue
		}

		s.logger.Info("VM database record created, creating Hetzner server",
			"vm_id", uuid.ToString(vmID),
			"vm_no", nextNo,
		)

		// Create actual Hetzner server if Hetzner is configured
		if s.hcloudClient != nil {
			if err := s.createHetznerServer(ctx, vm, region); err != nil {
				s.logger.Error("failed to create Hetzner server",
					"vm_id", uuid.ToString(vm.ID),
					"vm_no", vm.No,
					"error", err,
				)
				// Mark VM as failed
				// TODO: Add MarkVMFailed query
				continue
			}

			s.logger.Info("pool VM ready",
				"vm_id", uuid.ToString(vmID),
				"vm_no", nextNo,
				"region_id", uuid.ToString(region.ID),
			)
		} else {
			// No Hetzner client, just mark as pooling (for testing without Hetzner)
			s.logger.Warn("No Hetzner client available, marking VM as pooling without real server")
			// Update status to pooling
			if err := s.db.Queries().ReturnVMToPool(ctx, vm.ID); err != nil {
				s.logger.Error("failed to mark VM as pooling", "error", err)
			}
		}
	}
}

// createHetznerServer creates a Hetzner server and installs Docker
func (s *Service) createHetznerServer(ctx context.Context, vm *database.Vm, region *database.Region) error {
	// Get SSH key from Hetzner
	sshKey, _, err := s.hcloudClient.SSHKey.GetByName(ctx, s.cfg.HetznerSSHKeyName)
	if err != nil {
		return fmt.Errorf("failed to get SSH key: %w", err)
	}
	if sshKey == nil {
		return fmt.Errorf("SSH key '%s' not found in Hetzner", s.cfg.HetznerSSHKeyName)
	}

	// Get server type
	serverType, _, err := s.hcloudClient.ServerType.GetByName(ctx, s.cfg.HetznerServerType)
	if err != nil {
		return fmt.Errorf("failed to get server type: %w", err)
	}
	if serverType == nil {
		return fmt.Errorf("server type '%s' not found", s.cfg.HetznerServerType)
	}

	// Get image
	hetznerImage, _, err := s.hcloudClient.Image.GetByNameAndArchitecture(ctx, s.cfg.HetznerImage, hcloud.ArchitectureX86)
	if err != nil {
		return fmt.Errorf("failed to get image: %w", err)
	}
	if hetznerImage == nil {
		return fmt.Errorf("image '%s' not found", s.cfg.HetznerImage)
	}

	// Get location (from region.name which contains the Hetzner location code)
	location, _, err := s.hcloudClient.Location.GetByName(ctx, region.Name)
	if err != nil {
		return fmt.Errorf("failed to get location: %w", err)
	}
	if location == nil {
		return fmt.Errorf("location '%s' not found", region.Name)
	}

	// Create server name
	serverName := fmt.Sprintf("zeitwork-vm-%d", vm.No)

	s.logger.Info("creating Hetzner server",
		"server_name", serverName,
		"vm_no", vm.No,
		"location", region.Name,
		"server_type", s.cfg.HetznerServerType,
	)

	// Create server with public IPv4 for better compatibility
	result, _, err := s.hcloudClient.Server.Create(ctx, hcloud.ServerCreateOpts{
		Name:       serverName,
		ServerType: serverType,
		Image:      hetznerImage,
		Location:   location,
		SSHKeys:    []*hcloud.SSHKey{sshKey},
		UserData:   cloudInitTemplate,
		PublicNet: &hcloud.ServerCreatePublicNet{
			EnableIPv4: true, // Enable IPv4 for compatibility
			EnableIPv6: true, // Keep IPv6 as well
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	s.logger.Info("waiting for Hetzner server creation",
		"server_name", serverName,
		"server_id", result.Server.ID,
	)

	// Wait for server creation
	if err := s.hcloudClient.Action.WaitFor(ctx, result.Action); err != nil {
		return fmt.Errorf("failed to wait for server creation: %w", err)
	}

	// Get public IPv4 address
	var publicIP string
	if result.Server.PublicNet.IPv4.IP != nil {
		publicIP = result.Server.PublicNet.IPv4.IP.String()
	} else {
		return fmt.Errorf("server created without IPv4 address")
	}

	s.logger.Info("Hetzner server created successfully",
		"server_id", result.Server.ID,
		"server_name", serverName,
		"public_ip", publicIP,
	)

	// Update VM with Hetzner server ID (server_no)
	if err := s.db.Queries().UpdateVMHetznerID(ctx, &database.UpdateVMHetznerIDParams{
		ID:       vm.ID,
		ServerNo: pgtype.Int4{Int32: int32(result.Server.ID), Valid: true},
	}); err != nil {
		return fmt.Errorf("failed to update VM with Hetzner server ID: %w", err)
	}

	// Update VM with server details
	if err := s.db.Queries().UpdateVMServerDetails(ctx, &database.UpdateVMServerDetailsParams{
		ID:         vm.ID,
		ServerName: pgtype.Text{String: serverName, Valid: true},
		ServerType: pgtype.Text{String: serverType.Name, Valid: true},
		PublicIp:   pgtype.Text{String: publicIP, Valid: true},
	}); err != nil {
		return fmt.Errorf("failed to update VM with server details: %w", err)
	}

	// Docker is installed via cloud-init during server creation (see UserData in create call)
	// Wait for VM to be ready in the background (SSH accessible + Docker installed)
	s.logger.Info("VM created, waiting for readiness check in background",
		"vm_id", uuid.ToString(vm.ID),
		"vm_no", vm.No,
		"public_ip", publicIP,
	)

	// Fetch fresh VM record with updated public_ip for readiness check
	freshVM, err := s.db.Queries().GetVMByID(ctx, vm.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch updated VM: %w", err)
	}

	// Start background readiness check with fresh VM object
	go s.waitForVMReady(context.Background(), freshVM)

	return nil
}

// deployContainerToVM deploys a Docker container to a VM
func (s *Service) deployContainerToVM(ctx context.Context, vm *database.Vm, imageID pgtype.UUID) error {
	// Get image details from database
	img, err := s.db.Queries().GetImageByID(ctx, imageID)
	if err != nil {
		return fmt.Errorf("failed to get image details: %w", err)
	}

	// Construct full image name
	imageName := fmt.Sprintf("%s/%s:%s", img.Registry, img.Repository, img.Tag)
	containerName := fmt.Sprintf("zeitwork-deployment-%s", uuid.ToString(vm.ID)[:8])

	s.logger.Info("deploying container to VM using docker save/load",
		"vm_id", uuid.ToString(vm.ID),
		"vm_no", vm.No,
		"image_name", imageName,
		"container_name", containerName,
	)

	// 1. Pull image on reconciler (with credentials)
	if err := s.pullImageLocally(ctx, imageName); err != nil {
		return fmt.Errorf("failed to pull image locally: %w", err)
	}

	// 2. Save image to tar file
	tarPath := fmt.Sprintf("/tmp/zeitwork-image-%s.tar", uuid.ToString(vm.ID)[:8])
	if err := s.saveImageToTar(ctx, imageName, tarPath); err != nil {
		return fmt.Errorf("failed to save image to tar: %w", err)
	}
	defer os.Remove(tarPath) // Cleanup local tar

	s.logger.Info("image saved to tar",
		"tar_path", tarPath,
		"image_name", imageName,
	)

	// 3. Transfer tar to VM via SCP
	remoteTarPath := "/tmp/image.tar"
	if err := s.transferFileToVM(ctx, vm, tarPath, remoteTarPath); err != nil {
		return fmt.Errorf("failed to transfer image to VM: %w", err)
	}

	s.logger.Info("image transferred to VM", "vm_no", vm.No)

	// 4. Load image on VM from tar
	if err := s.loadImageOnVM(ctx, vm, remoteTarPath); err != nil {
		return fmt.Errorf("failed to load image on VM: %w", err)
	}

	s.logger.Info("image loaded on VM", "vm_no", vm.No)

	// 5. Run container (no registry login needed!)
	if err := s.runContainerOnVM(ctx, vm, imageName, containerName); err != nil {
		return fmt.Errorf("failed to run container on VM: %w", err)
	}

	s.logger.Info("container running on VM",
		"vm_id", uuid.ToString(vm.ID),
		"port", vm.Port,
	)

	// 6. Mark VM as running
	if err := s.db.Queries().MarkVMRunning(ctx, vm.ID); err != nil {
		return fmt.Errorf("failed to mark VM as running: %w", err)
	}

	s.logger.Info("deployment complete, VM marked as running",
		"vm_id", uuid.ToString(vm.ID),
	)

	return nil
}

// pullImageLocally pulls a Docker image on the reconciler with registry credentials
func (s *Service) pullImageLocally(ctx context.Context, imageName string) error {
	if s.dockerClient == nil {
		return fmt.Errorf("docker client not available")
	}

	s.logger.Info("pulling image locally", "image", imageName)

	// Authenticate to registry
	authConfig := registry.AuthConfig{
		Username: s.cfg.DockerRegistryUsername,
		Password: s.cfg.DockerRegistryPassword,
	}
	encodedAuth, err := json.Marshal(authConfig)
	if err != nil {
		return fmt.Errorf("failed to encode auth: %w", err)
	}
	authStr := base64.URLEncoding.EncodeToString(encodedAuth)

	// Pull image
	reader, err := s.dockerClient.ImagePull(ctx, imageName, image.PullOptions{
		RegistryAuth: authStr,
	})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	// Wait for pull to complete (read all output)
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("failed to read pull output: %w", err)
	}

	s.logger.Info("image pulled successfully", "image", imageName)
	return nil
}

// saveImageToTar saves a Docker image to a tar file
func (s *Service) saveImageToTar(ctx context.Context, imageName, tarPath string) error {
	if s.dockerClient == nil {
		return fmt.Errorf("docker client not available")
	}

	s.logger.Debug("saving image to tar", "image", imageName, "tar_path", tarPath)

	reader, err := s.dockerClient.ImageSave(ctx, []string{imageName})
	if err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}
	defer reader.Close()

	// Create tar file
	file, err := os.Create(tarPath)
	if err != nil {
		return fmt.Errorf("failed to create tar file: %w", err)
	}
	defer file.Close()

	// Copy image data to file
	written, err := io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to write tar file: %w", err)
	}

	s.logger.Debug("image saved to tar",
		"tar_path", tarPath,
		"size_bytes", written,
	)

	return nil
}

// transferFileToVM transfers a file to a VM via SCP over SSH
func (s *Service) transferFileToVM(ctx context.Context, vm *database.Vm, localPath, remotePath string) error {
	// Get SSH client
	sshClient, err := s.getSSHClient(ctx, vm)
	if err != nil {
		return fmt.Errorf("failed to get SSH client: %w", err)
	}
	defer sshClient.Close()

	// Open local file
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	// Get file size for logging
	stat, _ := localFile.Stat()
	fileSize := stat.Size()

	s.logger.Info("transferring file to VM",
		"vm_no", vm.No,
		"local_path", localPath,
		"remote_path", remotePath,
		"size_bytes", fileSize,
	)

	// Create SSH session for transfer
	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	// Pipe file content to remote cat command
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	// Start remote cat command
	cmd := fmt.Sprintf("cat > %s", remotePath)
	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("failed to start remote command: %w", err)
	}

	// Copy file to remote
	if _, err := io.Copy(stdin, localFile); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}
	stdin.Close()

	// Wait for command to finish
	if err := session.Wait(); err != nil {
		return fmt.Errorf("failed to complete file transfer: %w", err)
	}

	s.logger.Info("file transferred successfully",
		"vm_no", vm.No,
		"size_bytes", fileSize,
	)

	return nil
}

// loadImageOnVM loads a Docker image from tar file on the VM
func (s *Service) loadImageOnVM(ctx context.Context, vm *database.Vm, remoteTarPath string) error {
	sshClient, err := s.getSSHClient(ctx, vm)
	if err != nil {
		return err
	}
	defer sshClient.Close()

	s.logger.Info("loading image on VM", "vm_no", vm.No, "tar_path", remoteTarPath)

	// Load image and remove tar file
	loadCmd := fmt.Sprintf("docker load -i %s && rm %s", remoteTarPath, remoteTarPath)
	if err := s.executeSSHCommand(sshClient, loadCmd); err != nil {
		return err
	}

	s.logger.Info("image loaded successfully on VM", "vm_no", vm.No)
	return nil
}

// runContainerOnVM runs a Docker container on the VM
func (s *Service) runContainerOnVM(ctx context.Context, vm *database.Vm, imageName, containerName string) error {
	sshClient, err := s.getSSHClient(ctx, vm)
	if err != nil {
		return err
	}
	defer sshClient.Close()

	s.logger.Info("running container on VM",
		"vm_no", vm.No,
		"image", imageName,
		"container", containerName,
		"port", vm.Port,
	)

	// Run container with port mapping
	runCmd := fmt.Sprintf("docker run -d -p %d:3000 --name %s --restart unless-stopped %s",
		vm.Port,
		containerName,
		imageName,
	)

	if err := s.executeSSHCommand(sshClient, runCmd); err != nil {
		return err
	}

	s.logger.Info("container started successfully", "vm_no", vm.No, "container", containerName)
	return nil
}

// waitForVMReady waits for a VM to be SSH-accessible and have Docker installed
// This runs in a background goroutine and marks the VM as pooling when ready
func (s *Service) waitForVMReady(ctx context.Context, vm *database.Vm) {
	vmID := uuid.ToString(vm.ID)

	s.logger.Info("starting VM readiness check",
		"vm_id", vmID,
		"vm_no", vm.No,
		"public_ip", vm.PublicIp.String,
		"has_public_ip", vm.PublicIp.Valid,
	)

	// Create context with 5 minute timeout
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Poll every 10 seconds
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	checkCount := 0
	for {
		select {
		case <-ctx.Done():
			s.logger.Error("VM readiness check timed out",
				"vm_id", vmID,
				"vm_no", vm.No,
				"checks_performed", checkCount,
			)
			// Mark VM for deletion since it never became ready
			if err := s.db.Queries().MarkVMDeleting(context.Background(), vm.ID); err != nil {
				s.logger.Error("failed to mark unready VM for deletion",
					"vm_id", vmID,
					"error", err,
				)
			}
			return

		case <-ticker.C:
			checkCount++
			if s.checkVMReady(ctx, vm) {
				// VM is ready! Mark as pooling
				if err := s.db.Queries().ReturnVMToPool(context.Background(), vm.ID); err != nil {
					s.logger.Error("failed to mark VM as pooling",
						"vm_id", vmID,
						"error", err,
					)
					return
				}

				s.logger.Info("VM is ready and marked as pooling",
					"vm_id", vmID,
					"vm_no", vm.No,
					"checks_performed", checkCount,
				)
				return
			}
			// Not ready yet, will try again on next tick
		}
	}
}

// checkVMReady checks if VM is SSH-accessible and has Docker installed
func (s *Service) checkVMReady(ctx context.Context, vm *database.Vm) bool {
	// Try to establish SSH connection
	sshClient, err := s.getSSHClient(ctx, vm)
	if err != nil {
		s.logger.Debug("VM not yet SSH accessible",
			"vm_id", uuid.ToString(vm.ID),
			"vm_no", vm.No,
			"error", err,
		)
		return false
	}
	defer sshClient.Close()

	// Check if Docker is installed and running
	session, err := sshClient.NewSession()
	if err != nil {
		s.logger.Debug("failed to create SSH session",
			"vm_id", uuid.ToString(vm.ID),
			"error", err,
		)
		return false
	}
	defer session.Close()

	// Run simple docker command to verify it's working
	if err := session.Run("docker info > /dev/null 2>&1"); err != nil {
		s.logger.Debug("Docker not yet ready on VM",
			"vm_id", uuid.ToString(vm.ID),
			"vm_no", vm.No,
			"error", err,
		)
		return false
	}

	s.logger.Info("VM readiness check passed",
		"vm_id", uuid.ToString(vm.ID),
		"vm_no", vm.No,
	)
	return true
}

// getSSHClient creates an SSH client connection to a VM
func (s *Service) getSSHClient(ctx context.Context, vm *database.Vm) (*ssh.Client, error) {
	// Check if VM has public IP set
	if !vm.PublicIp.Valid || vm.PublicIp.String == "" {
		return nil, fmt.Errorf("VM has no public IP address set (vm_id=%s)", uuid.ToString(vm.ID))
	}

	publicIP := vm.PublicIp.String

	// Parse SSH key
	signer, err := ssh.ParsePrivateKey(s.sshPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// Connect via IP
	sshAddr := fmt.Sprintf("%s:22", publicIP)
	return ssh.Dial("tcp", sshAddr, sshConfig)
}

// executeSSHCommand executes a single command via SSH
func (s *Service) executeSSHCommand(sshClient *ssh.Client, command string) error {
	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return fmt.Errorf("command failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// initializeSSHKeys ensures SSH keys are set up in Hetzner
func (s *Service) initializeSSHKeys(ctx context.Context) error {
	// Decode private key from base64
	privKeyBytes, err := base64.StdEncoding.DecodeString(s.cfg.SSHPrivateKey)
	if err != nil {
		return fmt.Errorf("failed to decode private key: %w", err)
	}
	s.sshPrivateKey = privKeyBytes
	s.sshPublicKey = s.cfg.SSHPublicKey

	s.logger.Info("SSH keys loaded from environment variables")

	// Check if key exists in Hetzner by searching for matching public key
	allKeys, err := s.hcloudClient.SSHKey.All(ctx)
	if err != nil {
		return fmt.Errorf("failed to list SSH keys in Hetzner: %w", err)
	}

	var existingKey *hcloud.SSHKey
	for _, key := range allKeys {
		if key.PublicKey == s.cfg.SSHPublicKey {
			existingKey = key
			break
		}
	}

	// If key doesn't exist in Hetzner, create it with provided public key
	if existingKey == nil {
		keyName := s.cfg.HetznerSSHKeyName
		if keyName == "" {
			keyName = "zeitwork-reconciler-key"
		}

		s.logger.Info("SSH key not found in Hetzner, uploading", "key_name", keyName)

		_, _, err = s.hcloudClient.SSHKey.Create(ctx, hcloud.SSHKeyCreateOpts{
			Name:      keyName,
			PublicKey: s.cfg.SSHPublicKey,
		})
		if err != nil {
			return fmt.Errorf("failed to create SSH key in Hetzner: %w", err)
		}

		s.logger.Info("SSH key uploaded to Hetzner successfully", "key_name", keyName)
	} else {
		s.logger.Info("SSH key already exists in Hetzner", "key_name", existingKey.Name)
	}

	return nil
}
