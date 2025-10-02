package builder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type Config struct {
	BuilderID          string `env:"BUILDER_ID"`
	BuilderDatabaseURL string `env:"BUILDER_DATABASE_URL"`
	BuilderRuntimeMode string `env:"BUILDER_RUNTIME_MODE" envDefault:"docker"`
	BuilderWorkDir     string `env:"BUILDER_WORK_DIR" envDefault:"/tmp/zeitwork-builder"`
}

type Service struct {
	cfg       Config
	db        *database.DB
	logger    *slog.Logger
	builderID pgtype.UUID
}

func NewService(cfg Config, logger *slog.Logger) (*Service, error) {
	// Parse builder UUID
	builderUUID, err := uuid.Parse(cfg.BuilderID)
	if err != nil {
		return nil, fmt.Errorf("invalid builder id: %w", err)
	}

	// Initialize database
	db, err := database.NewDB(cfg.BuilderDatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Ensure work directory exists
	if err := os.MkdirAll(cfg.BuilderWorkDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	svc := &Service{
		cfg:       cfg,
		db:        db,
		logger:    logger,
		builderID: builderUUID,
	}

	return svc, nil
}

func (s *Service) Start() {
	s.logger.Info("builder started",
		"builder_id", s.cfg.BuilderID,
		"runtime", s.cfg.BuilderRuntimeMode,
	)

	for {
		s.logger.Info("starting build loop")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		err := s.buildNext(ctx)
		cancel()

		if err != nil {
			if err.Error() != "no pending builds" {
				s.logger.Error("build failed", "error", err)
			} else {
				s.logger.Info("no pending builds")
				// Only sleep if there are no pending builds
				offset := time.Duration(rand.Intn(11)-5) * time.Second
				sleepDuration := 15*time.Second + offset
				s.logger.Info("sleeping", "duration", sleepDuration)
				time.Sleep(sleepDuration)
			}
		} else {
			// Build completed successfully, check for next build immediately
			s.logger.Info("build completed, checking for next build immediately")
		}
	}
}

func (s *Service) Close() {
	s.logger.Info("shutting down builder")

	if s.db != nil {
		s.db.Close()
	}
}

func (s *Service) buildNext(ctx context.Context) error {
	s.logger.Info("querying for pending build")

	// Get next pending build with row-level locking
	build, err := s.db.Queries().GetPendingImageBuild(ctx)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return fmt.Errorf("no pending builds")
		}
		s.logger.Error("failed to query pending build", "error", err)
		return fmt.Errorf("failed to get pending build: %w", err)
	}

	buildID := uuid.ToString(build.ID)
	s.logger.Info("acquired pending build",
		"build_id", buildID,
		"repository", build.GithubRepository,
		"commit", build.GithubCommit,
	)

	// Mark as building
	s.logger.Info("transitioning build state: pending → building", "build_id", buildID)
	if err := s.db.Queries().UpdateImageBuildStarted(ctx, build.ID); err != nil {
		s.logger.Error("failed to update build status to building",
			"build_id", buildID,
			"error", err,
		)
		return fmt.Errorf("failed to update build status: %w", err)
	}
	s.logger.Info("state changed: pending → building", "build_id", buildID)

	// Create build workspace
	workDir := filepath.Join(s.cfg.BuilderWorkDir, buildID)
	s.logger.Info("creating build workspace",
		"build_id", buildID,
		"work_dir", workDir,
	)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		s.logger.Error("failed to create work directory",
			"build_id", buildID,
			"work_dir", workDir,
			"error", err,
		)
		s.markFailed(ctx, build.ID)
		return fmt.Errorf("failed to create work directory: %w", err)
	}
	defer func() {
		s.logger.Info("cleaning up build workspace",
			"build_id", buildID,
			"work_dir", workDir,
		)
		os.RemoveAll(workDir)
	}()

	// Clone repository
	repoDir := filepath.Join(workDir, "repo")
	s.logger.Info("cloning repository",
		"build_id", buildID,
		"repository", build.GithubRepository,
		"commit", build.GithubCommit,
		"destination", repoDir,
	)
	if err := s.cloneRepo(ctx, build.GithubRepository, build.GithubCommit, repoDir); err != nil {
		s.logger.Error("failed to clone repository",
			"build_id", buildID,
			"repository", build.GithubRepository,
			"error", err,
		)
		s.markFailed(ctx, build.ID)
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	s.logger.Info("repository cloned successfully",
		"build_id", buildID,
		"repository", build.GithubRepository,
	)

	// Build image
	imageName := s.generateImageName(build.GithubRepository, build.GithubCommit)
	s.logger.Info("building docker image",
		"build_id", buildID,
		"image_name", imageName,
		"source_dir", repoDir,
	)

	if err := s.buildImage(ctx, repoDir, imageName); err != nil {
		s.logger.Error("failed to build docker image",
			"build_id", buildID,
			"image_name", imageName,
			"error", err,
		)
		s.markFailed(ctx, build.ID)
		return fmt.Errorf("failed to build image: %w", err)
	}
	s.logger.Info("docker image built successfully",
		"build_id", buildID,
		"image_name", imageName,
	)

	// Get image details
	s.logger.Info("inspecting image details",
		"build_id", buildID,
		"image_name", imageName,
	)
	imageSize, imageHash, err := s.getImageDetails(ctx, imageName)
	if err != nil {
		s.logger.Error("failed to get image details",
			"build_id", buildID,
			"image_name", imageName,
			"error", err,
		)
		s.markFailed(ctx, build.ID)
		return fmt.Errorf("failed to get image details: %w", err)
	}
	s.logger.Info("image details retrieved",
		"build_id", buildID,
		"image_name", imageName,
		"size_bytes", imageSize,
		"hash", imageHash,
	)

	// Create image record
	imageID := uuid.New()
	s.logger.Info("creating image record in database",
		"build_id", buildID,
		"image_id", uuid.ToString(imageID),
		"image_name", imageName,
		"size_bytes", imageSize,
	)

	image, err := s.db.Queries().CreateImage(ctx, &database.CreateImageParams{
		ID:   imageID,
		Name: imageName,
		Size: pgtype.Int4{Int32: int32(imageSize), Valid: true},
		Hash: imageHash,
	})
	if err != nil {
		s.logger.Error("failed to create image record",
			"build_id", buildID,
			"image_id", uuid.ToString(imageID),
			"error", err,
		)
		s.markFailed(ctx, build.ID)
		return fmt.Errorf("failed to create image record: %w", err)
	}
	s.logger.Info("image record created",
		"build_id", buildID,
		"image_id", uuid.ToString(image.ID),
	)

	// Mark build as completed
	s.logger.Info("transitioning build state: building → completed",
		"build_id", buildID,
		"image_id", uuid.ToString(image.ID),
	)
	if err := s.db.Queries().UpdateImageBuildCompleted(ctx, &database.UpdateImageBuildCompletedParams{
		ID:      build.ID,
		ImageID: pgtype.UUID{Bytes: image.ID.Bytes, Valid: true},
	}); err != nil {
		s.logger.Error("failed to update build status to completed",
			"build_id", buildID,
			"error", err,
		)
		return fmt.Errorf("failed to update build status: %w", err)
	}

	s.logger.Info("build completed successfully",
		"build_id", buildID,
		"image_id", uuid.ToString(image.ID),
		"image_name", imageName,
		"image_size_bytes", imageSize,
		"state", "completed",
	)

	return nil
}

