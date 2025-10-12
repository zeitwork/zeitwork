package firecracker

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/zeitwork/zeitwork/internal/nodeagent/runtime"
)

//go:embed scripts/setup-host.sh
var setupHostScript string

//go:embed templates/init_wrapper.sh.tmpl
var initWrapperTemplate string

const (
	jailerPath       = "/opt/firecracker/jailer"
	defaultRootfsDir = "/var/lib/firecracker-runtime/vms"
)

// Config holds configuration for Firecracker runtime
type Config struct {
	// Path to firecracker binary
	FirecrackerBinary string
	// Path to kernel image
	KernelImagePath string
	// Kernel boot arguments
	KernelArgs string
	// Path to store VM configuration files
	ConfigDir string
	// Path to store VM rootfs images
	RootfsDir string
	// Path to store VM sockets
	SocketDir string
	// Registry configuration for pulling container images
	RegistryURL  string
	RegistryUser string
	RegistryPass string
}

// vmProcess represents a running Firecracker VM
type vmProcess struct {
	ID         string
	Index      int
	ImageName  string
	IPAddress  string
	Port       int
	TapDevice  string
	RootfsPath string
	ConfigPath string
	JailerDir  string
	StartTime  time.Time
}

// FirecrackerRuntime implements the Runtime interface using Firecracker
type FirecrackerRuntime struct {
	cfg         Config
	logger      *slog.Logger
	vms         map[string]*vmProcess
	mu          sync.RWMutex
	vmCounter   atomic.Int32
	usedIndices map[int]bool
	indexMu     sync.Mutex
	nextIP      atomic.Int32 // Next available IP in 172.16.0.0/16 (starts at 2)
}

// NewFirecrackerRuntime creates a new Firecracker runtime
func NewFirecrackerRuntime(cfg Config, logger *slog.Logger) (*FirecrackerRuntime, error) {
	logger.Info("initializing firecracker runtime")

	// Run host setup script
	logger.Info("running host setup script")
	if err := runSetupHostScript(logger); err != nil {
		return nil, fmt.Errorf("failed to run setup-host.sh: %w", err)
	}
	logger.Info("host setup completed successfully")

	// Validate firecracker binary exists
	if _, err := os.Stat(cfg.FirecrackerBinary); err != nil {
		return nil, fmt.Errorf("firecracker binary not found at %s: %w", cfg.FirecrackerBinary, err)
	}

	// Validate jailer binary exists
	if _, err := os.Stat(jailerPath); err != nil {
		return nil, fmt.Errorf("jailer binary not found at %s: %w", jailerPath, err)
	}

	// Validate kernel image exists
	if _, err := os.Stat(cfg.KernelImagePath); err != nil {
		return nil, fmt.Errorf("kernel image not found at %s: %w", cfg.KernelImagePath, err)
	}

	// Create working directories
	for _, dir := range []string{cfg.ConfigDir, cfg.RootfsDir, cfg.SocketDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Verify /dev/kvm access
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return nil, fmt.Errorf("/dev/kvm not accessible: %w", err)
	}

	fr := &FirecrackerRuntime{
		cfg:         cfg,
		logger:      logger,
		vms:         make(map[string]*vmProcess),
		usedIndices: make(map[int]bool),
	}
	fr.nextIP.Store(2) // Start IP allocation at 172.16.0.2

	// Cleanup any leftover VMs from previous runs
	logger.Info("cleaning up any leftover VMs from previous runs")
	if err := fr.cleanupAll(); err != nil {
		logger.Warn("cleanup failed", "error", err)
	}

	// Authenticate to registry if configured
	if cfg.RegistryURL != "" {
		logger.Info("registry configured, authenticating",
			"registry_url", cfg.RegistryURL,
			"registry_user", cfg.RegistryUser,
		)
		if err := fr.dockerLogin(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to authenticate to registry: %w", err)
		}
		logger.Info("successfully authenticated to registry")
	} else {
		logger.Info("no registry configured, using local Docker only")
	}

	logger.Info("firecracker runtime initialized successfully")
	return fr, nil
}

