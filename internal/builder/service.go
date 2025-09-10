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

	"github.com/jackc/pgx/v5"
	"github.com/zeitwork/zeitwork/internal/builder/runtime"
	"github.com/zeitwork/zeitwork/internal/builder/types"
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
	buildRuntime types.BuildRuntime

	// Build processing state
	buildsMu         sync.RWMutex
	activeBuildCount int
	stopChan         chan struct{}
	buildsWG         sync.WaitGroup
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

	// Initialize build runtime based on configuration
	buildRuntime, err := runtime.NewBuildRuntime(cfg, logger.With("component", "build-runtime"))
	if err != nil {
		return nil, fmt.Errorf("failed to create build runtime: %w", err)
	}

	s := &Service{
		logger:           logger,
		config:           cfg,
		db:               db,
		queries:          queries,
		natsClient:       natsClient,
		buildRuntime:     buildRuntime,
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

	s.logger.Info("Shutting down image builder service: stopping new work and waiting for in-flight builds")
	close(s.stopChan)

	// Wait for in-flight builds with grace period
	grace := s.config.ShutdownGracePeriod
	if grace <= 0 {
		grace = 30 * time.Second
	}
	done := make(chan struct{})
	go func() {
		s.buildsWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("All in-flight builds completed before shutdown")
	case <-time.After(grace):
		s.logger.Warn("Shutdown grace period elapsed; exiting with builds possibly still running",
			"in_flight", s.getActiveBuilds())
	}
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

		// Convert to ImageBuild and process
		build := &database.ImageBuild{
			ID:               imageBuild.ID,
			Status:           imageBuild.Status,
			GithubRepository: imageBuild.GithubRepository,
			GithubCommit:     imageBuild.GithubCommit,
			ImageID:          imageBuild.ImageID,
			StartedAt:        imageBuild.StartedAt,
			CompletedAt:      imageBuild.CompletedAt,
			FailedAt:         imageBuild.FailedAt,
			CreatedAt:        imageBuild.CreatedAt,
			UpdatedAt:        imageBuild.UpdatedAt,
		}

		// Process the build
		if err := s.processImageBuildFromRow(ctx, build); err != nil {
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

	// Use context-aware NATS client for auto-unsubscribe on cancel
	ctxNats := s.natsClient.WithContext(ctx)

	// Subscribe to image_build.created events
	_, err := ctxNats.QueueSubscribe("image_build.created", queueGroup, func(msg *nats.Msg) {
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
	_, err = ctxNats.QueueSubscribe("image_build.updated", queueGroup, func(msg *nats.Msg) {
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
				"build_id", build.ID)
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

	// First try to dequeue that specific build to avoid race with SKIP LOCKED
	if err := s.processSpecificBuild(ctx, imageBuildCreated.Id); err != nil {
		s.logger.Error("Failed to dequeue specific build", "build_id", imageBuildCreated.Id, "error", err)
	}

	// Fallback: try to process any pending build (legacy behaviour)
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

	// Try to dequeue that specific build first (in case it was reset to pending)
	if err := s.processSpecificBuild(ctx, imageBuildUpdated.Id); err != nil {
		s.logger.Error("Failed to dequeue specific build", "build_id", imageBuildUpdated.Id, "error", err)
	}

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

	// Convert to ImageBuild and process
	build := &database.ImageBuild{
		ID:               imageBuild.ID,
		Status:           imageBuild.Status,
		GithubRepository: imageBuild.GithubRepository,
		GithubCommit:     imageBuild.GithubCommit,
		ImageID:          imageBuild.ImageID,
		StartedAt:        imageBuild.StartedAt,
		CompletedAt:      imageBuild.CompletedAt,
		FailedAt:         imageBuild.FailedAt,
		CreatedAt:        imageBuild.CreatedAt,
		UpdatedAt:        imageBuild.UpdatedAt,
	}
	return s.processImageBuildFromRow(ctx, build)
}

// processImageBuildFromRow processes an image build from a database row
func (s *Service) processImageBuildFromRow(ctx context.Context, imageBuild *database.ImageBuild) error {
	// If shutting down, do not start new work
	select {
	case <-s.stopChan:
		s.logger.Info("Service is shutting down; skipping new build", "build_id", imageBuild.ID)
		return nil
	default:
	}

	// Check if we're at capacity
	s.buildsMu.RLock()
	atCapacity := s.activeBuildCount >= s.config.MaxConcurrentBuilds
	s.buildsMu.RUnlock()

	if atCapacity {
		s.logger.Debug("At build capacity, deferring build",
			"active_builds", s.activeBuildCount,
			"max_concurrent", s.config.MaxConcurrentBuilds,
			"build_id", imageBuild.ID)
		return nil
	}

	s.logger.Info("Starting build processing",
		"build_id", imageBuild.ID,
		"github_repo", imageBuild.GithubRepository,
		"commit_hash", imageBuild.GithubCommit)

	// Increment active build count and WG
	s.buildsMu.Lock()
	s.activeBuildCount++
	s.buildsMu.Unlock()
	s.buildsWG.Add(1)

	// Process the build in a separate goroutine
	go s.processBuild(ctx, imageBuild)

	return nil
}

// enrichImageBuild removed; no longer needed

// processBuild processes a single build
func (s *Service) processBuild(ctx context.Context, build *database.ImageBuild) {
	// Ensure we decrement the active build count when done
	defer func() {
		s.buildsMu.Lock()
		s.activeBuildCount--
		s.buildsMu.Unlock()
		s.buildsWG.Done()
	}()

	buildCtx, cancel := context.WithTimeout(ctx, s.config.BuildTimeout)
	defer cancel()

	s.logger.Info("Processing build", "build_id", build.ID)

	// Execute the build using the configured build runtime
	result := s.buildRuntime.Build(buildCtx, build)

	if result.Success {
		s.logger.Info("Build completed successfully",
			"build_id", build.ID,
			"image_tag", result.ImageTag,
			"image_hash", result.ImageHash[:12], // Log first 12 chars
			"image_size", result.ImageSize,
			"duration", result.Duration)

		// Create image record in database
		imageID, err := s.createImageRecord(buildCtx, result)
		if err != nil {
			s.logger.Error("Failed to create image record", "build_id", build.ID, "error", err)
			// Mark build as failed since we couldn't create the image record
			s.queries.ImageBuildsFail(buildCtx, build.ID)
			return
		}

		// Link image_build to created image
		if _, err := s.queries.ImageBuildsUpdateImageId(buildCtx, &database.ImageBuildsUpdateImageIdParams{
			ID:      build.ID,
			ImageID: pgtype.UUID{Valid: true, Bytes: imageID.Bytes},
		}); err != nil {
			s.logger.Error("Failed to update image_build image_id", "build_id", build.ID, "image_id", imageID, "error", err)
			// Do not fail the build solely due to linkage; manager can retry on next event
		}

		// Mark build as completed
		_, err = s.queries.ImageBuildsComplete(buildCtx, build.ID)
		if err != nil {
			s.logger.Error("Failed to mark build as completed", "build_id", build.ID, "error", err)
		}

		_ = imageID // image linking to build may be handled elsewhere
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
	}
}

// createImageRecord creates a new image record in the database
func (s *Service) createImageRecord(ctx context.Context, result *types.BuildResult) (pgtype.UUID, error) {
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
		ID:   imageUUID,
		Name: result.ImageTag,
		Size: imageSize,
		Hash: result.ImageHash,
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
	if s.buildRuntime != nil {
		if err := s.buildRuntime.Cleanup(); err != nil {
			s.logger.Warn("Failed to cleanup build runtime", "error", err)
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

func (s *Service) getActiveBuilds() int {
	s.buildsMu.RLock()
	defer s.buildsMu.RUnlock()
	return s.activeBuildCount
}

// processSpecificBuild tries to dequeue and process a specific build id if it is still pending.
func (s *Service) processSpecificBuild(ctx context.Context, id string) error {
	buildUUID := uuid.MustParseUUID(id)

	row, err := s.dequeueBuildByID(ctx, buildUUID)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Someone else picked it up or it is no longer pending
			return nil
		}
		return err
	}

	// Convert to ImageBuild and process
	build := &database.ImageBuild{
		ID:               row.ID,
		Status:           row.Status,
		GithubRepository: row.GithubRepository,
		GithubCommit:     row.GithubCommit,
		ImageID:          row.ImageID,
		StartedAt:        row.StartedAt,
		CompletedAt:      row.CompletedAt,
		FailedAt:         row.FailedAt,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}

	// Successfully dequeued the specific build – process it
	return s.processImageBuildFromRow(ctx, build)
}

// dequeueBuildByID atomically moves a pending build to building for a given id.
func (s *Service) dequeueBuildByID(ctx context.Context, id pgtype.UUID) (*database.ImageBuildsDequeueByIDRow, error) {
	return s.queries.ImageBuildsDequeueByID(ctx, id)
}
