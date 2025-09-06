package docker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
	runtimeTypes "github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// DockerRuntime implements the Runtime interface using Docker
type DockerRuntime struct {
	client *client.Client
	config *config.DockerRuntimeConfig
	logger *slog.Logger
}

// NewDockerRuntime creates a new Docker runtime
func NewDockerRuntime(cfg *config.DockerRuntimeConfig, logger *slog.Logger) (*DockerRuntime, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost(cfg.Endpoint),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker daemon: %w", err)
	}

	return &DockerRuntime{
		client: cli,
		config: cfg,
		logger: logger,
	}, nil
}

// CreateInstance creates a new Docker container
func (d *DockerRuntime) CreateInstance(ctx context.Context, spec *runtimeTypes.InstanceSpec) (*runtimeTypes.Instance, error) {
	d.logger.Info("Creating Docker container", "instance_id", spec.ID, "image", spec.ImageTag)

	// Ensure image is available
	if err := d.ensureImage(ctx, spec.ImageTag); err != nil {
		return nil, fmt.Errorf("failed to ensure image: %w", err)
	}

	// Prepare container configuration
	containerConfig := d.buildContainerConfig(spec)
	hostConfig := d.buildHostConfig(spec)
	networkConfig := d.buildNetworkConfig(spec)

	// Create container
	containerName := d.getContainerName(spec.ID)
	resp, err := d.client.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		networkConfig,
		nil,
		containerName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Create instance object
	instance := &runtimeTypes.Instance{
		ID:        spec.ID,
		ImageID:   spec.ImageID,
		ImageTag:  spec.ImageTag,
		State:     runtimeTypes.InstanceStateCreating,
		Resources: spec.Resources,
		EnvVars:   spec.EnvironmentVariables,
		CreatedAt: time.Now(),
		RuntimeID: resp.ID,
	}

	d.logger.Info("Docker container created",
		"instance_id", spec.ID,
		"container_id", resp.ID[:12])

	return instance, nil
}