// dockerLogin authenticates to the configured registry
func (f *FirecrackerRuntime) dockerLogin(ctx context.Context) error {
	if f.cfg.RegistryURL == "" {
		return nil // No registry configured
	}

	if f.cfg.RegistryUser == "" || f.cfg.RegistryPass == "" {
		return fmt.Errorf("registry URL configured but missing credentials")
	}

	// Extract registry host from URL (e.g., "ghcr.io/yourorg" -> "ghcr.io")
	registryHost := f.cfg.RegistryURL
	if strings.Contains(registryHost, "/") {
		registryHost = strings.Split(registryHost, "/")[0]
	}

	f.logger.Info("[REGISTRY] logging in to registry",
		"registry_host", registryHost,
		"username", f.cfg.RegistryUser,
	)

	cmd := exec.CommandContext(ctx, "docker", "login", registryHost,
		"--username", f.cfg.RegistryUser,
		"--password-stdin")

	// Pass password via stdin for security
	cmd.Stdin = strings.NewReader(f.cfg.RegistryPass)
	output, err := cmd.CombinedOutput()

	if err != nil {
		f.logger.Error("[REGISTRY] docker login failed",
			"registry_host", registryHost,
			"error", err,
			"output", string(output),
		)
		return fmt.Errorf("docker login failed: %w: %s", err, string(output))
	}

	f.logger.Info("[REGISTRY] successfully logged in to registry",
		"registry_host", registryHost,
	)

	return nil
}

// Start creates and starts a Firecracker VM
func (f *FirecrackerRuntime) Start(ctx context.Context, instanceID, imageName, ipAddress string, vcpus, memory, port int, envVars map[string]string) error {
	f.logger.Info("starting VM",
		"instance_id", instanceID,
		"image", imageName,
		"vcpus", vcpus,
		"memory_mb", memory,
		"port", port,
	)

	// Check if VM already exists
	f.mu.RLock()
	if _, exists := f.vms[instanceID]; exists {
		f.mu.RUnlock()
		return fmt.Errorf("VM %s already exists", instanceID)
	}
	f.mu.RUnlock()

	// Allocate a VM index and IP address (ignore DB-provided IP)
	vmIndex := f.allocateVMIndex()
	vmIP := f.allocateVMIP()

	f.logger.Info("allocated VM resources",
		"instance_id", instanceID,
		"vm_index", vmIndex,
		"vm_ip", vmIP,
	)

	defer func() {
		// If we fail, release the index
		if _, exists := f.vms[instanceID]; !exists {
			f.releaseVMIndex(vmIndex)
		}
	}()

	// Convert Docker image to rootfs
	f.logger.Info("converting Docker image to rootfs", "instance_id", instanceID)
	rootfsPath, err := f.pullAndConvertImage(ctx, imageName, instanceID, vmIP, vmIndex, envVars)
	if err != nil {
		return fmt.Errorf("failed to convert image: %w", err)
	}

	// Setup networking
	f.logger.Info("setting up networking", "instance_id", instanceID, "index", vmIndex, "ip", vmIP)
	tapDevice, err := f.setupNetworking(vmIndex, vmIP)
	if err != nil {
		return fmt.Errorf("failed to setup networking: %w", err)
	}

	// Cleanup network on failure
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			f.cleanupNetworking(vmIndex)
		}
	}()

	// Create VM configuration
	f.logger.Info("creating VM configuration", "instance_id", instanceID)
	configPath, err := f.createVMConfig(instanceID, vmIndex, vcpus, memory)
	if err != nil {
		return fmt.Errorf("failed to create VM config: %w", err)
	}

	// Start Firecracker with jailer
	f.logger.Info("starting firecracker with jailer", "instance_id", instanceID)
	jailerDir, err := f.startFirecrackerProcess(ctx, instanceID, vmIndex, rootfsPath, configPath)
	if err != nil {
		return fmt.Errorf("failed to start firecracker: %w", err)
	}

	// Track the VM
	vm := &vmProcess{
		ID:         instanceID,
		Index:      vmIndex,
		ImageName:  imageName,
		IPAddress:  vmIP,
		Port:       port,
		TapDevice:  tapDevice,
		RootfsPath: rootfsPath,
		ConfigPath: configPath,
		JailerDir:  jailerDir,
		StartTime:  time.Now(),
	}

	f.mu.Lock()
	f.vms[instanceID] = vm
	f.mu.Unlock()

	cleanupOnError = false // Success, don't cleanup

	// Wait for VM to be ready
	f.logger.Info("waiting for VM to be ready", "instance_id", instanceID, "ip", vmIP)
	if err := f.checkVMReady(ctx, vmIP, port); err != nil {
		f.logger.Warn("VM health check failed", "instance_id", instanceID, "error", err)
		// Don't fail here - VM might just need more time
	} else {
		f.logger.Info("VM is ready", "instance_id", instanceID)
	}

	return nil
}

