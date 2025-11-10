package builder

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/zeitwork/zeitwork/internal/database"
	githubpkg "github.com/zeitwork/zeitwork/internal/shared/github"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
	"golang.org/x/crypto/ssh"
)

// Config holds the builder configuration
type Config struct {
	DatabaseURL   string        // Database connection string
	BuildInterval time.Duration // How often to check for builds
	BuildTimeout  time.Duration // Max build duration

	// GitHub App configuration
	GitHubAppID  string // GitHub App ID
	GitHubAppKey string // GitHub App private key (base64 encoded PEM format)

	// Docker registry
	RegistryURL      string
	RegistryUsername string
	RegistryPassword string

	// Hetzner
	HetznerToken string

	// SSH for VM access
	SSHPublicKey  string
	SSHPrivateKey string // Base64 encoded
}

// Service is the builder service
type Service struct {
	cfg           Config
	db            *database.DB
	logger        *slog.Logger
	hcloudClient  *hcloud.Client
	sshPublicKey  string
	sshPrivateKey []byte
	cancel        context.CancelFunc
}

// NewService creates a new builder service
func NewService(cfg Config, logger *slog.Logger) (*Service, error) {
	// Set defaults
	if cfg.BuildInterval == 0 {
		cfg.BuildInterval = 10 * time.Second
	}
	if cfg.BuildTimeout == 0 {
		cfg.BuildTimeout = 30 * time.Minute
	}

	// Initialize database connection
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize Hetzner client (optional for local testing)
	var hcloudClient *hcloud.Client
	if cfg.HetznerToken != "" {
		hcloudClient = hcloud.NewClient(hcloud.WithToken(cfg.HetznerToken))
		logger.Info("Hetzner client initialized")
	}

	// Decode SSH private key
	privKeyBytes, err := base64.StdEncoding.DecodeString(cfg.SSHPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode SSH private key: %w", err)
	}

	return &Service{
		cfg:           cfg,
		db:            db,
		logger:        logger,
		hcloudClient:  hcloudClient,
		sshPublicKey:  cfg.SSHPublicKey,
		sshPrivateKey: privKeyBytes,
	}, nil
}

// Start starts the builder service
func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.logger.Info("starting builder service",
		"build_interval", s.cfg.BuildInterval,
		"build_timeout", s.cfg.BuildTimeout,
	)

	ticker := time.NewTicker(s.cfg.BuildInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("builder stopped")
			return nil
		case <-ticker.C:
			s.processPendingBuilds(ctx)
		}
	}
}

// Stop gracefully stops the builder service
func (s *Service) Stop() error {
	s.logger.Info("stopping builder")

	if s.cancel != nil {
		s.cancel()
	}

	if s.db != nil {
		s.db.Close()
	}

	return nil
}

// processPendingBuilds processes the next pending build
func (s *Service) processPendingBuilds(ctx context.Context) {
	// Get next pending build with row lock
	build, err := s.db.Queries().GetPendingBuild(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			s.logger.Debug("no pending builds")
			return
		}
		s.logger.Error("failed to get pending build", "error", err)
		return
	}

	buildID := uuid.ToString(build.ID)
	s.logger.Info("acquired pending build",
		"build_id", buildID,
		"project_id", uuid.ToString(build.ProjectID),
	)

	// Mark as building
	if err := s.db.Queries().MarkBuildBuilding(ctx, build.ID); err != nil {
		s.logger.Error("failed to mark build as building",
			"build_id", buildID,
			"error", err,
		)
		return
	}

	s.logger.Info("build marked as building", "build_id", buildID)

	// Execute build
	if err := s.executeBuild(ctx, build); err != nil {
		s.logger.Error("build failed",
			"build_id", buildID,
			"error", err,
		)
		// Mark as error
		s.db.Queries().MarkBuildError(ctx, build.ID)
		return
	}

	s.logger.Info("build completed successfully", "build_id", buildID)
}

