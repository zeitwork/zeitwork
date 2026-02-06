package zeitwork

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

// ServerIDFilePath is the path where the server ID is persisted
const ServerIDFilePath = "/data/server-id"

// LoadOrCreateServerID loads the server ID from disk, or creates a new one
func LoadOrCreateServerID() (uuid.UUID, error) {
	data, err := os.ReadFile(ServerIDFilePath)
	if err == nil {
		id, err := uuid.Parse(strings.TrimSpace(string(data)))
		if err == nil {
			return id, nil
		}
		slog.Warn("invalid server-id file, generating new one", "error", err)
	}

	// Generate new server ID
	id := uuid.New()
	if err := os.WriteFile(ServerIDFilePath, []byte(id.String()+"\n"), 0600); err != nil {
		return uuid.UUID{}, fmt.Errorf("failed to write server-id file: %w", err)
	}
	slog.Info("generated new server ID", "server_id", id)
	return id, nil
}

// registerServer registers this server in the database.
// If the server already exists (restart), it updates the heartbeat and status to active.
// If it's a brand new server, it allocates an IP range.
func (s *Service) registerServer(ctx context.Context) error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Try to find existing server record first (to preserve IP range across restarts)
	existingServer, err := s.db.ServerFindByID(ctx, s.serverID)
	if err == nil {
		// Server exists -- re-register (update heartbeat + status to active)
		server, err := s.db.ServerRegister(ctx, queries.ServerRegisterParams{
			ID:         s.serverID,
			Hostname:   hostname,
			InternalIp: s.cfg.InternalIP,
			IpRange:    existingServer.IpRange, // preserve existing IP range
		})
		if err != nil {
			return fmt.Errorf("failed to re-register server: %w", err)
		}
		s.server = server
		slog.Info("server re-registered", "server_id", s.serverID, "hostname", hostname, "ip_range", server.IpRange)
		return nil
	}

	// New server -- allocate an IP range
	ipRangeRaw, err := s.db.ServerAllocateIPRange(ctx)
	if err != nil {
		return fmt.Errorf("failed to allocate IP range: %w", err)
	}
	ipRange, ok := ipRangeRaw.(netip.Prefix)
	if !ok {
		// Try string conversion as fallback
		ipRangeStr, ok := ipRangeRaw.(string)
		if !ok {
			return fmt.Errorf("unexpected type for ip_range: %T", ipRangeRaw)
		}
		ipRange, err = netip.ParsePrefix(strings.TrimSpace(ipRangeStr))
		if err != nil {
			return fmt.Errorf("failed to parse allocated IP range %q: %w", ipRangeStr, err)
		}
	}

	server, err := s.db.ServerRegister(ctx, queries.ServerRegisterParams{
		ID:         s.serverID,
		Hostname:   hostname,
		InternalIp: s.cfg.InternalIP,
		IpRange:    ipRange,
	})
	if err != nil {
		return fmt.Errorf("failed to register server: %w", err)
	}

	s.server = server
	slog.Info("server registered", "server_id", s.serverID, "hostname", hostname, "ip_range", ipRange)
	return nil
}

// heartbeatLoop sends periodic heartbeats to the database
func (s *Service) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.db.ServerHeartbeat(ctx, s.serverID); err != nil {
				slog.Error("failed to send heartbeat", "error", err)
			}
		}
	}
}

// deadServerDetectionLoop periodically checks for dead servers and reassigns their VMs.
// Uses an advisory lock so only one server performs failover at a time.
func (s *Service) deadServerDetectionLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.detectAndFailoverDeadServers(ctx)
		}
	}
}

