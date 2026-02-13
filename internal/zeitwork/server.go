package zeitwork

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/vishvananda/netlink"
	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

const (
	serverIDPath = "/data/server-id"

	heartbeatInterval       = 10 * time.Second
	deadDetectionInterval   = 30 * time.Second
	drainHealthCheckTimeout = 5 * time.Minute
	leaderRetryInterval     = 5 * time.Second
)

// LoadOrCreateServerID reads the server ID from disk, or generates and persists a new one.
// The ID is stable across restarts — it's how a server maintains its identity in the cluster.
func LoadOrCreateServerID() (uuid.UUID, error) {
	data, err := os.ReadFile(serverIDPath)
	if err == nil {
		id, err := uuid.Parse(strings.TrimSpace(string(data)))
		if err == nil {
			slog.Info("loaded server ID from disk", "server_id", id)
			return id, nil
		}
		// A corrupt server-id file indicates something is truly wrong with the node.
		// Do not silently regenerate — fail hard so the operator can investigate.
		return uuid.UUID{}, fmt.Errorf("corrupt server-id file at %s: %w", serverIDPath, err)
	}

	// File doesn't exist — first boot, generate a new server ID
	id := uuid.New()
	if err := os.WriteFile(serverIDPath, []byte(id.String()), 0o644); err != nil {
		return uuid.UUID{}, fmt.Errorf("failed to write server-id: %w", err)
	}

	slog.Info("generated new server ID", "server_id", id)
	return id, nil
}

// registerServer upserts this server into the servers table and allocates an IP range
// if this is a first-time registration.
func (s *Service) registerServer(ctx context.Context) (queries.Server, error) {
	hostname, _ := os.Hostname()

	// Check if this server already exists (restarting)
	existing, err := s.db.ServerFindByID(ctx, s.serverID)
	if err == nil {
		// Server exists — re-register (updates heartbeat + sets active)
		server, err := s.db.ServerRegister(ctx, queries.ServerRegisterParams{
			ID:         s.serverID,
			Hostname:   hostname,
			InternalIp: s.cfg.InternalIP,
			IpRange:    existing.IpRange,
		})
		if err != nil {
			return queries.Server{}, fmt.Errorf("failed to re-register server: %w", err)
		}
		slog.Info("re-registered server", "server_id", server.ID, "ip_range", server.IpRange)
		return server, nil
	}

	// New server — allocate an IP range within a transaction
	var server queries.Server
	err = s.db.WithTx(ctx, func(q *queries.Queries) error {
		// Allocate next /20 range
		ipRange, err := q.ServerAllocateIPRange(ctx)
		if err != nil {
			return fmt.Errorf("failed to allocate IP range: %w", err)
		}

		server, err = q.ServerRegister(ctx, queries.ServerRegisterParams{
			ID:         s.serverID,
			Hostname:   hostname,
			InternalIp: s.cfg.InternalIP,
			IpRange:    ipRange,
		})
		if err != nil {
			return fmt.Errorf("failed to register server: %w", err)
		}

		return nil
	})
	if err != nil {
		return queries.Server{}, err
	}

	slog.Info("registered new server", "server_id", server.ID, "ip_range", server.IpRange, "internal_ip", server.InternalIp)
	return server, nil
}

// heartbeatLoop sends periodic heartbeats to the database.
func (s *Service) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.db.ServerHeartbeat(ctx, s.serverID); err != nil {
				slog.Error("failed to send heartbeat", "err", err)
			}
		}
	}
}

// clusterDutyLoop tries to become the cluster leader using a session-scoped
// advisory lock on a dedicated database connection. If this server becomes the
// leader, it runs cluster-wide duties (dead server detection, failover) until
// the context is cancelled or the connection drops. If another server already
// holds the lock, this server retries periodically until it can acquire it.
func (s *Service) clusterDutyLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(leaderRetryInterval):
			s.tryBecomeClusterLeader(ctx)
		}
	}
}

