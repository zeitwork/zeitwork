package firecracker

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

// Client wraps the firecracker-go-sdk for our use case
type Client struct {
	machine   *firecracker.Machine
	config    firecracker.Config
	machineID string
}

// VMInstance represents a VM instance configuration
type VMInstance struct {
	ID         string
	IPAddress  *net.IP
	ImagePath  string
	KernelPath string
	InitrdPath string
	SocketPath string
	WorkDir    string
	VCPUs      int
	MemoryMB   int
	NetworkTAP string
}

// NewClient creates a new Firecracker client using the firecracker-go-sdk
func NewClient(instance *VMInstance) (*Client, error) {
	// Validate the configuration
	if err := ValidateConfig(instance); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Ensure work directory exists
	if err := os.MkdirAll(instance.WorkDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory %s: %w", instance.WorkDir, err)
	}

	// Ensure socket directory exists
	socketDir := filepath.Dir(instance.SocketPath)
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create socket directory %s: %w", socketDir, err)
	}

	// Build the Firecracker configuration
	config := firecracker.Config{
		SocketPath:        instance.SocketPath,
		KernelImagePath:   instance.KernelPath,
		KernelArgs:        buildKernelArgs(),
		LogLevel:          "Info",
		DisableValidation: false,
		// Note: LogPath omitted - when using jailer, log path handling is complex
		// The jailer environment may not have the same directory structure
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(int64(instance.VCPUs)),
			MemSizeMib: firecracker.Int64(int64(instance.MemoryMB)),
			Smt:        firecracker.Bool(false),
		},
		Drives: []models.Drive{
			{
				DriveID:      firecracker.String("rootfs"),
				PathOnHost:   firecracker.String(instance.ImagePath),
				IsRootDevice: firecracker.Bool(true),
				IsReadOnly:   firecracker.Bool(false),
			},
		},
	}

	// Add initrd if provided
	if instance.InitrdPath != "" {
		config.InitrdPath = instance.InitrdPath
	}

	// Configure networking if TAP device is specified
	if instance.NetworkTAP != "" {
		config.NetworkInterfaces = firecracker.NetworkInterfaces{
			{
				StaticConfiguration: &firecracker.StaticNetworkConfiguration{
					MacAddress:  generateMACAddress(instance.ID),
					HostDevName: instance.NetworkTAP,
				},
				AllowMMDS: true, // Allow VM to access metadata for automatic IP configuration
			},
		}
	}

	// Configure jailer for security isolation (nodeagent always runs as root)
	// ✅ JAILER NOW WORKING! Fixed missing NumaNode and ChrootStrategy fields
	// Using latest SDK development version (v1.0.1-0.20250818195323) + Firecracker v1.13.1
	jailerBaseDir := "/srv/jailer" // Use default jailer location
	if err := os.MkdirAll(jailerBaseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create jailer base directory: %w", err)
	}

	config.JailerCfg = &firecracker.JailerConfig{
		GID:            firecracker.Int(1000),
		UID:            firecracker.Int(1000),
		ID:             instance.ID,
		NumaNode:       firecracker.Int(0), // CRITICAL: This was missing and causing nil pointer panic!
		ExecFile:       "/usr/local/bin/firecracker",
		JailerBinary:   "/usr/local/bin/jailer",
		ChrootBaseDir:  jailerBaseDir,
		CgroupVersion:  "2", // Use cgroup v2 since that's what's mounted
		Daemonize:      false,
		ChrootStrategy: firecracker.NewNaiveChrootStrategy(instance.KernelPath), // CRITICAL: Another missing field!
	}

	// Create the Firecracker machine
	machine, err := firecracker.NewMachine(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create firecracker machine for VM %s: %w", instance.ID, err)
	}

	return &Client{
		machine:   machine,
		config:    config,
		machineID: instance.ID,
	}, nil
}

// Start starts the VM
func (c *Client) Start(ctx context.Context) error {
	if err := c.machine.Start(ctx); err != nil {
		return fmt.Errorf("failed to start VM %s: %w", c.machineID, err)
	}
	return nil
}

// ConfigureNetworkMetadata sets up MMDS metadata for automatic IP configuration
func (c *Client) ConfigureNetworkMetadata(ctx context.Context, instance *VMInstance) error {
	if instance.IPAddress == nil {
		return nil // No IP to configure
	}

	metadata := map[string]interface{}{
		"network": map[string]interface{}{
			"ipv6_address": instance.IPAddress.String(),
			"interface":    "eth0",
			"prefix_len":   64,
			"gateway":      "fd00:fc::1", // Default gateway for zeitwork platform
		},
		"instance": map[string]interface{}{
			"id":   instance.ID,
			"type": "zeitwork-vm",
		},
	}

	if err := c.SetMetadata(ctx, metadata); err != nil {
		return fmt.Errorf("failed to set network metadata for VM %s: %w", c.machineID, err)
	}

	return nil
}

