package builder

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	natsClient "github.com/zeitwork/zeitwork/internal/shared/nats"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
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

	// Check for any existing pending image builds on startup
	go s.initialImageBuildCheck(ctx)

	// Subscribe to NATS build notifications
	go s.subscribeToBuilds(ctx)

	// Start cleanup routine for orphaned builds
	go s.cleanupOrphanedBuilds(ctx)

	// Wait for context cancellation
	<-ctx.Done()

	s.logger.Info("Shutting down image builder service")
	close(s.stopChan)
	return nil
}

// initialImageBuildCheck checks for any pending image builds on service startup
func (s *Service) initialImageBuildCheck(ctx context.Context) {
	s.logger.Info("Checking for pending image builds on startup")

	// Process any pending image builds that may have been created while the builder was down
	// We'll use a simple loop to check for pending builds and process them
	for {
		// Try to dequeue a pending image build
		imageBuild, err := s.queries.ImageBuildsDequeuePending(ctx)
		if err != nil {
			// This is expected when no builds are pending
			s.logger.Debug("No pending image builds to process on startup")
			break
		}

		if imageBuild == nil {
			break
		}

		// Process the build
		if err := s.processImageBuildFromRow(ctx, imageBuild); err != nil {
			s.logger.Error("Failed to process pending image build on startup",
				"build_id", imageBuild.ID,
				"error", err)
			// Continue with other builds instead of failing completely
			continue
		}

		s.logger.Info("Processed pending image build on startup",
			"build_id", imageBuild.ID)
	}

	s.logger.Info("Initial image build check completed")
}

// subscribeToBuilds subscribes to NATS notifications for image build events
func (s *Service) subscribeToBuilds(ctx context.Context) {
	s.logger.Info("Subscribing to image build notifications")

	// Use queue groups to ensure only one builder receives each message
	queueGroup := "builder-workers"

	// Subscribe to image_build.created events
	_, err := s.natsClient.QueueSubscribe("image_build.created", queueGroup, func(msg *nats.Msg) {
		s.logger.Debug("Received image build created notification",
			"data", string(msg.Data),
			"queue_group", queueGroup)

		// Handle the image build creation (start Docker build)
		if err := s.handleImageBuildCreated(ctx, msg.Data); err != nil {
			s.logger.Error("Failed to handle image build created notification", "error", err)
		}
	})

	if err != nil {
		s.logger.Error("Failed to subscribe to image build created notifications", "error", err)
		return
	}

	// Subscribe to image_build.updated events to handle reset builds
	_, err = s.natsClient.QueueSubscribe("image_build.updated", queueGroup, func(msg *nats.Msg) {
		s.logger.Debug("Received image build updated notification",
			"data", string(msg.Data),
			"queue_group", queueGroup)

		// Handle the image build update (check if it was reset to pending)
		if err := s.handleImageBuildUpdated(ctx, msg.Data); err != nil {
			s.logger.Error("Failed to handle image build updated notification", "error", err)
		}
	})

	if err != nil {
		s.logger.Error("Failed to subscribe to image build updated notifications", "error", err)
		return
	}

	s.logger.Info("Successfully subscribed to image build notifications", "queue_group", queueGroup)

	// Keep the subscription alive
	<-ctx.Done()
}

// cleanupOrphanedBuilds runs periodic cleanup of stale builds
func (s *Service) cleanupOrphanedBuilds(ctx context.Context) {
	// Run cleanup every 5 minutes (or configured interval)
	cleanupInterval := s.config.CleanupInterval
	if cleanupInterval == 0 {
		cleanupInterval = 5 * time.Minute // Default to 5 minutes
	}

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	// Also run immediately on startup
	s.performCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.performCleanup(ctx)
		}
	}
}

// performCleanup resets stale builds that have been building for too long
func (s *Service) performCleanup(ctx context.Context) {
	// Calculate timeout: BuildTimeout + 10 minute buffer
	timeout := s.config.BuildTimeout + (10 * time.Minute)
	timeoutMinutes := int(timeout.Minutes())

	s.logger.Debug("Performing cleanup of stale builds",
		"timeout", timeout,
		"timeout_minutes", timeoutMinutes)

	// Convert timeout to pgtype.Text for the query
	var timeoutParam pgtype.Text
	timeoutParam.Scan(strconv.Itoa(timeoutMinutes))

	// Reset builds that have been "building" for too long
	resetBuilds, err := s.queries.ImageBuildsResetStale(ctx, timeoutParam)
	if err != nil {
		s.logger.Error("Failed to reset stale builds", "error", err)
		return
	}

	if len(resetBuilds) > 0 {
		s.logger.Warn("Reset stale builds",
			"count", len(resetBuilds),
			"timeout_minutes", timeoutMinutes)

		// Log each reset build for debugging
		for _, build := range resetBuilds {
			s.logger.Warn("Reset stale build",
				"build_id", build.ID,
				"deployment_id", build.DeploymentID)
		}

		// Note: We don't republish events here - the listener will detect
		// the database changes and publish image_build.updated events
	}
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

	// Try to process any pending build (the specific build might already be taken by another builder)
	return s.processAnyPendingBuild(ctx)
}

