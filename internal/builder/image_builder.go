package builder

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/moby/go-archive"
)

// BuildResult represents the result of a build operation
type BuildResult struct {
	Success  bool
	ImageTag string
	BuildLog string
	Error    error
	Duration time.Duration
}

// EnrichedBuild combines ImageBuild with related deployment and project information
type EnrichedBuild struct {
	// From ImageBuild
	ID             pgtype.UUID
	Status         string
	DeploymentID   pgtype.UUID
	StartedAt      pgtype.Timestamptz
	CompletedAt    pgtype.Timestamptz
	FailedAt       pgtype.Timestamptz
	OrganisationID pgtype.UUID
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz

	// From Deployment
	CommitHash string
	ProjectID  pgtype.UUID

	// From Project
	GithubRepository string
	DefaultBranch    string
}

// ImageBuilder defines the interface for building container images
type ImageBuilder interface {
	// Build builds a container image from the given build configuration
	Build(ctx context.Context, build *EnrichedBuild) *BuildResult

	// Name returns the name of the builder implementation
	Name() string

	// Cleanup performs any necessary cleanup operations
	Cleanup() error
}

// DockerBuilder implements ImageBuilder using the Docker Go SDK
type DockerBuilder struct {
	logger       *slog.Logger
	workDir      string
	registry     string
	dockerClient *client.Client
}

// DockerBuilderConfig holds configuration for the Docker builder
type DockerBuilderConfig struct {
	WorkDir  string // Directory where builds are performed
	Registry string // Container registry to push images to
}