// tryBecomeClusterLeader acquires a dedicated connection, tries to become the
// cluster leader, and runs cluster duties if successful. Returns when leadership
// is lost (connection error, context cancelled, etc.).
func (s *Service) tryBecomeClusterLeader(ctx context.Context) {
	conn, err := s.db.Pool.Acquire(ctx)
	if err != nil {
		slog.Error("failed to acquire connection for cluster leader election", "err", err)
		return
	}
	defer conn.Release()

	// Try to acquire a session-scoped advisory lock (non-blocking).
	// The lock is held as long as this connection stays open.
	q := queries.New(conn)
	acquired, err := q.TrySessionAdvisoryLock(ctx, "cluster_leader")
	if err != nil {
		slog.Error("failed to try cluster leader lock", "err", err)
		return
	}
	if !acquired {
		return // Another server is the leader
	}

	slog.Info("this server is now the cluster leader", "server_id", s.serverID)

	// Run cluster duties until context is cancelled
	ticker := time.NewTicker(deadDetectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.detectAndFailoverDeadServers(ctx); err != nil {
				slog.Error("dead server detection failed", "err", err)
			}
		}
	}
}

// detectAndFailoverDeadServers finds dead servers and replaces their VMs.
// Only called by the cluster leader. Everything runs in a single transaction:
// if any step fails, the whole operation rolls back and will be retried on
// the next tick.
func (s *Service) detectAndFailoverDeadServers(ctx context.Context) error {
	deadServers, err := s.db.ServerFindDead(ctx)
	if err != nil {
		return fmt.Errorf("failed to find dead servers: %w", err)
	}

	if len(deadServers) == 0 {
		return nil
	}

	return s.db.WithTx(ctx, func(q *queries.Queries) error {
		for _, deadServer := range deadServers {
			slog.Warn("detected dead server", "server_id", deadServer.ID, "hostname", deadServer.Hostname,
				"last_heartbeat", deadServer.LastHeartbeatAt)

			if err := q.ServerUpdateStatus(ctx, queries.ServerUpdateStatusParams{
				ID:     deadServer.ID,
				Status: queries.ServerStatusDead,
			}); err != nil {
				slog.Error("failed to mark server as dead", "server_id", deadServer.ID, "err", err)
				continue
			}

			// Replace VMs from this dead server
			if err := s.replaceDeadServerVMs(ctx, q, deadServer.ID); err != nil {
				slog.Error("failed to replace VMs from dead server", "server_id", deadServer.ID, "err", err)
			}
		}

		return nil
	})
}

// replaceDeadServerVMs soft-deletes VMs on a dead server and creates fresh
// replacements on healthy servers. Following the K8s pod model: VMs are
// disposable — don't mutate in place, delete and recreate.
//
// Runs inside the failover transaction so mark-dead + VM replacement is atomic.
func (s *Service) replaceDeadServerVMs(ctx context.Context, q *queries.Queries, deadServerID uuid.UUID) error {
	vms, err := q.VMFindByServerID(ctx, deadServerID)
	if err != nil {
		return fmt.Errorf("failed to find VMs on dead server: %w", err)
	}

	if len(vms) == 0 {
		slog.Info("no VMs to replace from dead server", "server_id", deadServerID)
		return nil
	}

	slog.Info("replacing VMs from dead server", "server_id", deadServerID, "vm_count", len(vms))

	for _, vm := range vms {
		// Skip VMs that are already in terminal state
		if vm.Status == queries.VmStatusStopped || vm.Status == queries.VmStatusFailed {
			continue
		}

		if err := s.replaceVM(ctx, q, vm, deadServerID); err != nil {
			return fmt.Errorf("failed to replace VM %s: %w", vm.ID, err)
		}
	}

	return nil
}

