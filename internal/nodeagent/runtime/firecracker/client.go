package firecracker

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// FirecrackerRuntime implements the Runtime interface using containerd's aws.firecracker runtime
type FirecrackerRuntime struct {
	config *config.FirecrackerRuntimeConfig
	logger *slog.Logger
	client *containerd.Client
}

// NewFirecrackerRuntime creates a new Firecracker runtime
func NewFirecrackerRuntime(cfg *config.FirecrackerRuntimeConfig, logger *slog.Logger) (*FirecrackerRuntime, error) {
	logger.Info("Initializing firecracker runtime (containerd aws.firecracker)")

	// Connect to firecracker-containerd
	client, err := containerd.New(cfg.ContainerdSocket)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to firecracker-containerd: %w", err)
	}

	// Basic connectivity check
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.Version(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping firecracker-containerd: %w", err)
	}

	rt := &FirecrackerRuntime{
		config: cfg,
		logger: logger,
		client: client,
	}

	logger.Info("firecracker runtime initialized successfully")
	return rt, nil
}

// CreateInstance creates a new container (microVM) using aws.firecracker runtime
func (f *FirecrackerRuntime) CreateInstance(ctx context.Context, spec *types.InstanceSpec) (*types.Instance, error) {
	f.logger.Info("Creating Firecracker instance", "instance_id", spec.ID, "image", spec.ImageTag)

	// Use namespace
	ctx = namespaces.WithNamespace(ctx, f.config.ContainerdNamespace)

	// Pull (or find) image using devmapper snapshotter
	image, err := f.ensureImage(ctx, spec.ImageTag)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure image: %w", err)
	}

	// For now, default to port 3000
	defaultPort := int32(3000)

	// Build container spec
	labels := map[string]string{
		"zeitwork.instance.id":  spec.ID,
		"zeitwork.image.id":     spec.ImageID,
		"zeitwork.managed":      "true",
		"zeitwork.runtime":      "firecracker",
		"zeitwork.default_port": fmt.Sprintf("%d", defaultPort),
	}

	opts := []containerd.NewContainerOpts{
		containerd.WithImage(image),
		containerd.WithRuntime("aws.firecracker", nil),
		containerd.WithSnapshotter("devmapper"),
		containerd.WithNewSnapshot(spec.ID, image),
		containerd.WithNewSpec(f.buildOCISpecFromImage(spec, image)...),
		containerd.WithContainerLabels(labels),
	}

	container, err := f.client.NewContainer(ctx, spec.ID, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	info, err := container.Info(ctx)
	if err != nil {
		container.Delete(ctx)
		return nil, fmt.Errorf("failed to get container info: %w", err)
	}

	instance := &types.Instance{
		ID:        spec.ID,
		ImageID:   spec.ImageID,
		ImageTag:  spec.ImageTag,
		State:     types.InstanceStateCreating,
		Resources: spec.Resources,
		EnvVars:   spec.EnvironmentVariables,
		NetworkInfo: &types.NetworkInfo{
			DefaultPort:  defaultPort,
			PortMappings: map[int32]int32{},
			NetworkID:    "",
		},
		CreatedAt: info.CreatedAt,
		RuntimeID: spec.ID,
	}
	if spec.NetworkConfig != nil {
		instance.NetworkInfo.NetworkID = spec.NetworkConfig.NetworkName
	}

	f.logger.Info("Firecracker instance container created", "instance_id", spec.ID)
	return instance, nil
}