// Stop stops and cleans up a Firecracker VM
func (f *FirecrackerRuntime) Stop(ctx context.Context, instanceID string) error {
	f.logger.Info("stopping VM", "instance_id", instanceID)

	f.mu.Lock()
	vm, exists := f.vms[instanceID]
	if !exists {
		f.mu.Unlock()
		f.logger.Warn("VM not found", "instance_id", instanceID)
		return nil
	}
	delete(f.vms, instanceID)
	f.mu.Unlock()

	// Release the index
	f.releaseVMIndex(vm.Index)

	// Kill firecracker processes
	f.logger.Info("killing firecracker processes", "instance_id", instanceID)
	exec.Command("pkill", "-9", "-f", fmt.Sprintf("firecracker.*%s", instanceID)).Run()

	// Cleanup networking
	if err := f.cleanupNetworking(vm.Index); err != nil {
		f.logger.Warn("failed to cleanup networking", "instance_id", instanceID, "error", err)
	}

	// Remove jailer directory
	if vm.JailerDir != "" {
		f.logger.Info("removing jailer directory", "instance_id", instanceID, "path", vm.JailerDir)
		if err := os.RemoveAll(vm.JailerDir); err != nil {
			f.logger.Warn("failed to remove jailer directory", "error", err)
		}
	}

	// Remove VM directory
	vmDir := filepath.Join(defaultRootfsDir, instanceID)
	if err := os.RemoveAll(vmDir); err != nil {
		f.logger.Warn("failed to remove VM directory", "error", err)
	}

	f.logger.Info("VM stopped successfully", "instance_id", instanceID)
	return nil
}

// List returns all running VMs managed by this runtime
func (f *FirecrackerRuntime) List(ctx context.Context) ([]runtime.Container, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make([]runtime.Container, 0, len(f.vms))
	for _, vm := range f.vms {
		result = append(result, runtime.Container{
			ID:         vm.ID,
			InstanceID: vm.ID,
			ImageName:  vm.ImageName,
			State:      "running",
			IPAddress:  vm.IPAddress,
		})
	}

	return result, nil
}

// GetStatus returns the status of a specific VM
func (f *FirecrackerRuntime) GetStatus(ctx context.Context, instanceID string) (*runtime.Container, error) {
	f.mu.RLock()
	vm, exists := f.vms[instanceID]
	f.mu.RUnlock()

	if !exists {
		return nil, nil
	}

	return &runtime.Container{
		ID:         vm.ID,
		InstanceID: vm.ID,
		ImageName:  vm.ImageName,
		State:      "running",
		IPAddress:  vm.IPAddress,
	}, nil
}

// StreamLogs streams logs from a Firecracker VM's log file
func (f *FirecrackerRuntime) StreamLogs(ctx context.Context, instanceID string, follow bool) (io.ReadCloser, error) {
	f.mu.RLock()
	vm, exists := f.vms[instanceID]
	f.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("VM not found: %s", instanceID)
	}

	// Log file path in jailer directory
	logPath := filepath.Join(vm.JailerDir, "root", "firecracker.log")

	// Open the log file
	file, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// If not following, just return the file
	if !follow {
		return file, nil
	}

	// If following, we need to implement tail -f functionality
	// For now, just return the file and let the caller handle re-reading
	// TODO: Implement proper log following with inotify or similar
	return file, nil
}

