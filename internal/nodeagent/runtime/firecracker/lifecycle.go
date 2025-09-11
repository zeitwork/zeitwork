package firecracker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// StartInstance launches a VM-backed task via firecracker-ctr and configures IPv6 inside the guest.
func (r *Runtime) StartInstance(ctx context.Context, instance *types.Instance) error {
	name := instance.RuntimeID
	image := instance.ImageTag
	// Resolve to containerd's known reference if available
	if ref, err := r.resolveImageRef(ctx, image); err == nil {
		image = ref
	}

	// Ensure namespace exists and labels defaults.
	if err := r.ensureNamespace(ctx); err != nil {
		return err
	}

	// Run task detached with CAP_NET_ADMIN to allow in-guest net config, and host net for DNS if needed.
	args := []string{"run", "-d", "--snapshotter", "devmapper", "--runtime", "aws.firecracker", "--cap-add", "CAP_NET_ADMIN"}
	// Optional: enable host net for simplified DNS; adjust if pure CNI is desired.
	args = append(args, "--net-host")
	args = append(args, image, name)
	if out, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, args); err != nil {
		return fmt.Errorf("ctr run failed: %v: %s", err, out)
	}

	// Wait for task to appear
	if err := r.waitTask(ctx, name, 30*time.Second); err != nil {
		return err
	}

	// Discover VMID from recent logs and find the assigned IPv6 lease.
	vmID, err := findVMIDForTaskExec("/tmp/firecracker-containerd.log", "", name)
	if err != nil {
		// Fallback: try default containerd log path
		vmID, err = findVMIDForTaskExec("/var/log/firecracker-containerd.log", "", name)
	}
	if err != nil {
		return fmt.Errorf("vmID discovery failed: %w", err)
	}
	leaseIP, err := discoverIPv6Lease(r.cfg.CNIStateDir, r.cfg.NetworkName, vmID)
	if err != nil {
		return fmt.Errorf("ipv6 lease discovery failed: %w", err)
	}

	// Configure IPv6 inside guest
	if err := r.configureIPv6(ctx, name, leaseIP); err != nil {
		return fmt.Errorf("configure ipv6 failed: %w", err)
	}

	// Update instance state
	instance.State = types.InstanceStateRunning
	now := time.Now()
	instance.StartedAt = &now
	var defaultPort int32
	if instance.NetworkInfo != nil {
		defaultPort = instance.NetworkInfo.DefaultPort
	}
	instance.NetworkInfo = &types.NetworkInfo{IPAddress: leaseIP, DefaultPort: defaultPort}
	return nil
}

func (r *Runtime) ensureNamespace(ctx context.Context) error {
	// Create namespace and set defaults for runtime/snapshotter
	if _, err := runFC(ctx, r.cfg, []string{"--address", r.cfg.ContainerdSocket, "namespaces", "create", r.cfg.ContainerdNamespace}); err != nil {
		// ignore errors if exists
	}
	if _, err := runFC(ctx, r.cfg, []string{"--address", r.cfg.ContainerdSocket, "namespaces", "label", r.cfg.ContainerdNamespace, "containerd.io/defaults/runtime=aws.firecracker", "containerd.io/defaults/snapshotter=devmapper"}); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) waitTask(ctx context.Context, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, _ := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "list"})
		if strings.Contains(out, name) && strings.Contains(strings.ToUpper(out), "RUNNING") {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("task %s did not appear running within %s", name, timeout)
}

func (r *Runtime) configureIPv6(ctx context.Context, name, ipv6 string) error {
	// Wait for eth0
	for i := 0; i < 30; i++ {
		if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "exec", "--exec-id", "chk-" + randomHex(4), name, "sh", "-lc", "ip link show eth0 >/dev/null 2>&1"}); err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	// Configure link + IPv6 + default route
	cmds := []string{
		"ip link set eth0 up",
		fmt.Sprintf("ip -6 addr add %s/64 dev eth0 || true", ipv6),
		"ip -6 route add default via fd00:fc::1 dev eth0 || true",
	}
	for _, c := range cmds {
		if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "exec", "--exec-id", "cfg-" + randomHex(4), name, "sh", "-lc", c}); err != nil {
			return fmt.Errorf("guest cmd failed: %s: %w", c, err)
		}
	}
	return nil
}

