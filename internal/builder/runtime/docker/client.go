package docker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/docker/docker/client"
	"github.com/zeitwork/zeitwork/internal/builder/config"
	"github.com/zeitwork/zeitwork/internal/builder/types"
)

// DockerBuildRuntime implements BuildRuntime using direct Docker builds
// This provides fast builds for local development
type DockerBuildRuntime struct {
	logger       *slog.Logger
	config       config.DockerRuntimeConfig
	dockerClient *client.Client
}

// NewDockerBuildRuntime creates a new Docker build runtime
func NewDockerBuildRuntime(cfg config.DockerRuntimeConfig, logger *slog.Logger) (*DockerBuildRuntime, error) {
	if cfg.WorkDir == "" {
		cfg.WorkDir = "/tmp/zeitwork-builds"
	}

	// Ensure work directory exists
	if err := os.MkdirAll(cfg.WorkDir, 0755); err != nil {
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

	return &DockerBuildRuntime{
		logger:       logger,
		config:       cfg,
		dockerClient: dockerClient,
	}, nil
}

// Name returns the name of the Docker build runtime
func (d *DockerBuildRuntime) Name() string {
	return "docker"
}

// Build executes a build using direct Docker on the host
func (d *DockerBuildRuntime) Build(ctx context.Context, build *types.EnrichedBuild) *types.BuildResult {
	startTime := time.Now()

	result := &types.BuildResult{
		Success: false,
	}

	d.logger.Info("Starting Docker build",
		"build_id", build.ID,
		"repo", build.GithubRepository,
		"commit", build.CommitHash,
		"branch", build.DefaultBranch)

	// Create build directory
	buildDir, cleanup, err := d.createBuildDirectory(build)
	if err != nil {
		result.Error = fmt.Errorf("failed to create build directory: %w", err)
		result.BuildLog = fmt.Sprintf("Error: %v", result.Error)
		result.Duration = time.Since(startTime)
		return result
	}
	defer cleanup()

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

	// Get image information after successful build
	imageInfo, err := d.getImageInfo(ctx, imageTag)
	if err != nil {
		result.Error = fmt.Errorf("failed to get image info: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	result.ImageHash = imageInfo.Hash
	result.ImageSize = imageInfo.Size

	// Push the image to registry
	pushLog, err := d.pushDockerImage(ctx, imageTag)
	result.BuildLog += "\n" + pushLog

	if err != nil {
		result.Error = fmt.Errorf("docker push failed: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	result.Success = true
	result.Duration = time.Since(startTime)

	d.logger.Info("Docker build completed successfully",
		"build_id", build.ID,
		"image_tag", imageTag,
		"duration", result.Duration)

	return result
}

// Cleanup performs cleanup operations for the Docker build runtime
func (d *DockerBuildRuntime) Cleanup() error {
	// Close the Docker client
	if d.dockerClient != nil {
		if err := d.dockerClient.Close(); err != nil {
			d.logger.Warn("Failed to close Docker client", "error", err)
			return err
		}
	}

	// Clean up any dangling images or build cache if needed
	// For now, we rely on Docker's built-in garbage collection
	d.logger.Debug("Docker build runtime cleanup completed")
	return nil
}
