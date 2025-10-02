package docker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/zeitwork/zeitwork/internal/nodeagent/runtime"
)

const (
	labelInstanceID = "zeitwork.instance.id"
	networkName     = "zeitwork-br0" // Tailscale regional bridge network
)

// Config holds configuration for Docker runtime
type Config struct {
	RegistryURL  string // Registry URL (e.g., "ghcr.io/yourorg")
	RegistryUser string // Registry username
	RegistryPass string // Registry password or token
}

// DockerRuntime implements the Runtime interface using Docker
type DockerRuntime struct {
	client *client.Client
	logger *slog.Logger
	cfg    Config
}

// NewDockerRuntime creates a new Docker runtime
func NewDockerRuntime(cfg Config, logger *slog.Logger) (*DockerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	dr := &DockerRuntime{
		client: cli,
		logger: logger,
		cfg:    cfg,
	}

	// Authenticate to registry if configured
	if cfg.RegistryURL != "" {
		logger.Info("registry configured, authenticating",
			"registry_url", cfg.RegistryURL,
			"registry_user", cfg.RegistryUser,
		)
		if err := dr.dockerLogin(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to authenticate to registry: %w", err)
		}
		logger.Info("successfully authenticated to registry")
	} else {
		logger.Info("no registry configured, using local Docker only")
	}

	return dr, nil
}

// dockerLogin authenticates to the configured registry
func (d *DockerRuntime) dockerLogin(ctx context.Context) error {
	if d.cfg.RegistryURL == "" {
		return nil // No registry configured
	}

	if d.cfg.RegistryUser == "" || d.cfg.RegistryPass == "" {
		return fmt.Errorf("registry URL configured but missing credentials")
	}

	// Extract registry host from URL (e.g., "ghcr.io/yourorg" -> "ghcr.io")
	registryHost := d.cfg.RegistryURL
	if strings.Contains(registryHost, "/") {
		registryHost = strings.Split(registryHost, "/")[0]
	}

	d.logger.Info("[REGISTRY] logging in to registry",
		"registry_host", registryHost,
		"username", d.cfg.RegistryUser,
	)

	cmd := exec.CommandContext(ctx, "docker", "login", registryHost,
		"--username", d.cfg.RegistryUser,
		"--password-stdin")

	// Pass password via stdin for security
	cmd.Stdin = strings.NewReader(d.cfg.RegistryPass)
	output, err := cmd.CombinedOutput()

	if err != nil {
		d.logger.Error("[REGISTRY] docker login failed",
			"registry_host", registryHost,
			"error", err,
			"output", string(output),
		)
		return fmt.Errorf("docker login failed: %w: %s", err, string(output))
	}

	d.logger.Info("[REGISTRY] successfully logged in to registry",
		"registry_host", registryHost,
	)

	return nil
}

// Start creates and starts a container
func (d *DockerRuntime) Start(ctx context.Context, instanceID, imageName, ipAddress string, vcpus, memory, port int, envVars map[string]string) error {
	d.logger.Info("starting container",
		"instance_id", instanceID,
		"image", imageName,
		"ip", ipAddress,
		"network", networkName,
	)

	// Pull image if not already available
	d.logger.Info("[REGISTRY] pulling image",
		"instance_id", instanceID,
		"image", imageName,
	)

	pullOptions := image.PullOptions{}
	pullReader, err := d.client.ImagePull(ctx, imageName, pullOptions)
	if err != nil {
		d.logger.Error("[REGISTRY] failed to pull image",
			"instance_id", instanceID,
			"image", imageName,
			"error", err,
		)
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer pullReader.Close()

	// Consume pull output (required for pull to complete)
	_, err = io.Copy(io.Discard, pullReader)
	if err != nil {
		d.logger.Warn("[REGISTRY] failed to read pull output",
			"instance_id", instanceID,
			"error", err,
		)
	}

	d.logger.Info("[REGISTRY] image pulled successfully",
		"instance_id", instanceID,
		"image", imageName,
	)

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

	// Verify the regional bridge network exists
	networks, err := d.client.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	networkExists := false
	for _, net := range networks {
		if net.Name == networkName {
			networkExists = true
			break
		}
	}

	if !networkExists {
		d.logger.Error("regional bridge network not found",
			"network_name", networkName,
			"instance_id", instanceID,
		)
		return fmt.Errorf("regional bridge network '%s' not found - ensure Tailscale setup script has been run", networkName)
	}

	// Configure network settings with static IP
	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkName: {
				IPAMConfig: &network.EndpointIPAMConfig{
					IPv4Address: ipAddress,
				},
			},
		},
	}

	// Create container
	resp, err := d.client.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, fmt.Sprintf("zeitwork-%s", instanceID))
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	d.logger.Info("container created with network config",
		"instance_id", instanceID,
		"container_id", resp.ID,
		"ip_address", ipAddress,
		"network", networkName,
	)

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

		// Extract IP address from network settings
		ipAddress := ""
		if networkSettings, ok := c.NetworkSettings.Networks[networkName]; ok {
			ipAddress = networkSettings.IPAddress
		}

		result = append(result, runtime.Container{
			ID:         c.ID,
			InstanceID: instanceID,
			ImageName:  c.Image,
			State:      c.State,
			IPAddress:  ipAddress,
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

	// Extract IP address from network settings
	ipAddress := ""
	if networkSettings, ok := c.NetworkSettings.Networks[networkName]; ok {
		ipAddress = networkSettings.IPAddress
	}

	return &runtime.Container{
		ID:         c.ID,
		InstanceID: c.Labels[labelInstanceID],
		ImageName:  c.Image,
		State:      c.State,
		IPAddress:  ipAddress,
	}, nil
}

// Close cleans up the runtime
func (d *DockerRuntime) Close() error {
	return d.client.Close()
}