// Close cleans up the runtime
func (f *FirecrackerRuntime) Close() error {
	f.logger.Info("closing firecracker runtime")

	// Stop all VMs
	f.mu.RLock()
	instanceIDs := make([]string, 0, len(f.vms))
	for id := range f.vms {
		instanceIDs = append(instanceIDs, id)
	}
	f.mu.RUnlock()

	ctx := context.Background()
	for _, id := range instanceIDs {
		if err := f.Stop(ctx, id); err != nil {
			f.logger.Error("failed to stop VM", "instance_id", id, "error", err)
		}
	}

	// Final cleanup
	if err := f.cleanupAll(); err != nil {
		f.logger.Warn("final cleanup failed", "error", err)
	}

	f.logger.Info("firecracker runtime closed")
	return nil
}

// Helper methods

// allocateVMIndex allocates a new VM index
func (f *FirecrackerRuntime) allocateVMIndex() int {
	f.indexMu.Lock()
	defer f.indexMu.Unlock()

	// Find the first available index
	for i := 0; i < 1000; i++ {
		if !f.usedIndices[i] {
			f.usedIndices[i] = true
			return i
		}
	}

	// Fallback: use atomic counter
	return int(f.vmCounter.Add(1))
}

// releaseVMIndex releases a VM index
func (f *FirecrackerRuntime) releaseVMIndex(index int) {
	f.indexMu.Lock()
	defer f.indexMu.Unlock()
	delete(f.usedIndices, index)
}

// allocateVMIP allocates a new VM IP address in 172.16.0.0/16 range
func (f *FirecrackerRuntime) allocateVMIP() string {
	// Atomically increment and get next IP
	ipNum := f.nextIP.Add(1)

	// Convert to 172.16.X.Y format
	// ipNum goes from 2, 3, 4, ...
	// 172.16.0.2, 172.16.0.3, ..., 172.16.0.255, 172.16.1.0, ...
	octet3 := (ipNum - 1) / 256
	octet4 := (ipNum - 1) % 256

	return fmt.Sprintf("172.16.%d.%d", octet3, octet4)
}

// pullAndConvertImage pulls a container image and converts it to a rootfs
func (f *FirecrackerRuntime) pullAndConvertImage(ctx context.Context, imageName, instanceID, ipAddress string, vmIndex int, envVars map[string]string) (string, error) {
	f.logger.Info("pulling and converting image", "image", imageName)

	vmDir := filepath.Join(defaultRootfsDir, instanceID)
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create VM directory: %w", err)
	}

	rootfsPath := filepath.Join(vmDir, "rootfs.ext4")

	// Check if rootfs already exists
	if _, err := os.Stat(rootfsPath); err == nil {
		f.logger.Info("rootfs already exists, skipping conversion", "path", rootfsPath)
		return rootfsPath, nil
	}

	// Create temporary directory for extraction
	tempDir := filepath.Join(vmDir, "temp_rootfs")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Pull the image first
	f.logger.Info("pulling Docker image", "image", imageName)
	pullCmd := exec.CommandContext(ctx, "docker", "pull", imageName)
	if output, err := pullCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("docker pull failed: %w: %s", err, string(output))
	}

	// Export Docker container
	f.logger.Info("exporting Docker image")
	tarPath := filepath.Join(vmDir, "rootfs.tar")

	// Inspect the image to get entrypoint and command
	inspectCmd := exec.CommandContext(ctx, "docker", "inspect", imageName)
	inspectOut, err := inspectCmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker inspect failed: %w", err)
	}

	var imageInfo []struct {
		Config struct {
			Entrypoint []string `json:"Entrypoint"`
			Cmd        []string `json:"Cmd"`
			WorkingDir string   `json:"WorkingDir"`
		} `json:"Config"`
	}

	if err := json.Unmarshal(inspectOut, &imageInfo); err != nil {
		return "", fmt.Errorf("failed to parse inspect output: %w", err)
	}

	var entrypoint []string
	var cmd []string
	var workingDir string
	if len(imageInfo) > 0 {
		entrypoint = imageInfo[0].Config.Entrypoint
		cmd = imageInfo[0].Config.Cmd
		workingDir = imageInfo[0].Config.WorkingDir
	}

	f.logger.Info("extracted image metadata",
		"entrypoint", entrypoint,
		"cmd", cmd,
		"workingdir", workingDir)

	createCmd := exec.CommandContext(ctx, "docker", "create", imageName)
	createOut, err := createCmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker create failed: %w", err)
	}
	containerID := strings.TrimSpace(string(createOut))

	exportCmd := exec.CommandContext(ctx, "docker", "export", containerID, "-o", tarPath)
	if err := exportCmd.Run(); err != nil {
		return "", fmt.Errorf("docker export failed: %w", err)
	}

	// Remove container
	exec.CommandContext(ctx, "docker", "rm", containerID).Run()

	// Extract tar to temp directory
	f.logger.Info("extracting rootfs")
	extractCmd := exec.CommandContext(ctx, "tar", "-xf", tarPath, "-C", tempDir)
	if err := extractCmd.Run(); err != nil {
		return "", fmt.Errorf("tar extraction failed: %w", err)
	}

	// Create init wrapper script with environment variables
	initWrapper, err := f.createInitWrapper(ipAddress, workingDir, envVars, entrypoint, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to create init wrapper: %w", err)
	}

	initWrapperPath := filepath.Join(tempDir, "init_wrapper.sh")
	if err := os.WriteFile(initWrapperPath, []byte(initWrapper), 0755); err != nil {
		return "", fmt.Errorf("failed to write init wrapper: %w", err)
	}

	// Setup init
	sbinDir := filepath.Join(tempDir, "sbin")
	os.MkdirAll(sbinDir, 0755)
	originalInit := filepath.Join(sbinDir, "init")
	os.Rename(originalInit, filepath.Join(sbinDir, "init.original"))

	if err := os.Rename(initWrapperPath, originalInit); err != nil {
		// If rename fails, copy instead
		if err := copyFile(initWrapperPath, originalInit); err != nil {
			return "", fmt.Errorf("failed to setup init: %w", err)
		}
	}

	// Create ext4 image
	f.logger.Info("creating ext4 image")
	ddCmd := exec.CommandContext(ctx, "dd", "if=/dev/zero", "of="+rootfsPath, "bs=1M", "count=5120")
	if err := ddCmd.Run(); err != nil {
		return "", fmt.Errorf("dd failed: %w", err)
	}

	mkfsCmd := exec.CommandContext(ctx, "mkfs.ext4", "-F", "-d", tempDir, rootfsPath)
	if err := mkfsCmd.Run(); err != nil {
		return "", fmt.Errorf("mkfs.ext4 failed: %w", err)
	}

	// Cleanup tar file
	os.Remove(tarPath)

	f.logger.Info("rootfs created successfully", "path", rootfsPath)
	return rootfsPath, nil
}