// NewDockerBuilder creates a new DockerBuilder instance
func NewDockerBuilder(config DockerBuilderConfig, logger *slog.Logger) (*DockerBuilder, error) {
	if config.WorkDir == "" {
		config.WorkDir = "/tmp/zeitwork-builds"
	}

	// Ensure work directory exists
	if err := os.MkdirAll(config.WorkDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Initialize Docker client
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	// Verify Docker is available by pinging the daemon
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := dockerClient.Ping(ctx); err != nil {
		return nil, fmt.Errorf("docker daemon is not available: %w", err)
	}

	return &DockerBuilder{
		logger:       logger,
		workDir:      config.WorkDir,
		registry:     config.Registry,
		dockerClient: dockerClient,
	}, nil
}

// Name returns the name of the Docker builder
func (d *DockerBuilder) Name() string {
	return "docker"
}

// Build builds a container image using Docker
func (d *DockerBuilder) Build(ctx context.Context, build *EnrichedBuild) *BuildResult {
	startTime := time.Now()

	result := &BuildResult{
		Success: false,
	}

	d.logger.Info("Starting Docker build",
		"build_id", build.ID,
		"repo", build.GithubRepository,
		"commit", build.CommitHash,
		"branch", build.DefaultBranch)

	// Create a unique build directory
	buildDir := filepath.Join(d.workDir, fmt.Sprintf("build-%x-%d", build.ID.Bytes, time.Now().Unix()))
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		result.Error = fmt.Errorf("failed to create build directory: %w", err)
		result.BuildLog = fmt.Sprintf("Error: %v", result.Error)
		result.Duration = time.Since(startTime)
		return result
	}

	// Cleanup build directory when done
	defer func() {
		if err := os.RemoveAll(buildDir); err != nil {
			d.logger.Warn("Failed to cleanup build directory", "dir", buildDir, "error", err)
		}
	}()

	// Clone the repository
	if err := d.cloneRepository(ctx, build, buildDir); err != nil {
		result.Error = fmt.Errorf("failed to clone repository: %w", err)
		result.BuildLog = fmt.Sprintf("Clone failed: %v", result.Error)
		result.Duration = time.Since(startTime)
		return result
	}

	// Generate image tag
	imageTag := d.generateImageTag(build)
	result.ImageTag = imageTag

	// Build the Docker image
	buildLog, err := d.buildDockerImage(ctx, buildDir, imageTag)
	result.BuildLog = buildLog

	if err != nil {
		result.Error = fmt.Errorf("docker build failed: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Push the image to registry if configured
	if d.registry != "" {
		pushLog, err := d.pushDockerImage(ctx, imageTag)
		result.BuildLog += "\n" + pushLog

		if err != nil {
			result.Error = fmt.Errorf("docker push failed: %w", err)
			result.Duration = time.Since(startTime)
			return result
		}
	}

	result.Success = true
	result.Duration = time.Since(startTime)

	d.logger.Info("Docker build completed successfully",
		"build_id", build.ID,
		"image_tag", imageTag,
		"duration", result.Duration)

	return result
}

// cloneRepository clones the Git repository to the build directory using go-git
func (d *DockerBuilder) cloneRepository(ctx context.Context, build *EnrichedBuild, buildDir string) error {
	repoURL := fmt.Sprintf("https://github.com/%s.git", build.GithubRepository)

	d.logger.Debug("Cloning repository", "url", repoURL, "commit", build.CommitHash)

	// Clone the repository
	repo, err := git.PlainCloneContext(ctx, buildDir, false, &git.CloneOptions{
		URL:      repoURL,
		Progress: nil, // Could add progress logging here if needed
		Auth:     nil, // For public repos, no auth needed. For private repos, would need token
	})
	if err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	// Get the working tree
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Checkout the specific commit
	commitHash := plumbing.NewHash(build.CommitHash)
	err = worktree.Checkout(&git.CheckoutOptions{
		Hash: commitHash,
	})
	if err != nil {
		return fmt.Errorf("git checkout failed: %w", err)
	}

	d.logger.Debug("Repository cloned and checked out successfully", "commit", build.CommitHash)
	return nil
}

// generateImageTag generates a unique image tag for the build
func (d *DockerBuilder) generateImageTag(build *EnrichedBuild) string {
	// Extract repository name from github_repository (owner/repo -> repo)
	repoParts := strings.Split(build.GithubRepository, "/")
	repoName := repoParts[len(repoParts)-1]

	// Create tag with project ID and short commit hash
	shortCommit := build.CommitHash
	if len(shortCommit) > 7 {
		shortCommit = shortCommit[:7]
	}

	tag := fmt.Sprintf("%s:%x-%s", repoName, build.ProjectID.Bytes, shortCommit)

	// If registry is configured, prefix with registry
	if d.registry != "" {
		tag = fmt.Sprintf("%s/%s", d.registry, tag)
	}

	return tag
}

// buildDockerImage builds the Docker image using Docker Go SDK
func (d *DockerBuilder) buildDockerImage(ctx context.Context, buildDir, imageTag string) (string, error) {
	d.logger.Debug("Building Docker image", "tag", imageTag, "dir", buildDir)

	// Check if Dockerfile exists
	dockerfilePath := filepath.Join(buildDir, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		return "", fmt.Errorf("dockerfile not found in repository")
	}

	// Create a tar archive of the build context
	buildContext, err := archive.TarWithOptions(buildDir, &archive.TarOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create build context: %w", err)
	}
	defer buildContext.Close()

	// Build the image
	buildOptions := build.ImageBuildOptions{
		Tags:        []string{imageTag},
		Dockerfile:  "Dockerfile",
		Remove:      true,
		ForceRemove: true,
	}

	buildResponse, err := d.dockerClient.ImageBuild(ctx, buildContext, buildOptions)
	if err != nil {
		return "", fmt.Errorf("docker build failed: %w", err)
	}
	defer buildResponse.Body.Close()

	// Read the build output
	buildLog, err := io.ReadAll(buildResponse.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read build output: %w", err)
	}

	d.logger.Debug("Docker image built successfully", "tag", imageTag)
	return string(buildLog), nil
}

// pushDockerImage pushes the Docker image to the registry using Docker Go SDK
func (d *DockerBuilder) pushDockerImage(ctx context.Context, imageTag string) (string, error) {
	d.logger.Debug("Pushing Docker image", "tag", imageTag)

	// Push the image
	pushOptions := image.PushOptions{
		// For authentication, you would set RegistryAuth here
		// This would require proper registry credentials
	}

	pushResponse, err := d.dockerClient.ImagePush(ctx, imageTag, pushOptions)
	if err != nil {
		return "", fmt.Errorf("docker push failed: %w", err)
	}
	defer pushResponse.Close()

	// Read the push output
	pushLog, err := io.ReadAll(pushResponse)
	if err != nil {
		return "", fmt.Errorf("failed to read push output: %w", err)
	}

	d.logger.Debug("Docker image pushed successfully", "tag", imageTag)
	return string(pushLog), nil
}

// Cleanup performs cleanup operations for the Docker builder
func (d *DockerBuilder) Cleanup() error {
	// Close the Docker client
	if d.dockerClient != nil {
		if err := d.dockerClient.Close(); err != nil {
			d.logger.Warn("Failed to close Docker client", "error", err)
			return err
		}
	}

	// Clean up any dangling images or build cache if needed
	// For now, we rely on Docker's built-in garbage collection
	d.logger.Debug("Docker builder cleanup completed")
	return nil
}
