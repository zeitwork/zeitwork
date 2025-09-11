package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	ctx := context.Background()
	slog.Info("Starting Firecracker + containerd IPv6 hello experiment")

	if err := checkKVM(ctx); err != nil {
		slog.Error("KVM check failed", "error", err)
		os.Exit(1)
	}

	if err := ensurePrerequisites(ctx); err != nil {
		slog.Error("Prerequisites failed", "error", err)
		os.Exit(1)
	}
}

// --- utilities ---

func runShell(ctx context.Context, cmd string) (string, error) {
	slog.Info("RUN", "cmd", cmd)
	c := exec.CommandContext(ctx, "/bin/bash", "-lc", cmd)
	c.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	stdoutStderr, err := c.CombinedOutput()
	return string(stdoutStderr), err
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func detectPackageManager() string {
	if commandExists("apt-get") {
		return "apt"
	}
	if commandExists("yum") {
		return "yum"
	}
	if commandExists("dnf") {
		return "dnf"
	}
	return ""
}

func shellEscape(s string) string {
	// Basic escaping for file paths/simple args
	return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "'\\''"))
}

func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if st, err := os.Stat(path); err == nil && st.Mode().Type() != os.ModeDir {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("file %s not found after %s", path, timeout)
}

// --- setup helpers ---

func checkKVM(ctx context.Context) error {
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return fmt.Errorf("/dev/kvm missing: %w", err)
	}
	// best-effort R/W open to hint permission problems early
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		slog.Warn("/dev/kvm not accessible for R/W; continuing", "error", err)
		return nil
	}
	_ = f.Close()
	_ = ctx
	return nil
}

func ensurePrerequisites(ctx context.Context) error {
	pm := detectPackageManager()
	slog.Info("Detected package manager", "pm", pm)
	var installCmd string
	switch pm {
	case "apt":
		installCmd = "set -e; sudo DEBIAN_FRONTEND=noninteractive apt-get update && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y make git curl e2fsprogs util-linux bc gnupg gcc dmsetup jq iproute2 ca-certificates"
	case "yum":
		installCmd = "set -e; sudo yum -y install make git curl e2fsprogs util-linux bc gnupg gcc device-mapper jq iproute ca-certificates"
	case "dnf":
		installCmd = "set -e; sudo dnf -y install make git curl e2fsprogs util-linux bc gnupg2 gcc device-mapper jq iproute ca-certificates"
	default:
		slog.Warn("Unknown package manager; skipping package installation")
	}
	if installCmd != "" {
		if out, err := runShell(ctx, installCmd); err != nil {
			return fmt.Errorf("base packages failed: %v: %s", err, out)
		}
	}

	if !commandExists("docker") {
		dockerCmd := "set -e; curl -fsSL https://get.docker.com | sh; sudo systemctl enable --now docker"
		if out, err := runShell(ctx, dockerCmd); err != nil {
			return fmt.Errorf("docker install failed: %v: %s", err, out)
		}
	}
	return nil
}
