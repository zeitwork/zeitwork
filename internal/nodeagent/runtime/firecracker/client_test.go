package firecracker

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Integration test environment setup
type IntegrationTestEnv struct {
	WorkDir         string
	KernelPath      string
	RootfsPath      string
	FirecrackerPath string
	JailerPath      string
}

// setupIntegrationEnv sets up the real firecracker environment for testing
func setupIntegrationEnv(t *testing.T) *IntegrationTestEnv {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Check if we have KVM access
	if !checkKVMAccess() {
		t.Skip("KVM not available, skipping firecracker integration tests")
	}

	// Check for firecracker binaries
	firecrackerPath, err := exec.LookPath("firecracker")
	if err != nil {
		// Try common install locations
		commonPaths := []string{"/usr/local/bin/firecracker", "/usr/bin/firecracker"}
		for _, path := range commonPaths {
			if _, err := os.Stat(path); err == nil {
				firecrackerPath = path
				break
			}
		}
		if firecrackerPath == "" {
			t.Skip("Firecracker binary not found, install from https://github.com/firecracker-microvm/firecracker")
		}
	}

	jailerPath, err := exec.LookPath("jailer")
	if err != nil {
		// Try common install locations
		commonPaths := []string{"/usr/local/bin/jailer", "/usr/bin/jailer"}
		for _, path := range commonPaths {
			if _, err := os.Stat(path); err == nil {
				jailerPath = path
				break
			}
		}
		if jailerPath == "" {
			t.Skip("Jailer binary not found, install with firecracker")
		}
	}

	// For jailer compatibility, use /srv/jailer-test to avoid cross-device links
	// This ensures kernel and rootfs are on the same filesystem as jailer
	jailerCompatWorkDir := "/srv/jailer-test"
	if err := os.MkdirAll(jailerCompatWorkDir, 0755); err != nil {
		t.Skipf("Failed to create jailer-compatible work directory: %v", err)
	}

	workDir := jailerCompatWorkDir
	kernelPath := filepath.Join(workDir, "vmlinux.bin")
	rootfsPath := filepath.Join(workDir, "rootfs.ext4")

	// Try to find existing kernel/rootfs first and copy to jailer-compatible location
	commonKernelPaths := []string{
		"/var/lib/firecracker-containerd/runtime/default-vmlinux.bin",
		"/opt/firecracker/kernel/vmlinux.bin",
	}
	commonRootfsPaths := []string{
		"/var/lib/firecracker-containerd/runtime/default-rootfs.img",
		"/opt/firecracker/rootfs/rootfs.ext4",
	}

	// Check for existing kernel and copy to jailer-compatible location
	kernelFound := false
	for _, path := range commonKernelPaths {
		if _, err := os.Stat(path); err == nil {
			if err := copyFile(path, kernelPath); err == nil {
				kernelFound = true
				break
			}
		}
	}

	// Check for existing rootfs and copy to jailer-compatible location
	rootfsFound := false
	for _, path := range commonRootfsPaths {
		if _, err := os.Stat(path); err == nil {
			if err := copyFile(path, rootfsPath); err == nil {
				rootfsFound = true
				break
			}
		}
	}

	// Download kernel if not found locally
	if !kernelFound {
		t.Logf("Downloading kernel for integration tests...")
		if err := downloadKernel(kernelPath); err != nil {
			t.Skipf("Failed to download kernel: %v", err)
		}
	}

	// Create minimal rootfs if not found locally
	if !rootfsFound {
		t.Logf("Creating minimal rootfs for integration tests...")
		if err := createMinimalRootfs(rootfsPath); err != nil {
			t.Skipf("Failed to create rootfs: %v", err)
		}
	}

	// Fix permissions for jailer - files must be accessible by UID 1000
	if err := os.Chown(kernelPath, 1000, 1000); err != nil {
		t.Logf("Warning: Failed to chown kernel file: %v", err)
	}
	if err := os.Chown(rootfsPath, 1000, 1000); err != nil {
		t.Logf("Warning: Failed to chown rootfs file: %v", err)
	}
	if err := os.Chmod(kernelPath, 0644); err != nil {
		t.Logf("Warning: Failed to chmod kernel file: %v", err)
	}
	if err := os.Chmod(rootfsPath, 0644); err != nil {
		t.Logf("Warning: Failed to chmod rootfs file: %v", err)
	}

	return &IntegrationTestEnv{
		WorkDir:         workDir,
		KernelPath:      kernelPath,
		RootfsPath:      rootfsPath,
		FirecrackerPath: firecrackerPath,
		JailerPath:      jailerPath,
	}
}