// handleImageBuildUpdated handles image build update events from the listener
func (s *Service) handleImageBuildUpdated(ctx context.Context, data []byte) error {
	// Parse the image build updated message
	var imageBuildUpdated pb.ImageBuildUpdated
	if err := proto.Unmarshal(data, &imageBuildUpdated); err != nil {
		return fmt.Errorf("failed to unmarshal image build updated: %w", err)
	}

	s.logger.Debug("Processing image build updated event",
		"image_build_id", imageBuildUpdated.Id)

	// When a build is updated, it might have been reset to pending status
	// Try to process any pending build
	return s.processAnyPendingBuild(ctx)
}

// processAnyPendingBuild tries to process any pending build from the queue
func (s *Service) processAnyPendingBuild(ctx context.Context) error {
	// Try to dequeue any pending build
	imageBuild, err := s.queries.ImageBuildsDequeuePending(ctx)
	if err != nil {
		s.logger.Debug("No pending builds available")
		return nil
	}

	if imageBuild == nil {
		s.logger.Debug("No pending builds found")
		return nil
	}

	// Process the build we got
	return s.processImageBuildFromRow(ctx, imageBuild)
}

// processImageBuildFromRow processes an image build from a database row
func (s *Service) processImageBuildFromRow(ctx context.Context, imageBuild *database.ImageBuildsDequeuePendingRow) error {
	// Check if we're at capacity
	s.buildsMu.RLock()
	atCapacity := s.activeBuildCount >= s.config.MaxConcurrentBuilds
	s.buildsMu.RUnlock()

	if atCapacity {
		s.logger.Debug("At build capacity, deferring build",
			"active_builds", s.activeBuildCount,
			"max_concurrent", s.config.MaxConcurrentBuilds,
			"build_id", imageBuild.ID)
		// TODO: We should put the build back to pending status
		// For now, we'll just process it anyway
	}

	// Enrich the image build with deployment and project information
	enrichedBuild, err := s.enrichImageBuild(ctx, imageBuild)
	if err != nil {
		s.logger.Error("Failed to enrich image build", "build_id", imageBuild.ID, "error", err)
		return err
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

	return nil
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
			"image_hash", result.ImageHash[:12], // Log first 12 chars
			"image_size", result.ImageSize,
			"duration", result.Duration)

		// Create image record in database
		imageID, err := s.createImageRecord(buildCtx, result, build)
		if err != nil {
			s.logger.Error("Failed to create image record", "build_id", build.ID, "error", err)
			// Mark build as failed since we couldn't create the image record
			s.queries.ImageBuildsFail(buildCtx, build.ID)
			s.queries.DeploymentsUpdateStatus(buildCtx, &database.DeploymentsUpdateStatusParams{
				ID:     build.DeploymentID,
				Status: "failed",
			})
			return
		}

		// Update deployment with the created image_id
		_, err = s.queries.DeploymentsUpdateImageId(buildCtx, &database.DeploymentsUpdateImageIdParams{
			ID:      build.DeploymentID,
			ImageID: imageID,
		})
		if err != nil {
			s.logger.Error("Failed to update deployment image_id",
				"build_id", build.ID,
				"deployment_id", build.DeploymentID,
				"image_id", imageID,
				"error", err)
			// Continue anyway - the image was created successfully
		} else {
			s.logger.Info("Updated deployment with image_id",
				"build_id", build.ID,
				"deployment_id", build.DeploymentID,
				"image_id", imageID)
		}

		// Mark build as completed
		_, err = s.queries.ImageBuildsComplete(buildCtx, build.ID)
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

// createImageRecord creates a new image record in the database
func (s *Service) createImageRecord(ctx context.Context, result *BuildResult, build *EnrichedBuild) (pgtype.UUID, error) {
	// Generate a new UUID for the image
	imageUUID := uuid.GeneratePgUUID()

	// Convert image size to pgtype.Int4, handling overflow
	var imageSize pgtype.Int4
	if result.ImageSize > 2147483647 { // Max int32 value
		// For very large images, store max int32 value as a placeholder
		imageSize = pgtype.Int4{Int32: 2147483647, Valid: true}
	} else {
		imageSize = pgtype.Int4{Int32: int32(result.ImageSize), Valid: true}
	}

	// Create the image record
	_, err := s.queries.ImagesCreate(ctx, &database.ImagesCreateParams{
		ID:        imageUUID,
		Name:      result.ImageTag,
		Size:      imageSize,
		Hash:      result.ImageHash,
		ObjectKey: pgtype.Text{}, // We're not using S3 object key for registry images
	})

	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("failed to create image record: %w", err)
	}

	s.logger.Debug("Created image record",
		"image_id", imageUUID,
		"image_tag", result.ImageTag,
		"image_hash", result.ImageHash[:12])

	return imageUUID, nil
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