func (s *Service) detectAndFailoverDeadServers(ctx context.Context) {
	// Use a transaction with advisory lock so only one server runs failover
	err := s.db.WithTx(ctx, func(q *queries.Queries) error {
		// Try to acquire failover lock (non-blocking)
		locked, err := q.TryAdvisoryLock(ctx, "dead_server_failover")
		if err != nil || !locked {
			return nil // another server is handling failover
		}

		deadServers, err := q.ServerFindDead(ctx)
		if err != nil {
			return fmt.Errorf("failed to find dead servers: %w", err)
		}

		for _, dead := range deadServers {
			slog.Warn("detected dead server", "server_id", dead.ID, "hostname", dead.Hostname, "last_heartbeat", dead.LastHeartbeatAt)

			// Mark as dead
			if err := q.ServerUpdateStatus(ctx, queries.ServerUpdateStatusParams{
				ID:     dead.ID,
				Status: queries.ServerStatusDead,
			}); err != nil {
				slog.Error("failed to mark server as dead", "server_id", dead.ID, "error", err)
				continue
			}

			// Reassign orphaned VMs
			orphanedVMs, err := q.VMFindByServerID(ctx, dead.ID)
			if err != nil {
				slog.Error("failed to find orphaned VMs", "server_id", dead.ID, "error", err)
				continue
			}

			for _, vm := range orphanedVMs {
				if vm.DeletedAt.Valid {
					continue
				}
				// Find a healthy server to place this VM on
				targetServer, err := q.ServerFindLeastLoaded(ctx)
				if err != nil {
					slog.Error("failed to find target server for reassignment", "vm_id", vm.ID, "error", err)
					continue
				}

				// Allocate a new IP in the target server's range
				newIPRaw, err := q.VMNextIPAddress(ctx, queries.VMNextIPAddressParams{
					ServerID: targetServer.ID,
					Column2:  targetServer.IpRange,
				})
				if err != nil {
					slog.Error("failed to allocate new IP for reassignment", "vm_id", vm.ID, "error", err)
					continue
				}
				newIP, ok := newIPRaw.(netip.Prefix)
				if !ok {
					slog.Error("unexpected type for new IP", "vm_id", vm.ID, "type", fmt.Sprintf("%T", newIPRaw))
					continue
				}

				// Reassign the VM
				_, err = q.VMReassign(ctx, queries.VMReassignParams{
					ID:        vm.ID,
					ServerID:  targetServer.ID,
					IpAddress: newIP,
				})
				if err != nil {
					slog.Error("failed to reassign VM", "vm_id", vm.ID, "target_server", targetServer.ID, "error", err)
					continue
				}
				slog.Info("reassigned orphaned VM", "vm_id", vm.ID, "from_server", dead.ID, "to_server", targetServer.ID, "new_ip", newIP)
			}
		}
		return nil
	})
	if err != nil {
		slog.Error("dead server detection error", "error", err)
	}
}

// drainMonitorLoop monitors if this server has been marked as draining in the database.
// When draining is detected, it migrates all VMs off this server and then marks itself drained.
func (s *Service) drainMonitorLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			server, err := s.db.ServerFindByID(ctx, s.serverID)
			if err != nil {
				slog.Error("drain monitor: failed to fetch server status", "error", err)
				continue
			}

			if server.Status == queries.ServerStatusDraining {
				slog.Info("drain detected, migrating VMs off this server")
				s.drainServer(ctx)
			}
		}
	}
}