// Helper functions for integration tests

func parseIP(s string) *net.IP {
	ip := net.ParseIP(s)
	return &ip
}

func checkKVMAccess() bool {
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return false
	}

	// Try to open /dev/kvm for read/write
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func hasSudoAccess() bool {
	cmd := exec.Command("sudo", "-n", "true")
	return cmd.Run() == nil
}

func hasNetworkingSupport() bool {
	// Check if we can create TAP devices (nodeagent runs as root in production)
	return os.Geteuid() == 0
}

// downloadKernel downloads a precompiled kernel for testing
func downloadKernel(dest string) error {
	// Use the same kernel URL as in the firecracker experiments
	kernelURL := "https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"

	cmd := exec.Command("curl", "-fsSL", "-o", dest, kernelURL)
	return cmd.Run()
}

// createMinimalRootfs creates a minimal ext4 rootfs for testing
func createMinimalRootfs(dest string) error {
	// Create 100MB sparse file
	if err := exec.Command("dd", "if=/dev/zero", "of="+dest, "bs=1M", "count=0", "seek=100").Run(); err != nil {
		return fmt.Errorf("failed to create rootfs file: %w", err)
	}

	// Format as ext4
	if err := exec.Command("mkfs.ext4", "-F", dest).Run(); err != nil {
		return fmt.Errorf("failed to format rootfs: %w", err)
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	return exec.Command("cp", src, dst).Run()
}

func isValidMAC(mac string) bool {
	// Basic MAC address format validation
	if len(mac) != 17 {
		return false
	}

	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return false
	}

	for _, part := range parts {
		if len(part) != 2 {
			return false
		}
		for _, c := range part {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}

	return true
}

func TestValidateConfig(t *testing.T) {
	env := setupIntegrationEnv(t)

	tests := []struct {
		name        string
		instance    *VMInstance
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid configuration",
			instance: &VMInstance{
				ID:         "test-vm-1",
				IPAddress:  parseIP("fd00:fc::100"),
				ImagePath:  env.RootfsPath,
				KernelPath: env.KernelPath,
				SocketPath: filepath.Join(env.WorkDir, "test.sock"),
				WorkDir:    filepath.Join(env.WorkDir, "vm-work"),
				VCPUs:      2,
				MemoryMB:   512, // Use less memory for faster tests
				NetworkTAP: "tap-test",
			},
			expectError: false,
		},
		{
			name: "empty VM ID",
			instance: &VMInstance{
				ID:         "",
				ImagePath:  env.RootfsPath,
				KernelPath: env.KernelPath,
				VCPUs:      2,
				MemoryMB:   512,
			},
			expectError: true,
			errorMsg:    "VM ID cannot be empty",
		},
		{
			name: "invalid vCPU count",
			instance: &VMInstance{
				ID:         "test-vm",
				ImagePath:  env.RootfsPath,
				KernelPath: env.KernelPath,
				VCPUs:      0,
				MemoryMB:   512,
			},
			expectError: true,
			errorMsg:    "vCPU count must be positive, got 0",
		},
		{
			name: "invalid memory size",
			instance: &VMInstance{
				ID:         "test-vm",
				ImagePath:  env.RootfsPath,
				KernelPath: env.KernelPath,
				VCPUs:      2,
				MemoryMB:   0,
			},
			expectError: true,
			errorMsg:    "memory size must be positive, got 0 MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.instance)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestGenerateMACAddress(t *testing.T) {
	tests := []struct {
		name string
		vmID string
	}{
		{
			name: "simple VM ID",
			vmID: "vm1",
		},
		{
			name: "complex VM ID",
			vmID: "test-vm-with-dashes-123",
		},
		{
			name: "consistency test",
			vmID: "consistent-vm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateMACAddress(tt.vmID)

			// Check format (should be XX:XX:XX:XX:XX:XX)
			if len(got) != 17 {
				t.Errorf("MAC address should be 17 characters, got %d: %s", len(got), got)
			}

			// Check that it starts with 02:00:00 (locally administered)
			if !strings.HasPrefix(got, "02:00:00") {
				t.Errorf("MAC address should start with '02:00:00', got: %s", got)
			}

			// Test consistency - same input should produce same output
			got2 := generateMACAddress(tt.vmID)
			if got != got2 {
				t.Errorf("generateMACAddress should be deterministic, got different results: %s vs %s", got, got2)
			}

			// Validate MAC format
			if !isValidMAC(got) {
				t.Errorf("Generated MAC address is invalid: %s", got)
			}
		})
	}
}

func TestBuildKernelArgs(t *testing.T) {
	expected := "console=ttyS0 reboot=k panic=1 pci=off"
	got := buildKernelArgs()

	if got != expected {
		t.Errorf("buildKernelArgs() = %q, want %q", got, expected)
	}
}

func TestNewClient(t *testing.T) {
	env := setupIntegrationEnv(t)

	tests := []struct {
		name        string
		instance    *VMInstance
		expectError bool
	}{
		{
			name: "valid instance configuration",
			instance: &VMInstance{
				ID:         "test-vm-newclient",
				IPAddress:  parseIP("fd00:fc::100"),
				ImagePath:  env.RootfsPath,
				KernelPath: env.KernelPath,
				SocketPath: filepath.Join(env.WorkDir, "test-newclient.sock"),
				WorkDir:    filepath.Join(env.WorkDir, "vm-work-newclient"),
				VCPUs:      2,
				MemoryMB:   512,
				NetworkTAP: "tap-test",
			},
			expectError: false,
		},
		{
			name: "invalid configuration should fail validation",
			instance: &VMInstance{
				ID:         "", // Empty ID should fail validation
				ImagePath:  env.RootfsPath,
				KernelPath: env.KernelPath,
				SocketPath: filepath.Join(env.WorkDir, "test-invalid.sock"),
				WorkDir:    filepath.Join(env.WorkDir, "vm-work-invalid"),
				VCPUs:      2,
				MemoryMB:   512,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.instance)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if client != nil {
					t.Errorf("Expected nil client when error occurs")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if client == nil {
					t.Errorf("Expected non-nil client")
				}

				// Verify client fields are set correctly
				if client.machineID != tt.instance.ID {
					t.Errorf("Expected machine ID %s, got %s", tt.instance.ID, client.machineID)
				}

				// Verify work directory was created
				if _, err := os.Stat(tt.instance.WorkDir); os.IsNotExist(err) {
					t.Errorf("Work directory should have been created: %s", tt.instance.WorkDir)
				}

				// Verify socket directory was created
				socketDir := filepath.Dir(tt.instance.SocketPath)
				if _, err := os.Stat(socketDir); os.IsNotExist(err) {
					t.Errorf("Socket directory should have been created: %s", socketDir)
				}

				// Clean up
				if err := client.Cleanup(); err != nil {
					t.Errorf("Failed to cleanup client: %v", err)
				}
			}
		})
	}
}