// replaceVM soft-deletes an old VM, creates a replacement on the least loaded
// server, and updates the deployment pointer.
func (s *Service) replaceVM(ctx context.Context, q *queries.Queries, oldVM queries.Vm, deadServerID uuid.UUID) error {
	// Soft-delete the old VM
	if err := q.VMSoftDelete(ctx, oldVM.ID); err != nil {
		return fmt.Errorf("failed to soft-delete old VM: %w", err)
	}

	// Find target server
	target, err := q.ServerFindLeastLoaded(ctx)
	if err != nil {
		return fmt.Errorf("no healthy server available: %w", err)
	}

	// Allocate IP on the target server
	ipAddress, err := q.VMNextIPAddress(ctx, queries.VMNextIPAddressParams{
		ServerID: target.ID,
		IpRange:  target.IpRange,
	})
	if err != nil {
		return fmt.Errorf("failed to allocate IP: %w", err)
	}

	// Create replacement VM
	newVM, err := q.VMCreate(ctx, queries.VMCreateParams{
		ID:           uuid.New(),
		Vcpus:        oldVM.Vcpus,
		Memory:       oldVM.Memory,
		Status:       queries.VmStatusPending,
		ImageID:      oldVM.ImageID,
		ServerID:     target.ID,
		Port:         oldVM.Port,
		IpAddress:    ipAddress,
		EnvVariables: oldVM.EnvVariables,
		Metadata:     nil,
	})
	if err != nil {
		return fmt.Errorf("failed to create replacement VM: %w", err)
	}

	// Update deployment to point to the new VM (if one exists)
	if dep, err := q.DeploymentFindByVMID(ctx, oldVM.ID); err == nil {
		if err := q.DeploymentUpdateVMID(ctx, queries.DeploymentUpdateVMIDParams{
			ID:   dep.ID,
			VmID: newVM.ID,
		}); err != nil {
			return fmt.Errorf("failed to update deployment: %w", err)
		}
	}

	slog.Info("replaced VM from dead server",
		"old_vm_id", oldVM.ID,
		"new_vm_id", newVM.ID,
		"from_server", deadServerID,
		"to_server", newVM.ServerID)
	return nil
}

// reconcileServer handles server-level reconciliation:
// - Syncs host routes so this server can reach VMs on the changed server
// - If the reconciled server is this server and it's draining, starts drain
func (s *Service) reconcileServer(ctx context.Context, objectID uuid.UUID) error {
	// Sync host routes - any server change may affect routing
	if err := s.syncHostRoutes(ctx); err != nil {
		return fmt.Errorf("failed to sync host routes: %w", err)
	}

	// Check if this server should drain (only relevant for our own server ID)
	if objectID == s.serverID {
		server, err := s.db.ServerFindByID(ctx, s.serverID)
		if err != nil {
			return fmt.Errorf("failed to check server status for drain: %w", err)
		}
		if server.Status == queries.ServerStatusDraining {
			slog.Info("server is draining, starting migration")
			s.drainServer(ctx)
		}
	}

	return nil
}

// drainServer migrates all running deployments to other servers, then marks itself as drained.
func (s *Service) drainServer(ctx context.Context) {
	// Find all running deployments on this server
	deployments, err := s.db.DeploymentFindRunningByServerID(ctx, s.serverID)
	if err != nil {
		slog.Error("failed to find running deployments for drain", "err", err)
		return
	}

	slog.Info("draining server", "deployment_count", len(deployments), "server_id", s.serverID)

	for _, dep := range deployments {
		if err := s.drainDeployment(ctx, dep); err != nil {
			slog.Error("failed to drain deployment", "deployment_id", dep.ID, "err", err)
			// Continue trying to drain other deployments
		}
	}

	// Kill any remaining non-deployment VMs (build VMs are ephemeral)
	vms, err := s.db.VMFindByServerID(ctx, s.serverID)
	if err != nil {
		slog.Error("failed to find remaining VMs for drain cleanup", "err", err)
	} else {
		for _, vm := range vms {
			if vm.Status != queries.VmStatusStopped && vm.Status != queries.VmStatusFailed {
				slog.Info("killing remaining VM during drain", "vm_id", vm.ID)
				if err := s.db.VMSoftDelete(ctx, vm.ID); err != nil {
					slog.Error("failed to soft-delete VM during drain", "vm_id", vm.ID, "err", err)
				}
			}
		}
	}

	// Mark server as drained
	if err := s.db.ServerSetDrained(ctx, s.serverID); err != nil {
		slog.Error("failed to mark server as drained", "err", err)
		return
	}

	slog.Info("server drain complete", "server_id", s.serverID)
}

