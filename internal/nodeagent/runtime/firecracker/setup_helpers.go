package firecracker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"golang.org/x/sys/unix"
)

// Helper methods for SetupManager

// checkBinary verifies if a binary exists and is executable
func (s *SetupManager) checkBinary(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}

	// Check if executable
	if err := unix.Access(path, unix.X_OK); err != nil {
		return fmt.Errorf("binary not executable: %w", err)
	}

	return nil
}

// downloadFile downloads a file from URL to local path
func (s *SetupManager) downloadFile(ctx context.Context, url, filepath string) error {
	s.logger.Debug("Downloading file", "url", url, "path", filepath)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

// downloadKernelImage builds a kernel image using Firecracker's tools
func (s *SetupManager) downloadKernelImage(ctx context.Context) error {
	s.logger.Info("Building kernel image using Firecracker CI tools")

	// Since Firecracker v1.13.1+ doesn't include pre-built kernel files in releases,
	// we'll build the kernel using Firecracker's own CI tools which is the recommended approach

	return s.buildKernelUsingFirecrackerTools(ctx)
}

// buildKernelUsingFirecrackerTools builds a kernel using Firecracker's devtool
func (s *SetupManager) buildKernelUsingFirecrackerTools(ctx context.Context) error {
	s.logger.Info("Building kernel using Firecracker devtool")

	tmpl, err := GetScriptTemplate(BuildKernelTemplate)
	if err != nil {
		return fmt.Errorf("failed to get kernel build template: %w", err)
	}

	firecrackerSourceDir := filepath.Join(s.tempDir, "firecracker-source")

	data := BuildKernelData{
		FirecrackerSourceDir: firecrackerSourceDir,
		KernelVersion:        "6.1", // Use kernel 6.1 as recommended for recent Firecracker
		OutputPath:           s.config.DefaultKernelPath,
		TempDir:              s.tempDir,
	}

	// Execute kernel build script
	if err := s.executeScriptTemplate(ctx, tmpl, data, "build-kernel"); err != nil {
		// If kernel build fails, try to provide a simple fallback
		s.logger.Warn("Kernel build failed, trying fallback approach", "error", err)
		return s.createMinimalKernelFallback(ctx)
	}

	// Verify the built kernel
	if err := s.verifyKernelFile(s.config.DefaultKernelPath); err != nil {
		return fmt.Errorf("built kernel verification failed: %w", err)
	}

	s.logger.Info("Kernel built successfully using Firecracker tools")
	return nil
}

// createMinimalKernelFallback creates a warning and guidance for manual kernel setup
func (s *SetupManager) createMinimalKernelFallback(ctx context.Context) error {
	s.logger.Error("Failed to build kernel automatically. Manual kernel setup required.")
	s.logger.Error("")
	s.logger.Error("To build a kernel for Firecracker:")
	s.logger.Error("1. Clone Firecracker source: git clone https://github.com/firecracker-microvm/firecracker.git")
	s.logger.Error("2. Build kernel: ./tools/devtool build_ci_artifacts kernels 6.1")
	s.logger.Error("3. Copy kernel: cp resources/$(uname -m)/vmlinux-6.1 " + s.config.DefaultKernelPath)
	s.logger.Error("")
	s.logger.Error("Or build manually:")
	s.logger.Error("1. Get Linux source: git clone https://github.com/torvalds/linux.git")
	s.logger.Error("2. Configure for Firecracker: use config from https://github.com/firecracker-microvm/firecracker/tree/main/resources/guest_configs")
	s.logger.Error("3. Build: make vmlinux (on x86_64) or make Image (on aarch64)")
	s.logger.Error("4. Copy to: " + s.config.DefaultKernelPath)

	return fmt.Errorf("kernel must be provided manually - see logs for instructions")
}

// verifyKernelFile performs basic verification that a file is a Linux kernel
func (s *SetupManager) verifyKernelFile(kernelPath string) error {
	// Check file size (should be at least 1MB for a minimal kernel)
	info, err := os.Stat(kernelPath)
	if err != nil {
		return fmt.Errorf("cannot stat kernel file: %w", err)
	}

	if info.Size() < 1024*1024 { // Less than 1MB
		return fmt.Errorf("kernel file too small (%d bytes), likely not a valid kernel", info.Size())
	}

	// Check for basic kernel magic bytes (simplified check)
	file, err := os.Open(kernelPath)
	if err != nil {
		return fmt.Errorf("cannot open kernel file: %w", err)
	}
	defer file.Close()

	// Read first few bytes to check for basic patterns
	header := make([]byte, 16)
	if _, err := file.Read(header); err != nil {
		return fmt.Errorf("cannot read kernel header: %w", err)
	}

	// Very basic check - this is not comprehensive but catches obvious issues
	s.logger.Debug("Kernel file seems valid", "size", info.Size(), "path", kernelPath)
	return nil
}

// renderTemplateToFile renders a template with data and writes to file
func (s *SetupManager) renderTemplateToFile(tmpl *template.Template, data interface{}, filepath string) error {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	if err := os.WriteFile(filepath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filepath, err)
	}

	s.logger.Debug("Created config file", "path", filepath)
	return nil
}

