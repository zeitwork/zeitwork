package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/image"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/moby/go-archive"
	"github.com/zeitwork/zeitwork/internal/database"
)

// createBuildDirectory creates a unique build directory and returns cleanup function
func (d *DockerBuildRuntime) createBuildDirectory(build *database.ImageBuild) (string, func(), error) {
	buildDir := filepath.Join(d.config.WorkDir, fmt.Sprintf("build-%x-%d", build.ID.Bytes, time.Now().Unix()))
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return "", nil, err
	}

	cleanup := func() {
		if err := os.RemoveAll(buildDir); err != nil {
			d.logger.Warn("Failed to cleanup build directory", "dir", buildDir, "error", err)
		}
	}

	return buildDir, cleanup, nil
}

// cloneRepository clones the Git repository to the build directory using go-git
func (d *DockerBuildRuntime) cloneRepository(ctx context.Context, build *database.ImageBuild, buildDir string) error {
	repoURL := fmt.Sprintf("https://github.com/%s.git", build.GithubRepository)

	d.logger.Debug("Cloning repository", "url", repoURL, "commit", build.GithubCommit)

	// Clone the repository
	repo, err := git.PlainCloneContext(ctx, buildDir, false, &git.CloneOptions{
		URL:      repoURL,
		Progress: nil, // Could add progress logging here if needed
		Auth:     nil, // TODO: For public repos, no auth needed. For private repos, would need token
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
	commitHash := plumbing.NewHash(build.GithubCommit)
	err = worktree.Checkout(&git.CheckoutOptions{
		Hash: commitHash,
	})
	if err != nil {
		return fmt.Errorf("git checkout failed: %w", err)
	}

	d.logger.Debug("Repository cloned and checked out successfully", "commit", build.GithubCommit)
	return nil
}

// generateImageTag generates a unique image tag for the build
func (d *DockerBuildRuntime) generateImageTag(build *database.ImageBuild) string {
	// Extract repository name from github_repository (owner/repo -> repo)
	repoParts := strings.Split(build.GithubRepository, "/")
	repoName := repoParts[len(repoParts)-1]

	// Create tag with short commit hash
	shortCommit := build.GithubCommit
	if len(shortCommit) > 7 {
		shortCommit = shortCommit[:7]
	}

	tag := fmt.Sprintf("%s:%s", repoName, shortCommit)

	// Use distribution registry from docker-compose (localhost:5001)
	// This overrides the configured registry to ensure we use the distribution registry
	distributionRegistry := "localhost:5001"
	tag = fmt.Sprintf("%s/%s", distributionRegistry, tag)

	return tag
}

// buildDockerImage builds the Docker image using Docker Go SDK
func (d *DockerBuildRuntime) buildDockerImage(ctx context.Context, buildDir, imageTag string) (string, error) {
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

// ImageInfo holds information about a Docker image
type ImageInfo struct {
	Hash string
	Size int64
}

// getImageInfo retrieves information about a Docker image
func (d *DockerBuildRuntime) getImageInfo(ctx context.Context, imageTag string) (*ImageInfo, error) {
	d.logger.Debug("Getting image info", "tag", imageTag)

	// Inspect the image to get its information
	imageInspect, err := d.dockerClient.ImageInspect(ctx, imageTag)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect image: %w", err)
	}

	// Get the image ID (which is the SHA256 hash)
	imageHash := strings.TrimPrefix(imageInspect.ID, "sha256:")

	// Get the image size
	imageSize := imageInspect.Size

	d.logger.Debug("Retrieved image info",
		"tag", imageTag,
		"hash", imageHash[:12], // Log first 12 chars of hash
		"size", imageSize)

	return &ImageInfo{
		Hash: imageHash,
		Size: imageSize,
	}, nil
}

// pushDockerImage pushes the Docker image to the registry using Docker Go SDK
func (d *DockerBuildRuntime) pushDockerImage(ctx context.Context, imageTag string) (string, error) {
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