// StopInstance sends SIGTERM to task and waits up to timeout.
func (r *Runtime) StopInstance(ctx context.Context, instance *types.Instance) error {
	name := instance.RuntimeID
	if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "kill", name}); err != nil {
		return err
	}
	instance.State = types.InstanceStateStopped
	return nil
}

// DeleteInstance removes the task and container entries.
func (r *Runtime) DeleteInstance(ctx context.Context, instance *types.Instance) error {
	name := instance.RuntimeID
	// Delete task if present
	_, _ = runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "kill", name})
	// Delete container
	if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"containers", "delete", name}); err != nil {
		// ignore if not found
	}
	instance.State = types.InstanceStateTerminated
	return nil
}

func (r *Runtime) GetInstanceState(ctx context.Context, instance *types.Instance) (types.InstanceState, error) {
	out, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "list"})
	if err != nil {
		return types.InstanceStateFailed, err
	}
	state := types.InstanceStatePending
	if strings.Contains(out, instance.RuntimeID) {
		// naive parse
		state = mapStatus(out)
	}
	instance.State = state
	return state, nil
}

func (r *Runtime) ListInstances(ctx context.Context) ([]*types.Instance, error) {
	out, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "list"})
	if err != nil {
		return nil, err
	}
	var instances []*types.Instance
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, ln := range lines[1:] { // skip header
		fields := strings.Fields(ln)
		if len(fields) < 3 {
			continue
		}
		name := fields[0]
		status := fields[2]
		instances = append(instances, &types.Instance{
			ID:        name,
			RuntimeID: name,
			State:     mapStatus(status),
		})
	}
	return instances, nil
}

func (r *Runtime) IsInstanceRunning(ctx context.Context, instance *types.Instance) (bool, error) {
	st, err := r.GetInstanceState(ctx, instance)
	if err != nil {
		return false, err
	}
	return st == types.InstanceStateRunning, nil
}

func (r *Runtime) GetStats(ctx context.Context, instance *types.Instance) (*types.InstanceStats, error) {
	// Not implemented: containerd top-level stats would require parsing or cgroups.
	return &types.InstanceStats{InstanceID: instance.ID, Timestamp: time.Now()}, nil
}

func (r *Runtime) ExecuteCommand(ctx context.Context, instance *types.Instance, cmd []string) (string, error) {
	if len(cmd) == 0 {
		return "", nil
	}
	args := []string{"tasks", "exec", "--exec-id", "exec-" + randomHex(4), instance.RuntimeID, "sh", "-lc", strings.Join(cmd, " ")}
	return runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, args)
}

func (r *Runtime) GetLogs(ctx context.Context, instance *types.Instance, lines int) ([]string, error) {
	// firecracker-ctr has no direct logs command; return tail of daemon log filtered by vmID
	vmID, err := findVMIDForTaskExec("/tmp/firecracker-containerd.log", "", instance.RuntimeID)
	if err != nil {
		vmID, _ = findVMIDForTaskExec("/var/log/firecracker-containerd.log", "", instance.RuntimeID)
	}
	path := "/tmp/firecracker-containerd.log"
	if _, err := os.Stat(path); err != nil {
		path = "/var/log/firecracker-containerd.log"
	}
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var res []string
	for _, ln := range strings.Split(string(b), "\n") {
		if vmID != "" && strings.Contains(ln, vmID) {
			res = append(res, ln)
		}
	}
	if len(res) > lines && lines > 0 {
		res = res[len(res)-lines:]
	}
	return res, nil
}

func (r *Runtime) CleanupOrphanedInstances(ctx context.Context, desired []*types.Instance) error {
	// List running tasks and stop those not present in desired
	actual, err := r.ListInstances(ctx)
	if err != nil {
		return err
	}
	wanted := make(map[string]bool)
	for _, d := range desired {
		wanted[d.RuntimeID] = true
	}
	for _, a := range actual {
		if !wanted[a.RuntimeID] {
			_ = r.DeleteInstance(ctx, a)
		}
	}
	return nil
}

func (r *Runtime) GetRuntimeInfo() *types.RuntimeInfo {
	return &types.RuntimeInfo{Type: "firecracker", Version: "firecracker-containerd", Status: "unknown"}
}
