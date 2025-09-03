package builder

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	natsClient "github.com/zeitwork/zeitwork/internal/shared/nats"
	pb "github.com/zeitwork/zeitwork/proto"
)

// Service represents the image builder service
type Service struct {
	logger       *slog.Logger
	config       *config.BuilderConfig
	db           *pgxpool.Pool
	queries      *database.Queries
	natsClient   *natsClient.Client
	imageBuilder ImageBuilder

	// Build processing state
	buildsMu         sync.RWMutex
	activeBuildCount int
	stopChan         chan struct{}
}

// NewService creates a new image builder service
func NewService(cfg *config.BuilderConfig, logger *slog.Logger) (*Service, error) {
	// Initialize database connection pool
	dbConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	db, err := pgxpool.NewWithConfig(context.Background(), dbConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	queries := database.New(db)

	// Initialize NATS client
	natsClient, err := natsClient.NewSimpleClient()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Initialize image builder based on configuration
	var imageBuilder ImageBuilder
	switch cfg.BuilderType {
	case "docker":
		dockerConfig := DockerBuilderConfig{
			WorkDir:  cfg.BuildWorkDir,
			Registry: cfg.ContainerRegistry,
		}
		imageBuilder, err = NewDockerBuilder(dockerConfig, logger.With("component", "docker-builder"))
		if err != nil {
			return nil, fmt.Errorf("failed to create docker builder: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported builder type: %s", cfg.BuilderType)
	}

	s := &Service{
		logger:           logger,
		config:           cfg,
		db:               db,
		queries:          queries,
		natsClient:       natsClient,
		imageBuilder:     imageBuilder,
		activeBuildCount: 0,
		stopChan:         make(chan struct{}),
	}

	return s, nil
}

// Start starts the image builder service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting image builder service")

	// Check for any existing pending builds on startup
	go s.initialBuildCheck(ctx)

	// Subscribe to NATS build notifications
	go s.subscribeToBuilds(ctx)

	// Start the main build processing loop
	go s.buildProcessingLoop(ctx)

	// Wait for context cancellation
	<-ctx.Done()

	s.logger.Info("Shutting down image builder service")
	close(s.stopChan)
	return nil
}

// initialBuildCheck checks for any pending deployments without builds on service startup
func (s *Service) initialBuildCheck(ctx context.Context) {
	s.logger.Info("Checking for pending deployments without builds on startup")

	// Get all pending deployments that don't have builds yet
	pendingDeployments, err := s.queries.DeploymentsGetPendingWithoutBuilds(ctx)
	if err != nil {
		s.logger.Error("Failed to get pending deployments", "error", err)
		return
	}

	if len(pendingDeployments) == 0 {
		s.logger.Info("No pending deployments found on startup")
		return
	}

	s.logger.Info("Found pending deployments without builds", "count", len(pendingDeployments))

	// Create builds for each pending deployment
	for _, deployment := range pendingDeployments {
		if err := s.createBuildForPendingDeployment(ctx, deployment); err != nil {
			s.logger.Error("Failed to create build for pending deployment",
				"deployment_id", deployment.ID,
				"error", err)
			// Continue with other deployments instead of failing completely
			continue
		}

		s.logger.Info("Created build for pending deployment",
			"deployment_id", deployment.ID,
			"project_id", deployment.ProjectID)
	}

	s.logger.Info("Initial build check completed", "builds_created", len(pendingDeployments))
}

// subscribeToBuilds subscribes to NATS notifications for image build events
func (s *Service) subscribeToBuilds(ctx context.Context) {
	s.logger.Info("Subscribing to image build notifications")

	// Subscribe to deployment creation events to create image builds
	_, err := s.natsClient.Subscribe("deployment.created", func(msg *nats.Msg) {
		s.logger.Debug("Received deployment created notification", "data", string(msg.Data))

		// Handle deployment creation (create image build if needed)
		if err := s.handleDeploymentCreated(ctx, msg.Data); err != nil {
			s.logger.Error("Failed to handle deployment created notification", "error", err)
		}
	})

	if err != nil {
		s.logger.Error("Failed to subscribe to deployment created notifications", "error", err)
		return
	}

	// Subscribe to image build creation events from the database
	_, err = s.natsClient.Subscribe("image_build.created", func(msg *nats.Msg) {
		s.logger.Debug("Received image build created notification", "data", string(msg.Data))

		// Handle the image build creation (start Docker build)
		if err := s.handleImageBuildCreated(ctx, msg.Data); err != nil {
			s.logger.Error("Failed to handle image build created notification", "error", err)
		}
	})

	if err != nil {
		s.logger.Error("Failed to subscribe to image build created notifications", "error", err)
		return
	}

	s.logger.Info("Successfully subscribed to image build notifications")

	// Keep the subscription alive
	<-ctx.Done()
}

// handleImageBuildCreated handles image build creation events from the listener
func (s *Service) handleImageBuildCreated(ctx context.Context, data []byte) error {
	// Parse the image build created message
	var imageBuildCreated pb.ImageBuildCreated
	if err := proto.Unmarshal(data, &imageBuildCreated); err != nil {
		return fmt.Errorf("failed to unmarshal image build created: %w", err)
	}

	s.logger.Info("Processing image build created event",
		"image_build_id", imageBuildCreated.Id)

	// Start the actual Docker build process
	s.logger.Info("Starting Docker build process",
		"image_build_id", imageBuildCreated.Id)

	// TODO: Implement the actual Docker build logic here
	// This would involve:
	// 1. Fetching the deployment and project details
	// 2. Cloning the repository
	// 3. Building the Docker image
	// 4. Pushing to registry
	// 5. Updating the image build status

	return nil
}

// handleDeploymentCreated creates an image build when a deployment is created
func (s *Service) handleDeploymentCreated(ctx context.Context, data []byte) error {
	// Parse deployment created message
	var deploymentCreated pb.DeploymentCreated
	if err := proto.Unmarshal(data, &deploymentCreated); err != nil {
		return fmt.Errorf("failed to unmarshal deployment created: %w", err)
	}

	s.logger.Info("Processing deployment created event", "deployment_id", deploymentCreated.Id)

	// Parse deployment UUID
	deploymentUUID, err := s.parseUUID(deploymentCreated.Id)
	if err != nil {
		return fmt.Errorf("failed to parse deployment ID: %w", err)
	}

	// Check if this deployment already has a build
	existingBuilds, err := s.queries.ImageBuildsGetByDeployment(ctx, deploymentUUID)
	if err != nil {
		return fmt.Errorf("failed to check existing builds: %w", err)
	}

	if len(existingBuilds) > 0 {
		s.logger.Debug("Skipping deployment - already has builds",
			"deployment_id", deploymentCreated.Id,
			"existing_builds", len(existingBuilds))
		return nil
	}

	// Fetch deployment details from database
	deployment, err := s.queries.DeploymentsGetById(ctx, deploymentUUID)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Only create builds for pending deployments
	if deployment.Status != "pending" {
		s.logger.Debug("Skipping deployment - not pending",
			"deployment_id", deploymentCreated.Id,
			"status", deployment.Status)
		return nil
	}

	// Generate a new UUID for the image build
	buildUUID := pgtype.UUID{}
	if err := buildUUID.Scan(s.generateUUID()); err != nil {
		return fmt.Errorf("failed to generate build UUID: %w", err)
	}

	// Create image build in database (this will trigger image_build.created event)
	createParams := &database.ImageBuildsCreateParams{
		ID:             buildUUID,
		DeploymentID:   deploymentUUID,
		OrganisationID: deployment.OrganisationID,
	}

	build, err := s.queries.ImageBuildsCreate(ctx, createParams)
	if err != nil {
		return fmt.Errorf("failed to create image build: %w", err)
	}

	s.logger.Info("Created image build for deployment",
		"build_id", build.ID,
		"deployment_id", build.DeploymentID,
		"status", build.Status)

	return nil
}

// parseUUID parses a string UUID into pgtype.UUID
func (s *Service) parseUUID(uuidStr string) (pgtype.UUID, error) {
	var uuid pgtype.UUID
	if err := uuid.Scan(uuidStr); err != nil {
		return uuid, fmt.Errorf("invalid UUID format: %w", err)
	}
	return uuid, nil
}

// createBuildForPendingDeployment creates an image build for a pending deployment from startup check
func (s *Service) createBuildForPendingDeployment(ctx context.Context, deployment *database.DeploymentsGetPendingWithoutBuildsRow) error {
	// Generate a new UUID for the image build
	buildUUID := pgtype.UUID{}
	if err := buildUUID.Scan(s.generateUUID()); err != nil {
		return fmt.Errorf("failed to generate build UUID: %w", err)
	}

	// Create image build in database
	createParams := &database.ImageBuildsCreateParams{
		ID:             buildUUID,
		DeploymentID:   deployment.ID,
		OrganisationID: deployment.OrganisationID,
	}

	build, err := s.queries.ImageBuildsCreate(ctx, createParams)
	if err != nil {
		return fmt.Errorf("failed to create image build: %w", err)
	}

	s.logger.Info("Created image build",
		"build_id", build.ID,
		"deployment_id", build.DeploymentID,
		"status", build.Status)

	return nil
}

// buildProcessingLoop is the main loop that processes builds
func (s *Service) buildProcessingLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.BuildPollInterval)
	defer ticker.Stop()

	s.logger.Info("Starting build processing loop", "poll_interval", s.config.BuildPollInterval)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Build processing loop stopped")
			return
		case <-s.stopChan:
			s.logger.Info("Build processing loop stopped via stop channel")
			return
		case <-ticker.C:
			s.processPendingBuilds(ctx)
		}
	}
}

