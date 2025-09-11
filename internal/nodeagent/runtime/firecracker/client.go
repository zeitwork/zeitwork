package firecracker

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// Runtime implements the Zeitwork Runtime interface using firecracker-containerd.
// It shells out to firecracker-ctr to avoid direct gRPC dependencies and mirrors
// the working flow validated in experiments/firecracker/scripts.
type Runtime struct {
	cfg    *config.FirecrackerRuntimeConfig
	logger *slog.Logger
}

// NewFirecrackerRuntime creates a new Firecracker runtime backed by firecracker-containerd.
func NewFirecrackerRuntime(cfg *config.FirecrackerRuntimeConfig, logger *slog.Logger) (*Runtime, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil firecracker runtime config")
	}
	// Basic socket check
	if _, err := os.Stat(cfg.ContainerdSocket); err != nil {
		return nil, fmt.Errorf("containerd socket not found at %s: %w", cfg.ContainerdSocket, err)
	}
	// Try a cheap call to verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := runFC(ctx, cfg, []string{"version"}); err != nil {
		logger.Warn("firecracker-ctr version probe failed", "error", err)
	}
	return &Runtime{cfg: cfg, logger: logger}, nil
}

func (r *Runtime) addressArgs() []string {
	return []string{"--address", r.cfg.ContainerdSocket}
}

func (r *Runtime) nsArgs() []string {
	if r.cfg.ContainerdNamespace == "" {
		return nil
	}
	return []string{"-n", r.cfg.ContainerdNamespace}
}

// runFC executes firecracker-ctr with context, returning combined output.
func runFC(ctx context.Context, cfg *config.FirecrackerRuntimeConfig, args []string) (string, error) {
	base := []string{"/usr/local/bin/firecracker-ctr", "--address", cfg.ContainerdSocket}
	cmd := exec.CommandContext(ctx, base[0], append(base[1:], args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runFCNS runs firecracker-ctr with namespace flags.
func runFCNS(ctx context.Context, cfg *config.FirecrackerRuntimeConfig, ns string, args []string) (string, error) {
	full := []string{"--address", cfg.ContainerdSocket}
	if ns != "" {
		full = append(full, "-n", ns)
	}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "/usr/local/bin/firecracker-ctr", full...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// generateName returns a stable task/container name for an instance.
func generateName(instanceID string) string {
	// Keep short, DNS-safe
	return fmt.Sprintf("fc-%s", sanitizeName(instanceID))
}

func sanitizeName(s string) string {
	// replace any non-alnum with hyphen
	re := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	return strings.Trim(re.ReplaceAllString(s, "-"), "-")
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// findVMIDForTaskExec tries to map an ExecID (or TaskID) to a VM ID using the daemon log.
// This relies on debug logging enabled and is a pragmatic approach validated in experiments.
func findVMIDForTaskExec(logPath, execID, taskID string) (string, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var vmID string
	needleExec := fmt.Sprintf("ExecID=%s", execID)
	needleTask := fmt.Sprintf("TaskID=%s", taskID)
	needleVM := "vmID="
	for scanner.Scan() {
		line := scanner.Text()
		if (execID != "" && strings.Contains(line, needleExec)) || (taskID != "" && strings.Contains(line, needleTask)) {
			if idx := strings.Index(line, needleVM); idx >= 0 {
				vmID = strings.TrimSpace(line[idx+len(needleVM):])
				// vmID may include trailing fields; split on space if so
				if sp := strings.IndexAny(vmID, " \t"); sp >= 0 {
					vmID = vmID[:sp]
				}
			}
		}
	}
	if vmID == "" {
		return "", fmt.Errorf("vmID not found in log for execID=%s taskID=%s", execID, taskID)
	}
	return vmID, nil
}

// discoverIPv6Lease finds the IPv6 allocated by host-local IPAM for a given VM ID.
func discoverIPv6Lease(cniStateDir, networkName, vmID string) (string, error) {
	leaseDir := filepath.Join(cniStateDir, networkName)
	d, err := os.ReadDir(leaseDir)
	if err != nil {
		return "", fmt.Errorf("read lease dir: %w", err)
	}
	for _, de := range d {
		name := de.Name()
		if !strings.HasPrefix(name, "fd") { // crude filter for IPv6 ULA/fd00
			continue
		}
		content, err := os.ReadFile(filepath.Join(leaseDir, name))
		if err != nil {
			continue
		}
		if strings.Contains(string(content), vmID) {
			return name, nil // filename is the IPv6 address
		}
	}
	return "", fmt.Errorf("no IPv6 lease found for vmID %s", vmID)
}

// mapStatus maps textual ctr task STATUS to InstanceState.
func mapStatus(s string) types.InstanceState {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch {
	case strings.Contains(s, "RUNNING"):
		return types.InstanceStateRunning
	case strings.Contains(s, "STOPPED") || strings.Contains(s, "EXITED"):
		return types.InstanceStateStopped
	case strings.Contains(s, "CREATED"):
		return types.InstanceStateCreating
	default:
		return types.InstanceStatePending
	}
}
