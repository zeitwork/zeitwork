package zeitwork

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"slices"
	"time"

	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

func (s *Service) reconcileVM2(ctx context.Context, objectID uuid.UUID) error {
	vm, err := s.db.VMFirstByID(ctx, objectID)
	if err != nil {
		slog.Error("failed to find vm by id", "objectId", objectID)
		return err
	}

	if vm.DeletedAt.Valid {
		return s.reconcileVmDelete(ctx, vm)
	}

	// if the vm is currently pending, advance to status starting
	vm, err = s.reconcileVMUpdateStatusIf(ctx, vm, queries.VmStatusStarting, queries.VmStatusPending)
	if err != nil {
		return err
	}

	// if the vm already has a running cloud-hypervisor, skip
	if _, ok := s.vmToCmd[vm.ID]; ok {
		return nil
	}

	err = s.reconcileVMDownloadImage(ctx, vm)
	if err != nil {
		return err
	}

	err = s.reconcileVMEnsureWorkdisk(ctx, vm)
	if err != nil {
		return err
	}

	// let's go
	stdout, _ := os.OpenFile(fmt.Sprintf("/tmp/%s.out", vm.ID.String()), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	stderr, _ := os.OpenFile(fmt.Sprintf("/tmp/%s.err", vm.ID.String()), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	defer stderr.Close()
	defer stdout.Close()

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
		"--disk", fmt.Sprintf("path=/data/work/%s.qcow2,direct=on,queue_size=256", vm.ID.String()),
		"--initramfs", "/data/initramfs.cpio.gz",
		"--cmdline", fmt.Sprintf(
			"console=hvc0 config=%s",
			base64.StdEncoding.EncodeToString(vmConfigBytes)),
		"--cpus", fmt.Sprintf("boot=%d", vm.Vcpus),
		"--memory", fmt.Sprintf("size=%dM", vm.Memory),
		"--net", fmt.Sprintf("tap=tap%d,mac=,ip=%s,mask=255.255.255.254", s.nextTap.Add(1), hostIp.Addr())) // todo mask might not be /31 theoretically but who cares
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Start()
	if err != nil {
		return err
	}

	s.vmToCmd[vm.ID] = cmd
	go func() {
		err := cmd.Wait()
		slog.Error("hypervisor exited! ", "err", err)

		delete(s.vmToCmd, vm.ID)
		s.vmScheduler.Schedule(vm.ID, time.Now().Add(5*time.Second))
	}()

	return nil
}

func (s *Service) reconcileVMDownloadImage(ctx context.Context, vm queries.Vm) error {
	image, err := s.db.ImageFindByID(ctx, vm.ImageID)
	if err != nil {
		return err
	}

	// if the image already exists, skip
	_, err = os.Stat(fmt.Sprintf("/data/base/%s.qcow2", image.ID.String()))
	if err == nil {
		slog.Info("Image already exists!", "id", image.ID)
		return nil
	}

	// pull max one image at a time
	s.imageMu.Lock()
	defer s.imageMu.Unlock()

	// try to pull the image
	err = s.runCommand("skopeo", "copy", fmt.Sprintf("docker://%s/%s:%s", image.Registry, image.Repository, image.Tag), fmt.Sprintf("oci:%s:latest", image.ID.String()))
	if err != nil {
		return err
	}

	// extract the rootfs
	tmpdir, err := os.MkdirTemp("", "image")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	rootfs := tmpdir + "/"
	err = s.runCommand("umoci", "unpack", "--image", image.ID.String()+":latest", rootfs)
	if err != nil {
		return err
	}

	// pack the rootfs as qcow2
	err = s.runCommand("virt-make-fs", "--format=qcow2", "--type=ext4", rootfs, "--size=+5G", fmt.Sprintf("/data/base/%s.qcow2", image.ID.String()))
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) reconcileVMEnsureWorkdisk(ctx context.Context, vm queries.Vm) error {
	// skip if the workdisk already exists
	_, err := os.Stat(fmt.Sprintf("/data/work/%s.qcow2", vm.ID.String()))
	if err == nil {
		return nil
	}

	err = s.runCommand("qemu-img", "create", "-f", "qcow2", "-b", fmt.Sprintf("/data/base/%s.qcow2", vm.ImageID.String()), "-F", "qcow2", fmt.Sprintf("/data/work/%s.qcow2", vm.ID.String()))
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) reconcileVmDelete(ctx context.Context, vm queries.Vm) error {
	slog.Info("Reconcile Delete the vm", "name", vm.ID.String())
	// if the vm is currently running, kill it
	if app, ok := s.vmToCmd[vm.ID]; ok {
		if app.Process != nil {
			err := app.Process.Kill()
			if err != nil {
				return err
			}
		}
	}

	// cleanup the qcow2
	err := os.Remove(fmt.Sprintf("/data/work/%s.qcow2", vm.ID.String()))
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (s *Service) runCommand(name string, args ...string) error {
	slog.Info("Running command ", "name", name, "args", args)
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		slog.Error("Error while running command", "name", name, "err", err)
		return err
	}

	slog.Info("output of command", "name", name, "args", args, "output", string(out))

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

type VMConfig struct {
	AppID  string `json:"app_id"`
	IPAddr string `json:"ip_addr"`
	IPGw   string `json:"ip_gw"`
}