// processPendingBuilds checks for and processes any pending builds
func (s *Service) processPendingBuilds(ctx context.Context) {
	// Check if we're at capacity
	s.buildsMu.RLock()
	atCapacity := s.activeBuildCount >= s.config.MaxConcurrentBuilds
	s.buildsMu.RUnlock()

	if atCapacity {
		s.logger.Debug("At build capacity, skipping build check",
			"active_builds", s.activeBuildCount,
			"max_concurrent", s.config.MaxConcurrentBuilds)
		return
	}

	// Try to dequeue a pending image build
	imageBuild, err := s.queries.ImageBuildsDequeuePending(ctx)
	if err != nil {
		// This is expected when no builds are pending
		s.logger.Debug("No pending image builds to process")
		return
	}

	if imageBuild == nil {
		return
	}

	// Enrich the image build with deployment and project information
	enrichedBuild, err := s.enrichImageBuild(ctx, imageBuild)
	if err != nil {
		s.logger.Error("Failed to enrich image build", "build_id", imageBuild.ID, "error", err)
		return
	}

	s.logger.Info("Starting build processing",
		"build_id", enrichedBuild.ID,
		"project_id", enrichedBuild.ProjectID,
		"github_repo", enrichedBuild.GithubRepository,
		"commit_hash", enrichedBuild.CommitHash)

	// Increment active build count
	s.buildsMu.Lock()
	s.activeBuildCount++
	s.buildsMu.Unlock()

	// Process the build in a separate goroutine
	go s.processBuild(ctx, enrichedBuild)
}

