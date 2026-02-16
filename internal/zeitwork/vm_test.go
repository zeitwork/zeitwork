package zeitwork_test

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/testsuite"
)

func (s *Suite) Test_CreateVMSimple() {
	// Test: Create a simple vm. The VM should be running on the server. The HTTP-Port should be reachable
	vm := s.CreateVM(testsuite.CreateVMArgs{
		Registry:   "docker.io",
		Repository: "traefik/whoami",
		Tag:        "latest",
		Port:       80,
		Server:     testsuite.Server1ID,
	})
	slog.Info("Created VM", "id", vm.ID)

	s.WaitUntil(func() bool {
		vm, err := s.DB.VMFirstByID(s.Context(), vm.ID)
		s.NoError(err)

		// wait until vm is status running
		if vm.Status != queries.VmStatusRunning {
			slog.Warn("Not running")
			return false
		}

		// ensure we can ping the vm
		_, err = s.TryRunCommand("ping", "-c", "1", "-W", "2", vm.IpAddress.Addr().String())
		if err != nil {
			slog.Warn("Not pingable")
			return false
		}

		// ensure we can curl
		_, err = s.TryRunCommand("curl", "-f", "-s", "--connect-timeout", "2", fmt.Sprintf("http://%s:%d", vm.IpAddress.Addr(), vm.Port.Int32))
		if err != nil {
			slog.Warn("Not curlable")
			return false
		}

		return true
	})

	// delete the vm
	s.DeleteVM(vm.ID)

	// there should not be any cloud hypervisor processes or taps left.
	output, err := s.TryRunCommand("bash", "-c", "ps aux | grep [c]loud-hypervisor")
	s.Error(err)
	s.Empty(output)

	output, err = s.TryRunCommand("bash", "-c", "ip a | grep ztap")
	s.Error(err)
	s.Empty(output)
}

func (s *Suite) Test_CrossServerVM() {
	// Create one VM on server1, one on server2
	vm1 := s.CreateVM(testsuite.CreateVMArgs{
		Registry:   "docker.io",
		Repository: "traefik/whoami",
		Tag:        "latest",
		Port:       80,
		Server:     testsuite.Server1ID,
	})
	slog.Info("Created VM on server1", "id", vm1.ID)

	vm2 := s.CreateVM(testsuite.CreateVMArgs{
		Registry:   "docker.io",
		Repository: "traefik/whoami",
		Tag:        "latest",
		Port:       80,
		Server:     testsuite.Server2ID,
	})
	slog.Info("Created VM on server2", "id", vm2.ID)

	// Wait for vm1 to be running and reachable
	s.WaitUntil(func() bool {
		vm, err := s.DB.VMFirstByID(s.Context(), vm1.ID)
		s.NoError(err)
		if vm.Status != queries.VmStatusRunning {
			slog.Warn("VM1 not running", "status", vm.Status)
			return false
		}
		_, err = s.TryRunCommand("curl", "-f", "-s", "--connect-timeout", "2",
			fmt.Sprintf("http://%s:%d", vm.IpAddress.Addr(), vm.Port.Int32))
		if err != nil {
			slog.Warn("VM1 not curlable")
			return false
		}
		return true
	})

	// Wait for vm2 to be running and reachable
	s.WaitUntil(func() bool {
		vm, err := s.DB.VMFirstByID(s.Context(), vm2.ID)
		s.NoError(err)
		if vm.Status != queries.VmStatusRunning {
			slog.Warn("VM2 not running", "status", vm.Status)
			return false
		}
		_, err = s.TryRunCommand("curl", "-f", "-s", "--connect-timeout", "2",
			fmt.Sprintf("http://%s:%d", vm.IpAddress.Addr(), vm.Port.Int32))
		if err != nil {
			slog.Warn("VM2 not curlable")
			return false
		}
		return true
	})

	// Verify server1 can curl both VMs
	vm1Addr := fmt.Sprintf("http://%s:%d", vm1.IpAddress.Addr(), vm1.Port.Int32)
	vm2Addr := fmt.Sprintf("http://%s:%d", vm2.IpAddress.Addr(), vm2.Port.Int32)

	_, err := s.TryRunCommand("curl", "-f", "-s", "--connect-timeout", "2", vm1Addr)
	s.NoErrorf(err, "server1 should reach vm1 at %s", vm1Addr)

	_, err = s.TryRunCommand("curl", "-f", "-s", "--connect-timeout", "2", vm2Addr)
	s.NoErrorf(err, "server1 should reach vm2 at %s", vm2Addr)

	// Verify server2 can curl both VMs
	_, err = s.TryRunCommandRemote(testsuite.Server2IP, "curl", "-f", "-s", "--connect-timeout", "2", vm1Addr)
	s.NoErrorf(err, "server2 should reach vm1 at %s", vm1Addr)

	_, err = s.TryRunCommandRemote(testsuite.Server2IP, "curl", "-f", "-s", "--connect-timeout", "2", vm2Addr)
	s.NoErrorf(err, "server2 should reach vm2 at %s", vm2Addr)

	// Delete both VMs
	s.DeleteVM(vm1.ID)
	s.DeleteVM(vm2.ID)

	// Verify no leftover cloud-hypervisor processes or tap devices on server1
	output, err := s.TryRunCommand("bash", "-c", "ps aux | grep [c]loud-hypervisor")
	s.Error(err)
	s.Empty(output)

	output, err = s.TryRunCommand("bash", "-c", "ip a | grep ztap")
	s.Error(err)
	s.Empty(output)

	// Verify no leftover cloud-hypervisor processes or tap devices on server2
	output, err = s.TryRunCommandRemote(testsuite.Server2IP, "bash -c 'ps aux | grep [c]loud-hypervisor'")
	s.Error(err)
	s.Empty(output)

	output, err = s.TryRunCommandRemote(testsuite.Server2IP, "bash -c 'ip a | grep ztap'")
	s.Error(err)
	s.Empty(output)
}

