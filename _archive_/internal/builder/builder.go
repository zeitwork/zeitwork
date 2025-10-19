package builder

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
	githubpkg "github.com/zeitwork/zeitwork/internal/shared/github"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type Config struct {
	BuilderID           string `env:"BUILDER_ID"`
	BuilderDatabaseURL  string `env:"BUILDER_DATABASE_URL"`
	BuilderRuntimeMode  string `env:"BUILDER_RUNTIME_MODE" envDefault:"docker"`
	BuilderWorkDir      string `env:"BUILDER_WORK_DIR" envDefault:"/tmp/zeitwork-builder"`
	BuilderRegistryURL  string `env:"BUILDER_REGISTRY_URL"`   // e.g., "ghcr.io/yourorg" - empty means local Docker only
	BuilderRegistryUser string `env:"BUILDER_REGISTRY_USER"`  // Registry username for authentication
	BuilderRegistryPass string `env:"BUILDER_REGISTRY_PASS"`  // Registry password or token
	BuilderGitHubAppID  string `env:"BUILDER_GITHUB_APP_ID"`  // GitHub App ID for private repo access
	BuilderGitHubAppKey string `env:"BUILDER_GITHUB_APP_KEY"` // GitHub App private key (PEM format)
}

type logEntry struct {
	level    string
	message  string
	loggedAt time.Time
}

type logBuffer struct {
	mu           sync.Mutex
	logs         []logEntry
	imageBuildID pgtype.UUID
	flushThresh  int
	db           *database.DB
	ctx          context.Context
	cancel       context.CancelFunc
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

	// Authenticate to registry if configured
	if cfg.BuilderRegistryURL != "" {
		logger.Info("registry configured, authenticating",
			"registry_url", cfg.BuilderRegistryURL,
			"registry_user", cfg.BuilderRegistryUser,
		)
		if err := svc.dockerLogin(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to authenticate to registry: %w", err)
		}
		logger.Info("successfully authenticated to registry")
	} else {
		logger.Info("no registry configured, using local Docker only")
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

// newLogBuffer creates a new log buffer for the given image build
func newLogBuffer(ctx context.Context, db *database.DB, imageBuildID pgtype.UUID) *logBuffer {
	ctx, cancel := context.WithCancel(ctx)
	lb := &logBuffer{
		logs:         make([]logEntry, 0, 100),
		imageBuildID: imageBuildID,
		flushThresh:  10,
		db:           db,
		ctx:          ctx,
		cancel:       cancel,
	}

	// Start ticker to flush logs every 1 second
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				lb.flush()
			case <-ctx.Done():
				lb.flush() // Final flush on shutdown
				return
			}
		}
	}()

	return lb
}

// append adds a log entry to the buffer
func (lb *logBuffer) append(level, message string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.logs = append(lb.logs, logEntry{
		level:    level,
		message:  message,
		loggedAt: time.Now(),
	})

	// Flush if threshold reached
	if len(lb.logs) >= lb.flushThresh {
		go lb.flush()
	}
}

// flush writes all buffered logs to the database
func (lb *logBuffer) flush() {
	lb.mu.Lock()
	if len(lb.logs) == 0 {
		lb.mu.Unlock()
		return
	}

	// Copy logs to insert
	logsToInsert := make([]logEntry, len(lb.logs))
	copy(logsToInsert, lb.logs)
	lb.logs = lb.logs[:0] // Clear buffer
	lb.mu.Unlock()

	// Prepare batch insert
	params := make([]*database.InsertLogsParams, len(logsToInsert))
	for i, log := range logsToInsert {
		params[i] = &database.InsertLogsParams{
			ID:           uuid.New(),
			ImageBuildID: lb.imageBuildID,
			InstanceID:   pgtype.UUID{Valid: false},
			Level:        pgtype.Text{String: log.level, Valid: log.level != ""},
			Message:      log.message,
			LoggedAt:     pgtype.Timestamptz{Time: log.loggedAt, Valid: true},
		}
	}

	// Insert logs
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := lb.db.Queries().InsertLogs(ctx, params)
	if err != nil {
		// Log error but don't fail the build
		fmt.Fprintf(os.Stderr, "failed to insert logs: %v\n", err)
	}
}