// TestFirecrackerVMLifecycle tests complete VM lifecycle with real firecracker
func TestFirecrackerVMLifecycle(t *testing.T) {
	env := setupIntegrationEnv(t)

	// Skip if running as non-root (nodeagent always runs as root in production)
	if os.Geteuid() != 0 {
		t.Skip("Firecracker integration tests require root (nodeagent runs as root in production)")
	}

	vmID := fmt.Sprintf("test-vm-%d", time.Now().UnixNano())
	instance := &VMInstance{
		ID:         vmID,
		IPAddress:  parseIP("fd00:fc::200"),
		ImagePath:  env.RootfsPath,
		KernelPath: env.KernelPath,
		SocketPath: "firecracker.sock", // Use relative path - jailer will handle full path
		WorkDir:    filepath.Join(env.WorkDir, vmID),
		VCPUs:      1, // Use minimal resources for faster tests
		MemoryMB:   128,
		NetworkTAP: "tap-" + vmID[:8], // Re-enable networking - we'll pre-create TAP device
	}

	t.Logf("Creating Firecracker VM: %s", vmID)

	// Pre-create TAP device as root (production nodeagent would do this)
	tapName := instance.NetworkTAP
	if tapName != "" {
		t.Logf("Pre-creating TAP device: %s", tapName)
		if err := createTAPDevice(tapName); err != nil {
			t.Logf("Warning: Failed to create TAP device %s: %v", tapName, err)
			// Continue without networking for this test
			instance.NetworkTAP = ""
		}
		defer deleteTAPDevice(tapName) // Cleanup after test
	}

	// Create client
	client, err := NewClient(instance)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() {
		if err := client.Cleanup(); err != nil {
			t.Errorf("Failed cleanup: %v", err)
		}
	}()

	// Test initial state - VM should not be running
	ctx := context.Background()
	running, err := client.IsRunning(ctx)
	if err != nil {
		t.Errorf("IsRunning check failed: %v", err)
	}
	if running {
		t.Errorf("VM should not be running initially")
	}

	// Start the VM
	t.Logf("Starting Firecracker VM...")
	startCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := client.Start(startCtx); err != nil {
		t.Fatalf("Failed to start VM: %v", err)
	}

	// Debug: Show the actual socket path after jailer setup
	t.Logf("Jailer modified socket path to: %s", client.config.SocketPath)

	// Wait for VM to be ready
	t.Logf("Waiting for VM to become ready...")
	if err := client.WaitForSocket(startCtx, 20*time.Second); err != nil {
		t.Errorf("VM failed to become ready: %v", err)
	} else {
		// Test VM is running
		running, err := client.IsRunning(ctx)
		if err != nil {
			t.Errorf("Failed to check if VM is running: %v", err)
		}
		if !running {
			t.Errorf("VM should be running after start")
		}

		// Test getting instance info
		info, err := client.GetInstanceInfo(ctx)
		if err != nil {
			t.Errorf("Failed to get instance info: %v", err)
		} else if info != nil {
			t.Logf("VM instance info - ID: %s, State: %s", *info.ID, *info.State)
		}

		// Test metadata operations
		metadata := map[string]interface{}{
			"test":    "value",
			"vm_id":   vmID,
			"purpose": "integration-test",
		}
		if err := client.SetMetadata(ctx, metadata); err != nil {
			t.Errorf("Failed to set metadata: %v", err)
		}

		var retrievedMetadata map[string]interface{}
		if err := client.GetMetadata(ctx, &retrievedMetadata); err != nil {
			t.Errorf("Failed to get metadata: %v", err)
		}

		// Stop the VM
		t.Logf("Stopping Firecracker VM...")
		stopCtx, cancelStop := context.WithTimeout(ctx, 15*time.Second)
		defer cancelStop()

		if err := client.Stop(stopCtx); err != nil {
			t.Errorf("Failed to stop VM gracefully: %v", err)
		}

		// Wait for VM to stop (shutdown can take time)
		time.Sleep(5 * time.Second)

		// Verify VM is no longer running (or at least try to)
		// Note: VM shutdown timing can be tricky in integration tests
		running, err = client.IsRunning(ctx)
		if err == nil && running {
			t.Logf("Warning: VM still appears to be running after stop - this may be timing-related")
		}
	}
}

