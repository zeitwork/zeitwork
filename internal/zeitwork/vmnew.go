package zeitwork

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type VMCreateParams struct {
	VCPUs   int32
	Memory  int32
	ImageID uuid.UUID
	Port    int32
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

	// if the vm already has a running cloud-hypervisor, skip
	if _, ok := s.vmToCmd[vm.ID]; ok {
		vm, err = s.reconcileVMUpdateStatusIf(ctx, vm, queries.VmStatusRunning, queries.VmStatusStarting)
		if err != nil {
			return err
		}
		vm, err = s.reconcileVMUpdateStatusIf(ctx, vm, queries.VmStatusRunning, queries.VmStatusPending)
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

	vmIp := vm.IpAddress
	hostIp := netip.PrefixFrom(vm.IpAddress.Addr().Prev(), vmIp.Bits())

	slog.Info("STARTING DA VM", "id", vm.ID, "hostIp", hostIp, "vmIp", vmIp)
	vmConfig := VMConfig{
		AppID:  vm.ID.String(),
		IPAddr: vmIp.String(),
		IPGw:   hostIp.Addr().String(),
	}
	vmConfigBytes, err := json.Marshal(vmConfig)
	if err != nil {
		return err
	}

	cmd := exec.Command("/usr/local/bin/cloud-hypervisor", "--kernel", "/root/linux-cloud-hypervisor/arch/x86/boot/compressed/vmlinux.bin",
		"--disk", fmt.Sprintf("path=/data/work/%s.qcow2,direct=on,queue_size=256", vm.ImageID.String()),
		"--initramfs", "/data/initramfs.cpio.gz",
		"--cmdline", fmt.Sprintf(
			"console=hvc0 config=%s",
			base64.StdEncoding.EncodeToString(vmConfigBytes)),
		"--cpus", "boot=4",
		"--memory", "size=1024M",
		"--net", fmt.Sprintf("tap=tap%d,mac=,ip=%s,mask=255.255.255.254", s.nextTap.Add(1), hostIp.Addr())) // todo mask might not be /31 theoretically but who cares
	cmd.Stdout = stdout
	cmd.Stderr = stderr
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

		if err != nil {
			slog.Error("hypervisor exited with error", "vm", vm.ID.String(), "err", err)
			_, err = s.db.VMUpdateStatus(context.Background(), queries.VMUpdateStatusParams{
				Status: queries.VmStatusFailed,
				ID:     vm.ID,
			})
			if err != nil {
				slog.Error("failed to update VM status to failed", "vm", vm.ID.String(), "err", err)
			}
		} else {
			slog.Info("hypervisor exited cleanly", "vm", vm.ID.String())
			_, err = s.db.VMUpdateStatus(context.Background(), queries.VMUpdateStatusParams{
				Status: queries.VmStatusStopped,
				ID:     vm.ID,
			})
			if err != nil {
				slog.Error("failed to update VM status to stopped", "vm", vm.ID.String(), "err", err)
			}
		}

		delete(s.vmToCmd, vm.ID)
		s.vmScheduler.Schedule(vm.ID, time.Now().Add(5*time.Second))
	}()

	return nil
}

func (s *Service) VMCreate(ctx context.Context, params VMCreateParams) (*queries.Vm, error) {
	ipAdress, err := s.nextIpAddress(ctx)
	if err != nil {
		return nil, err
	}
	vm, err := s.db.VMCreate(ctx, queries.VMCreateParams{
		ID:        uuid.New(),
		Vcpus:     params.VCPUs,
		Memory:    params.Memory,
		Status:    queries.VmStatusPending,
		ImageID:   params.ImageID,
		Port:      pgtype.Int4{Int32: params.Port, Valid: true},
		IpAddress: ipAdress,
		Metadata:  nil,
	})

	return &vm, err
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