// close stops the log buffer and flushes remaining logs
func (lb *logBuffer) close() {
	if lb.cancel != nil {
		lb.cancel()
	}
	lb.flush()
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

	// Create log buffer for this build
	logBuf := newLogBuffer(ctx, s.db, build.ID)
	defer logBuf.close()

	logBuf.append("info", fmt.Sprintf("Starting build for %s @ %s", build.GithubRepository, build.GithubCommit))

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
		"has_installation_id", build.GithubInstallationID.Valid,
	)
	logBuf.append("info", fmt.Sprintf("Cloning repository %s...", build.GithubRepository))
	if err := s.cloneRepo(ctx, build.GithubRepository, build.GithubCommit, build.GithubInstallationID, repoDir, logBuf); err != nil {
		s.logger.Error("failed to clone repository",
			"build_id", buildID,
			"repository", build.GithubRepository,
			"error", err,
		)
		logBuf.append("error", fmt.Sprintf("Failed to clone repository: %v", err))
		s.markFailed(ctx, build.ID)
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	s.logger.Info("repository cloned successfully",
		"build_id", buildID,
		"repository", build.GithubRepository,
	)
	logBuf.append("info", "Repository cloned successfully")

	// Create image record first to get the image ID for naming
	imageID := uuid.New()
	s.logger.Info("generating image ID for build",
		"build_id", buildID,
		"image_id", uuid.ToString(imageID),
	)

	// Build image with the proper name based on repository
	imageName := s.generateImageName(build.GithubRepository, build.GithubCommit)
	s.logger.Info("building docker image",
		"build_id", buildID,
		"image_id", uuid.ToString(imageID),
		"image_name", imageName,
		"source_dir", repoDir,
	)

	logBuf.append("info", fmt.Sprintf("Building Docker image %s...", imageName))
	if err := s.buildImage(ctx, repoDir, imageName, logBuf); err != nil {
		s.logger.Error("failed to build docker image",
			"build_id", buildID,
			"image_name", imageName,
			"error", err,
		)
		logBuf.append("error", fmt.Sprintf("Failed to build image: %v", err))
		s.markFailed(ctx, build.ID)
		return fmt.Errorf("failed to build image: %w", err)
	}
	s.logger.Info("docker image built successfully",
		"build_id", buildID,
		"image_name", imageName,
	)
	logBuf.append("info", "Docker image built successfully")

	// Push to registry if configured
	if s.cfg.BuilderRegistryURL != "" {
		s.logger.Info("pushing image to registry",
			"build_id", buildID,
			"image_name", imageName,
			"registry_url", s.cfg.BuilderRegistryURL,
		)
		logBuf.append("info", "Pushing image to registry...")
		if err := s.pushImage(ctx, imageName, logBuf); err != nil {
			s.logger.Error("failed to push image to registry",
				"build_id", buildID,
				"image_name", imageName,
				"error", err,
			)
			logBuf.append("error", fmt.Sprintf("Failed to push image: %v", err))
			s.markFailed(ctx, build.ID)
			return fmt.Errorf("failed to push image: %w", err)
		}
		s.logger.Info("image pushed to registry successfully",
			"build_id", buildID,
			"image_name", imageName,
		)
		logBuf.append("info", "Image pushed to registry successfully")
	}

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

	// Create image record (imageID was generated earlier for naming)
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

func (s *Service) cloneRepo(ctx context.Context, repository, commit string, installationID pgtype.UUID, destDir string, logBuf *logBuffer) error {
	// Determine the clone URL (with or without authentication)
	var repoURL string
	var tokenUsed bool

	if installationID.Valid {
		// Check if GitHub App credentials are configured
		if s.cfg.BuilderGitHubAppID == "" || s.cfg.BuilderGitHubAppKey == "" {
			return fmt.Errorf("repository requires GitHub App authentication but credentials are not configured")
		}

		// Generate installation token for private repo access
		s.logger.Info("generating GitHub installation token",
			"repository", repository,
			"installation_id", uuid.ToString(installationID),
		)

		tokenService, err := githubpkg.NewTokenService(s.cfg.BuilderGitHubAppID, s.cfg.BuilderGitHubAppKey)
		if err != nil {
			return fmt.Errorf("failed to create GitHub token service: %w", err)
		}

		token, err := tokenService.GetInstallationToken(ctx, s.db, uuid.ToString(installationID))
		if err != nil {
			return fmt.Errorf("failed to get GitHub installation token: %w", err)
		}

		// Use token in clone URL (never log the token itself)
		repoURL = fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", token, repository)
		tokenUsed = true
		s.logger.Info("using authenticated GitHub clone",
			"repository", repository,
		)
	} else {
		// Return an error if no installation ID is provided
		return fmt.Errorf("repository requires GitHub App authentication but no installation ID is provided")
	}

	s.logger.Info("executing git clone",
		"repository", repository,
		"authenticated", tokenUsed,
		"destination", destDir,
	)
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", repoURL, destDir)

	// Capture output
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("git clone failed to start: %w", err)
	}

	// Stream output to log buffer
	go s.streamOutput(stdout, logBuf, "info")
	go s.streamOutput(stderr, logBuf, "info")

	if err := cmd.Wait(); err != nil {
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

	stdout, _ = cmd.StdoutPipe()
	stderr, _ = cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("git checkout failed to start: %w", err)
	}

	// Stream output to log buffer
	go s.streamOutput(stdout, logBuf, "info")
	go s.streamOutput(stderr, logBuf, "info")

	if err := cmd.Wait(); err != nil {
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

// streamOutput reads from a reader and appends each line to the log buffer
func (s *Service) streamOutput(r io.Reader, logBuf *logBuffer, level string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			logBuf.append(level, line)
		}
	}
}