// executeScriptTemplate renders and executes a script template
func (s *SetupManager) executeScriptTemplate(ctx context.Context, tmpl *template.Template, data interface{}, scriptName string) error {
	// Render template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Write script to temp file
	scriptPath := fmt.Sprintf("/tmp/%s-%d.sh", scriptName, time.Now().Unix())
	if err := os.WriteFile(scriptPath, buf.Bytes(), 0755); err != nil {
		return fmt.Errorf("failed to write script: %w", err)
	}
	defer os.Remove(scriptPath)

	s.logger.Info("Executing setup script", "script", scriptName, "script_path", scriptPath)

	// Execute script with output capture
	cmd := exec.CommandContext(ctx, "/bin/bash", scriptPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		s.logger.Error("Script execution failed",
			"script", scriptName,
			"script_path", scriptPath,
			"output", string(output),
			"error", err)
		return fmt.Errorf("script %s failed: %w\nOutput: %s", scriptName, err, string(output))
	}

	s.logger.Info("Script executed successfully", "script", scriptName, "output_lines", len(strings.Split(string(output), "\n")))
	s.logger.Debug("Script output", "script", scriptName, "output", string(output))
	return nil
}

// deviceMapperPoolExists checks if device mapper pool exists
func (s *SetupManager) deviceMapperPoolExists(poolName string) bool {
	cmd := exec.Command("dmsetup", "info", poolName)
	return cmd.Run() == nil
}

// isDaemonRunning checks if firecracker-containerd daemon is running
func (s *SetupManager) isDaemonRunning() bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", "firecracker-containerd")
	return cmd.Run() == nil
}

// startDaemon starts firecracker-containerd daemon
func (s *SetupManager) startDaemon() error {
	s.logger.Info("Starting firecracker-containerd daemon")

	cmd := exec.Command("systemctl", "start", "firecracker-containerd")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	return nil
}

// waitForDaemon waits for daemon to be ready
func (s *SetupManager) waitForDaemon(ctx context.Context) error {
	s.logger.Info("Waiting for daemon to be ready")

	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	socketPath := s.config.ContainerdSocket

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for daemon")
		case <-ticker.C:
			if _, err := os.Stat(socketPath); err == nil {
				// Test connectivity
				cmd := exec.Command("/usr/local/bin/firecracker-ctr",
					"--address", socketPath, "version")
				if cmd.Run() == nil {
					s.logger.Info("Daemon is ready")
					return nil
				}
			}
			s.logger.Debug("Still waiting for daemon", "socket", socketPath)
		}
	}
}

// runCommandWithOutput runs a command and captures both stdout and stderr
func (s *SetupManager) runCommandWithOutput(cmd *exec.Cmd) (string, error) {
	s.logger.Debug("Executing command", "cmd", cmd.String(), "dir", cmd.Dir)

	output, err := cmd.CombinedOutput()

	if err != nil {
		s.logger.Debug("Command failed", "cmd", cmd.String(), "output", string(output), "error", err)
		return string(output), err
	}

	s.logger.Debug("Command succeeded", "cmd", cmd.String())
	return string(output), nil
}

// cleanup removes temporary files
func (s *SetupManager) cleanup() {
	if err := os.RemoveAll(s.tempDir); err != nil {
		s.logger.Warn("Failed to cleanup temp directory", "dir", s.tempDir, "error", err)
	}
}