// executeBuild executes a build on a VM
func (s *Service) executeBuild(ctx context.Context, build *database.Build) error {
	buildID := uuid.ToString(build.ID)

	// Create build context with timeout
	buildCtx, cancel := context.WithTimeout(ctx, s.cfg.BuildTimeout)
	defer cancel()

	// Get or create VM for build
	vm, err := s.assignBuildVM(buildCtx, build)
	if err != nil {
		return fmt.Errorf("failed to assign build VM: %w", err)
	}

	s.logger.Info("VM assigned for build",
		"build_id", buildID,
		"vm_id", uuid.ToString(vm.ID),
		"vm_no", vm.No,
	)

	// Delete VM and Hetzner server after build (defer)
	defer func() {
		s.logger.Info("cleaning up build VM", "vm_id", uuid.ToString(vm.ID), "vm_no", vm.No)

		// Delete Hetzner server if client is available and server_no is set
		if s.hcloudClient != nil && vm.ServerNo.Valid {
			server, _, err := s.hcloudClient.Server.GetByID(buildCtx, int64(vm.ServerNo.Int32))
			if err != nil {
				s.logger.Error("failed to get Hetzner server for deletion",
					"vm_id", uuid.ToString(vm.ID),
					"server_no", vm.ServerNo.Int32,
					"error", err,
				)
			} else if server != nil {
				_, _, err := s.hcloudClient.Server.DeleteWithResult(buildCtx, server)
				if err != nil {
					s.logger.Error("failed to delete Hetzner server",
						"vm_id", uuid.ToString(vm.ID),
						"server_no", vm.ServerNo.Int32,
						"error", err,
					)
				} else {
					s.logger.Info("Hetzner server deleted",
						"vm_id", uuid.ToString(vm.ID),
						"server_no", vm.ServerNo.Int32,
					)
				}
			}
		}

		// Mark VM as deleted in database
		if err := s.db.Queries().MarkVMDeleted(buildCtx, vm.ID); err != nil {
			s.logger.Error("failed to mark VM as deleted",
				"vm_id", uuid.ToString(vm.ID),
				"error", err,
			)
		} else {
			s.logger.Info("VM marked as deleted", "vm_id", uuid.ToString(vm.ID))
		}
	}()

	// Get project to retrieve GitHub installation ID
	project, err := s.db.Queries().GetProjectByID(buildCtx, build.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// Get GitHub installation for auth
	installation, err := s.db.Queries().GetGithubInstallationByID(buildCtx, project.GithubInstallationID)
	if err != nil {
		return fmt.Errorf("failed to get GitHub installation: %w", err)
	}

	// Generate GitHub installation token
	tokenService, err := githubpkg.NewTokenService(s.cfg.GitHubAppID, s.cfg.GitHubAppKey)
	if err != nil {
		return fmt.Errorf("failed to create GitHub token service: %w", err)
	}

	githubToken, err := tokenService.GetInstallationToken(buildCtx, s.db, uuid.ToString(installation.ID))
	if err != nil {
		return fmt.Errorf("failed to get GitHub installation token: %w", err)
	}

	s.logger.Info("GitHub installation token generated", "build_id", buildID)

	// Get SSH client to VM
	sshClient, err := s.getSSHClient(buildCtx, vm)
	if err != nil {
		return fmt.Errorf("failed to get SSH client: %w", err)
	}
	defer sshClient.Close()

	s.logger.Info("SSH connection established to build VM",
		"build_id", buildID,
		"vm_no", vm.No,
	)

	// Get repository and commit from build and project
	repository := project.GithubRepository
	commit := build.GithubCommit

	// Clone repository on VM
	repoURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", githubToken, repository)
	cloneCmd := fmt.Sprintf("cd /tmp && git clone --depth 1 %s repo && cd repo && git checkout %s",
		repoURL,
		commit,
	)

	s.logger.Info("cloning repository on VM",
		"build_id", buildID,
		"repository", repository,
		"commit", commit,
		"branch", build.GithubBranch,
	)

	if err := s.executeSSHCommand(sshClient, cloneCmd); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	s.logger.Info("repository cloned successfully", "build_id", buildID)

	// Generate image name from repository (e.g., "dokedu/frontend" -> "ghcr.io/zeitwork/build-dokedu-frontend")
	// Replace slashes and convert to lowercase for valid Docker image name
	repoName := strings.ToLower(strings.ReplaceAll(repository, "/", "-"))
	imageName := fmt.Sprintf("%s/zeitwork/build-%s:latest",
		s.cfg.RegistryURL,
		repoName,
	)

	// Build Docker image on VM
	buildCmd := fmt.Sprintf("cd /tmp/repo && docker build -t %s .", imageName)

	s.logger.Info("building Docker image on VM",
		"build_id", buildID,
		"image_name", imageName,
	)

	if err := s.executeSSHCommand(sshClient, buildCmd); err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	s.logger.Info("Docker image built successfully", "build_id", buildID)

	// Login to registry
	loginCmd := fmt.Sprintf("echo '%s' | docker login %s -u %s --password-stdin",
		s.cfg.RegistryPassword,
		s.cfg.RegistryURL,
		s.cfg.RegistryUsername,
	)

	if err := s.executeSSHCommand(sshClient, loginCmd); err != nil {
		return fmt.Errorf("failed to login to registry: %w", err)
	}

	// Push image to registry
	pushCmd := fmt.Sprintf("docker push %s", imageName)

	s.logger.Info("pushing image to registry",
		"build_id", buildID,
		"image_name", imageName,
	)

	if err := s.executeSSHCommand(sshClient, pushCmd); err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	s.logger.Info("image pushed to registry successfully", "build_id", buildID)

	// Get image digest
	digestCmd := fmt.Sprintf("docker inspect --format='{{index .RepoDigests 0}}' %s", imageName)
	output, err := s.executeSSHCommandWithOutput(sshClient, digestCmd)
	if err != nil {
		return fmt.Errorf("failed to get image digest: %w", err)
	}

	digest := strings.TrimSpace(output)

	// Extract registry, repository, and tag from image name
	// Format: ghcr.io/zeitwork/build-dokedu-nuxt-demo:latest
	parts := strings.SplitN(imageName, "/", 2) // Split into registry and rest
	if len(parts) != 2 {
		return fmt.Errorf("invalid image name format: %s", imageName)
	}
	imageRegistry := parts[0]

	// Split repository and tag
	repoAndTag := parts[1]
	tagParts := strings.Split(repoAndTag, ":")
	if len(tagParts) != 2 {
		return fmt.Errorf("invalid image name format (no tag): %s", imageName)
	}
	imageRepository := tagParts[0]
	imageTag := tagParts[1]

	// Create image record
	imageID := uuid.New()
	if err := s.db.Queries().CreateImage(ctx, &database.CreateImageParams{
		ID:         imageID,
		Registry:   imageRegistry,
		Repository: imageRepository,
		Tag:        imageTag,
		Digest:     digest,
	}); err != nil {
		return fmt.Errorf("failed to create image record: %w", err)
	}

	s.logger.Info("image record created",
		"build_id", buildID,
		"image_id", uuid.ToString(imageID),
		"registry", imageRegistry,
		"repository", imageRepository,
		"tag", imageTag,
	)

	s.logger.Info("build completed, marking as ready",
		"build_id", buildID,
		"image_id", uuid.ToString(imageID),
		"image_name", imageName,
	)

	// Mark build as ready
	if err := s.db.Queries().MarkBuildReady(ctx, &database.MarkBuildReadyParams{
		ID:      build.ID,
		ImageID: imageID,
	}); err != nil {
		return fmt.Errorf("failed to mark build as ready: %w", err)
	}

	s.logger.Info("build marked as ready",
		"build_id", buildID,
		"image_id", uuid.ToString(imageID),
	)

	return nil
}

// assignBuildVM gets a VM from pool or creates a new one for building
func (s *Service) assignBuildVM(ctx context.Context, build *database.Build) (*database.Vm, error) {
	// Get available pool VM
	poolVMs, err := s.db.Queries().GetPoolVMs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool VMs: %w", err)
	}

	if len(poolVMs) == 0 {
		return nil, fmt.Errorf("no pool VMs available for build")
	}

	// Take first available VM
	vm := poolVMs[0]

	// Update VM status to "building" and assign to build
	// TODO: Add query to update VM status to building
	if err := s.db.Queries().UpdateBuildVM(ctx, &database.UpdateBuildVMParams{
		ID:   build.ID,
		VmID: vm.ID,
	}); err != nil {
		return nil, fmt.Errorf("failed to assign VM to build: %w", err)
	}

	s.logger.Info("VM assigned from pool",
		"vm_id", uuid.ToString(vm.ID),
		"vm_no", vm.No,
	)

	return vm, nil
}

// getSSHClient creates an SSH client connection to a VM
func (s *Service) getSSHClient(ctx context.Context, vm *database.Vm) (*ssh.Client, error) {
	// Check if VM has public IP set
	if !vm.PublicIp.Valid || vm.PublicIp.String == "" {
		return nil, fmt.Errorf("VM has no public IP address set")
	}

	ipv6 := vm.PublicIp.String

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

	// Connect via IPv6
	sshAddr := fmt.Sprintf("[%s]:22", ipv6)
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

// executeSSHCommandWithOutput executes a command and returns output
func (s *Service) executeSSHCommandWithOutput(sshClient *ssh.Client, command string) (string, error) {
	session, err := sshClient.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return "", fmt.Errorf("command failed: %w (output: %s)", err, string(output))
	}

	return string(output), nil
}