// Stop stops the VM gracefully
func (c *Client) Stop(ctx context.Context) error {
	if err := c.machine.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to stop VM %s: %w", c.machineID, err)
	}
	return nil
}

// Wait waits for the VM to finish
func (c *Client) Wait(ctx context.Context) error {
	return c.machine.Wait(ctx)
}

// IsRunning checks if the VM is currently running
func (c *Client) IsRunning(ctx context.Context) (bool, error) {
	// With jailer, trust the SDK to handle socket connections properly
	// Just check if we can get instance info - this confirms VM is running and API responsive
	_, err := c.machine.DescribeInstanceInfo(ctx)
	if err != nil {
		return false, nil // VM not running or not responsive
	}

	return true, nil
}

// GetInstanceInfo retrieves VM instance information
func (c *Client) GetInstanceInfo(ctx context.Context) (*models.InstanceInfo, error) {
	info, err := c.machine.DescribeInstanceInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance info for VM %s: %w", c.machineID, err)
	}
	return &info, nil
}

// SetMetadata sets metadata for the VM (MMDS)
func (c *Client) SetMetadata(ctx context.Context, metadata interface{}) error {
	if err := c.machine.SetMetadata(ctx, metadata); err != nil {
		return fmt.Errorf("failed to set metadata for VM %s: %w", c.machineID, err)
	}
	return nil
}

// GetMetadata retrieves metadata from the VM (MMDS)
func (c *Client) GetMetadata(ctx context.Context, metadata interface{}) error {
	if err := c.machine.GetMetadata(ctx, metadata); err != nil {
		return fmt.Errorf("failed to get metadata for VM %s: %w", c.machineID, err)
	}
	return nil
}

// Cleanup cleans up resources (socket, temporary files)
func (c *Client) Cleanup() error {
	// Remove socket if it exists
	if _, err := os.Stat(c.config.SocketPath); err == nil {
		if err := os.Remove(c.config.SocketPath); err != nil {
			return fmt.Errorf("failed to remove socket %s: %w", c.config.SocketPath, err)
		}
	}

	// Additional cleanup can be added here (log files, temporary directories, etc.)
	return nil
}

// WaitForSocket waits for the Firecracker socket to be available
func (c *Client) WaitForSocket(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// When using jailer, need to check if the API is actually responding
		// The socket path gets modified by jailer, so we rely on API responsiveness
		if running, _ := c.IsRunning(ctx); running {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Continue polling
		}
	}

	return fmt.Errorf("timeout waiting for Firecracker API to respond (socket: %s)", c.config.SocketPath)
}

// buildKernelArgs constructs the kernel boot arguments
func buildKernelArgs() string {
	return "console=ttyS0 reboot=k panic=1 pci=off"
}

// generateMACAddress generates a deterministic MAC address for the VM
func generateMACAddress(vmID string) string {
	// Generate a deterministic MAC address based on VM ID
	// This ensures the same VM always gets the same MAC address
	hash := 0
	for _, c := range vmID {
		hash = hash*31 + int(c)
	}

	// Ensure it's a locally administered MAC address (second bit of first byte set)
	return fmt.Sprintf("02:00:00:%02x:%02x:%02x",
		(hash>>16)&0xff,
		(hash>>8)&0xff,
		hash&0xff)
}

// ValidateConfig validates the VM configuration
func ValidateConfig(instance *VMInstance) error {
	if instance.ID == "" {
		return fmt.Errorf("VM ID cannot be empty")
	}

	if instance.KernelPath == "" {
		return fmt.Errorf("kernel path cannot be empty")
	}

	if _, err := os.Stat(instance.KernelPath); os.IsNotExist(err) {
		return fmt.Errorf("kernel file does not exist: %s", instance.KernelPath)
	}

	if instance.ImagePath == "" {
		return fmt.Errorf("image path cannot be empty")
	}

	if _, err := os.Stat(instance.ImagePath); os.IsNotExist(err) {
		return fmt.Errorf("image file does not exist: %s", instance.ImagePath)
	}

	if instance.VCPUs <= 0 {
		return fmt.Errorf("vCPU count must be positive, got %d", instance.VCPUs)
	}

	if instance.MemoryMB <= 0 {
		return fmt.Errorf("memory size must be positive, got %d MB", instance.MemoryMB)
	}

	return nil
}