// drainDeployment creates a replacement VM on another server, waits for health, then swaps.
func (s *Service) drainDeployment(ctx context.Context, dep queries.Deployment) error {
	if !dep.VmID.Valid || !dep.ImageID.Valid {
		return nil
	}

	oldVM, err := s.db.VMFirstByID(ctx, dep.VmID)
	if err != nil {
		return fmt.Errorf("failed to fetch old VM: %w", err)
	}

	// Create a replacement VM on a healthy server
	newVM, err := s.VMCreate(ctx, VMCreateParams{
		VCPUs:        oldVM.Vcpus,
		Memory:       oldVM.Memory,
		ImageID:      oldVM.ImageID,
		Port:         oldVM.Port.Int32,
		EnvVariables: oldVM.EnvVariables.String,
	})
	if err != nil {
		return fmt.Errorf("failed to create replacement VM: %w", err)
	}

	slog.Info("created replacement VM for drain",
		"deployment_id", dep.ID,
		"old_vm", oldVM.ID,
		"new_vm", newVM.ID,
		"new_server", newVM.ServerID)

	// Wait for the replacement VM to pass health checks
	if err := s.waitForVMHealth(ctx, newVM, drainHealthCheckTimeout); err != nil {
		// Cleanup the failed replacement
		_ = s.db.VMSoftDelete(ctx, newVM.ID)
		return fmt.Errorf("replacement VM failed health check: %w", err)
	}

	// Atomic swap: point the deployment to the new VM
	if err := s.db.DeploymentUpdateVMID(ctx, queries.DeploymentUpdateVMIDParams{
		ID:   dep.ID,
		VmID: newVM.ID,
	}); err != nil {
		return fmt.Errorf("failed to swap deployment VM: %w", err)
	}

	// Soft-delete the old VM (triggers cleanup via reconciler)
	if err := s.db.VMSoftDelete(ctx, oldVM.ID); err != nil {
		slog.Error("failed to soft-delete old VM after drain swap", "vm_id", oldVM.ID, "err", err)
	}

	slog.Info("drained deployment",
		"deployment_id", dep.ID,
		"old_vm", oldVM.ID,
		"new_vm", newVM.ID)

	// Notify route change so edge proxy picks up the new VM
	s.notifyRouteChange()

	return nil
}

// waitForVMHealth polls the VM's health endpoint until it responds or times out.
func (s *Service) waitForVMHealth(ctx context.Context, vm *queries.Vm, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("health check timed out after %s", timeout)
		case <-ticker.C:
			if s.checkDeploymentHealth(vm.IpAddress.Addr().String(), vm.Port.Int32) {
				return nil
			}
		}
	}
}

// notifyRouteChange sends a non-blocking signal on the route change channel.
func (s *Service) notifyRouteChange() {
	if s.routeChangeNotify != nil {
		select {
		case s.routeChangeNotify <- struct{}{}:
		default:
			// Channel full — a notification is already pending
		}
	}
}

// syncHostRoutes queries the servers table and configures kernel routes
// so this server can reach VMs on other servers via the VLAN.
// Uses vishvananda/netlink for direct netlink calls instead of shelling out.
func (s *Service) syncHostRoutes(ctx context.Context) error {
	servers, err := s.db.ServerFindActive(ctx)
	if err != nil {
		return fmt.Errorf("failed to find active servers: %w", err)
	}

	for _, server := range servers {
		// Skip self
		if server.ID == s.serverID {
			continue
		}

		// Convert netip.Prefix to *net.IPNet for the netlink API
		prefix := server.IpRange.Masked()
		dst := &net.IPNet{
			IP:   prefix.Addr().AsSlice(),
			Mask: net.CIDRMask(prefix.Bits(), prefix.Addr().BitLen()),
		}
		gw := net.ParseIP(server.InternalIp)

		// RouteReplace is idempotent — adds the route or updates if it exists.
		err := netlink.RouteReplace(&netlink.Route{
			Dst: dst,
			Gw:  gw,
		})
		if err != nil {
			slog.Error("failed to add host route",
				"range", server.IpRange,
				"via", server.InternalIp,
				"server_id", server.ID,
				"err", err)
		} else {
			slog.Debug("synced host route", "range", server.IpRange, "via", server.InternalIp)
		}
	}

	return nil
}
