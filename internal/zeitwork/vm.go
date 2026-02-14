package zeitwork

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

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
	EnvVariables string    // Encrypted JSON array of "KEY=value" strings
	ServerID     uuid.UUID // Explicit server placement (zero value = auto-place)
}

func (s *Service) reconcileVM(ctx context.Context, objectID uuid.UUID) error {
	vm, err := s.db.VMFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	// Only reconcile VMs belonging to this server
	if vm.ServerID != s.serverID {
		return nil
	}

	if vm.DeletedAt.Valid {
		return s.reconcileVmDelete(ctx, vm)
	}

	// ensure the image has a disk image
	image, err := s.db.ImageFindByID(ctx, vm.ImageID)
	if err != nil {
		return err
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
	err = s.reconcileVMBaseImage(ctx, vm, image)
	if err != nil {
		return err
	}

	err = s.reconcileVMWorkImage(ctx, vm, image)
	if err != nil {
		return err
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
		"--disk", fmt.Sprintf("path=/data/work/%s.qcow2,direct=on,queue_size=256", vm.ID.String()),
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
	// Determine target server for placement
	targetServerID := params.ServerID
	var targetIPRange netip.Prefix

	if targetServerID == (uuid.UUID{}) {
		// Auto-place on least loaded server
		target, err := s.db.ServerFindLeastLoaded(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to find server for VM placement: %w", err)
		}
		targetServerID = target.ID
		targetIPRange = target.IpRange
	} else {
		// Explicit placement — look up the server's IP range
		server, err := s.db.ServerFindByID(ctx, targetServerID)
		if err != nil {
			return nil, fmt.Errorf("failed to find target server: %w", err)
		}
		targetIPRange = server.IpRange
	}

	// Allocate IP and create VM in a single transaction so the
	// pg_advisory_xact_lock in VMNextIPAddress serializes correctly.
	var vm queries.Vm
	err := s.db.WithTx(ctx, func(q *queries.Queries) error {
		ipAddress, err := q.VMNextIPAddress(ctx, queries.VMNextIPAddressParams{
			ServerID: targetServerID,
			IpRange:  targetIPRange,
		})
		if err != nil {
			return fmt.Errorf("failed to allocate IP: %w", err)
		}

		vm, err = q.VMCreate(ctx, queries.VMCreateParams{
			ID:           uuid.New(),
			Vcpus:        params.VCPUs,
			Memory:       params.Memory,
			Status:       queries.VmStatusPending,
			ImageID:      params.ImageID,
			ServerID:     targetServerID,
			Port:         pgtype.Int4{Int32: params.Port, Valid: true},
			IpAddress:    ipAddress,
			EnvVariables: pgtype.Text{String: params.EnvVariables, Valid: true},
			Metadata:     nil,
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	return &vm, nil
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

func (s *Service) reconcileVMBaseImage(ctx context.Context, vm queries.Vm, image queries.Image) error {
	baseImagePath := fmt.Sprintf("/data/base/%s.qcow2", image.ID.String())

	// check if the image already exists on the server
	_, err := os.Stat(baseImagePath)
	if err == nil {
		// the image exist, nothing to do.
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	// download the image from source repo
	tmpdir, err := os.MkdirTemp("", "image")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	imageRef := fmt.Sprintf("%s/%s:%s", image.Registry, image.Repository, image.Tag)
	ociPath := filepath.Join(tmpdir, "oci")
	if strings.Index(imageRef, "ghcr.io/zeitwork") == 0 {
		srcCreds := fmt.Sprintf("%s:%s", s.cfg.DockerRegistryUsername, s.cfg.DockerRegistryPAT)
		err = s.runCommand("skopeo", "copy", "--src-creds", srcCreds, fmt.Sprintf("docker://%s", imageRef), fmt.Sprintf("oci:%s:latest", ociPath))
	} else {
		err = s.runCommand("skopeo", "copy", fmt.Sprintf("docker://%s", imageRef), fmt.Sprintf("oci:%s:latest", ociPath))
	}
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// unpack the oci image
	bundlePath := filepath.Join(tmpdir, "bundle")
	err = s.runCommand("umoci", "unpack", "--image", ociPath+":latest", bundlePath)
	if err != nil {
		return fmt.Errorf("failed to unpack OCI image: %w", err)
	}

	// convert bundle to qcow2
	err = s.runCommand("virt-make-fs", "--format=qcow2", "--type=ext4", "--size=+5G", bundlePath, baseImagePath)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) reconcileVMWorkImage(ctx context.Context, vm queries.Vm, image queries.Image) error {
	baseImagePath := fmt.Sprintf("/data/base/%s.qcow2", image.ID.String())
	workImagePath := fmt.Sprintf("/data/work/%s.qcow2", vm.ID.String())

	_ = os.Remove(workImagePath)
	err := s.runCommand("qemu-img", "create", "-f", "qcow2", "-b", baseImagePath, "-F", "qcow2", workImagePath)
	if err != nil {
		return fmt.Errorf("failed to create VM disk: %w", err)
	}

	return nil
}