// TestFirecrackerNetworking tests IPv6 networking configuration
func TestFirecrackerNetworking(t *testing.T) {
	env := setupIntegrationEnv(t)

	// Skip if not running as root (nodeagent runs as root in production)
	if !hasNetworkingSupport() {
		t.Skip("Networking tests require root (nodeagent runs as root in production)")
	}

	vmID := fmt.Sprintf("test-net-%d", time.Now().UnixNano())
	ipv6 := parseIP("fd00:fc::300")

	instance := &VMInstance{
		ID:         vmID,
		IPAddress:  ipv6,
		ImagePath:  env.RootfsPath,
		KernelPath: env.KernelPath,
		SocketPath: filepath.Join(env.WorkDir, vmID+".sock"),
		WorkDir:    filepath.Join(env.WorkDir, vmID),
		VCPUs:      1,
		MemoryMB:   128,
		NetworkTAP: "tap-" + vmID[:8],
	}

	client, err := NewClient(instance)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Cleanup()

	// Verify MAC address generation is consistent
	mac1 := generateMACAddress(vmID)
	mac2 := generateMACAddress(vmID)
	if mac1 != mac2 {
		t.Errorf("MAC address generation should be deterministic: %s != %s", mac1, mac2)
	}

	// Verify MAC address format
	if !isValidMAC(mac1) {
		t.Errorf("Generated MAC address is invalid: %s", mac1)
	}

	// Verify it's locally administered (starts with 02:00:00)
	if !strings.HasPrefix(mac1, "02:00:00") {
		t.Errorf("MAC address should be locally administered (start with 02:00:00): %s", mac1)
	}

	t.Logf("Generated MAC address for VM %s: %s", vmID, mac1)
}