// createInitWrapper creates the init wrapper script from template
func (f *FirecrackerRuntime) createInitWrapper(ipAddress, workingDir string, envVars map[string]string, entrypoint, cmd []string) (string, error) {
	tmpl, err := template.New("init_wrapper").Parse(initWrapperTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse init wrapper template: %w", err)
	}

	data := struct {
		IPAddress  string
		WorkingDir string
		EnvVars    map[string]string
		Entrypoint []string
		Cmd        []string
	}{
		IPAddress:  ipAddress,
		WorkingDir: workingDir,
		EnvVars:    envVars,
		Entrypoint: entrypoint,
		Cmd:        cmd,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute init wrapper template: %w", err)
	}

	return buf.String(), nil
}

// setupNetworking creates and configures networking for a VM using TAP + bridge
func (f *FirecrackerRuntime) setupNetworking(vmIndex int, vmIP string) (string, error) {
	tapName := fmt.Sprintf("tap%d", vmIndex)

	f.logger.Info("setting up TAP device", "tap", tapName, "vm_ip", vmIP)

	// Create TAP device with fcuser ownership so jailer can access it
	createCmd := exec.Command("ip", "tuntap", "add", tapName, "mode", "tap", "user", "fcuser")
	if err := createCmd.Run(); err != nil {
		// Check if already exists
		checkCmd := exec.Command("ip", "link", "show", tapName)
		if checkCmd.Run() != nil {
			return "", fmt.Errorf("failed to create TAP device %s: %w", tapName, err)
		}
		f.logger.Info("TAP device already exists", "tap", tapName)
	}

	// Attach TAP to bridge
	attachCmd := exec.Command("ip", "link", "set", tapName, "master", "br0")
	if err := attachCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to attach %s to br0: %w", tapName, err)
	}

	// Bring TAP device up
	upCmd := exec.Command("ip", "link", "set", tapName, "up")
	if err := upCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to bring up %s: %w", tapName, err)
	}

	f.logger.Info("TAP device configured", "tap", tapName)
	return tapName, nil
}

