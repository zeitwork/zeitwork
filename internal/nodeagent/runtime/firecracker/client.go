package firecracker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// FirecrackerRuntime implements the Runtime interface using firecracker-containerd
type FirecrackerRuntime struct {
	config *config.FirecrackerRuntimeConfig
	logger *slog.Logger
	client *containerd.Client
}

// NewFirecrackerRuntime creates a new Firecracker runtime using firecracker-containerd
func NewFirecrackerRuntime(cfg *config.FirecrackerRuntimeConfig, logger *slog.Logger) (*FirecrackerRuntime, error) {
	logger.Info("Initializing firecracker-containerd runtime")

	// Setup and validate firecracker-containerd environment if auto-setup is enabled
	if cfg.EnableAutoSetup {
		logger.Info("Auto-setup enabled, ensuring firecracker-containerd environment")
		setupManager := NewSetupManager(cfg, logger)
		setupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		if err := setupManager.EnsureSetup(setupCtx); err != nil {
			return nil, fmt.Errorf("failed to setup firecracker-containerd environment: %w", err)
		}
	} else {
		logger.Info("Auto-setup disabled, assuming firecracker-containerd is already configured")
	}

	// Connect to firecracker-containerd
	client, err := containerd.New(cfg.ContainerdSocket)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to firecracker-containerd: %w", err)
	}

	// Test connection by pinging
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := client.Version(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping firecracker-containerd: %w", err)
	}

	runtime := &FirecrackerRuntime{
		config: cfg,
		logger: logger,
		client: client,
	}

	logger.Info("firecracker-containerd runtime initialized successfully")
	return runtime, nil
}

// CreateInstance creates a new Firecracker microVM container
func (f *FirecrackerRuntime) CreateInstance(ctx context.Context, spec *types.InstanceSpec) (*types.Instance, error) {
	f.logger.Info("Creating Firecracker container", "instance_id", spec.ID, "image", spec.ImageTag)

	// Create containerd namespace context
	ctx = namespaces.WithNamespace(ctx, f.config.ContainerdNamespace)

	// Pull image if needed
	image, err := f.ensureImage(ctx, spec.ImageTag)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure image: %w", err)
	}

	// Create container spec with Firecracker runtime using standard containerd approach
	containerOpts := []containerd.NewContainerOpts{
		containerd.WithImage(image),
		containerd.WithRuntime("aws.firecracker", nil), // Use firecracker-containerd runtime
		containerd.WithSnapshotter("devmapper"),        // Use devmapper snapshotter (required by firecracker-containerd)
		containerd.WithNewSnapshot(spec.ID, image),     // Create snapshot from image
		containerd.WithNewSpec(f.buildOCISpecFromImage(spec, image)...),
		containerd.WithContainerLabels(map[string]string{
			"zeitwork.instance.id": spec.ID,
			"zeitwork.image.id":    spec.ImageID,
			"zeitwork.managed":     "true",
		}),
	}

	// Create the container (but don't start it yet)
	container, err := f.client.NewContainer(ctx, spec.ID, containerOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Get container info
	info, err := container.Info(ctx)
	if err != nil {
		container.Delete(ctx)
		return nil, fmt.Errorf("failed to get container info: %w", err)
	}

	// Create instance object
	instance := &types.Instance{
		ID:        spec.ID,
		ImageID:   spec.ImageID,
		ImageTag:  spec.ImageTag,
		State:     types.InstanceStateCreating,
		Resources: spec.Resources,
		EnvVars:   spec.EnvironmentVariables,
		NetworkInfo: &types.NetworkInfo{
			// Network info will be populated when VM starts
			DefaultPort:  spec.NetworkConfig.DefaultPort,
			PortMappings: make(map[int32]int32),
		},
		CreatedAt: info.CreatedAt,
		RuntimeID: spec.ID, // Container ID is the same as instance ID
	}

	f.logger.Info("Firecracker container created successfully", "instance_id", spec.ID)
	return instance, nil
}

// StartInstance starts a Firecracker microVM container
func (f *FirecrackerRuntime) StartInstance(ctx context.Context, instance *types.Instance) error {
	f.logger.Info("Starting Firecracker container", "instance_id", instance.ID)

	ctx = namespaces.WithNamespace(ctx, f.config.ContainerdNamespace)

	// Get the container
	container, err := f.client.LoadContainer(ctx, instance.ID)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	// Create task (this starts the Firecracker VM)
	task, err := container.NewTask(ctx, cio.NullIO)
	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	// Start the task
	if err := task.Start(ctx); err != nil {
		task.Delete(ctx)
		return fmt.Errorf("failed to start task: %w", err)
	}

	// Wait for VM to be ready (firecracker-containerd handles VM startup)
	if err := f.waitForTaskReady(ctx, task); err != nil {
		task.Kill(ctx, 9)
		task.Delete(ctx)
		return fmt.Errorf("task failed to become ready: %w", err)
	}

	// Update instance state
	instance.State = types.InstanceStateRunning
	now := time.Now()
	instance.StartedAt = &now

	// Update network info if available
	if networkInfo, err := f.getContainerNetworkInfo(ctx, container); err == nil {
		instance.NetworkInfo = networkInfo
	}

	f.logger.Info("Firecracker container started successfully", "instance_id", instance.ID)
	return nil
}

