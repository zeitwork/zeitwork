package zeitwork_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/crypto"
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
	vm := s.CreateAndWaitVM(testsuite.CreateVMArgs{
		Registry:   "docker.io",
		Repository: "traefik/whoami",
		Tag:        "latest",
		Port:       80,
		Server:     testsuite.Server1ID,
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

func (s *Suite) Test_VMRecovery() {
	// Test: Kill the cloud-hypervisor process, verify the reconciler detects failure and restarts the VM.
	vm := s.CreateAndWaitVM(testsuite.CreateVMArgs{
		Registry:   "docker.io",
		Repository: "traefik/whoami",
		Tag:        "latest",
		Port:       80,
		Server:     testsuite.Server1ID,
	})
	slog.Info("VM is running, killing cloud-hypervisor process", "id", vm.ID)

	// Kill all cloud-hypervisor processes (there should only be one from our VM)
	s.RunCommand("killall", "-9", "cloud-hypervisor")
	slog.Info("Killed cloud-hypervisor, waiting for reconciler to recover")

	// Wait for the reconciler to detect the failure and restart the VM
	s.WaitUntil(func() bool {
		vm, err := s.DB.VMFirstByID(s.Context(), vm.ID)
		s.NoError(err)
		if vm.Status != queries.VmStatusRunning {
			slog.Warn("VM not running after recovery", "status", vm.Status)
			return false
		}
		_, err = s.TryRunCommand("curl", "-f", "-s", "--connect-timeout", "2",
			fmt.Sprintf("http://%s:%d", vm.IpAddress.Addr(), vm.Port.Int32))
		if err != nil {
			slog.Warn("VM not curlable after recovery")
			return false
		}
		return true
	})

	// Verify exactly one cloud-hypervisor process
	output, _ := s.TryRunCommand("bash", "-c", "ps aux | grep [c]loud-hypervisor")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var processLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			processLines = append(processLines, line)
		}
	}
	s.Equalf(1, len(processLines), "expected exactly 1 cloud-hypervisor process after recovery, got %d:\n%s", len(processLines), output)

	s.DeleteVM(vm.ID)

	// Verify no leftover processes or tap devices
	output, err := s.TryRunCommand("bash", "-c", "ps aux | grep [c]loud-hypervisor")
	s.Error(err)
	s.Empty(output)

	output, err = s.TryRunCommand("bash", "-c", "ip a | grep ztap")
	s.Error(err)
	s.Empty(output)
}

func (s *Suite) Test_DeadServerFailover() {
	// Test: Stop server2's zeitwork, fake a dead heartbeat, verify the leader creates a replacement VM on server1.
	vm := s.CreateAndWaitVM(testsuite.CreateVMArgs{
		Registry:   "docker.io",
		Repository: "traefik/whoami",
		Tag:        "latest",
		Port:       80,
		Server:     testsuite.Server2ID,
	})
	originalVMID := vm.ID
	slog.Info("VM running on server2, stopping zeitwork and faking dead heartbeat", "id", originalVMID)

	// Stop zeitwork on server2
	s.RunCommandRemote(testsuite.Server2IP, "systemctl", "stop", "zeitwork.service")

	// Fake a dead heartbeat (set last_heartbeat_at to 120 seconds ago)
	_, err := s.DB.Pool.Exec(s.Context(),
		"UPDATE servers SET last_heartbeat_at = now() - interval '120 seconds' WHERE id = $1",
		testsuite.Server2ID)
	s.NoError(err)
	slog.Info("Faked dead heartbeat for server2, waiting for leader to detect and failover")

	// Wait for the leader to detect the dead server and create a replacement VM
	// The dead detection loop runs every 30 seconds, so wait up to 60s
	var replacementVM queries.Vm
	s.WaitUntil(func() bool {
		// Check if original VM was soft-deleted
		oldVM, err := s.DB.VMFirstByID(s.Context(), originalVMID)
		s.NoError(err)
		if !oldVM.DeletedAt.Valid {
			slog.Warn("Original VM not yet soft-deleted")
			return false
		}

		// Look for a running replacement VM on a different server with the same image
		vms, err := s.DB.VMFindByImageID(s.Context(), vm.ImageID)
		s.NoError(err)
		for _, v := range vms {
			if v.ID != originalVMID && v.ServerID != testsuite.Server2ID && v.Status == queries.VmStatusRunning {
				replacementVM = v
				slog.Info("Found replacement VM", "id", v.ID, "server", v.ServerID)
				return true
			}
		}
		slog.Warn("Replacement VM not yet running")
		return false
	})

	// Verify the replacement VM is curlable
	_, err = s.TryRunCommand("curl", "-f", "-s", "--connect-timeout", "2",
		fmt.Sprintf("http://%s:%d", replacementVM.IpAddress.Addr(), replacementVM.Port.Int32))
	s.NoErrorf(err, "replacement VM should be curlable at %s:%d", replacementVM.IpAddress.Addr(), replacementVM.Port.Int32)

	s.DeleteVM(replacementVM.ID)

	// Restart zeitwork on server2 for cleanup
	s.RunCommandRemote(testsuite.Server2IP, "systemctl", "start", "zeitwork.service")

	// Verify no leftover processes or tap devices
	output, err := s.TryRunCommand("bash", "-c", "ps aux | grep [c]loud-hypervisor")
	s.Error(err)
	s.Empty(output)

	output, err = s.TryRunCommand("bash", "-c", "ip a | grep ztap")
	s.Error(err)
	s.Empty(output)
}