// cleanupNetworking removes networking for a VM
func (f *FirecrackerRuntime) cleanupNetworking(vmIndex int) error {
	tapName := fmt.Sprintf("tap%d", vmIndex)

	f.logger.Info("cleaning up networking", "index", vmIndex, "tap", tapName)

	// Delete TAP device
	exec.Command("ip", "link", "del", tapName).Run()

	return nil
}

// createVMConfig generates Firecracker VM configuration
func (f *FirecrackerRuntime) createVMConfig(instanceID string, vmIndex, vcpus, memoryMB int) (string, error) {
	config := map[string]interface{}{
		"boot-source": map[string]interface{}{
			"kernel_image_path": "vmlinux",
			"boot_args":         f.cfg.KernelArgs,
		},
		"drives": []map[string]interface{}{
			{
				"drive_id":       "rootfs",
				"path_on_host":   "rootfs.ext4",
				"is_root_device": true,
				"is_read_only":   false,
			},
		},
		"machine-config": map[string]interface{}{
			"vcpu_count":   vcpus,
			"mem_size_mib": memoryMB,
		},
		"network-interfaces": []map[string]interface{}{
			{
				"iface_id":      "eth0",
				"guest_mac":     fmt.Sprintf("06:00:AC:10:%02X:%02X", vmIndex, vmIndex+2),
				"host_dev_name": fmt.Sprintf("tap%d", vmIndex),
			},
		},
	}

	vmDir := filepath.Join(defaultRootfsDir, instanceID)
	configPath := filepath.Join(vmDir, "vm_config.json")

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write config: %w", err)
	}

	return configPath, nil
}

// startFirecrackerProcess starts the Firecracker VMM process with jailer
func (f *FirecrackerRuntime) startFirecrackerProcess(ctx context.Context, instanceID string, vmIndex int, rootfsPath, configPath string) (string, error) {
	f.logger.Info("starting firecracker with jailer", "instance_id", instanceID)

	vmDir := filepath.Join(defaultRootfsDir, instanceID)

	// Copy kernel to VM directory
	kernelCopy := filepath.Join(vmDir, "vmlinux")
	if err := copyFile(f.cfg.KernelImagePath, kernelCopy); err != nil {
		return "", fmt.Errorf("failed to copy kernel: %w", err)
	}

	// Get fcuser UID/GID
	getUIDCmd := exec.Command("id", "-u", "fcuser")
	uidOut, err := getUIDCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get fcuser UID: %w", err)
	}
	uid := strings.TrimSpace(string(uidOut))

	getGIDCmd := exec.Command("id", "-g", "fcuser")
	gidOut, err := getGIDCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get fcuser GID: %w", err)
	}
	gid := strings.TrimSpace(string(gidOut))

	// Change ownership of VM files
	chownCmd := exec.Command("chown", "-R", uid+":"+gid, vmDir)
	if err := chownCmd.Run(); err != nil {
		f.logger.Warn("chown failed", "error", err)
	}

	// Create hard links for jailer
	// First, remove any existing jailer directory for this instance
	jailerDir := filepath.Join("/srv/jailer/firecracker", instanceID)
	os.RemoveAll(jailerDir)

	jailerRoot := filepath.Join(jailerDir, "root")
	os.MkdirAll(jailerRoot, 0755)

	// Link kernel
	kernelLink := filepath.Join(jailerRoot, "vmlinux")
	os.Remove(kernelLink)
	if err := os.Link(kernelCopy, kernelLink); err != nil {
		return "", fmt.Errorf("failed to link kernel: %w", err)
	}

	// Link rootfs
	rootfsLink := filepath.Join(jailerRoot, "rootfs.ext4")
	os.Remove(rootfsLink)
	if err := os.Link(rootfsPath, rootfsLink); err != nil {
		return "", fmt.Errorf("failed to link rootfs: %w", err)
	}

	// Link config
	configLink := filepath.Join(jailerRoot, "vm_config.json")
	os.Remove(configLink)
	if err := os.Link(configPath, configLink); err != nil {
		return "", fmt.Errorf("failed to link config: %w", err)
	}

	// Change ownership of jailer root
	chownJailCmd := exec.Command("chown", "-R", uid+":"+gid, jailerRoot)
	chownJailCmd.Run()

	// Prepare jailer command
	// Create log file for firecracker
	logPath := filepath.Join(jailerRoot, "firecracker.log")
	// Pre-create the log file with proper ownership so Firecracker can write to it
	if logFile, err := os.Create(logPath); err == nil {
		logFile.Close()
		chownLogCmd := exec.Command("chown", uid+":"+gid, logPath)
		chownLogCmd.Run()
	}

	jailerArgs := []string{
		"--id", instanceID,
		"--exec-file", f.cfg.FirecrackerBinary,
		"--uid", uid,
		"--gid", gid,
		"--daemonize",
		"--",
		"--config-file", "vm_config.json",
		"--log-path", "firecracker.log",
		"--level", "Debug",
		"--no-api",
	}

	jailerCmd := exec.CommandContext(ctx, jailerPath, jailerArgs...)
	output, err := jailerCmd.CombinedOutput()
	if err != nil {
		f.logger.Error("jailer execution failed",
			"error", err,
			"output", string(output),
			"log_path", logPath,
		)
		return "", fmt.Errorf("jailer execution failed: %w (output: %s)", err, string(output))
	}

	f.logger.Info("firecracker started successfully", "instance_id", instanceID)
	return jailerDir, nil
}