// StartInstance starts the microVM task
func (f *FirecrackerRuntime) StartInstance(ctx context.Context, instance *types.Instance) error {
	f.logger.Info("Starting Firecracker instance", "instance_id", instance.ID)
	ctx = namespaces.WithNamespace(ctx, f.config.ContainerdNamespace)

	container, err := f.client.LoadContainer(ctx, instance.ID)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	// If any task exists already, something went wrong; clean it up then recreate
	if _, tErr := container.Task(ctx, nil); tErr == nil {
		f.logger.Warn("Pre-existing task found; cleaning up before recreate", "instance_id", instance.ID)
		if err := f.cleanupExistingTask(ctx, container); err != nil {
			return fmt.Errorf("failed to cleanup existing task: %w", err)
		}
	} else if !errdefs.IsNotFound(tErr) && !strings.Contains(strings.ToLower(tErr.Error()), "not found") {
		return fmt.Errorf("failed to check existing task: %w", tErr)
	}

	// Snapshot current CNI leases to detect the new IP after start
	preLeases := f.listCNIHostLocalIPs()

	// Create a fresh task
	task, err := container.NewTask(ctx, cio.NullIO)
	if err != nil {
		if errdefs.IsAlreadyExists(err) || strings.Contains(strings.ToLower(err.Error()), "already exists") {
			f.logger.Warn("Task already exists on create; cleaning up and retrying", "instance_id", instance.ID)
			if err := f.cleanupExistingTask(ctx, container); err != nil {
				return fmt.Errorf("failed to cleanup existing task prior to retry: %w", err)
			}
			// retry once
			task, err = container.NewTask(ctx, cio.NullIO)
			if err != nil {
				return fmt.Errorf("failed to create task after cleanup: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create task: %w", err)
		}
	}

	if err := task.Start(ctx); err != nil {
		// If start fails, try one cleanup + recreate cycle
		_ = f.cleanupExistingTask(ctx, container)
		task, err = container.NewTask(ctx, cio.NullIO)
		if err != nil {
			return fmt.Errorf("failed to create task after start failure: %w", err)
		}
		if err := task.Start(ctx); err != nil {
			task.Delete(ctx)
			return fmt.Errorf("failed to start task after cleanup: %w", err)
		}
	}

	// Wait for running
	startDeadline := time.After(f.config.StartTimeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-startDeadline:
			task.Kill(ctx, 9)
			task.Delete(ctx)
			return fmt.Errorf("timeout waiting for VM to start")
		case <-ticker.C:
			st, err := task.Status(ctx)
			if err != nil {
				return fmt.Errorf("failed to get task status: %w", err)
			}
			if st.Status == containerd.Running {
				instance.State = types.InstanceStateRunning
				now := time.Now()
				instance.StartedAt = &now

				// Resolve IP by diffing host-local leases
				postLeases := f.listCNIHostLocalIPs()
				if ip := pickNewIP(preLeases, postLeases); ip != "" {
					if instance.NetworkInfo == nil {
						instance.NetworkInfo = &types.NetworkInfo{PortMappings: map[int32]int32{}}
					}
					instance.NetworkInfo.IPAddress = ip
					f.logger.Info("Resolved instance IP from CNI host-local", "instance_id", instance.ID, "ip", ip)
				} else if ip, ierr := f.getIPFromCNIHostLocal(instance.ID); ierr == nil {
					if instance.NetworkInfo == nil {
						instance.NetworkInfo = &types.NetworkInfo{PortMappings: map[int32]int32{}}
					}
					instance.NetworkInfo.IPAddress = ip
					f.logger.Info("Resolved instance IP via fallback lookup", "instance_id", instance.ID, "ip", ip)
				} else {
					f.logger.Warn("Could not resolve instance IP from CNI leases", "instance_id", instance.ID)
				}

				// Best-effort TCP probe to ip:default_port
				if instance.NetworkInfo != nil && instance.NetworkInfo.IPAddress != "" {
					addr := fmt.Sprintf("%s:%d", instance.NetworkInfo.IPAddress, instance.NetworkInfo.DefaultPort)
					if f.probeTCP(addr, 2*time.Second) {
						f.logger.Info("TCP probe succeeded", "instance_id", instance.ID, "addr", addr)
					} else {
						f.logger.Warn("TCP probe failed", "instance_id", instance.ID, "addr", addr)
					}
				}

				f.logger.Info("Firecracker instance started", "instance_id", instance.ID)
				return nil
			}
			if st.Status == containerd.Stopped {
				// Try to fetch exit status for diagnostics
				statusC, wErr := task.Wait(ctx)
				if wErr == nil {
					select {
					case es := <-statusC:
						if es.ExitCode() != 0 {
							return fmt.Errorf("task stopped unexpectedly during start (exit=%d)", es.ExitCode())
						}
					default:
					}
				}
				return fmt.Errorf("task stopped unexpectedly during start")
			}
		}
	}
}

