package firecracker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
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
		// Check if the error is due to snapshot or container already existing
		if strings.Contains(out, "already exists") && (strings.Contains(out, "snapshot") || strings.Contains(out, "container")) {
			r.logger.Warn("Container or snapshot already exists, cleaning up and retrying",
				"instance_id", instance.ID,
				"name", name,
				"error", out)

			// Clean up the existing container and snapshot
			cleanupErr := r.cleanupContainerAndSnapshot(ctx, name)
			if cleanupErr != nil {
				r.logger.Warn("Failed to cleanup existing snapshot, but will attempt retry anyway",
					"instance_id", instance.ID,
					"snapshot_name", name,
					"cleanup_error", cleanupErr)
			}

			// Give containerd time to fully process the cleanup
			time.Sleep(200 * time.Millisecond)

			// Retry the run command regardless of cleanup result
			// Sometimes the snapshot issue resolves itself
			// Use a fresh context for retry to avoid cancellation issues
			retryCtx, retryCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer retryCancel()

			if retryOut, retryErr := runFCNS(retryCtx, r.cfg, r.cfg.ContainerdNamespace, args); retryErr != nil {
				// If retry also fails, check if it's the same snapshot error
				if strings.Contains(retryOut, "already exists") && strings.Contains(retryOut, "snapshot") {
					// Still having snapshot issues - this might be a race condition
					r.logger.Error("Snapshot conflict persists after cleanup and retry",
						"instance_id", instance.ID,
						"snapshot_name", name,
						"retry_error", retryOut)
					return fmt.Errorf("persistent snapshot conflict: original error: %v: %s, retry error: %v: %s", err, out, retryErr, retryOut)
				} else {
					// Different error on retry
					return fmt.Errorf("ctr run failed on retry after snapshot cleanup: %v: %s", retryErr, retryOut)
				}
			} else {
				r.logger.Info("Successfully created container after snapshot cleanup and retry",
					"instance_id", instance.ID,
					"snapshot_name", name)
			}
		} else {
			return fmt.Errorf("ctr run failed: %v: %s", err, out)
		}
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
		// Fallback: deterministically generate a unique IPv6 within fd00:fc::/64
		if r.logger != nil {
			r.logger.Warn("ipv6 lease not found; generating fallback IPv6", "vmID", vmID, "error", err)
		}
		leaseIP, err = generateFallbackIPv6(r.cfg.CNIStateDir, r.cfg.NetworkName, vmID)
		if err != nil {
			return fmt.Errorf("ipv6 lease discovery failed and fallback generation failed: %w", err)
		}
	}

	// Ensure the IP is unique across all instances in the database
	if r.queries != nil {
		inUse, checkErr := r.queries.InstancesCheckIpInUse(ctx, leaseIP)
		if checkErr != nil {
			r.logger.Warn("Failed to verify IP uniqueness; proceeding with current IP",
				"instance_id", instance.ID,
				"ip_address", leaseIP,
				"error", checkErr)
		} else if inUse {
			r.logger.Warn("IP address already in use; finding alternative",
				"instance_id", instance.ID,
				"ip_address", leaseIP)

			// Generate a unique IP that's not in use
			newIP, genErr := r.findUnusedIPv6(ctx, instance.ID)
			if genErr != nil {
				r.logger.Error("Failed to generate a unique IPv6; proceeding with original",
					"instance_id", instance.ID,
					"original_ip", leaseIP,
					"error", genErr)
			} else {
				leaseIP = newIP
				r.logger.Info("Using alternative unique IPv6",
					"instance_id", instance.ID,
					"ip_address", leaseIP)
			}
		}
	}

	// Configure IPv6 using the most robust approach available
	if err := r.injectNetworkConfigIntoVM(ctx, name, leaseIP); err != nil {
		r.logger.Warn("Advanced network injection failed, trying basic approach", "error", err)
		// Fallback to manual configuration
		if fallbackErr := r.configureIPv6Manual(ctx, name, leaseIP); fallbackErr != nil {
			return fmt.Errorf("all IPv6 configuration methods failed: primary=%w, fallback=%w", err, fallbackErr)
		}
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
	// Wait for eth0 to appear in the VM
	for i := 0; i < 30; i++ {
		// Try multiple approaches to check for eth0
		checkCmds := []string{
			"/sbin/ip link show eth0 >/dev/null 2>&1",
			"/bin/ip link show eth0 >/dev/null 2>&1",
			"ip link show eth0 >/dev/null 2>&1",
			"ls /sys/class/net/eth0 >/dev/null 2>&1",
		}

		ethFound := false
		for _, checkCmd := range checkCmds {
			if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "exec", "--exec-id", "chk-" + randomHex(4), name, "sh", "-c", checkCmd}); err == nil {
				ethFound = true
				break
			}
		}

		if ethFound {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Configure link + IPv6 + default route with multiple command path attempts
	cmds := []struct {
		desc         string
		alternatives []string
	}{
		{"set eth0 up", []string{
			"/sbin/ip link set eth0 up",
			"/bin/ip link set eth0 up",
			"ip link set eth0 up",
		}},
		{"add IPv6 address", []string{
			fmt.Sprintf("/sbin/ip -6 addr add %s/64 dev eth0 || true", ipv6),
			fmt.Sprintf("/bin/ip -6 addr add %s/64 dev eth0 || true", ipv6),
			fmt.Sprintf("ip -6 addr add %s/64 dev eth0 || true", ipv6),
		}},
		{"add default route", []string{
			"/sbin/ip -6 route add default via fd00:fc::1 dev eth0 || true",
			"/bin/ip -6 route add default via fd00:fc::1 dev eth0 || true",
			"ip -6 route add default via fd00:fc::1 dev eth0 || true",
		}},
	}

	for _, cmdGroup := range cmds {
		success := false
		var lastErr error

		for _, cmd := range cmdGroup.alternatives {
			if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "exec", "--exec-id", "cfg-" + randomHex(4), name, "sh", "-c", cmd}); err == nil {
				r.logger.Debug("IPv6 config command succeeded", "command", cmd)
				success = true
				break
			} else {
				lastErr = err
			}
		}

		if !success {
			return fmt.Errorf("guest cmd failed for %s (tried all alternatives): %w", cmdGroup.desc, lastErr)
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

		// Extract the actual instance ID from the container name (remove "fc-" prefix)
		instanceID := name
		if strings.HasPrefix(name, "fc-") {
			instanceID = name[3:] // Remove "fc-" prefix
		}

		instances = append(instances, &types.Instance{
			ID:        instanceID,
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

// findUnusedIPv6 generates a unique IPv6 address in the fd00:fc::/64 range that's not already in use
func (r *Runtime) findUnusedIPv6(ctx context.Context, instanceID string) (string, error) {
	base := "fd00:fc::"

	// Start with a deterministic hash of the instance ID
	sum := sha256.Sum256([]byte(instanceID))
	h0 := binary.BigEndian.Uint16(sum[0:2])
	h1 := binary.BigEndian.Uint16(sum[2:4])
	h2 := binary.BigEndian.Uint16(sum[4:6])
	h3 := binary.BigEndian.Uint16(sum[6:8])

	// Try deterministic variants first
	for i := 0; i < 64; i++ {
		ip := fmt.Sprintf("%s%x:%x:%x:%x", base, h0, h1, h2, h3+uint16(i))
		if ip == "fd00:fc::1" {
			continue // Skip gateway address
		}

		if r.queries != nil {
			inUse, err := r.queries.InstancesCheckIpInUse(ctx, ip)
			if err != nil {
				return "", fmt.Errorf("failed to check IP usage: %w", err)
			}
			if !inUse {
				return ip, nil
			}
		} else {
			// If no database queries available, just return the deterministic IP
			return ip, nil
		}
	}

	// Fall back to random generation
	for i := 0; i < 128; i++ {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		ip := fmt.Sprintf("%s%x:%x:%x:%x", base,
			binary.BigEndian.Uint16(b[0:2]),
			binary.BigEndian.Uint16(b[2:4]),
			binary.BigEndian.Uint16(b[4:6]),
			binary.BigEndian.Uint16(b[6:8]))

		if ip == "fd00:fc::1" {
			continue // Skip gateway address
		}

		if r.queries != nil {
			inUse, err := r.queries.InstancesCheckIpInUse(ctx, ip)
			if err != nil {
				return "", fmt.Errorf("failed to check IP usage: %w", err)
			}
			if !inUse {
				return ip, nil
			}
		} else {
			// If no database queries available, just return the random IP
			return ip, nil
		}
	}

	return "", fmt.Errorf("unable to find unused IPv6 address after 192 attempts")
}

// cleanupContainerAndSnapshot removes existing container and snapshot to allow for retry
func (r *Runtime) cleanupContainerAndSnapshot(ctx context.Context, name string) error {
	r.logger.Info("Cleaning up existing container and snapshot", "name", name)

	// Step 1: Try to stop any running tasks first with SIGKILL
	stopArgs := []string{"tasks", "kill", "--signal", "KILL", name}
	if stopOut, stopErr := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, stopArgs); stopErr != nil {
		// Check if error is because task doesn't exist (which is fine)
		if !strings.Contains(stopOut, "not found") && !strings.Contains(stopOut, "no such task") {
			r.logger.Debug("Task kill failed (may not exist)",
				"task_name", name,
				"error", stopErr,
				"output", stopOut)
		}
	} else {
		r.logger.Debug("Successfully stopped task", "task_name", name)

		// Wait for the task to actually terminate
		if err := r.waitForTaskTermination(ctx, name, 5*time.Second); err != nil {
			r.logger.Warn("Task may not have fully terminated", "task_name", name, "error", err)
		}
	}

	// Step 2: Try to delete the container (with retries for persistent state issues)
	containerArgs := []string{"containers", "delete", name}
	for attempt := 0; attempt < 3; attempt++ {
		if containerOut, containerErr := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, containerArgs); containerErr != nil {
			// Check if error is because container doesn't exist (which is fine)
			if strings.Contains(containerOut, "not found") || strings.Contains(containerOut, "no such container") {
				r.logger.Debug("Container does not exist (already cleaned up)", "container_name", name)
				break
			}

			// If it's a "cannot delete non stopped container" error, try killing again
			if strings.Contains(containerOut, "cannot delete a non stopped container") {
				r.logger.Debug("Container still running, attempting additional cleanup",
					"container_name", name,
					"attempt", attempt+1)

				// Try task kill again with SIGKILL and wait for termination
				killArgs := []string{"tasks", "kill", "--signal", "KILL", name}
				if killOut, killErr := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, killArgs); killErr == nil {
					r.logger.Debug("Additional task kill succeeded", "task_name", name)
					// Wait for actual termination
					if termErr := r.waitForTaskTermination(ctx, name, 2*time.Second); termErr != nil {
						r.logger.Debug("Task termination wait failed", "task_name", name, "error", termErr)
					}
				} else {
					r.logger.Debug("Additional task kill failed", "task_name", name, "error", killErr, "output", killOut)
				}

				// Try force deleting the container
				forceArgs := []string{"containers", "delete", "--force", name}
				if forceOut, forceErr := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, forceArgs); forceErr == nil {
					r.logger.Debug("Force container delete succeeded", "container_name", name)
					break
				} else {
					r.logger.Debug("Force container delete failed", "container_name", name, "error", forceErr, "output", forceOut)
				}

				// Wait longer for containerd state to update
				time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
				continue
			}

			// Other container delete errors
			r.logger.Debug("Container delete failed",
				"container_name", name,
				"attempt", attempt+1,
				"error", containerErr,
				"output", containerOut)

			if attempt == 2 {
				// Final attempt failed, but continue with snapshot cleanup
				break
			}
			time.Sleep(100 * time.Millisecond)
		} else {
			r.logger.Debug("Successfully deleted container", "container_name", name)
			break
		}
	}

	// Step 3: Try to remove the snapshot
	args := []string{"snapshots", "remove", name}
	if out, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, args); err != nil {
		// Check if the snapshot actually doesn't exist (success case)
		if strings.Contains(out, "not found") || strings.Contains(out, "does not exist") {
			r.logger.Info("Snapshot already removed or does not exist", "snapshot_name", name)
			return nil
		}

		// If it's a different error, try to list snapshots to see what's there
		listArgs := []string{"snapshots", "list"}
		if listOut, listErr := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, listArgs); listErr == nil {
			r.logger.Debug("Current snapshots", "output", listOut)
		}

		// Try one more aggressive cleanup approach - use the devmapper snapshotter directly
		r.logger.Debug("Attempting aggressive snapshot cleanup using devmapper snapshotter", "snapshot_name", name)
		devmapperArgs := []string{"snapshots", "--snapshotter", "devmapper", "remove", name}
		if devOut, devErr := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, devmapperArgs); devErr != nil {
			if strings.Contains(devOut, "not found") || strings.Contains(devOut, "does not exist") {
				r.logger.Info("Snapshot cleaned up via devmapper snapshotter", "snapshot_name", name)
				return nil
			}
			r.logger.Debug("Devmapper cleanup also failed", "error", devErr, "output", devOut)
		} else {
			r.logger.Info("Successfully cleaned up snapshot via devmapper snapshotter", "snapshot_name", name)
			return nil
		}

		return fmt.Errorf("failed to remove snapshot: %v: %s", err, out)
	}

	r.logger.Info("Successfully cleaned up container and snapshot", "name", name)
	return nil
}

// waitForTaskTermination waits for a task to actually terminate after kill
func (r *Runtime) waitForTaskTermination(ctx context.Context, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "list"})
		if err != nil {
			return err
		}

		// Check if task still exists and is running
		lines := strings.Split(strings.TrimSpace(out), "\n")
		taskFound := false
		taskRunning := false

		for _, line := range lines {
			if strings.Contains(line, name) {
				taskFound = true
				if strings.Contains(strings.ToUpper(line), "RUNNING") {
					taskRunning = true
				}
				break
			}
		}

		if !taskFound {
			// Task completely removed
			return nil
		}

		if taskFound && !taskRunning {
			// Task exists but not running (STOPPED) - good enough
			return nil
		}

		// Task still running, wait a bit more
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("task %s did not terminate within %v", name, timeout)
}