func (s *Service) buildImage(ctx context.Context, repoDir, imageName string, logBuf *logBuffer) error {
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

	// Build using docker build
	s.logger.Info("executing docker build",
		"image_name", imageName,
		"context_dir", repoDir,
	)

	cmd := exec.CommandContext(ctx, "docker", "build", "-t", imageName, ".")
	cmd.Dir = repoDir

	// Capture output
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("docker build failed to start: %w", err)
	}

	// Stream output to log buffer
	go s.streamOutput(stdout, logBuf, "info")
	go s.streamOutput(stderr, logBuf, "info")

	if err := cmd.Wait(); err != nil {
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

func (s *Service) generateImageName(githubRepository string, commit string) string {
	// Format: [registry/]zeitwork-image-${user}-${repo}:commit
	// Example: ghcr.io/zeitwork/zeitwork-image-tomharter-myapp:abc1234567890abcdef
	//       or zeitwork-image-tomharter-myapp:abc1234567890abcdef (local)

	// Parse repository (format: "username/repo-name")
	parts := strings.Split(githubRepository, "/")
	user := "unknown"
	repo := "unknown"

	if len(parts) >= 2 {
		user = parts[0]
		repo = parts[1]
	} else if len(parts) == 1 {
		repo = parts[0]
	}

	// Sanitize: replace non-alphanumeric chars with hyphens and convert to lowercase
	sanitize := func(s string) string {
		var result strings.Builder
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				result.WriteRune(r)
			} else {
				result.WriteRune('-')
			}
		}
		return strings.ToLower(result.String())
	}

	sanitizedUser := sanitize(user)
	sanitizedRepo := sanitize(repo)

	// Use full commit hash (no shortening)
	imageName := fmt.Sprintf("zeitwork-image-%s-%s:%s", sanitizedUser, sanitizedRepo, commit)

	// Add registry prefix if configured
	if s.cfg.BuilderRegistryURL != "" {
		// Ensure registry URL doesn't have trailing slash
		registryURL := strings.TrimSuffix(s.cfg.BuilderRegistryURL, "/")
		imageName = fmt.Sprintf("%s/%s", registryURL, imageName)
	}

	return imageName
}

// dockerLogin authenticates to the configured registry
func (s *Service) dockerLogin(ctx context.Context) error {
	if s.cfg.BuilderRegistryURL == "" {
		return nil // No registry configured
	}

	if s.cfg.BuilderRegistryUser == "" || s.cfg.BuilderRegistryPass == "" {
		return fmt.Errorf("registry URL configured but missing credentials")
	}

	// Extract registry host from URL (e.g., "ghcr.io/yourorg" -> "ghcr.io")
	registryHost := s.cfg.BuilderRegistryURL
	if strings.Contains(registryHost, "/") {
		registryHost = strings.Split(registryHost, "/")[0]
	}

	s.logger.Info("[REGISTRY] logging in to registry",
		"registry_host", registryHost,
		"username", s.cfg.BuilderRegistryUser,
	)

	cmd := exec.CommandContext(ctx, "docker", "login", registryHost,
		"--username", s.cfg.BuilderRegistryUser,
		"--password-stdin")

	// Pass password via stdin for security
	cmd.Stdin = strings.NewReader(s.cfg.BuilderRegistryPass)
	output, err := cmd.CombinedOutput()

	if err != nil {
		s.logger.Error("[REGISTRY] docker login failed",
			"registry_host", registryHost,
			"error", err,
			"output", string(output),
		)
		return fmt.Errorf("docker login failed: %w: %s", err, string(output))
	}

	s.logger.Info("[REGISTRY] successfully logged in to registry",
		"registry_host", registryHost,
		"output", string(output),
	)

	return nil
}

// pushImage pushes the built image to the configured registry
func (s *Service) pushImage(ctx context.Context, imageName string, logBuf *logBuffer) error {
	s.logger.Info("[REGISTRY] pushing image",
		"image_name", imageName,
	)

	cmd := exec.CommandContext(ctx, "docker", "push", imageName)

	// Capture output
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("docker push failed to start: %w", err)
	}

	// Stream output to log buffer
	go s.streamOutput(stdout, logBuf, "info")
	go s.streamOutput(stderr, logBuf, "info")

	if err := cmd.Wait(); err != nil {
		s.logger.Error("[REGISTRY] docker push failed",
			"image_name", imageName,
			"error", err,
		)
		return fmt.Errorf("docker push failed: %w", err)
	}

	s.logger.Info("[REGISTRY] image pushed successfully",
		"image_name", imageName,
	)

	return nil
}