// enrichImageBuild enriches an ImageBuild with deployment and project information
func (s *Service) enrichImageBuild(ctx context.Context, imageBuild *database.ImageBuildsDequeuePendingRow) (*EnrichedBuild, error) {
	// Get deployment information
	deployment, err := s.queries.DeploymentsGetById(ctx, imageBuild.DeploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	// Get project information
	project, err := s.queries.ProjectsGetById(ctx, deployment.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	// Create enriched build
	enrichedBuild := &EnrichedBuild{
		// From ImageBuild
		ID:             imageBuild.ID,
		Status:         imageBuild.Status,
		DeploymentID:   imageBuild.DeploymentID,
		StartedAt:      imageBuild.StartedAt,
		CompletedAt:    imageBuild.CompletedAt,
		FailedAt:       imageBuild.FailedAt,
		OrganisationID: imageBuild.OrganisationID,
		CreatedAt:      imageBuild.CreatedAt,
		UpdatedAt:      imageBuild.UpdatedAt,

		// From Deployment
		CommitHash: deployment.CommitHash,
		ProjectID:  deployment.ProjectID,

		// From Project
		GithubRepository: project.GithubRepository,
		DefaultBranch:    project.DefaultBranch,
	}

	return enrichedBuild, nil
}

// processBuild processes a single build
func (s *Service) processBuild(ctx context.Context, build *EnrichedBuild) {
	// Ensure we decrement the active build count when done
	defer func() {
		s.buildsMu.Lock()
		s.activeBuildCount--
		s.buildsMu.Unlock()
	}()

	buildCtx, cancel := context.WithTimeout(ctx, s.config.BuildTimeout)
	defer cancel()

	s.logger.Info("Processing build", "build_id", build.ID)

	// Update deployment status to "building" when we start the build
	_, err := s.queries.DeploymentsUpdateStatus(buildCtx, &database.DeploymentsUpdateStatusParams{
		ID:     build.DeploymentID,
		Status: "building",
	})
	if err != nil {
		s.logger.Error("Failed to update deployment status to building",
			"build_id", build.ID,
			"deployment_id", build.DeploymentID,
			"error", err)
		// Continue with build even if status update fails
	} else {
		s.logger.Info("Updated deployment status to building",
			"build_id", build.ID,
			"deployment_id", build.DeploymentID)
	}

	// Execute the build using the configured image builder
	result := s.imageBuilder.Build(buildCtx, build)

	if result.Success {
		s.logger.Info("Build completed successfully",
			"build_id", build.ID,
			"image_tag", result.ImageTag,
			"duration", result.Duration)

		// Mark build as completed
		_, err := s.queries.ImageBuildsComplete(buildCtx, build.ID)
		if err != nil {
			s.logger.Error("Failed to mark build as completed", "build_id", build.ID, "error", err)
		}

		// Update deployment status to "deploying" when build succeeds
		_, err = s.queries.DeploymentsUpdateStatus(buildCtx, &database.DeploymentsUpdateStatusParams{
			ID:     build.DeploymentID,
			Status: "deploying",
		})
		if err != nil {
			s.logger.Error("Failed to update deployment status to deploying",
				"build_id", build.ID,
				"deployment_id", build.DeploymentID,
				"error", err)
		} else {
			s.logger.Info("Updated deployment status to deploying",
				"build_id", build.ID,
				"deployment_id", build.DeploymentID)
		}
	} else {
		s.logger.Error("Build failed",
			"build_id", build.ID,
			"error", result.Error,
			"duration", result.Duration)

		// Mark build as failed
		_, err := s.queries.ImageBuildsFail(buildCtx, build.ID)
		if err != nil {
			s.logger.Error("Failed to mark build as failed", "build_id", build.ID, "error", err)
		}

		// Update deployment status to "failed" when build fails
		_, err = s.queries.DeploymentsUpdateStatus(buildCtx, &database.DeploymentsUpdateStatusParams{
			ID:     build.DeploymentID,
			Status: "failed",
		})
		if err != nil {
			s.logger.Error("Failed to update deployment status to failed",
				"build_id", build.ID,
				"deployment_id", build.DeploymentID,
				"error", err)
		} else {
			s.logger.Info("Updated deployment status to failed",
				"build_id", build.ID,
				"deployment_id", build.DeploymentID)
		}
	}
}

// generateUUID generates a new UUIDv7 string
func (s *Service) generateUUID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// Close closes the builder service
func (s *Service) Close() error {
	if s.imageBuilder != nil {
		if err := s.imageBuilder.Cleanup(); err != nil {
			s.logger.Warn("Failed to cleanup image builder", "error", err)
		}
	}
	if s.natsClient != nil {
		s.natsClient.Close()
	}
	if s.db != nil {
		s.db.Close()
	}
	return nil
}
