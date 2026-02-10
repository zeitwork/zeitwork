package zeitwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/crypto"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type VMCreateParams struct {
	VCPUs        int32
	Memory       int32
	ImageID      uuid.UUID
	Port         int32
	EnvVariables string // Encrypted JSON array of "KEY=value" strings
}

func (s *Service) reconcileVM(ctx context.Context, objectID uuid.UUID) error {
	vm, err := s.db.VMFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	if vm.DeletedAt.Valid {
		return s.reconcileVmDelete(ctx, vm)
	}

	// ensure the image has a disk image
	image, err := s.db.ImageFindByID(ctx, vm.ImageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// skip if image does not (yet) have a disk image
	if !image.DiskImageKey.Valid {
		slog.Error("image has no disk image", "reconciler_name", "vm", "vm_id", vm.ID)
		return nil
	}

	// if the vm is currently pending, advance to status starting
	vm, err = s.reconcileVMUpdateStatusIf(ctx, vm, queries.VmStatusStarting, queries.VmStatusPending)
	if err != nil {
		return err
	}

	// Handle VMs that were stopped/failed due to service restart.
	// Reset them to starting so they can be restarted.
	// This is safe because build VMs are soft-deleted after use, and deployment VMs should always run.
	if vm.Status == queries.VmStatusFailed || vm.Status == queries.VmStatusStopped {
		slog.Info("recovering VM from stopped/failed state", "vm_id", vm.ID, "previous_status", vm.Status)
		vm, err = s.reconcileVMUpdateStatusIf(ctx, vm, queries.VmStatusStarting, queries.VmStatusFailed, queries.VmStatusStopped)
		if err != nil {
			return err
		}
	}

	// if the vm already has a running cloud-hypervisor, skip
	if _, ok := s.vmToCmd[vm.ID]; ok {
		vm, err = s.reconcileVMUpdateStatusIf(ctx, vm, queries.VmStatusRunning, queries.VmStatusStarting, queries.VmStatusPending)
		if err != nil {
			return err
		}

		return nil
	}

	// let's go
	stdout, err := os.OpenFile(fmt.Sprintf("/tmp/%s.out", vm.ID.String()), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open stdout file: %w", err)
	}
	stderr, err := os.OpenFile(fmt.Sprintf("/tmp/%s.err", vm.ID.String()), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		stdout.Close()
		return fmt.Errorf("failed to open stderr file: %w", err)
	}

	// Create per-VM CoW disk backed by the base image
	vmDiskPath := fmt.Sprintf("/data/work/%s.qcow2", vm.ID.String())
	baseDiskPath := fmt.Sprintf("/data/base/%s.qcow2", vm.ImageID.String())

	// Check if base image exists
	if _, err := os.Stat(baseDiskPath); os.IsNotExist(err) {
		return fmt.Errorf("base image does not exist: %s", baseDiskPath)
	}

	// Always recreate the CoW disk from the clean base image.
	// If the disk exists from a previous run, it may have a dirty ext4 journal
	// from an unclean shutdown (SIGKILL). A fresh CoW snapshot is instant and guaranteed clean.
	_ = os.Remove(vmDiskPath)
	err = s.runCommand("qemu-img", "create", "-f", "qcow2", "-b", baseDiskPath, "-F", "qcow2", vmDiskPath)
	if err != nil {
		return fmt.Errorf("failed to create VM disk: %w", err)
	}

	vmIp := vm.IpAddress
	hostIp := netip.PrefixFrom(vm.IpAddress.Addr().Prev(), vmIp.Bits())

	// Decrypt environment variables if present
	var envVars []string
	if vm.EnvVariables.Valid && vm.EnvVariables.String != "" {
		decryptedEnvJSON, err := crypto.Decrypt(vm.EnvVariables.String)
		if err != nil {
			return fmt.Errorf("failed to decrypt environment variables: %w", err)
		}
		if err := json.Unmarshal([]byte(decryptedEnvJSON), &envVars); err != nil {
			return fmt.Errorf("failed to unmarshal environment variables: %w", err)
		}
	}

	// Register VM with VSOCK manager (sets up UDS listener for guest-initiated connections)
	vsockPath := VSocketPath(vm.ID)
	hostname := fmt.Sprintf("zeit-%s", vm.ID.String())
	if err := s.vsockManager.RegisterVM(vm.ID, envVars, vmIp.String(), hostIp.Addr().String(), hostname); err != nil {
		return fmt.Errorf("failed to register VM with VSOCK manager: %w", err)
	}

	slog.Info("starting DA VM", "id", vm.ID, "hostIp", hostIp, "vmIp", vmIp, "vcpus", vm.Vcpus, "memory_mb", vm.Memory, "envVarsCount", len(envVars))

	cmd := exec.Command("/data/cloud-hypervisor", "--kernel", "/data/vmlinuz.bin",
		"--disk", fmt.Sprintf("path=%s,direct=on,queue_size=256", vmDiskPath),
		"--initramfs", "/data/initramfs.cpio.gz",
		"--cmdline", "console=hvc0",
		"--cpus", fmt.Sprintf("boot=%d", vm.Vcpus),
		"--memory", fmt.Sprintf("size=%dM", vm.Memory),
		"--net", fmt.Sprintf("tap=tap%d,mac=,ip=%s,mask=255.255.255.254", s.nextTap.Add(1), hostIp.Addr()), // todo mask might not be /31 theoretically but who cares
		"--vsock", fmt.Sprintf("cid=3,socket=%s", vsockPath))
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	slog.Info("Starting VM", "cmd", cmd)
	err = cmd.Start()
	if err != nil {
		slog.Error("failed to start hypervisor", "vm_id", vm.ID, "err", err)
		return err
	}
	slog.Info("about to update to running", "vm_id", vm.ID, "vm_status", vm.Status)
	vm, err = s.reconcileVMUpdateStatusIf(ctx, vm, queries.VmStatusRunning, queries.VmStatusStarting)
	if err != nil {
		slog.Error("failed to update to running", "vm_id", vm.ID, "err", err)
		return err
	}
	slog.Info("updated to running", "vm_id", vm.ID, "vm_status", vm.Status)

	s.vmToCmd[vm.ID] = cmd

	go func() {
		err := cmd.Wait()
		stdout.Close()
		stderr.Close()

		// Fetch current VM status before updating (avoid unnecessary DB update)
		currentVM, fetchErr := s.db.VMFirstByID(context.Background(), vm.ID)
		if fetchErr != nil {
			slog.Error("failed to fetch VM for status update", "vm", vm.ID.String(), "err", fetchErr)
		}

		if err != nil {
			slog.Error("hypervisor exited with error", "vm", vm.ID.String(), "err", err, "pid", cmd.Process.Pid, "processState", cmd.ProcessState.String())
			// Only update if status is not already failed
			if fetchErr == nil && currentVM.Status != queries.VmStatusFailed {
				_, updateErr := s.db.VMUpdateStatus(context.Background(), queries.VMUpdateStatusParams{
					Status: queries.VmStatusFailed,
					ID:     vm.ID,
				})
				if updateErr != nil {
					slog.Error("failed to update VM status to failed", "vm", vm.ID.String(), "err", updateErr)
				}
			}
		} else {
			slog.Info("hypervisor exited cleanly", "vm", vm.ID.String())
			// Only update if status is not already stopped
			if fetchErr == nil && currentVM.Status != queries.VmStatusStopped {
				_, updateErr := s.db.VMUpdateStatus(context.Background(), queries.VMUpdateStatusParams{
					Status: queries.VmStatusStopped,
					ID:     vm.ID,
				})
				if updateErr != nil {
					slog.Error("failed to update VM status to stopped", "vm", vm.ID.String(), "err", updateErr)
				}
			}
		}

		// Unregister from VSOCK manager (connection is dead when VM exits)
		s.vsockManager.UnregisterVM(vm.ID)

		delete(s.vmToCmd, vm.ID)
		s.vmScheduler.Schedule(vm.ID, time.Now().Add(5*time.Second))
	}()

	return nil
}

func (s *Service) VMCreate(ctx context.Context, params VMCreateParams) (*queries.Vm, error) {
	var lastErr error

	// todo: Wrap into TX for pg_advisory_xact_lock to work correctly.
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			slog.Warn("retrying VM IP allocation", "attempt", attempt+1)
			time.Sleep(100 * time.Millisecond)
		}

		ipAddress, err := s.nextIpAddress(ctx)
		if err != nil {
			return nil, err
		}

		vm, err := s.db.VMCreate(ctx, queries.VMCreateParams{
			ID:           uuid.New(),
			Vcpus:        params.VCPUs,
			Memory:       params.Memory,
			Status:       queries.VmStatusPending,
			ImageID:      params.ImageID,
			Port:         pgtype.Int4{Int32: params.Port, Valid: true},
			IpAddress:    ipAddress,
			EnvVariables: pgtype.Text{String: params.EnvVariables, Valid: true},
			Metadata:     nil,
		})
		if err != nil {
			if isIPConflictError(err) {
				lastErr = err
				continue
			}
			return nil, err
		}

		return &vm, nil
	}
	return nil, fmt.Errorf("failed to allocate IP after 5 attempts: %w", lastErr)
}

