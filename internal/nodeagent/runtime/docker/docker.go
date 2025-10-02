package docker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/zeitwork/zeitwork/internal/nodeagent/runtime"
)

const labelInstanceID = "zeitwork.instance.id"

// DockerRuntime implements the Runtime interface using Docker
type DockerRuntime struct {
	client *client.Client
	logger *slog.Logger
}

// NewDockerRuntime creates a new Docker runtime
func NewDockerRuntime(logger *slog.Logger) (*DockerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerRuntime{
		client: cli,
		logger: logger,
	}, nil
}

// Start creates and starts a container
func (d *DockerRuntime) Start(ctx context.Context, instanceID, imageName, ipAddress string, vcpus, memory, port int, envVars map[string]string) error {
	d.logger.Info("starting container",
		"instance_id", instanceID,
		"image", imageName,
		"ip", ipAddress,
	)

	// Pull image
	// For MVP, we assume the image is already pulled or available
	// In production, you'd want to pull it here

	// Convert env vars to array
	env := make([]string, 0, len(envVars))
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create container config
	config := &container.Config{
		Image: imageName,
		Env:   env,
		Labels: map[string]string{
			labelInstanceID: instanceID,
		},
		ExposedPorts: nat.PortSet{
			nat.Port(fmt.Sprintf("%d/tcp", port)): struct{}{},
		},
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
		Resources: container.Resources{
			NanoCPUs: int64(vcpus * 1e9),
			Memory:   int64(memory * 1024 * 1024), // memory in MB
		},
	}

	// Create container
	resp, err := d.client.ContainerCreate(ctx, config, hostConfig, nil, nil, fmt.Sprintf("zeitwork-%s", instanceID))
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	if err := d.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	d.logger.Info("container started",
		"instance_id", instanceID,
		"container_id", resp.ID,
	)

	return nil
}

// Stop stops and removes a container
func (d *DockerRuntime) Stop(ctx context.Context, instanceID string) error {
	d.logger.Info("stopping container", "instance_id", instanceID)

	// Find container by label
	containers, err := d.client.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", labelInstanceID, instanceID)),
		),
	})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		d.logger.Warn("container not found", "instance_id", instanceID)
		return nil
	}

	// Stop and remove container
	for _, c := range containers {
		if err := d.client.ContainerStop(ctx, c.ID, container.StopOptions{}); err != nil {
			d.logger.Error("failed to stop container", "error", err, "container_id", c.ID)
		}

		if err := d.client.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
			d.logger.Error("failed to remove container", "error", err, "container_id", c.ID)
		}
	}

	d.logger.Info("container stopped", "instance_id", instanceID)
	return nil
}

// List returns all running containers managed by this runtime
func (d *DockerRuntime) List(ctx context.Context) ([]runtime.Container, error) {
	containers, err := d.client.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", labelInstanceID),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]runtime.Container, 0, len(containers))
	for _, c := range containers {
		instanceID := c.Labels[labelInstanceID]
		result = append(result, runtime.Container{
			ID:         c.ID,
			InstanceID: instanceID,
			ImageName:  c.Image,
			State:      c.State,
		})
	}

	return result, nil
}

// GetStatus returns the status of a specific container
func (d *DockerRuntime) GetStatus(ctx context.Context, instanceID string) (*runtime.Container, error) {
	containers, err := d.client.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", labelInstanceID, instanceID)),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		return nil, nil
	}

	c := containers[0]
	return &runtime.Container{
		ID:         c.ID,
		InstanceID: c.Labels[labelInstanceID],
		ImageName:  c.Image,
		State:      c.State,
	}, nil
}

// Close cleans up the runtime
func (d *DockerRuntime) Close() error {
	return d.client.Close()
}
