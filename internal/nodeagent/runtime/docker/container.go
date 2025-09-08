package docker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	runtimeTypes "github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// buildContainerConfig creates Docker container configuration
func (d *DockerRuntime) buildContainerConfig(spec *runtimeTypes.InstanceSpec) *container.Config {
	// Convert environment variables
	env := make([]string, 0, len(spec.EnvironmentVariables))
	for key, value := range spec.EnvironmentVariables {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	// Prepare port exposures
	exposedPorts := make(nat.PortSet)
	if spec.NetworkConfig != nil {
		// Expose default port
		if spec.NetworkConfig.DefaultPort > 0 {
			port := nat.Port(fmt.Sprintf("%d/tcp", spec.NetworkConfig.DefaultPort))
			exposedPorts[port] = struct{}{}
		}

		// Expose additional ports
		for internalPort := range spec.NetworkConfig.PortMappings {
			port := nat.Port(fmt.Sprintf("%d/tcp", internalPort))
			exposedPorts[port] = struct{}{}
		}
	}

	config := &container.Config{
		Image:        spec.ImageTag,
		Env:          env,
		ExposedPorts: exposedPorts,
		Labels: map[string]string{
			"zeitwork.instance.id": spec.ID,
			"zeitwork.image.id":    spec.ImageID,
			"zeitwork.managed":     "true",
			"zeitwork.runtime":     "docker",
		},
		// Add health check if needed
		Healthcheck: &container.HealthConfig{
			Test:     []string{"CMD-SHELL", "exit 0"}, // Basic health check
			Interval: 30000000000,                     // 30 seconds
			Timeout:  10000000000,                     // 10 seconds
			Retries:  3,
		},
	}

	return config
}

// buildHostConfig creates Docker host configuration
func (d *DockerRuntime) buildHostConfig(spec *runtimeTypes.InstanceSpec) *container.HostConfig {
	hostConfig := &container.HostConfig{
		// RestartPolicy: container.RestartPolicy{
		// 	// Name: "unless-stopped",
		// 	// MaximumRetryCount can only be used with "on-failure" restart policy
		// },
		NetworkMode: container.NetworkMode(d.config.NetworkName),
	}

	// Set resource limits
	if spec.Resources != nil {
		// CPU limits
		if spec.Resources.CPULimit > 0 {
			// Docker uses nano CPUs (1 CPU = 1e9 nano CPUs)
			hostConfig.NanoCPUs = int64(spec.Resources.CPULimit * 1e9)
		}
		if spec.Resources.VCPUs > 0 {
			hostConfig.CPUCount = int64(spec.Resources.VCPUs)
		}

		// Memory limits
		if spec.Resources.MemoryLimit > 0 {
			hostConfig.Memory = spec.Resources.MemoryLimit
		} else if spec.Resources.Memory > 0 {
			hostConfig.Memory = int64(spec.Resources.Memory) * 1024 * 1024 // Convert MB to bytes
		}
	}

	// No port mappings needed - edge proxy connects directly to container IP addresses

	// Volume mounts
	if spec.VolumeConfig != nil {
		mounts := make([]mount.Mount, 0, len(spec.VolumeConfig.Mounts))
		for _, volumeMount := range spec.VolumeConfig.Mounts {
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   volumeMount.Source,
				Target:   volumeMount.Destination,
				ReadOnly: volumeMount.ReadOnly,
			})
		}
		hostConfig.Mounts = mounts
	}

	// Security settings
	hostConfig.SecurityOpt = []string{
		"no-new-privileges:true",
	}

	return hostConfig
}

// buildNetworkConfig creates Docker network configuration
func (d *DockerRuntime) buildNetworkConfig(spec *runtimeTypes.InstanceSpec) *network.NetworkingConfig {
	if spec.NetworkConfig == nil {
		return nil
	}

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: make(map[string]*network.EndpointSettings),
	}

	endpointConfig := &network.EndpointSettings{}

	// For development: Use Docker's automatic IPv4 assignment (172.20.x.x)
	// The edge proxy will connect directly to these IPv4 addresses
	// TODO: For production, add support for static IP assignment if needed

	networkConfig.EndpointsConfig[d.config.NetworkName] = endpointConfig

	return networkConfig
}