// drainServer performs a zero-downtime drain of this server.
//
// For deployment VMs: creates a replacement VM on a healthy server, waits for it
// to become running and pass health checks, then swaps the deployment's vm_id
// and soft-deletes the old VM. Traffic cuts over with zero downtime.
//
// For non-deployment VMs (build VMs): just soft-deletes them. Builds are ephemeral
// and will be retried by the build reconciler.
func (s *Service) drainServer(ctx context.Context) {
	// Step 1: Handle running deployments with zero-downtime replacement
	deployments, err := s.db.DeploymentFindRunningByServerID(ctx, s.serverID)
	if err != nil {
		slog.Error("drain: failed to find running deployments", "error", err)
		return
	}

	for _, dep := range deployments {
		if !dep.VmID.Valid || !dep.ImageID.Valid {
			continue
		}

		oldVM, err := s.db.VMFirstByID(ctx, dep.VmID)
		if err != nil {
			slog.Error("drain: failed to fetch old VM", "deployment_id", dep.ID, "vm_id", dep.VmID, "error", err)
			continue
		}

		// Create replacement VM on a healthy server (auto-placed via ServerFindLeastLoaded,
		// which excludes draining servers). Reuse same image, port, env vars.
		slog.Info("drain: creating replacement VM for deployment", "deployment_id", dep.ID, "old_vm_id", oldVM.ID)

		newVM, err := s.VMCreate(ctx, VMCreateParams{
			VCPUs:          oldVM.Vcpus,
			Memory:         oldVM.Memory,
			ImageID:        oldVM.ImageID,
			Port:           oldVM.Port.Int32,
			EnvVariables:   oldVM.EnvVariables.String,
			DeploymentID:   dep.ID,
			OrganisationID: dep.OrganisationID,
		})
		if err != nil {
			slog.Error("drain: failed to create replacement VM", "deployment_id", dep.ID, "error", err)
			continue
		}
		slog.Info("drain: replacement VM created, waiting for it to become running",
			"deployment_id", dep.ID, "new_vm_id", newVM.ID, "new_server_id", newVM.ServerID)

		// Wait for the replacement VM to become running (poll with timeout)
		healthy := false
		for attempt := 0; attempt < 60; attempt++ { // up to 5 minutes (60 * 5s)
			time.Sleep(5 * time.Second)

			vm, err := s.db.VMFirstByID(ctx, newVM.ID)
			if err != nil {
				slog.Error("drain: failed to check replacement VM status", "vm_id", newVM.ID, "error", err)
				continue
			}

			if vm.Status == queries.VmStatusFailed {
				slog.Error("drain: replacement VM failed", "vm_id", newVM.ID)
				break
			}

			if vm.Status == queries.VmStatusRunning {
				// VM is running -- perform health check
				if s.checkDeploymentHealth(vm.IpAddress.Addr().String(), vm.Port.Int32) {
					healthy = true
					slog.Info("drain: replacement VM is healthy", "vm_id", newVM.ID)
					break
				}
				slog.Debug("drain: replacement VM running but health check failed, retrying", "vm_id", newVM.ID)
			}
		}

		if !healthy {
			slog.Error("drain: replacement VM never became healthy, skipping deployment", "deployment_id", dep.ID, "new_vm_id", newVM.ID)
			// Soft-delete the failed replacement VM
			s.db.VMSoftDelete(ctx, newVM.ID)
			continue
		}

		// Swap: point deployment to the new VM
		err = s.db.DeploymentUpdateVMID(ctx, queries.DeploymentUpdateVMIDParams{
			ID:   dep.ID,
			VmID: newVM.ID,
		})
		if err != nil {
			slog.Error("drain: failed to update deployment vm_id", "deployment_id", dep.ID, "error", err)
			continue
		}

		// Soft-delete the old VM (triggers cleanup via VM reconciler)
		if err := s.db.VMSoftDelete(ctx, oldVM.ID); err != nil {
			slog.Error("drain: failed to soft-delete old VM", "vm_id", oldVM.ID, "error", err)
		}

		// Kill local process for the old VM
		if cmd, ok := s.vmToCmd[oldVM.ID]; ok {
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			delete(s.vmToCmd, oldVM.ID)
		}

		slog.Info("drain: deployment migrated with zero downtime",
			"deployment_id", dep.ID, "old_vm_id", oldVM.ID, "new_vm_id", newVM.ID)
	}

	// Step 2: Handle non-deployment VMs (build VMs etc.) -- just kill and soft-delete
	remainingVMs, err := s.db.VMFindByServerID(ctx, s.serverID)
	if err != nil {
		slog.Error("drain: failed to find remaining VMs", "error", err)
		return
	}

	activeCount := 0
	for _, vm := range remainingVMs {
		if vm.DeletedAt.Valid {
			continue
		}
		activeCount++

		// Kill local process
		if cmd, ok := s.vmToCmd[vm.ID]; ok {
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			delete(s.vmToCmd, vm.ID)
		}

		// Soft-delete the VM
		if err := s.db.VMSoftDelete(ctx, vm.ID); err != nil {
			slog.Error("drain: failed to soft-delete non-deployment VM", "vm_id", vm.ID, "error", err)
		} else {
			slog.Info("drain: soft-deleted non-deployment VM", "vm_id", vm.ID)
		}
	}

	// Step 3: Check if fully drained
	finalVMs, err := s.db.VMFindByServerID(ctx, s.serverID)
	if err != nil {
		slog.Error("drain: failed to check final VM count", "error", err)
		return
	}

	finalActive := 0
	for _, vm := range finalVMs {
		if !vm.DeletedAt.Valid {
			finalActive++
		}
	}

	if finalActive == 0 {
		slog.Info("drain: all VMs migrated/deleted, marking server as drained")
		if err := s.db.ServerSetDrained(ctx, s.serverID); err != nil {
			slog.Error("drain: failed to mark server as drained", "error", err)
		}
	} else {
		slog.Info("drain: still has active VMs, will retry next cycle", "remaining", finalActive)
	}
}