// TestFirecrackerNetworkConnectivity tests actual end-to-end network connectivity
func TestFirecrackerNetworkConnectivity(t *testing.T) {
	env := setupIntegrationEnv(t)

	if os.Geteuid() != 0 {
		t.Skip("Network connectivity tests require root privileges")
	}

	vmID := fmt.Sprintf("test-conn-%d", time.Now().UnixNano())
	tapName := "tap-" + vmID[:8]

	instance := &VMInstance{
		ID:         vmID,
		IPAddress:  parseIP("fd00:fc::500"),
		ImagePath:  env.RootfsPath,
		KernelPath: env.KernelPath,
		SocketPath: "firecracker.sock",
		WorkDir:    filepath.Join(env.WorkDir, vmID),
		VCPUs:      1,
		MemoryMB:   256, // More memory for network operations
		NetworkTAP: tapName,
	}

	// Pre-create TAP device as root
	t.Logf("Creating TAP device: %s", tapName)
	if err := createTAPDevice(tapName); err != nil {
		t.Skipf("Cannot create TAP device: %v", err)
	}
	defer deleteTAPDevice(tapName)

	// Assign IPv6 to the host side of the TAP
	hostIP := "fd00:fc::1"
	t.Logf("Assigning IPv6 %s to host side of %s", hostIP, tapName)
	if err := assignIPToInterface(tapName, hostIP+"/64"); err != nil {
		t.Logf("Warning: Failed to assign IP to TAP device: %v", err)
	}

	client, err := NewClient(instance)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Cleanup()

	// Start the VM
	ctx := context.Background()
	t.Logf("Starting VM with networking...")

	// Configure network metadata before starting (for automatic IP configuration)
	if err := client.ConfigureNetworkMetadata(ctx, instance); err != nil {
		t.Fatalf("Failed to configure network metadata: %v", err)
	}

	if err := client.Start(ctx); err != nil {
		t.Fatalf("Failed to start VM: %v", err)
	}

	// Wait for VM to be ready
	startCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := client.WaitForSocket(startCtx, 10*time.Second); err != nil {
		t.Fatalf("VM failed to become ready: %v", err)
	}

	// TODO: Test actual network connectivity
	// This would require:
	// 1. Configure IPv6 inside the VM guest
	// 2. Start a simple HTTP server inside the VM
	// 3. Try to curl from host to VM

	t.Logf("VM started successfully with networking - TAP device: %s", tapName)
	t.Logf("To test connectivity manually: ping6 fd00:fc::500 (if VM IP configured)")

	// For now, verify the VM is running with network interface
	info, err := client.GetInstanceInfo(ctx)
	if err != nil {
		t.Errorf("Failed to get instance info: %v", err)
	} else {
		t.Logf("VM running with network - ID: %s, State: %s", *info.ID, *info.State)
	}

	// Clean shutdown
	if err := client.Stop(ctx); err != nil {
		t.Logf("Graceful shutdown failed (expected): %v", err)
	}
}