// StopInstance stops a running instance
func (f *FirecrackerRuntime) StopInstance(ctx context.Context, instance *types.Instance) error {
	f.logger.Info("Stopping Firecracker instance", "instance_id", instance.ID)
	ctx = namespaces.WithNamespace(ctx, f.config.ContainerdNamespace)

	container, err := f.client.LoadContainer(ctx, instance.ID)
	if err != nil {
		instance.State = types.InstanceStateStopped
		return nil
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		instance.State = types.InstanceStateStopped
		return nil
	}

	if err := task.Kill(ctx, 15); err != nil {
		f.logger.Warn("SIGTERM failed, sending SIGKILL", "instance_id", instance.ID, "error", err)
		task.Kill(ctx, 9)
	}

	stopCtx, cancel := context.WithTimeout(ctx, f.config.StopTimeout)
	defer cancel()
	exitCh, err := task.Wait(stopCtx)
	if err == nil {
		select {
		case <-stopCtx.Done():
			// timeout; proceed to delete task
		case <-exitCh:
			// exited
		}
	} else {
		f.logger.Warn("Task wait setup failed", "instance_id", instance.ID, "error", err)
	}

	if _, err := task.Delete(ctx); err != nil {
		f.logger.Warn("Failed to delete task", "instance_id", instance.ID, "error", err)
	}

	instance.State = types.InstanceStateStopped
	f.logger.Info("Firecracker instance stopped", "instance_id", instance.ID)
	return nil
}

// DeleteInstance removes the container and snapshot
func (f *FirecrackerRuntime) DeleteInstance(ctx context.Context, instance *types.Instance) error {
	f.logger.Info("Deleting Firecracker instance", "instance_id", instance.ID)

	ctx = namespaces.WithNamespace(ctx, f.config.ContainerdNamespace)
	container, err := f.client.LoadContainer(ctx, instance.ID)
	if err != nil {
		instance.State = types.InstanceStateTerminated
		return nil
	}

	// Ensure any existing task is cleaned up before deletion
	if err := f.cleanupExistingTask(ctx, container); err != nil {
		f.logger.Warn("Failed to cleanup task before delete", "instance_id", instance.ID, "error", err)
	}

	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		lower := strings.ToLower(err.Error())
		if errdefs.IsFailedPrecondition(err) || strings.Contains(lower, "failed precondition") || strings.Contains(lower, "running task") {
			f.logger.Warn("Delete precondition failed; cleaning up task and retrying", "instance_id", instance.ID)
			_ = f.cleanupExistingTask(ctx, container)
			if derr := container.Delete(ctx, containerd.WithSnapshotCleanup); derr != nil {
				return fmt.Errorf("failed to delete container after cleanup: %w", derr)
			}
		} else {
			return fmt.Errorf("failed to delete container: %w", err)
		}
	}

	instance.State = types.InstanceStateTerminated
	f.logger.Info("Firecracker instance deleted", "instance_id", instance.ID)
	return nil
}