// StopInstance stops a Firecracker microVM container
func (f *FirecrackerRuntime) StopInstance(ctx context.Context, instance *types.Instance) error {
	f.logger.Info("Stopping Firecracker container", "instance_id", instance.ID)

	ctx = namespaces.WithNamespace(ctx, f.config.ContainerdNamespace)

	// Get the container
	container, err := f.client.LoadContainer(ctx, instance.ID)
	if err != nil {
		f.logger.Debug("Container not found, assuming already stopped", "instance_id", instance.ID)
		instance.State = types.InstanceStateStopped
		return nil
	}

	// Get the task
	task, err := container.Task(ctx, nil)
	if err != nil {
		f.logger.Debug("Task not found, assuming already stopped", "instance_id", instance.ID)
		instance.State = types.InstanceStateStopped
		return nil
	}

	// Stop the task gracefully
	if err := task.Kill(ctx, 15); err != nil { // SIGTERM
		f.logger.Warn("Failed to send SIGTERM, trying SIGKILL", "instance_id", instance.ID, "error", err)
		task.Kill(ctx, 9) // SIGKILL
	}

	// Wait for task to exit
	stopCtx, cancel := context.WithTimeout(ctx, f.config.StopTimeout)
	defer cancel()

	_, err = task.Wait(stopCtx)
	if err != nil {
		f.logger.Warn("Task did not exit gracefully", "instance_id", instance.ID, "error", err)
	}

	// Delete the task
	if _, err := task.Delete(ctx); err != nil {
		f.logger.Warn("Failed to delete task", "instance_id", instance.ID, "error", err)
	}

	instance.State = types.InstanceStateStopped
	f.logger.Info("Firecracker container stopped successfully", "instance_id", instance.ID)
	return nil
}

// DeleteInstance removes a Firecracker microVM container
func (f *FirecrackerRuntime) DeleteInstance(ctx context.Context, instance *types.Instance) error {
	f.logger.Info("Deleting Firecracker container", "instance_id", instance.ID)

	// Stop first if running
	if instance.State == types.InstanceStateRunning {
		if err := f.StopInstance(ctx, instance); err != nil {
			f.logger.Warn("Failed to stop container before deletion", "instance_id", instance.ID, "error", err)
		}
	}

	ctx = namespaces.WithNamespace(ctx, f.config.ContainerdNamespace)

	// Get the container
	container, err := f.client.LoadContainer(ctx, instance.ID)
	if err != nil {
		f.logger.Debug("Container not found, assuming already deleted", "instance_id", instance.ID)
		instance.State = types.InstanceStateTerminated
		return nil
	}

	// Delete the container
	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		f.logger.Warn("Failed to delete container", "instance_id", instance.ID, "error", err)
		return fmt.Errorf("failed to delete container: %w", err)
	}

	instance.State = types.InstanceStateTerminated
	f.logger.Info("Firecracker container deleted successfully", "instance_id", instance.ID)
	return nil
}

// GetInstanceState gets the current state of a Firecracker microVM container
func (f *FirecrackerRuntime) GetInstanceState(ctx context.Context, instance *types.Instance) (types.InstanceState, error) {
	ctx = namespaces.WithNamespace(ctx, f.config.ContainerdNamespace)

	// Get the container
	container, err := f.client.LoadContainer(ctx, instance.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return types.InstanceStateTerminated, nil
		}
		return "", fmt.Errorf("failed to load container: %w", err)
	}

	// Get the task
	task, err := container.Task(ctx, nil)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return types.InstanceStateStopped, nil
		}
		return "", fmt.Errorf("failed to get task: %w", err)
	}

	// Get task status
	status, err := task.Status(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get task status: %w", err)
	}

	// Map containerd task states to our runtime states
	switch status.Status {
	case containerd.Created:
		return types.InstanceStateCreating, nil
	case containerd.Running:
		return types.InstanceStateRunning, nil
	case containerd.Stopped:
		return types.InstanceStateStopped, nil
	case containerd.Paused:
		return types.InstanceStateStopped, nil
	default:
		return types.InstanceStateFailed, nil
	}
}