func (s *Service) markFailed(ctx context.Context, buildID pgtype.UUID) {
	buildIDStr := uuid.ToString(buildID)
	s.logger.Info("transitioning build state: building → failed",
		"build_id", buildIDStr,
	)
	if err := s.db.Queries().UpdateImageBuildFailed(ctx, buildID); err != nil {
		s.logger.Error("failed to mark build as failed",
			"build_id", buildIDStr,
			"error", err,
		)
		return
	}
	s.logger.Info("state changed: building → failed",
		"build_id", buildIDStr,
	)
}

func (s *Service) cloneRepo(ctx context.Context, repository, commit, destDir string) error {
	// Clone repository
	repoURL := fmt.Sprintf("https://github.com/%s.git", repository)
	s.logger.Info("executing git clone",
		"repository", repository,
		"url", repoURL,
		"destination", destDir,
	)
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", repoURL, destDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		s.logger.Error("git clone failed",
			"repository", repository,
			"url", repoURL,
			"error", err,
		)
		return fmt.Errorf("git clone failed: %w", err)
	}

	s.logger.Info("git clone completed, checking out commit",
		"repository", repository,
		"commit", commit,
	)

	// Checkout specific commit
	cmd = exec.CommandContext(ctx, "git", "checkout", commit)
	cmd.Dir = destDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		s.logger.Error("git checkout failed",
			"repository", repository,
			"commit", commit,
			"error", err,
		)
		return fmt.Errorf("git checkout failed: %w", err)
	}

	s.logger.Info("git checkout completed",
		"repository", repository,
		"commit", commit,
	)

	return nil
}

func (s *Service) buildImage(ctx context.Context, repoDir, imageName string) error {
	// Check if Dockerfile exists
	dockerfilePath := filepath.Join(repoDir, "Dockerfile")
	s.logger.Info("checking for Dockerfile",
		"dockerfile_path", dockerfilePath,
	)
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		s.logger.Error("Dockerfile not found in repository",
			"expected_path", dockerfilePath,
		)
		return fmt.Errorf("no Dockerfile found in repository")
	}
	s.logger.Info("Dockerfile found", "path", dockerfilePath)

	// Build using docker
	s.logger.Info("executing docker build",
		"image_name", imageName,
		"context_dir", repoDir,
	)
	cmd := exec.CommandContext(ctx, "docker", "build", "-t", imageName, ".")
	cmd.Dir = repoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		s.logger.Error("docker build failed",
			"image_name", imageName,
			"context_dir", repoDir,
			"error", err,
		)
		return fmt.Errorf("docker build failed: %w", err)
	}

	s.logger.Info("docker build completed successfully",
		"image_name", imageName,
	)

	return nil
}

func (s *Service) getImageDetails(ctx context.Context, imageName string) (int64, string, error) {
	// Get image size
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", imageName, "--format", "{{.Size}}")
	output, err := cmd.Output()
	if err != nil {
		return 0, "", fmt.Errorf("failed to get image size: %w", err)
	}

	var size int64
	if _, err := fmt.Sscanf(string(output), "%d", &size); err != nil {
		return 0, "", fmt.Errorf("failed to parse image size: %w", err)
	}

	// Compute hash from image name for simplicity
	hash := sha256.Sum256([]byte(imageName))
	imageHash := hex.EncodeToString(hash[:])

	return size, imageHash, nil
}

func (s *Service) generateImageName(repository, commit string) string {
	// Format: org/repo:commit-short
	shortCommit := commit
	if len(commit) > 7 {
		shortCommit = commit[:7]
	}
	return fmt.Sprintf("%s:%s", repository, shortCommit)
}