// GetInstanceState returns the container/VM state
func (f *FirecrackerRuntime) GetInstanceState(ctx context.Context, instance *types.Instance) (types.InstanceState, error) {
	ctx = namespaces.WithNamespace(ctx, f.config.ContainerdNamespace)
	container, err := f.client.LoadContainer(ctx, instance.ID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return types.InstanceStateTerminated, nil
		}
		return "", fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return types.InstanceStateStopped, nil
		}
		return "", fmt.Errorf("failed to get task: %w", err)
	}

	st, err := task.Status(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get task status: %w", err)
	}

	switch st.Status {
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

// IsInstanceRunning checks if the instance is running
func (f *FirecrackerRuntime) IsInstanceRunning(ctx context.Context, instance *types.Instance) (bool, error) {
	state, err := f.GetInstanceState(ctx, instance)
	if err != nil {
		return false, err
	}
	return state == types.InstanceStateRunning, nil
}

// ListInstances lists all managed instances
func (f *FirecrackerRuntime) ListInstances(ctx context.Context) ([]*types.Instance, error) {
	ctx = namespaces.WithNamespace(ctx, f.config.ContainerdNamespace)
	containers, err := f.client.Containers(ctx, "labels.\"zeitwork.managed\"==true")
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var instances []*types.Instance
	for _, c := range containers {
		inst, err := f.containerToInstance(ctx, c)
		if err != nil {
			f.logger.Warn("Failed to convert container to instance", "container_id", c.ID(), "error", err)
			continue
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

// GetStats retrieves basic placeholder stats for Firecracker VMs
func (f *FirecrackerRuntime) GetStats(ctx context.Context, instance *types.Instance) (*types.InstanceStats, error) {
	instanceID := ""
	if instance != nil {
		instanceID = instance.ID
	}

	stats := &types.InstanceStats{
		InstanceID: instanceID,
		Timestamp:  time.Now(),
		CPUPercent: 0.0,
		CPUUsage:   0,
		// Memory fields filled below if limits known
		MemoryUsed:     0,
		MemoryLimit:    0,
		MemoryPercent:  0.0,
		NetworkRxBytes: 0,
		NetworkTxBytes: 0,
		DiskReadBytes:  0,
		DiskWriteBytes: 0,
	}

	// Best-effort: derive memory limit from instance resources
	if instance != nil && instance.Resources != nil {
		if instance.Resources.MemoryLimit > 0 {
			stats.MemoryLimit = uint64(instance.Resources.MemoryLimit)
		} else if instance.Resources.Memory > 0 {
			stats.MemoryLimit = uint64(instance.Resources.Memory) * 1024 * 1024
		}
	}

	// Without cgroup/metrics integration, we cannot know MemoryUsed/CPUUsage yet
	// Keep zeros to indicate unknown usage rather than erroring.
	return stats, nil
}

// ExecuteCommand not implemented yet for Firecracker
func (f *FirecrackerRuntime) ExecuteCommand(ctx context.Context, instance *types.Instance, cmd []string) (string, error) {
	f.logger.Debug("ExecuteCommand not implemented for firecracker runtime", "instance_id", instance.ID)
	return "", fmt.Errorf("ExecuteCommand not implemented yet for firecracker runtime")
}

// GetLogs not implemented yet for Firecracker
func (f *FirecrackerRuntime) GetLogs(ctx context.Context, instance *types.Instance, lines int) ([]string, error) {
	f.logger.Debug("GetLogs not implemented for firecracker runtime", "instance_id", instance.ID)
	return nil, fmt.Errorf("GetLogs not implemented yet for firecracker runtime")
}

// CleanupOrphanedInstances removes containers not in desired set
func (f *FirecrackerRuntime) CleanupOrphanedInstances(ctx context.Context, desiredInstances []*types.Instance) error {
	f.logger.Info("Cleaning up orphaned Firecracker instances")
	actual, err := f.ListInstances(ctx)
	if err != nil {
		return fmt.Errorf("failed to list instances: %w", err)
	}

	desired := make(map[string]bool)
	for _, inst := range desiredInstances {
		desired[inst.ID] = true
	}

	cleaned := 0
	for _, inst := range actual {
		if !desired[inst.ID] {
			if err := f.DeleteInstance(ctx, inst); err != nil {
				f.logger.Error("Failed to cleanup orphaned instance", "instance_id", inst.ID, "error", err)
				continue
			}
			cleaned++
		}
	}

	f.logger.Info("Orphan cleanup done", "cleaned_up", cleaned)
	return nil
}

// GetRuntimeInfo returns basic runtime info
func (f *FirecrackerRuntime) GetRuntimeInfo() *types.RuntimeInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	version, err := f.client.Version(ctx)
	if err != nil {
		return &types.RuntimeInfo{Type: "firecracker", Version: "unknown", Status: "error"}
	}
	return &types.RuntimeInfo{Type: "firecracker", Version: version.Version, Status: "healthy"}
}

// ----------------- helpers -----------------

// getIPFromCNIHostLocal attempts to discover the VM's IPv4 address by reading
// host-local IPAM lease files under the configured CNI state directory.
// It looks for a file named after the network with the content lines of IPs.
// firecracker-containerd typically uses a fixed MAC -> IP mapping; since we
// don't have the MAC here, we fallback to the single-IP heuristic.
func (f *FirecrackerRuntime) getIPFromCNIHostLocal(instanceID string) (string, error) {
	// Expected layout for host-local IPAM: /var/lib/cni/networks/<network>/<ip>
	// Files are named by IP address and contain the container ID. We'll scan the
	// directory and find the file whose content matches our instanceID.
	stateDir := f.config.CNIStateDir
	if stateDir == "" {
		stateDir = "/var/lib/cni/networks"
	}
	netDir := fmt.Sprintf("%s/%s", stateDir, f.config.NetworkName)
	entries, err := os.ReadDir(netDir)
	if err != nil {
		return "", fmt.Errorf("read cni state dir: %w", err)
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		// skip meta files
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".lock") {
			continue
		}
		path := fmt.Sprintf("%s/%s", netDir, name)
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == instanceID {
			// filename is the IP address
			return name, nil
		}
	}
	return "", fmt.Errorf("ip not found in cni host-local leases for %s", instanceID)
}

// listCNIHostLocalIPs lists IPv4 lease filenames (IPs) under the host-local dir for the configured network
func (f *FirecrackerRuntime) listCNIHostLocalIPs() []string {
	stateDir := f.config.CNIStateDir
	if stateDir == "" {
		stateDir = "/var/lib/cni/networks"
	}
	netDir := fmt.Sprintf("%s/%s", stateDir, f.config.NetworkName)
	entries, err := os.ReadDir(netDir)
	if err != nil {
		return nil
	}
	var ips []string
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if strings.Count(name, ".") == 3 { // naive IPv4 filename check
			ips = append(ips, name)
		}
	}
	return ips
}

// pickNewIP returns IPs present in after but not in before; if exactly one, return it
func pickNewIP(before, after []string) string {
	set := make(map[string]struct{}, len(before))
	for _, ip := range before {
		set[ip] = struct{}{}
	}
	var news []string
	for _, ip := range after {
		if _, ok := set[ip]; !ok {
			news = append(news, ip)
		}
	}
	if len(news) == 1 {
		return news[0]
	}
	return ""
}

// probeTCP tries to connect to addr within timeout
func (f *FirecrackerRuntime) probeTCP(addr string, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	c, err := d.Dial("tcp", addr)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// cleanupExistingTask attempts to stop and delete any existing task for the container.
// It is safe to call if no task exists.
func (f *FirecrackerRuntime) cleanupExistingTask(ctx context.Context, container containerd.Container) error {
	// Try to load existing task
	task, err := container.Task(ctx, nil)
	if err != nil {
		if errdefs.IsNotFound(err) || strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil
		}
		return fmt.Errorf("load existing task: %w", err)
	}

	// Attempt graceful shutdown
	if err := task.Kill(ctx, 15); err != nil {
		f.logger.Warn("Failed to SIGTERM existing task, sending SIGKILL", "container_id", container.ID(), "error", err)
		_ = task.Kill(ctx, 9)
	}

	// Wait with timeout
	waitCtx, cancel := context.WithTimeout(ctx, f.config.StopTimeout)
	defer cancel()
	if exitCh, werr := task.Wait(waitCtx); werr == nil {
		select {
		case <-exitCh:
		case <-waitCtx.Done():
		}
	}

	// Delete task; treat not found as success
	if _, derr := task.Delete(ctx); derr != nil {
		if !errdefs.IsNotFound(derr) && !strings.Contains(strings.ToLower(derr.Error()), "not found") {
			return fmt.Errorf("delete existing task: %w", derr)
		}
	}
	return nil
}

// ensureImage pulls the image via devmapper snapshotter when missing
func (f *FirecrackerRuntime) ensureImage(ctx context.Context, imageTag string) (containerd.Image, error) {
	// Try get existing image first
	img, err := f.client.GetImage(ctx, imageTag)
	if err == nil {
		f.logger.Debug("Image found locally", "image", imageTag)
		return img, nil
	}

	// Ensure registry prefix
	ref := f.ensureRegistryPrefix(imageTag)
	// Use a dedicated pull timeout
	pullTimeout := f.config.StartTimeout
	if f.config.PullTimeout > 0 {
		pullTimeout = f.config.PullTimeout
	}
	pctx, cancel := context.WithTimeout(ctx, pullTimeout)
	defer cancel()
	f.logger.Info("Pulling image for firecracker", "image", ref, "snapshotter", "devmapper", "timeout", pullTimeout.String())
	img, err = f.client.Pull(pctx, ref, containerd.WithPullUnpack, containerd.WithPullSnapshotter("devmapper"))
	if err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %w", ref, err)
	}
	return img, nil
}