func (s *Suite) Test_SpamSchedule() {
	// Test: spamming updated_at on a VM should not cause duplicate cloud-hypervisor processes.
	// Each update triggers a WAL event and reconciliation — the scheduler must handle this gracefully.
	vm := s.CreateVM(testsuite.CreateVMArgs{
		Registry:   "docker.io",
		Repository: "traefik/whoami",
		Tag:        "latest",
		Port:       80,
		Server:     testsuite.Server1ID,
	})
	slog.Info("Created VM", "id", vm.ID)

	// Immediately spam 50 updates to updated_at, each triggering a WAL notification
	for i := 0; i < 200; i++ {
		_, err := s.DB.Pool.Exec(s.Context(), "UPDATE vms SET updated_at = now() WHERE id = $1", vm.ID)
		s.NoErrorf(err, "spam update %d failed", i)
	}
	slog.Info("Finished spamming 50 updated_at writes", "vm", vm.ID)

	// Wait for the VM to be running and curlable despite the spam
	s.WaitUntil(func() bool {
		vm, err := s.DB.VMFirstByID(s.Context(), vm.ID)
		s.NoError(err)
		if vm.Status != queries.VmStatusRunning {
			slog.Warn("Not running", "status", vm.Status)
			return false
		}
		_, err = s.TryRunCommand("curl", "-f", "-s", "--connect-timeout", "2",
			fmt.Sprintf("http://%s:%d", vm.IpAddress.Addr(), vm.Port.Int32))
		if err != nil {
			slog.Warn("Not curlable")
			return false
		}
		return true
	})

	// give the reconcilers time
	time.Sleep(5 * time.Second)

	// The critical assertion: there must be exactly one cloud-hypervisor process for this VM.
	output, _ := s.TryRunCommand("bash", "-c", "ps aux | grep [c]loud-hypervisor")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	// Filter out empty lines
	var processLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			processLines = append(processLines, line)
		}
	}
	s.Equalf(1, len(processLines), "expected exactly 1 cloud-hypervisor process, got %d:\n%s", len(processLines), output)

	// Delete the VM
	s.DeleteVM(vm.ID)

	// Verify no leftover processes or tap devices
	output, err := s.TryRunCommand("bash", "-c", "ps aux | grep [c]loud-hypervisor")
	s.Error(err)
	s.Empty(output)

	output, err = s.TryRunCommand("bash", "-c", "ip a | grep ztap")
	s.Error(err)
	s.Empty(output)
}

func (s *Suite) Test_VMLogs() {
	// Test: after a VM starts, the guest streams logs over vsock into the vm_logs table.
	vm := s.CreateVM(testsuite.CreateVMArgs{
		Registry:   "docker.io",
		Repository: "traefik/whoami",
		Tag:        "latest",
		Port:       80,
		Server:     testsuite.Server1ID,
	})
	slog.Info("Created VM", "id", vm.ID)

	// Wait for VM to be running and curlable
	s.WaitUntil(func() bool {
		vm, err := s.DB.VMFirstByID(s.Context(), vm.ID)
		s.NoError(err)
		if vm.Status != queries.VmStatusRunning {
			slog.Warn("Not running", "status", vm.Status)
			return false
		}
		_, err = s.TryRunCommand("curl", "-f", "-s", "--connect-timeout", "2",
			fmt.Sprintf("http://%s:%d", vm.IpAddress.Addr(), vm.Port.Int32))
		if err != nil {
			slog.Warn("Not curlable")
			return false
		}
		return true
	})

	// Verify that vm_logs have been written for this VM
	var logCount int
	err := s.DB.Pool.QueryRow(s.Context(),
		"SELECT COUNT(*) FROM vm_logs WHERE vm_id = $1", vm.ID).Scan(&logCount)
	s.NoError(err)
	s.Greaterf(logCount, 0, "expected vm_logs to contain entries for vm %s, got 0", vm.ID)
	slog.Info("VM logs found", "vm", vm.ID, "count", logCount)

	// Verify the specific "Starting up on port 80" log line exists
	var hasStartupLog bool
	err = s.DB.Pool.QueryRow(s.Context(),
		"SELECT EXISTS(SELECT 1 FROM vm_logs WHERE vm_id = $1 AND message LIKE '%Starting up on port 80%')",
		vm.ID).Scan(&hasStartupLog)
	s.NoError(err)
	s.Truef(hasStartupLog, "expected to find 'Starting up on port 80' log line for vm %s", vm.ID)

	// Delete the VM
	s.DeleteVM(vm.ID)

	// Verify no leftover processes or tap devices
	output, err := s.TryRunCommand("bash", "-c", "ps aux | grep [c]loud-hypervisor")
	s.Error(err)
	s.Empty(output)

	output, err = s.TryRunCommand("bash", "-c", "ip a | grep ztap")
	s.Error(err)
	s.Empty(output)
}
