package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// tunnelMapping defines a single reverse SSH port forward.
// Remote port on the server → local port on the developer's machine.
type tunnelMapping struct {
	RemotePort int // Port on the bare metal server (localhost:RemotePort)
	LocalPort  int // Port on the developer's machine (localhost:LocalPort)
	Label      string
}

// Default tunnel mappings: server localhost → developer laptop.
var defaultTunnels = []tunnelMapping{
	{RemotePort: 5432, LocalPort: 15432, Label: "postgres"},
	{RemotePort: 6432, LocalPort: 16432, Label: "pgbouncer"},
	{RemotePort: 9000, LocalPort: 19000, Label: "minio"},
}

// tunnelState tracks PIDs of running SSH tunnel processes.
type tunnelState struct {
	// Map of "host:remotePort" → PID
	Tunnels map[string]int `json:"tunnels"`
}

// startTunnels establishes reverse SSH tunnels to all servers.
func startTunnels(cluster *ClusterState) error {
	state := &tunnelState{Tunnels: make(map[string]int)}

	for _, server := range cluster.Servers {
		for _, t := range defaultTunnels {
			key := fmt.Sprintf("%s:%d", server.PublicIP, t.RemotePort)

			// Build the -R argument: remote binds on server → local service
			reverseArg := fmt.Sprintf("%d:127.0.0.1:%d", t.RemotePort, t.LocalPort)

			cmd := exec.Command("ssh",
				"-i", cluster.SSHKeyPath,
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
				"-o", "ExitOnForwardFailure=yes",
				"-o", "ServerAliveInterval=15",
				"-o", "ServerAliveCountMax=3",
				"-N", // no remote command
				"-R", reverseArg,
				"root@"+server.PublicIP,
			)
			// Detach from parent process group so tunnels survive CLI exit
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

			if err := cmd.Start(); err != nil {
				// Clean up any tunnels we already started
				stopTunnelsFromState(state)
				return fmt.Errorf("failed to start %s tunnel to %s: %w", t.Label, server.Hostname, err)
			}

			state.Tunnels[key] = cmd.Process.Pid
			slog.Info("tunnel started", "server", server.Hostname, "tunnel", t.Label,
				"remote", fmt.Sprintf("localhost:%d", t.RemotePort),
				"local", fmt.Sprintf("localhost:%d", t.LocalPort),
				"pid", cmd.Process.Pid)

			// Detach: don't wait for the process
			go cmd.Wait() //nolint:errcheck
		}
	}

	// Save tunnel state so we can stop them later
	if err := saveTunnelState(state); err != nil {
		return fmt.Errorf("failed to save tunnel state: %w", err)
	}

	// Wait a moment then verify tunnels are alive
	time.Sleep(2 * time.Second)
	if err := checkTunnels(cluster); err != nil {
		slog.Warn("tunnel health check failed after start", "err", err)
	}

	return nil
}

// stopTunnels kills all tracked SSH tunnel processes.
func stopTunnels() error {
	state, err := loadTunnelState()
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no tunnels to stop
		}
		return err
	}
	stopTunnelsFromState(state)
	return os.Remove(tunnelFile)
}

func stopTunnelsFromState(state *tunnelState) {
	for key, pid := range state.Tunnels {
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			// Process might already be dead
			slog.Debug("tunnel process already stopped", "key", key, "pid", pid)
			continue
		}
		slog.Info("tunnel stopped", "key", key, "pid", pid)
	}
}

// checkTunnels verifies that reverse tunnels are working by checking remote connectivity.
func checkTunnels(cluster *ClusterState) error {
	var failures int
	for _, server := range cluster.Servers {
		for _, t := range defaultTunnels {
			// Check if the remote port is reachable on the server via the tunnel
			_, err := sshRun(cluster.SSHKeyPath, server.PublicIP,
				fmt.Sprintf("nc -z -w 2 127.0.0.1 %d", t.RemotePort))
			if err != nil {
				slog.Warn("tunnel check failed", "server", server.Hostname, "tunnel", t.Label, "port", t.RemotePort)
				failures++
			}
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d tunnel(s) not reachable", failures)
	}
	slog.Info("all tunnels healthy")
	return nil
}

func saveTunnelState(state *tunnelState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tunnelFile, data, 0o644)
}

func loadTunnelState() (*tunnelState, error) {
	data, err := os.ReadFile(tunnelFile)
	if err != nil {
		return nil, err
	}
	var state tunnelState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// areTunnelsRunning checks if the tunnel processes are still alive.
func areTunnelsRunning() bool {
	state, err := loadTunnelState()
	if err != nil {
		return false
	}
	for _, pid := range state.Tunnels {
		proc, err := os.FindProcess(pid)
		if err != nil {
			return false
		}
		// Signal 0 checks if process exists without actually sending a signal
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			return false
		}
	}
	return len(state.Tunnels) > 0
}

// tunnelSummary returns a human-readable summary of tunnel state.
func tunnelSummary() string {
	state, err := loadTunnelState()
	if err != nil {
		return "no tunnels"
	}
	alive := 0
	for _, pid := range state.Tunnels {
		proc, err := os.FindProcess(pid)
		if err == nil {
			if proc.Signal(syscall.Signal(0)) == nil {
				alive++
			}
		}
	}
	return strconv.Itoa(alive) + "/" + strconv.Itoa(len(state.Tunnels)) + " alive"
}