// StartInstance starts a Docker container
func (d *DockerRuntime) StartInstance(ctx context.Context, instance *runtimeTypes.Instance) error {
	d.logger.Info("Starting Docker container", "instance_id", instance.ID, "container_id", instance.RuntimeID[:12])

	startCtx, cancel := context.WithTimeout(ctx, d.config.StartTimeout)
	defer cancel()

	if err := d.client.ContainerStart(startCtx, instance.RuntimeID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Update instance state
	instance.State = runtimeTypes.InstanceStateRunning
	now := time.Now()
	instance.StartedAt = &now

	// Get network information
	networkInfo, err := d.getNetworkInfo(ctx, instance.RuntimeID)
	if err != nil {
		d.logger.Warn("Failed to get network info", "instance_id", instance.ID, "error", err)
	} else {
		instance.NetworkInfo = networkInfo
	}

	d.logger.Info("Docker container started", "instance_id", instance.ID)
	return nil
}

// StopInstance stops a Docker container
func (d *DockerRuntime) StopInstance(ctx context.Context, instance *runtimeTypes.Instance) error {
	d.logger.Info("Stopping Docker container", "instance_id", instance.ID, "container_id", instance.RuntimeID[:12])

	stopCtx, cancel := context.WithTimeout(ctx, d.config.StopTimeout)
	defer cancel()

	if err := d.client.ContainerStop(stopCtx, instance.RuntimeID, container.StopOptions{}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	instance.State = runtimeTypes.InstanceStateStopped
	d.logger.Info("Docker container stopped", "instance_id", instance.ID)
	return nil
}

// DeleteInstance removes a Docker container
func (d *DockerRuntime) DeleteInstance(ctx context.Context, instance *runtimeTypes.Instance) error {
	d.logger.Info("Deleting Docker container", "instance_id", instance.ID, "container_id", instance.RuntimeID[:12])

	// Stop container if running
	if instance.State == runtimeTypes.InstanceStateRunning {
		if err := d.StopInstance(ctx, instance); err != nil {
			d.logger.Warn("Failed to stop container before deletion", "instance_id", instance.ID, "error", err)
		}
	}

	// Remove container
	if err := d.client.ContainerRemove(ctx, instance.RuntimeID, container.RemoveOptions{
		Force: true,
	}); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	instance.State = runtimeTypes.InstanceStateTerminated
	d.logger.Info("Docker container deleted", "instance_id", instance.ID)
	return nil
}

// GetInstanceState gets the current state of a Docker container
func (d *DockerRuntime) GetInstanceState(ctx context.Context, instance *runtimeTypes.Instance) (runtimeTypes.InstanceState, error) {
	containerJSON, err := d.client.ContainerInspect(ctx, instance.RuntimeID)
	if err != nil {
		if client.IsErrNotFound(err) {
			return runtimeTypes.InstanceStateTerminated, nil
		}
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	state := d.mapContainerState(containerJSON.State)
	instance.State = state
	return state, nil
}

// IsInstanceRunning checks if a Docker container is running
func (d *DockerRuntime) IsInstanceRunning(ctx context.Context, instance *runtimeTypes.Instance) (bool, error) {
	state, err := d.GetInstanceState(ctx, instance)
	if err != nil {
		return false, err
	}
	return state == runtimeTypes.InstanceStateRunning, nil
}

// ListInstances lists all Docker containers managed by this node agent
func (d *DockerRuntime) ListInstances(ctx context.Context) ([]*runtimeTypes.Instance, error) {
	containers, err := d.client.ContainerList(ctx, container.ListOptions{
		All: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var instances []*runtimeTypes.Instance
	for _, container := range containers {
		// Only include containers managed by zeitwork
		if !d.isZeitworkContainer(container) {
			continue
		}

		instance, err := d.containerToInstance(ctx, container)
		if err != nil {
			d.logger.Warn("Failed to convert container to instance",
				"container_id", container.ID[:12], "error", err)
			continue
		}

		instances = append(instances, instance)
	}

	return instances, nil
}

// GetRuntimeInfo returns information about the Docker runtime
func (d *DockerRuntime) GetRuntimeInfo() *runtimeTypes.RuntimeInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	version, err := d.client.ServerVersion(ctx)
	if err != nil {
		return &runtimeTypes.RuntimeInfo{
			Type:    "docker",
			Version: "unknown",
			Status:  "error",
		}
	}

	return &runtimeTypes.RuntimeInfo{
		Type:    "docker",
		Version: version.Version,
		Status:  "healthy",
	}
}

// Helper methods

// ensureImage ensures the Docker image is available locally
func (d *DockerRuntime) ensureImage(ctx context.Context, imageTag string) error {
	// Check if image exists locally
	images, err := d.client.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list images: %w", err)
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == imageTag {
				return nil // Image exists
			}
		}
	}

	// Pull image
	d.logger.Info("Pulling Docker image", "image", imageTag)
	pullCtx, cancel := context.WithTimeout(ctx, d.config.PullTimeout)
	defer cancel()

	reader, err := d.client.ImagePull(pullCtx, imageTag, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	// Wait for pull to complete
	// Note: In production, you might want to stream and log the pull progress
	// For now, just read and discard the output
	_, err = io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read pull response: %w", err)
	}

	d.logger.Info("Docker image pulled successfully", "image", imageTag)
	return nil
}

// getContainerName generates a container name for an instance
func (d *DockerRuntime) getContainerName(instanceID string) string {
	return fmt.Sprintf("zeitwork-%s", instanceID)
}

// isZeitworkContainer checks if a container is managed by zeitwork
func (d *DockerRuntime) isZeitworkContainer(container types.Container) bool {
	for _, name := range container.Names {
		if strings.HasPrefix(name, "/zeitwork-") {
			return true
		}
	}
	return false
}

// mapContainerState maps Docker container state to runtime state
func (d *DockerRuntime) mapContainerState(state *types.ContainerState) runtimeTypes.InstanceState {
	if state.Running {
		return runtimeTypes.InstanceStateRunning
	}
	if state.Paused {
		return runtimeTypes.InstanceStateStopped
	}
	if state.Restarting {
		return runtimeTypes.InstanceStateStarting
	}
	if state.Dead || state.OOMKilled {
		return runtimeTypes.InstanceStateFailed
	}
	if state.ExitCode != 0 {
		return runtimeTypes.InstanceStateFailed
	}
	return runtimeTypes.InstanceStateStopped
}