// checkVMReady polls VM until it's ready to accept traffic
func (f *FirecrackerRuntime) checkVMReady(ctx context.Context, ipAddress string, port int) error {
	target := fmt.Sprintf("http://%s:%d", ipAddress, port)
	maxRetries := 30
	retryDelay := 5 * time.Second

	f.logger.Info("performing health check", "target", target)

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := client.Get(target)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 500 {
				f.logger.Info("health check successful",
					"attempt", attempt,
					"status", resp.StatusCode,
				)
				return nil
			}
		}

		if attempt < maxRetries {
			if attempt == 1 || attempt%5 == 0 {
				f.logger.Info("health check attempt failed, retrying",
					"attempt", attempt,
					"max_retries", maxRetries,
					"error", err,
				)
			}
			time.Sleep(retryDelay)
		}
	}

	return fmt.Errorf("health check failed after %d attempts", maxRetries)
}

// cleanupAll performs a full cleanup of all firecracker resources
func (f *FirecrackerRuntime) cleanupAll() error {
	f.logger.Info("performing full cleanup")

	// Stop all Firecracker processes
	exec.Command("pkill", "-9", "firecracker").Run()
	exec.Command("pkill", "-9", "jailer").Run()

	// Give processes time to terminate
	time.Sleep(1 * time.Second)

	// Clean up jailer directories
	jailerBaseDir := "/srv/jailer/firecracker"
	if entries, err := os.ReadDir(jailerBaseDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				jailerDir := filepath.Join(jailerBaseDir, entry.Name())
				os.RemoveAll(jailerDir)
			}
		}
	}

	// Remove TAP devices
	f.logger.Info("cleaning up TAP devices")
	for i := 0; i < 100; i++ {
		tapName := fmt.Sprintf("tap%d", i)
		exec.Command("ip", "link", "del", tapName).Run() // Ignore errors
	}

	// Note: We do NOT delete bridge br0 here - it's managed by setup-host.sh
	// and should persist across VM restarts. We only clean up the TAP devices
	// that are attached to it.

	f.logger.Info("cleanup complete")
	return nil
}

// runSetupHostScript executes the embedded setup-host.sh script
func runSetupHostScript(logger *slog.Logger) error {
	// Create temporary file with the script contents
	tmpFile, err := os.CreateTemp("", "setup-host-*.sh")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write embedded script to temp file
	if _, err := tmpFile.WriteString(setupHostScript); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write script to temp file: %w", err)
	}

	// Make script executable
	if err := tmpFile.Chmod(0755); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to make script executable: %w", err)
	}

	tmpFile.Close()

	// Execute the script
	cmd := exec.Command("/bin/bash", tmpFile.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logger.Info("executing setup-host.sh", "script_path", tmpFile.Name())

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setup script failed: %w", err)
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Copy permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}