// isIPConflictError checks if the error is an exclusion constraint violation (SQLSTATE 23P01)
func isIPConflictError(err error) bool {
	return strings.Contains(err.Error(), "23P01") ||
		strings.Contains(err.Error(), "exclude_overlapping_networks")
}

func (s *Service) nextIpAddress(ctx context.Context) (netip.Prefix, error) {
	res, err := s.db.VMNextIPAddress(ctx)
	if err != nil {
		return netip.Prefix{}, err
	}
	// The inet type comes back as netip.Prefix when scanned into interface{}
	prefix, ok := res.(netip.Prefix)
	if !ok {
		return netip.Prefix{}, fmt.Errorf("unexpected type for next_ip: %T", res)
	}
	return prefix, nil
}

func (s *Service) reconcileVmDelete(ctx context.Context, vm queries.Vm) error {
	slog.Info("deleting VM", "vm_id", vm.ID.String())

	// Unregister VM from VSOCK manager (stops gRPC listener, cleans up UDS sockets)
	s.vsockManager.UnregisterVM(vm.ID)

	// If the VM is currently running, kill it
	if cmd, ok := s.vmToCmd[vm.ID]; ok {
		if cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil {
				slog.Error("failed to kill VM process", "vm_id", vm.ID.String(), "err", err)
			}
		}
		delete(s.vmToCmd, vm.ID)
	}

	// Cleanup the work disk
	vmDiskPath := fmt.Sprintf("/data/work/%s.qcow2", vm.ID.String())
	if err := os.Remove(vmDiskPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove VM disk: %w", err)
	}

	return nil
}

func (s *Service) runCommand(name string, args ...string) error {
	slog.Info("Running command", "name", name, "args", args)
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("Error running command", "name", name, "err", err, "output", string(out))
		return err
	}
	slog.Info("command output", "name", name, "args", args, "output", string(out))
	return nil
}

func (s *Service) reconcileVMUpdateStatusIf(ctx context.Context, vm queries.Vm, statusAfter queries.VmStatus, statusBefore ...queries.VmStatus) (queries.Vm, error) {
	if slices.Contains(statusBefore, vm.Status) {
		return s.db.VMUpdateStatus(ctx, queries.VMUpdateStatusParams{
			Status: statusAfter,
			ID:     vm.ID,
		})
	}
	return vm, nil
}