// IsInstanceRunning checks if a Firecracker microVM container is running
func (f *FirecrackerRuntime) IsInstanceRunning(ctx context.Context, instance *types.Instance) (bool, error) {
	state, err := f.GetInstanceState(ctx, instance)
	if err != nil {
		return false, err
	}
	return state == types.InstanceStateRunning, nil
}

// ListInstances lists all Firecracker microVM containers managed by this node agent
func (f *FirecrackerRuntime) ListInstances(ctx context.Context) ([]*types.Instance, error) {
	ctx = namespaces.WithNamespace(ctx, f.config.ContainerdNamespace)

	// List containers with zeitwork label
	containers, err := f.client.Containers(ctx, "labels.\"zeitwork.managed\"==true")
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var instances []*types.Instance
	for _, container := range containers {
		instance, err := f.containerToInstance(ctx, container)
		if err != nil {
			f.logger.Warn("Failed to convert container to instance", "container_id", container.ID(), "error", err)
			continue
		}
		instances = append(instances, instance)
	}

	return instances, nil
}

// GetStats retrieves resource usage statistics for a Firecracker microVM container
func (f *FirecrackerRuntime) GetStats(ctx context.Context, instance *types.Instance) (*types.InstanceStats, error) {
	// TODO: Implement using containerd metrics
	f.logger.Debug("Getting Firecracker container stats", "instance_id", instance.ID)
	return nil, fmt.Errorf("GetStats not implemented yet for firecracker-containerd")
}

// ExecuteCommand executes a command inside a running Firecracker microVM container
func (f *FirecrackerRuntime) ExecuteCommand(ctx context.Context, instance *types.Instance, cmd []string) (string, error) {
	// TODO: Implement using containerd exec
	f.logger.Debug("Executing command in Firecracker container", "instance_id", instance.ID)
	return "", fmt.Errorf("ExecuteCommand not implemented yet for firecracker-containerd")
}

// GetLogs retrieves logs from a Firecracker microVM container
func (f *FirecrackerRuntime) GetLogs(ctx context.Context, instance *types.Instance, lines int) ([]string, error) {
	// TODO: Implement using containerd logs
	f.logger.Debug("Getting Firecracker container logs", "instance_id", instance.ID)
	return nil, fmt.Errorf("GetLogs not implemented yet for firecracker-containerd")
}

// CleanupOrphanedInstances removes containers that are not in the desired state
func (f *FirecrackerRuntime) CleanupOrphanedInstances(ctx context.Context, desiredInstances []*types.Instance) error {
	f.logger.Info("Cleaning up orphaned Firecracker containers")

	// Get all actual instances
	actualInstances, err := f.ListInstances(ctx)
	if err != nil {
		return fmt.Errorf("failed to list actual instances: %w", err)
	}

	// Create map of desired instance IDs
	desiredMap := make(map[string]bool)
	for _, instance := range desiredInstances {
		desiredMap[instance.ID] = true
	}

	// Find and clean up orphaned instances
	cleanedCount := 0
	for _, actual := range actualInstances {
		if !desiredMap[actual.ID] {
			f.logger.Info("Cleaning up orphaned container", "instance_id", actual.ID)
			if err := f.DeleteInstance(ctx, actual); err != nil {
				f.logger.Error("Failed to cleanup orphaned container", "instance_id", actual.ID, "error", err)
				continue
			}
			cleanedCount++
		}
	}

	f.logger.Info("Cleanup completed", "orphaned_found", len(actualInstances)-len(desiredInstances), "cleaned_up", cleanedCount)
	return nil
}

// GetRuntimeInfo returns information about the Firecracker runtime
func (f *FirecrackerRuntime) GetRuntimeInfo() *types.RuntimeInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	version, err := f.client.Version(ctx)
	if err != nil {
		return &types.RuntimeInfo{
			Type:    "firecracker-containerd",
			Version: "unknown",
			Status:  "error",
		}
	}

	return &types.RuntimeInfo{
		Type:    "firecracker-containerd",
		Version: version.Version,
		Status:  "healthy",
	}
}

// Helper methods

// ensureImage pulls an image if it doesn't exist locally
func (f *FirecrackerRuntime) ensureImage(ctx context.Context, imageTag string) (containerd.Image, error) {
	// Check if image exists locally
	image, err := f.client.GetImage(ctx, imageTag)
	if err == nil {
		f.logger.Debug("Image found locally", "image", imageTag)
		return image, nil
	}

	// Pull the image
	f.logger.Info("Pulling image", "image", imageTag)
	image, err = f.client.Pull(ctx, imageTag, containerd.WithPullUnpack)
	if err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %w", imageTag, err)
	}

	f.logger.Info("Image pulled successfully", "image", imageTag)
	return image, nil
}

