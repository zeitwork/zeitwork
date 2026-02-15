package zeitwork_test

import (
	"fmt"
	"log/slog"

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
	err := s.DB.VMSoftDelete(s.Context(), vm.ID)
	s.NoError(err)

	s.WaitUntil(func() bool {
		vm, err := s.DB.VMFirstByID(s.Context(), vm.ID)
		s.NoError(err)

		// wait until vm is stopped
		if vm.Status != queries.VmStatusFailed { // TODO, bug, why? want to remove status anyway.
			return false
		}

		// ensure we can no longer ping
		_, err = s.TryRunCommand("ping", "-c", "1", "-W", "2", vm.IpAddress.Addr().String())
		if err == nil {
			return false
		}

		return true
	})

	// there should not be any cloud hypervisor processes or taps left.
	output, err := s.TryRunCommand("bash", "-c", "ps aux | grep [c]loud-hypervisor")
	s.Error(err)
	s.Empty(output)

	output, err = s.TryRunCommand("bash", "-c", "ip a | grep ztap")
	s.Error(err)
	s.Empty(output)
}