// containerToInstance converts a Docker container to a runtime instance
func (d *DockerRuntime) containerToInstance(ctx context.Context, container container.Summary) (*runtimeTypes.Instance, error) {
	// Get instance ID from labels
	instanceID, ok := container.Labels["zeitwork.instance.id"]
	if !ok {
		return nil, fmt.Errorf("container missing zeitwork.instance.id label")
	}

	imageID, ok := container.Labels["zeitwork.image.id"]
	if !ok {
		return nil, fmt.Errorf("container missing zeitwork.image.id label")
	}

	// Get detailed container info
	containerJSON, err := d.client.ContainerInspect(ctx, container.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	// Extract environment variables
	envVars := make(map[string]string)
	for _, env := range containerJSON.Config.Env {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}

	// Extract resource information
	resources := &runtimeTypes.ResourceSpec{
		VCPUs:       int32(containerJSON.HostConfig.CPUCount),
		Memory:      int32(containerJSON.HostConfig.Memory / (1024 * 1024)), // Convert bytes to MB
		CPULimit:    float64(containerJSON.HostConfig.NanoCPUs) / 1e9,       // Convert nano CPUs to CPU count
		MemoryLimit: containerJSON.HostConfig.Memory,
	}

	// Get network info
	networkInfo := d.extractNetworkInfo(containerJSON)

	instance := &runtimeTypes.Instance{
		ID:          instanceID,
		ImageID:     imageID,
		ImageTag:    container.Image,
		State:       d.mapContainerState(containerJSON.State),
		Resources:   resources,
		EnvVars:     envVars,
		NetworkInfo: networkInfo,
		CreatedAt:   time.Now(), // TODO: Parse containerJSON.Created properly
		RuntimeID:   container.ID,
	}

	if containerJSON.State.StartedAt != "" {
		startedAt := containerJSON.State.StartedAt
		// Parse started time - simplified version
		if startedAt != "" {
			// In a real implementation, you'd parse the Docker timestamp format
			// For now, just use current time as placeholder
			now := time.Now()
			instance.StartedAt = &now
		}
	}

	return instance, nil
}

// extractNetworkInfo extracts network information from container inspect response
func (d *DockerRuntime) extractNetworkInfo(containerJSON container.InspectResponse) *runtimeTypes.NetworkInfo {
	networkInfo := &runtimeTypes.NetworkInfo{
		PortMappings: make(map[int32]int32),
	}

	// Get network settings
	if containerJSON.NetworkSettings != nil {
		// Get IPv4 address from the zeitwork network
		for networkName, network := range containerJSON.NetworkSettings.Networks {
			if network.IPAddress != "" {
				networkInfo.IPAddress = network.IPAddress
				networkInfo.NetworkID = networkName
				break // Use the first available IPv4 address
			}
		}

		// Fallback to default IP address if no network-specific IP found
		if networkInfo.IPAddress == "" && containerJSON.NetworkSettings.IPAddress != "" {
			networkInfo.IPAddress = containerJSON.NetworkSettings.IPAddress
		}

		// Extract port mappings
		for containerPort, hostPorts := range containerJSON.NetworkSettings.Ports {
			if len(hostPorts) > 0 {
				// Parse container port
				portStr := strings.TrimSuffix(string(containerPort), "/tcp")
				if internalPort, err := strconv.Atoi(portStr); err == nil {
					// Parse host port
					if externalPort, err := strconv.Atoi(hostPorts[0].HostPort); err == nil {
						networkInfo.PortMappings[int32(internalPort)] = int32(externalPort)
					}
				}
			}
		}
	}

	return networkInfo
}

// getNetworkInfo gets network information for a running container
func (d *DockerRuntime) getNetworkInfo(ctx context.Context, containerID string) (*runtimeTypes.NetworkInfo, error) {
	containerJSON, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container for network info: %w", err)
	}

	return d.extractNetworkInfo(containerJSON), nil
}