// buildOCISpecFromImage creates OCI spec options from the image and instance spec
func (f *FirecrackerRuntime) buildOCISpecFromImage(spec *types.InstanceSpec, image containerd.Image) []oci.SpecOpts {
	opt := []oci.SpecOpts{
		// Preserve CMD, ENTRYPOINT, ENV from image
		oci.WithImageConfig(image),
		// Set hostname
		oci.WithHostname(fmt.Sprintf("zeitwork-%s", spec.ID[:8])),
		// Ensure rootfs path is set for containerd
		oci.WithRootFSPath("rootfs"),
		// Default working directory
		oci.WithProcessCwd("/"),
	}

	// Custom env vars
	if len(spec.EnvironmentVariables) > 0 {
		env := make([]string, 0, len(spec.EnvironmentVariables))
		for key, value := range spec.EnvironmentVariables {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
		opt = append(opt, oci.WithEnv(env))
	}

	// Memory limit
	if spec.Resources != nil && spec.Resources.Memory > 0 {
		memLimit := uint64(spec.Resources.Memory) * 1024 * 1024
		opt = append(opt, oci.WithMemoryLimit(memLimit))
	}

	f.logger.Debug("Built OCI spec options from image",
		"instance_id", spec.ID,
		"image", spec.ImageTag,
		"env_vars_count", len(spec.EnvironmentVariables))

	return opt
}

// buildFallbackOCISpec creates a minimal working OCI spec as fallback
func (f *FirecrackerRuntime) buildFallbackOCISpec(instanceSpec *types.InstanceSpec, image containerd.Image) *oci.Spec {
	return &oci.Spec{
		Version: "1.0.2",
		Process: &specs.Process{
			Terminal:        false,
			Args:            []string{"/bin/sh"}, // Basic fallback
			Cwd:             "/",
			NoNewPrivileges: true,
		},
		Root: &specs.Root{
			Path:     "rootfs",
			Readonly: false,
		},
		Hostname: fmt.Sprintf("zeitwork-%s", instanceSpec.ID[:8]),
	}
}

// waitForTaskReady waits for the task to be ready
func (f *FirecrackerRuntime) waitForTaskReady(ctx context.Context, task containerd.Task) error {
	// For firecracker-containerd, the task is ready when it starts
	// The underlying Firecracker VM startup is handled automatically
	timeout := time.After(f.config.StartTimeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for task to be ready")
		case <-ticker.C:
			status, err := task.Status(ctx)
			if err != nil {
				return fmt.Errorf("failed to get task status: %w", err)
			}

			if status.Status == containerd.Running {
				return nil
			}

			if status.Status == containerd.Stopped {
				return fmt.Errorf("task stopped unexpectedly")
			}
		}
	}
}

// getContainerNetworkInfo extracts network information from a container
func (f *FirecrackerRuntime) getContainerNetworkInfo(ctx context.Context, container containerd.Container) (*types.NetworkInfo, error) {
	// For now, return basic network info
	// In a full implementation, you'd extract actual network details from the VM
	return &types.NetworkInfo{
		IPAddress:    "", // Would be populated by CNI
		NetworkID:    f.config.NetworkNamespace,
		DefaultPort:  0, // Would be set based on container config
		PortMappings: make(map[int32]int32),
	}, nil
}

// containerToInstance converts a containerd container to a runtime instance
func (f *FirecrackerRuntime) containerToInstance(ctx context.Context, container containerd.Container) (*types.Instance, error) {
	info, err := container.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container info: %w", err)
	}

	// Get instance ID from labels
	instanceID, ok := info.Labels["zeitwork.instance.id"]
	if !ok {
		return nil, fmt.Errorf("container missing zeitwork.instance.id label")
	}

	imageID, ok := info.Labels["zeitwork.image.id"]
	if !ok {
		return nil, fmt.Errorf("container missing zeitwork.image.id label")
	}

	// Get current state
	state, err := f.GetInstanceState(ctx, &types.Instance{ID: instanceID})
	if err != nil {
		state = types.InstanceStateFailed
	}

	instance := &types.Instance{
		ID:        instanceID,
		ImageID:   imageID,
		ImageTag:  info.Image,
		State:     state,
		CreatedAt: info.CreatedAt,
		RuntimeID: container.ID(),
		NetworkInfo: &types.NetworkInfo{
			NetworkID:    f.config.NetworkNamespace,
			DefaultPort:  0,
			PortMappings: make(map[int32]int32),
		},
	}

	return instance, nil
}