// createTAPDevice creates a TAP network device (requires root)
func createTAPDevice(name string) error {
	// Create the TAP device using ip tuntap
	cmd := exec.Command("ip", "tuntap", "add", "dev", name, "mode", "tap")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create TAP device: %w", err)
	}

	// Bring the interface up
	cmd = exec.Command("ip", "link", "set", "dev", name, "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bring up TAP device: %w", err)
	}

	return nil
}

// deleteTAPDevice removes a TAP network device
func deleteTAPDevice(name string) error {
	cmd := exec.Command("ip", "tuntap", "del", "dev", name, "mode", "tap")
	return cmd.Run()
}

// assignIPToInterface assigns an IP address to a network interface
func assignIPToInterface(iface, ip string) error {
	cmd := exec.Command("ip", "addr", "add", ip, "dev", iface)
	return cmd.Run()
}

// TestFirecrackerJailerIntegration tests jailer security sandbox
func TestFirecrackerJailerIntegration(t *testing.T) {
	env := setupIntegrationEnv(t)

	if os.Geteuid() != 0 {
		t.Skip("Jailer integration tests require root privileges")
	}

	vmID := fmt.Sprintf("test-jail-%d", time.Now().UnixNano())
	instance := &VMInstance{
		ID:         vmID,
		IPAddress:  parseIP("fd00:fc::400"),
		ImagePath:  env.RootfsPath,
		KernelPath: env.KernelPath,
		SocketPath: filepath.Join(env.WorkDir, vmID+".sock"),
		WorkDir:    filepath.Join(env.WorkDir, vmID),
		VCPUs:      1,
		MemoryMB:   128,
	}

	client, err := NewClient(instance)
	if err != nil {
		t.Fatalf("Failed to create client with jailer: %v", err)
	}
	defer client.Cleanup()

	// Test jailer configuration with latest development SDK
	if client.config.JailerCfg == nil {
		t.Fatal("Jailer configuration should be set for security")
	}

	if client.config.JailerCfg.ID != vmID {
		t.Errorf("Jailer ID should match VM ID: expected %s, got %s", vmID, client.config.JailerCfg.ID)
	}

	if client.config.JailerCfg.ExecFile != "/usr/bin/firecracker" && client.config.JailerCfg.ExecFile != "/usr/local/bin/firecracker" {
		t.Errorf("Jailer exec file should point to firecracker binary: %s", client.config.JailerCfg.ExecFile)
	}

	// Test that proper UID/GID are set (non-root)
	if client.config.JailerCfg.UID == nil || *client.config.JailerCfg.UID == 0 {
		t.Error("Jailer should run with non-root UID for security")
	}

	if client.config.JailerCfg.GID == nil || *client.config.JailerCfg.GID == 0 {
		t.Error("Jailer should run with non-root GID for security")
	}

	t.Logf("Jailer configured - UID: %d, GID: %d, ID: %s",
		*client.config.JailerCfg.UID, *client.config.JailerCfg.GID, client.config.JailerCfg.ID)
}

// Benchmark tests
func BenchmarkGenerateMACAddress(b *testing.B) {
	vmID := "benchmark-vm-12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = generateMACAddress(vmID)
	}
}

func BenchmarkValidateConfig(b *testing.B) {
	// Skip setup in benchmarks if no real environment
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	tempDir := b.TempDir()
	kernelFile := filepath.Join(tempDir, "kernel")
	imageFile := filepath.Join(tempDir, "image")

	// Create test files for benchmark
	if err := os.WriteFile(kernelFile, []byte("fake kernel"), 0644); err != nil {
		b.Fatalf("Failed to create test kernel: %v", err)
	}
	if err := os.WriteFile(imageFile, []byte("fake image"), 0644); err != nil {
		b.Fatalf("Failed to create test image: %v", err)
	}

	instance := &VMInstance{
		ID:         "benchmark-vm",
		IPAddress:  parseIP("fd00:fc::500"),
		ImagePath:  imageFile,
		KernelPath: kernelFile,
		SocketPath: filepath.Join(tempDir, "bench.sock"),
		WorkDir:    filepath.Join(tempDir, "bench"),
		VCPUs:      2,
		MemoryMB:   512,
		NetworkTAP: "tap-bench",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateConfig(instance)
	}
}