// buildOCISpecFromImage builds an OCI spec from image + instance spec
func (f *FirecrackerRuntime) buildOCISpecFromImage(spec *types.InstanceSpec, image containerd.Image) []oci.SpecOpts {
	opts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithHostname(fmt.Sprintf("zeitwork-%s", trimID(spec.ID, 8))),
		oci.WithRootFSPath("rootfs"),
		// Respect the image's configured working directory; do not override CWD
	}

	// Env vars
	if len(spec.EnvironmentVariables) > 0 {
		env := make([]string, 0, len(spec.EnvironmentVariables))
		for k, v := range spec.EnvironmentVariables {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		opts = append(opts, oci.WithEnv(env))
	}

	// Memory limit
	if spec.Resources != nil && spec.Resources.Memory > 0 {
		mem := uint64(spec.Resources.Memory) * 1024 * 1024
		opts = append(opts, oci.WithMemoryLimit(mem))
	}
	return opts
}

func (f *FirecrackerRuntime) containerToInstance(ctx context.Context, c containerd.Container) (*types.Instance, error) {
	info, err := c.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container info: %w", err)
	}
	instanceID, ok := info.Labels["zeitwork.instance.id"]
	if !ok {
		instanceID = c.ID()
	}
	imageID := info.Labels["zeitwork.image.id"]

	state, err := f.GetInstanceState(ctx, &types.Instance{ID: instanceID})
	if err != nil {
		state = types.InstanceStateFailed
	}
	inst := &types.Instance{
		ID:        instanceID,
		ImageID:   imageID,
		ImageTag:  info.Image,
		State:     state,
		CreatedAt: info.CreatedAt,
		RuntimeID: c.ID(),
		NetworkInfo: &types.NetworkInfo{
			NetworkID:    f.config.NetworkName,
			DefaultPort:  0,
			PortMappings: map[int32]int32{},
		},
	}
	if dpStr, ok := info.Labels["zeitwork.default_port"]; ok {
		if v, err := strconv.Atoi(dpStr); err == nil {
			inst.NetworkInfo.DefaultPort = int32(v)
		}
	}
	if inst.NetworkInfo.DefaultPort == 0 {
		inst.NetworkInfo.DefaultPort = 3000
	}

	// Best-effort IP discovery from CNI cache (no guarantee if created long ago)
	if ip, err := f.getIPFromCNIHostLocal(instanceID); err == nil {
		inst.NetworkInfo.IPAddress = ip
	}
	return inst, nil
}

func (f *FirecrackerRuntime) ensureRegistryPrefix(imageTag string) string {
	// If tag already has a registry prefix (host:port/...), return as-is
	if strings.Contains(imageTag, "/") && strings.Contains(strings.Split(imageTag, "/")[0], ":") {
		return imageTag
	}
	if f.config.ImageRegistry != "" {
		return fmt.Sprintf("%s/%s", f.config.ImageRegistry, imageTag)
	}
	return imageTag
}

func trimID(id string, n int) string {
	if len(id) <= n {
		return id
	}
	return id[:n]
}
