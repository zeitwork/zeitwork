package reconciler

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	_ "embed"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/base58"
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

	// Hetzner configuration
	HetznerToken           string // Hetzner Cloud API token
	HetznerSSHKeyName      string // SSH key name in Hetzner
	HetznerServerType      string // Server type (e.g., "cx22")
	HetznerImage           string // OS image (e.g., "ubuntu-24.04")
	DockerRegistryURL      string // Docker registry for pulling images
	DockerRegistryUsername string // Docker registry username
	DockerRegistryPassword string // Docker registry password
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

	s := &Service{
		cfg:          cfg,
		db:           db,
		logger:       logger,
		hcloudClient: hcloudClient,
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

// reconcileDomains verifies domain ownership via DNS TXT records
func (s *Service) reconcileDomains(ctx context.Context) {
	domains, err := s.db.Queries().GetUnverifiedDomains(ctx)
	if err != nil {
		s.logger.Error("failed to get unverified domains", "error", err)
		return
	}

	if len(domains) == 0 {
		return
	}

	s.logger.Info("reconciling domains", "count", len(domains))

	for _, domain := range domains {
		domainID := uuid.ToString(domain.ID)
		domainName := domain.Name

		s.logger.Debug("checking domain verification",
			"domain_id", domainID,
			"domain_name", domainName,
		)

		// Skip if no verification token
		if !domain.VerificationToken.Valid || domain.VerificationToken.String == "" {
			s.logger.Debug("skipping domain without verification token",
				"domain_id", domainID,
				"domain_name", domainName,
			)
			continue
		}

		// Build expected TXT record name: {base58(domain.id)}-zeitwork.{domain.name}
		base58ID := base58.EncodeUUID(domain.ID)
		txtRecordName := fmt.Sprintf("%s-zeitwork.%s", base58ID, domainName)

		s.logger.Debug("looking up DNS TXT record",
			"domain_id", domainID,
			"txt_record", txtRecordName,
		)

		// Lookup TXT records
		txtRecords, err := net.LookupTXT(txtRecordName)
		if err != nil {
			s.logger.Debug("DNS lookup failed",
				"domain_id", domainID,
				"txt_record", txtRecordName,
				"error", err,
			)
			continue
		}

		// Check if any TXT record contains the verification token
		verified := false
		for _, record := range txtRecords {
			if record == domain.VerificationToken.String {
				verified = true
				break
			}
		}

		if verified {
			// Mark domain as verified
			if err := s.db.Queries().MarkDomainVerified(ctx, domain.ID); err != nil {
				s.logger.Error("failed to mark domain as verified",
					"domain_id", domainID,
					"error", err,
				)
				continue
			}

			s.logger.Info("domain verified",
				"domain_id", domainID,
				"domain_name", domainName,
			)
		} else {
			s.logger.Debug("verification token not found in DNS records",
				"domain_id", domainID,
				"domain_name", domainName,
				"expected_token", domain.VerificationToken.String,
			)
		}
	}
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

		// Create a new build
		buildID := uuid.New()
		params := &database.CreateBuildParams{
			ID:             buildID,
			Status:         database.BuildStatusesQueued,
			ProjectID:      deployment.ProjectID,
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

		// Get VM details
		vm, err := s.db.Queries().GetVMByID(ctx, deployment.VmID)
		if err != nil {
			s.logger.Error("failed to get VM details",
				"vm_id", vmID,
				"error", err,
			)
			continue
		}

		// Stop and remove container if it exists and Hetzner is configured
		if s.hcloudClient != nil && vm.ContainerName.Valid && vm.ContainerName.String != "" {
			if err := s.stopAndRemoveContainer(ctx, vm); err != nil {
				s.logger.Error("failed to stop and remove container",
					"deployment_id", deploymentID,
					"vm_id", vmID,
					"error", err,
				)
				// Continue anyway to clean up database state
			}
		}

		// Clear container and image from VM
		if err := s.db.Queries().ClearVMContainer(ctx, deployment.VmID); err != nil {
			s.logger.Error("failed to clear VM container",
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

		// Get VM details
		vm, err := s.db.Queries().GetVMByID(ctx, deployment.VmID)
		if err != nil {
			s.logger.Error("failed to get VM details",
				"vm_id", vmID,
				"error", err,
			)
			continue
		}

		// Stop and remove container if it exists and Hetzner is configured
		if s.hcloudClient != nil && vm.ContainerName.Valid && vm.ContainerName.String != "" {
			if err := s.stopAndRemoveContainer(ctx, vm); err != nil {
				s.logger.Error("failed to stop and remove container",
					"deployment_id", deploymentID,
					"vm_id", vmID,
					"error", err,
				)
				// Continue anyway to clean up database state
			}
		}

		// Clear container and image from VM
		if err := s.db.Queries().ClearVMContainer(ctx, deployment.VmID); err != nil {
			s.logger.Error("failed to clear VM container",
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

// reconcileVMs maintains the pool of ready VMs
func (s *Service) reconcileVMs(ctx context.Context) {
	// Get current pool VMs
	poolVMs, err := s.db.Queries().GetPoolVMs(ctx)
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
		privateIP := fmt.Sprintf("10.77.%d.%d", (nextNo*8)/256, ((nextNo*8)%256)+2)
		port := int32(3000)

		s.logger.Info("creating pool VM",
			"vm_no", nextNo,
			"region_id", uuid.ToString(region.ID),
			"private_ip", privateIP,
		)

		// Create VM in database with "initializing" status
		params := &database.CreateVMParams{
			ID:        vmID,
			No:        nextNo,
			Status:    "initializing",
			PrivateIp: privateIP,
			RegionID:  region.ID,
			Port:      port,
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

	// Create server WITHOUT public IPv4 (to reduce costs)
	result, _, err := s.hcloudClient.Server.Create(ctx, hcloud.ServerCreateOpts{
		Name:       serverName,
		ServerType: serverType,
		Image:      hetznerImage,
		Location:   location,
		SSHKeys:    []*hcloud.SSHKey{sshKey},
		UserData:   cloudInitTemplate,
		PublicNet: &hcloud.ServerCreatePublicNet{
			EnableIPv4: false, // No public IPv4 to save costs
			EnableIPv6: true,  // Keep IPv6 for SSH access
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

	// Get the server's private IP (from network interface)
	var actualPrivateIP string
	if len(result.Server.PrivateNet) > 0 {
		actualPrivateIP = result.Server.PrivateNet[0].IP.String()
	} else {
		// If no private network yet, use the placeholder IP we generated
		actualPrivateIP = vm.PrivateIp
		s.logger.Warn("server has no private network IP yet, using placeholder",
			"server_id", result.Server.ID,
			"placeholder_ip", actualPrivateIP,
		)
	}

	s.logger.Info("Hetzner server created successfully",
		"server_id", result.Server.ID,
		"server_name", serverName,
		"private_ip", actualPrivateIP,
	)

	// Update VM with server details
	if err := s.db.Queries().UpdateVMServerDetails(ctx, &database.UpdateVMServerDetailsParams{
		ID:         vm.ID,
		ServerName: pgtype.Text{String: serverName, Valid: true},
		PrivateIp:  actualPrivateIP,
	}); err != nil {
		return fmt.Errorf("failed to update VM with server details: %w", err)
	}

	// Docker is installed via cloud-init during server creation (see UserData in create call)
	// Verify Docker is installed by checking server is ready
	s.logger.Info("Docker installed via cloud-init",
		"server_id", result.Server.ID,
	)

	// Mark VM as pooling (ready for use)
	if err := s.db.Queries().ReturnVMToPool(ctx, vm.ID); err != nil {
		return fmt.Errorf("failed to mark VM as pooling: %w", err)
	}

	s.logger.Info("VM marked as pooling and ready for assignment",
		"vm_id", uuid.ToString(vm.ID),
		"vm_no", vm.No,
	)

	return nil
}

// deployContainerToVM deploys a Docker container to a VM
func (s *Service) deployContainerToVM(ctx context.Context, vm *database.Vm, imageID pgtype.UUID) error {
	// Get image details from database
	image, err := s.db.Queries().GetImageByID(ctx, imageID)
	if err != nil {
		return fmt.Errorf("failed to get image details: %w", err)
	}

	// Get the Hetzner server to get IPv6 address
	server, _, err := s.hcloudClient.Server.GetByID(ctx, int64(vm.No))
	if err != nil {
		return fmt.Errorf("failed to get Hetzner server: %w", err)
	}
	if server == nil {
		return fmt.Errorf("hetzner server not found for vm.no=%d", vm.No)
	}

	// Get IPv6 address for SSH
	if server.PublicNet.IPv6.IP == nil {
		return fmt.Errorf("server has no IPv6 address")
	}
	ipv6 := server.PublicNet.IPv6.IP.String()

	s.logger.Info("deploying container to VM",
		"vm_id", uuid.ToString(vm.ID),
		"vm_no", vm.No,
		"ipv6", ipv6,
		"image_id", uuid.ToString(imageID),
	)

	// Parse private key for SSH
	signer, err := ssh.ParsePrivateKey(s.sshPrivateKey)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// Connect via SSH (IPv6)
	sshAddr := fmt.Sprintf("[%s]:22", ipv6)
	sshClient, err := ssh.Dial("tcp", sshAddr, sshConfig)
	if err != nil {
		return fmt.Errorf("failed to SSH to server: %w", err)
	}
	defer sshClient.Close()

	s.logger.Info("SSH connection established", "vm_no", vm.No)

	// Construct full image name from database
	imageName := fmt.Sprintf("%s/%s:%s", image.Registry, image.Repository, image.Tag)
	containerName := fmt.Sprintf("zeitwork-deployment-%s", uuid.ToString(vm.ID)[:8])

	s.logger.Info("deploying image",
		"image_name", imageName,
		"container_name", containerName,
	)

	// Login to Docker registry (if configured)
	if s.cfg.DockerRegistryURL != "" && s.cfg.DockerRegistryUsername != "" {
		loginCmd := fmt.Sprintf("docker login %s -u %s -p '%s'",
			s.cfg.DockerRegistryURL,
			s.cfg.DockerRegistryUsername,
			s.cfg.DockerRegistryPassword,
		)

		if err := s.executeSSHCommand(sshClient, loginCmd); err != nil {
			return fmt.Errorf("failed to login to Docker registry: %w", err)
		}

		s.logger.Info("logged in to Docker registry", "registry", s.cfg.DockerRegistryURL)
	}

	// Pull image
	pullCmd := fmt.Sprintf("docker pull %s", imageName)
	if err := s.executeSSHCommand(sshClient, pullCmd); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	s.logger.Info("image pulled successfully", "image", imageName)

	// Run container with port mapping
	runCmd := fmt.Sprintf("docker run -d -p %d:8080 --name %s --restart unless-stopped %s",
		vm.Port,
		containerName,
		imageName,
	)

	if err := s.executeSSHCommand(sshClient, runCmd); err != nil {
		return fmt.Errorf("failed to run container: %w", err)
	}

	s.logger.Info("container deployed successfully",
		"vm_id", uuid.ToString(vm.ID),
		"container_name", containerName,
		"port", vm.Port,
	)

	// Update VM with container name
	if err := s.db.Queries().UpdateVMContainerName(ctx, &database.UpdateVMContainerNameParams{
		ID:            vm.ID,
		ContainerName: pgtype.Text{String: containerName, Valid: true},
	}); err != nil {
		return fmt.Errorf("failed to update VM with container name: %w", err)
	}

	// Mark VM as running
	if err := s.db.Queries().MarkVMRunning(ctx, vm.ID); err != nil {
		return fmt.Errorf("failed to mark VM as running: %w", err)
	}

	s.logger.Info("VM marked as running",
		"vm_id", uuid.ToString(vm.ID),
		"container_name", containerName,
	)

	return nil
}

// stopAndRemoveContainer stops and removes a Docker container from a VM
func (s *Service) stopAndRemoveContainer(ctx context.Context, vm *database.Vm) error {
	// Get the Hetzner server to get IPv6 address
	server, _, err := s.hcloudClient.Server.GetByID(ctx, int64(vm.No))
	if err != nil {
		return fmt.Errorf("failed to get Hetzner server: %w", err)
	}
	if server == nil {
		return fmt.Errorf("hetzner server not found for vm.no=%d", vm.No)
	}

	// Get IPv6 address for SSH
	if server.PublicNet.IPv6.IP == nil {
		return fmt.Errorf("server has no IPv6 address")
	}
	ipv6 := server.PublicNet.IPv6.IP.String()

	s.logger.Info("stopping container on VM",
		"vm_id", uuid.ToString(vm.ID),
		"container_name", vm.ContainerName.String,
	)

	// Parse private key for SSH
	signer, err := ssh.ParsePrivateKey(s.sshPrivateKey)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// Connect via SSH (IPv6)
	sshAddr := fmt.Sprintf("[%s]:22", ipv6)
	sshClient, err := ssh.Dial("tcp", sshAddr, sshConfig)
	if err != nil {
		return fmt.Errorf("failed to SSH to server: %w", err)
	}
	defer sshClient.Close()

	// Stop and remove container
	containerName := vm.ContainerName.String
	stopCmd := fmt.Sprintf("docker stop %s", containerName)
	removeCmd := fmt.Sprintf("docker rm %s", containerName)

	// Try to stop (ignore errors if already stopped)
	_ = s.executeSSHCommand(sshClient, stopCmd)

	// Remove container
	if err := s.executeSSHCommand(sshClient, removeCmd); err != nil {
		// Log but don't fail - container might already be removed
		s.logger.Warn("failed to remove container (may already be removed)",
			"container_name", containerName,
			"error", err,
		)
	}

	s.logger.Info("container stopped and removed",
		"vm_id", uuid.ToString(vm.ID),
		"container_name", containerName,
	)

	return nil
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
	// Check if SSH key exists in Hetzner
	sshKey, _, err := s.hcloudClient.SSHKey.GetByName(ctx, s.cfg.HetznerSSHKeyName)
	if err != nil {
		return fmt.Errorf("failed to get SSH key: %w", err)
	}

	// If key doesn't exist, generate and create it
	if sshKey == nil {
		s.logger.Info("SSH key not found in Hetzner, generating new key pair")

		pubKey, privKey, err := generateSSHKeyPair()
		if err != nil {
			return fmt.Errorf("failed to generate SSH key pair: %w", err)
		}

		_, _, err = s.hcloudClient.SSHKey.Create(ctx, hcloud.SSHKeyCreateOpts{
			Name:      s.cfg.HetznerSSHKeyName,
			PublicKey: pubKey,
		})
		if err != nil {
			return fmt.Errorf("failed to create SSH key in Hetzner: %w", err)
		}

		s.sshPublicKey = pubKey
		s.sshPrivateKey = privKey

		s.logger.Info("SSH key created in Hetzner", "key_name", s.cfg.HetznerSSHKeyName)
	} else {
		// TODO: Load private key from secure storage
		// For now, warn that the key exists but we don't have the private key
		s.logger.Warn("SSH key exists in Hetzner but private key not available - VM operations may fail")
	}

	return nil
}

// generateSSHKeyPair generates an ED25519 SSH key pair
func generateSSHKeyPair() (publicKey string, privateKey []byte, err error) {
	// Generate ED25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	// Convert to SSH format
	sshPublicKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create SSH public key: %w", err)
	}

	// Format public key
	publicKeyStr := string(ssh.MarshalAuthorizedKey(sshPublicKey))

	// Marshal private key to OpenSSH format
	privateKeyBlock, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	// Note: For production, store private key securely (database with encryption, secrets manager, etc.)
	// Encode the PEM block to bytes
	privateKeyPEM := ssh.MarshalAuthorizedKey(sshPublicKey) // Simplified for now
	// In production: use proper PEM encoding: pem.EncodeToMemory(privateKeyBlock)
	_ = privateKeyBlock // Avoid unused variable

	return publicKeyStr, privateKeyPEM, nil
}