func (s *Suite) Test_VMEnvVariables() {
	// Test: Create a VM with environment variables, verify they appear in the HTTP response.

	// Prepare encrypted env vars
	envVars := []string{"MY_TEST_VAR=hello_zeitwork", "ANOTHER_VAR=test_123"}
	envJSON, err := json.Marshal(envVars)
	s.NoError(err)
	encryptedEnvVars, err := crypto.Encrypt(string(envJSON))
	s.NoError(err)

	vm := s.CreateAndWaitVM(testsuite.CreateVMArgs{
		Registry:     "docker.io",
		Repository:   "ealen/echo-server",
		Tag:          "latest",
		Port:         80,
		Server:       testsuite.Server1ID,
		EnvVariables: encryptedEnvVars,
	})
	slog.Info("VM with env vars is running", "id", vm.ID)

	// Curl the VM and parse the JSON response
	output, err := s.TryRunCommand("curl", "-f", "-s", "--connect-timeout", "2",
		fmt.Sprintf("http://%s:%d", vm.IpAddress.Addr(), vm.Port.Int32))
	s.NoError(err)

	// Parse the response to extract environment variables
	var response map[string]interface{}
	err = json.Unmarshal([]byte(output), &response)
	s.NoError(err)

	environment, ok := response["environment"].(map[string]interface{})
	s.Truef(ok, "expected 'environment' key in response")

	// Verify our test env vars are present
	myTestVar, ok := environment["MY_TEST_VAR"].(string)
	s.Truef(ok, "expected MY_TEST_VAR in environment")
	s.Equalf("hello_zeitwork", myTestVar, "MY_TEST_VAR should be 'hello_zeitwork'")

	anotherVar, ok := environment["ANOTHER_VAR"].(string)
	s.Truef(ok, "expected ANOTHER_VAR in environment")
	s.Equalf("test_123", anotherVar, "ANOTHER_VAR should be 'test_123'")

	s.DeleteVM(vm.ID)

	// Verify no leftover processes or tap devices
	output, err = s.TryRunCommand("bash", "-c", "ps aux | grep [c]loud-hypervisor")
	s.Error(err)
	s.Empty(output)

	output, err = s.TryRunCommand("bash", "-c", "ip a | grep ztap")
	s.Error(err)
	s.Empty(output)
}

func (s *Suite) Test_ConcurrentVMCreation() {
	// Test: Create 10 VMs concurrently, verify all get unique IPs and all reach running state.

	// Create 10 VMs concurrently using goroutines
	vmCount := 10
	vmChan := make(chan queries.Vm, vmCount)
	var wg sync.WaitGroup

	slog.Info("Creating 10 VMs concurrently")
	for i := 0; i < vmCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			vm := s.CreateVM(testsuite.CreateVMArgs{
				Registry:   "docker.io",
				Repository: "traefik/whoami",
				Tag:        "latest",
				Port:       80,
				Server:     testsuite.Server1ID,
			})
			slog.Info("Created VM", "index", index, "id", vm.ID)
			vmChan <- vm
		}(i)
	}

	wg.Wait()
	close(vmChan)

	// Collect all VMs
	vms := make([]queries.Vm, 0, vmCount)
	for vm := range vmChan {
		vms = append(vms, vm)
	}
	s.Equalf(vmCount, len(vms), "expected %d VMs to be created", vmCount)

	// Verify all VMs have unique IP addresses
	ipMap := make(map[string]bool)
	for _, vm := range vms {
		ipStr := vm.IpAddress.Addr().String()
		s.Falsef(ipMap[ipStr], "duplicate IP address found: %s", ipStr)
		ipMap[ipStr] = true
	}
	slog.Info("All VMs have unique IPs")

	// Wait for all VMs to reach running state and be curlable
	for i, vm := range vms {
		slog.Info("Waiting for VM to be ready", "index", i, "id", vm.ID)
		s.WaitUntil(func() bool {
			vmFresh, err := s.DB.VMFirstByID(s.Context(), vm.ID)
			s.NoError(err)
			if vmFresh.Status != queries.VmStatusRunning {
				return false
			}
			_, err = s.TryRunCommand("curl", "-f", "-s", "--connect-timeout", "2",
				fmt.Sprintf("http://%s:%d", vmFresh.IpAddress.Addr(), vmFresh.Port.Int32))
			return err == nil
		})
	}
	slog.Info("All VMs are running and curlable")

	// Delete all VMs
	for _, vm := range vms {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.DeleteVM(vm.ID)
		}()
	}
	wg.Wait()

	// Verify no leftover processes or tap devices
	output, err := s.TryRunCommand("bash", "-c", "ps aux | grep [c]loud-hypervisor")
	s.Error(err)
	s.Empty(output)

	output, err = s.TryRunCommand("bash", "-c", "ip a | grep ztap")
	s.Error(err)
	s.Empty(output)
}
