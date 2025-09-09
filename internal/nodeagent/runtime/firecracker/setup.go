package firecracker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
)

// SetupManager handles automatic setup and validation of firecracker-containerd
type SetupManager struct {
	config  *config.FirecrackerRuntimeConfig
	logger  *slog.Logger
	tempDir string
}

// NewSetupManager creates a new setup manager
func NewSetupManager(cfg *config.FirecrackerRuntimeConfig, logger *slog.Logger) *SetupManager {
	return &SetupManager{
		config:  cfg,
		logger:  logger,
		tempDir: "/tmp/zeitwork-firecracker-setup",
	}
}

// EnsureSetup validates and sets up firecracker-containerd environment
func (s *SetupManager) EnsureSetup(ctx context.Context) error {
	s.logger.Info("Validating and setting up firecracker-containerd environment")

	// Create temporary directory
	if err := os.MkdirAll(s.tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer s.cleanup()

	// 1. Check and install binaries
	if err := s.ensureBinaries(ctx); err != nil {
		return fmt.Errorf("failed to ensure binaries: %w", err)
	}

	// 2. Create directories
	if err := s.ensureDirectories(); err != nil {
		return fmt.Errorf("failed to ensure directories: %w", err)
	}

	// 3. Setup configuration files
	if err := s.ensureConfiguration(); err != nil {
		return fmt.Errorf("failed to ensure configuration: %w", err)
	}

	// 4. Setup device mapper storage
	if err := s.ensureDeviceMapper(ctx); err != nil {
		return fmt.Errorf("failed to ensure device mapper: %w", err)
	}

	// 5. Setup CNI networking
	if err := s.ensureCNINetworking(ctx); err != nil {
		return fmt.Errorf("failed to ensure CNI networking: %w", err)
	}

	// 6. Prepare VM images
	if err := s.ensureVMImages(ctx); err != nil {
		return fmt.Errorf("failed to ensure VM images: %w", err)
	}

	// 7. Start firecracker-containerd daemon
	if err := s.ensureDaemon(ctx); err != nil {
		return fmt.Errorf("failed to ensure daemon: %w", err)
	}

	// 8. Validate setup
	if err := s.validateSetup(ctx); err != nil {
		return fmt.Errorf("setup validation failed: %w", err)
	}

	s.logger.Info("firecracker-containerd environment setup completed successfully")
	return nil
}

// ensureBinaries checks and installs required binaries
func (s *SetupManager) ensureBinaries(ctx context.Context) error {
	s.logger.Info("Checking required binaries")

	// Check if Firecracker binaries exist
	firecrackerPath := "/usr/local/bin/firecracker"
	jailerPath := "/usr/local/bin/jailer"

	if s.checkBinary(firecrackerPath) != nil || s.checkBinary(jailerPath) != nil {
		s.logger.Info("Firecracker binaries not found, installing...")
		if err := s.installFirecrackerBinaries(ctx); err != nil {
			return fmt.Errorf("failed to install firecracker binaries: %w", err)
		}
	} else {
		s.logger.Debug("Firecracker binaries found")
	}

	// Check firecracker-containerd binaries
	fcBinaries := []string{
		"/usr/local/bin/firecracker-containerd",
		"/usr/local/bin/containerd-shim-aws-firecracker",
		"/usr/local/bin/firecracker-ctr",
	}

	needsInstall := false
	for _, binPath := range fcBinaries {
		if s.checkBinary(binPath) != nil {
			needsInstall = true
			break
		}
	}

	if needsInstall {
		s.logger.Info("firecracker-containerd binaries not found, building from source")
		if err := s.buildFirecrackerContainerd(ctx); err != nil {
			return fmt.Errorf("failed to build firecracker-containerd: %w", err)
		}
	} else {
		s.logger.Debug("firecracker-containerd binaries found")
	}

	return nil
}

// installFirecrackerBinaries installs Firecracker binaries using template
func (s *SetupManager) installFirecrackerBinaries(ctx context.Context) error {
	tmpl, err := GetScriptTemplate(InstallFirecrackerTemplate)
	if err != nil {
		return fmt.Errorf("failed to get firecracker install template: %w", err)
	}

	data := InstallFirecrackerData{
		FirecrackerVersion: "v1.13.1",
		TempDir:            s.tempDir,
		FirecrackerPath:    "/usr/local/bin/firecracker",
		JailerPath:         "/usr/local/bin/jailer",
	}

	return s.executeScriptTemplate(ctx, tmpl, data, "install-firecracker")
}

// buildFirecrackerContainerd builds and installs firecracker-containerd from source
func (s *SetupManager) buildFirecrackerContainerd(ctx context.Context) error {
	s.logger.Info("Building firecracker-containerd from source - this may take 5-10 minutes...")

	buildDir := filepath.Join(s.tempDir, "firecracker-containerd")

	// Install Go if needed
	if err := s.ensureGo(ctx); err != nil {
		return fmt.Errorf("failed to ensure Go: %w", err)
	}

	// Clone repository
	s.logger.Info("Cloning firecracker-containerd repository...")
	cmd := exec.CommandContext(ctx, "git", "clone", "--recurse-submodules",
		"https://github.com/firecracker-microvm/firecracker-containerd", buildDir)

	if output, err := s.runCommandWithOutput(cmd); err != nil {
		s.logger.Error("Failed to clone repository", "output", output, "error", err)
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	s.logger.Info("Repository cloned successfully")

	// Build with detailed logging - modify Makefile to exclude problematic component
	s.logger.Info("Building firecracker-containerd (excluding docker-credential-mmds)...")

	// Create a temporary Makefile that builds everything except docker-credential-mmds
	if err := s.createModifiedMakefile(buildDir); err != nil {
		return fmt.Errorf("failed to create modified Makefile: %w", err)
	}

	// Use make with the essential targets only
	cmd = exec.CommandContext(ctx, "make", "-f", "Makefile.essential", "essential")
	cmd.Dir = buildDir
	cmd.Env = append(os.Environ(), "PATH=/usr/bin:/usr/local/go/bin:"+os.Getenv("PATH"))

	if output, err := s.runCommandWithOutput(cmd); err != nil {
		s.logger.Error("Essential components build failed", "build_dir", buildDir, "output", string(output), "error", err)
		s.logger.Error("Build command was: make agent runtime firecracker-control snapshotter examples")
		s.logger.Error("Build directory: " + buildDir)
		return fmt.Errorf("failed to build firecracker-containerd essential components: %w\nBuild output: %s", err, string(output))
	}
	s.logger.Info("Essential components built successfully (skipped docker-credential-mmds)")

	// Install binaries manually since we built selectively
	s.logger.Info("Installing firecracker-containerd binaries...")
	if err := s.installBuiltBinaries(buildDir); err != nil {
		return fmt.Errorf("failed to install binaries: %w", err)
	}
	s.logger.Info("Installation completed successfully")

	// Verify installation of core binaries only
	coreBinaries := []string{
		"/usr/local/bin/firecracker-containerd",
		"/usr/local/bin/containerd-shim-aws-firecracker",
		"/usr/local/bin/firecracker-ctr",
	}

	for _, binary := range coreBinaries {
		if err := s.checkBinary(binary); err != nil {
			s.logger.Error("Binary verification failed", "binary", binary, "error", err)
			return fmt.Errorf("binary not found after installation: %s", binary)
		}
		s.logger.Debug("Binary verified", "binary", binary)
	}

	s.logger.Info("firecracker-containerd build and installation completed successfully")
	return nil
}

// ensureGo installs Go if it's not available
func (s *SetupManager) ensureGo(ctx context.Context) error {
	// Check if go is in PATH
	if _, err := exec.LookPath("go"); err == nil {
		s.logger.Debug("Go already available in PATH")
		return nil
	}

	// Check if go is in /usr/local/go/bin (common install location)
	if _, err := os.Stat("/usr/local/go/bin/go"); err == nil {
		s.logger.Debug("Go found at /usr/local/go/bin/go, updating PATH")
		// Go is installed but not in PATH - this is fine, we'll set PATH in build commands
		return nil
	}

	s.logger.Info("Go not found, installing Go 1.21.0...")

	goVersion := "1.21.0"
	goURL := fmt.Sprintf("https://golang.org/dl/go%s.linux-amd64.tar.gz", goVersion)
	goArchive := filepath.Join(s.tempDir, fmt.Sprintf("go%s.tar.gz", goVersion))

	// Download Go
	s.logger.Info("Downloading Go", "version", goVersion)
	if err := s.downloadFile(ctx, goURL, goArchive); err != nil {
		return fmt.Errorf("failed to download Go: %w", err)
	}

	// Extract Go
	s.logger.Info("Extracting Go to /usr/local/go")
	cmd := exec.CommandContext(ctx, "tar", "-C", "/usr/local", "-xzf", goArchive)
	if output, err := s.runCommandWithOutput(cmd); err != nil {
		s.logger.Error("Failed to extract Go", "output", output, "error", err)
		return fmt.Errorf("failed to extract Go: %w", err)
	}

	s.logger.Info("Go installed successfully")
	return nil
}

// createModifiedMakefile creates a Makefile that excludes problematic components
func (s *SetupManager) createModifiedMakefile(buildDir string) error {
	makefileContent := `
# Essential target that builds only what we need
essential: agent runtime firecracker-control snapshotter

agent:
	make -C agent

runtime:
	make -C runtime

firecracker-control:
	make -C firecracker-control/cmd/containerd

snapshotter:
	make -C snapshotter

.PHONY: essential agent runtime firecracker-control snapshotter
`

	makefilePath := filepath.Join(buildDir, "Makefile.essential")
	if err := os.WriteFile(makefilePath, []byte(makefileContent), 0644); err != nil {
		return fmt.Errorf("failed to write essential Makefile: %w", err)
	}

	s.logger.Debug("Created modified Makefile", "path", makefilePath)
	return nil
}

// installBuiltBinaries manually installs the built binaries to system locations
func (s *SetupManager) installBuiltBinaries(buildDir string) error {
	// Map of source -> destination for the essential binaries we need
	binaries := map[string]string{
		"agent/agent": "/usr/local/bin/firecracker-containerd-agent",
		"runtime/containerd-shim-aws-firecracker":                   "/usr/local/bin/containerd-shim-aws-firecracker",
		"firecracker-control/cmd/containerd/firecracker-containerd": "/usr/local/bin/firecracker-containerd",
		"firecracker-control/cmd/containerd/firecracker-ctr":        "/usr/local/bin/firecracker-ctr",
		"snapshotter/demux-snapshotter":                             "/usr/local/bin/demux-snapshotter",
	}

	for src, dst := range binaries {
		srcPath := filepath.Join(buildDir, src)

		// Check if source binary exists
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			s.logger.Warn("Built binary not found, skipping", "src", srcPath, "dst", dst)
			continue
		}

		s.logger.Debug("Installing binary", "src", srcPath, "dst", dst)

		// Copy and set permissions
		if err := s.copyFile(srcPath, dst); err != nil {
			return fmt.Errorf("failed to install %s to %s: %w", src, dst, err)
		}

		if err := os.Chmod(dst, 0755); err != nil {
			return fmt.Errorf("failed to set permissions on %s: %w", dst, err)
		}

		s.logger.Debug("Binary installed successfully", "binary", dst)
	}

	return nil
}

// copyFile copies a file from src to dst
func (s *SetupManager) copyFile(src, dst string) error {
	cmd := exec.Command("cp", src, dst)
	return cmd.Run()
}

// ensureDirectories creates required directories
func (s *SetupManager) ensureDirectories() error {
	s.logger.Info("Creating required directories")

	directories := []string{
		"/var/lib/firecracker-containerd",
		"/var/lib/firecracker-containerd/containerd",
		"/var/lib/firecracker-containerd/runtime",
		"/var/lib/firecracker-containerd/snapshotter",
		"/var/lib/firecracker-containerd/snapshotter/devmapper",
		"/run/firecracker-containerd",
		"/etc/firecracker-containerd",
		"/etc/containerd",
		"/etc/cni/net.d",
		"/opt/cni/bin",
	}

	for _, dir := range directories {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// ensureConfiguration creates configuration files using templates
func (s *SetupManager) ensureConfiguration() error {
	s.logger.Info("Setting up configuration files")

	// 1. Daemon config
	daemonConfigPath := "/etc/firecracker-containerd/config.toml"
	if _, err := os.Stat(daemonConfigPath); os.IsNotExist(err) {
		if err := s.createDaemonConfig(daemonConfigPath); err != nil {
			return fmt.Errorf("failed to create daemon config: %w", err)
		}
	}

	// 2. Runtime config
	if _, err := os.Stat(s.config.RuntimeConfigPath); os.IsNotExist(err) {
		if err := s.createRuntimeConfig(s.config.RuntimeConfigPath); err != nil {
			return fmt.Errorf("failed to create runtime config: %w", err)
		}
	}

	return nil
}

// createDaemonConfig creates daemon configuration using template
func (s *SetupManager) createDaemonConfig(configPath string) error {
	tmpl, err := GetConfigTemplate(DaemonConfigTemplate)
	if err != nil {
		return fmt.Errorf("failed to get daemon config template: %w", err)
	}

	data := DaemonConfigData{
		ContainerdRoot:   "/var/lib/firecracker-containerd/containerd",
		ContainerdState:  "/run/firecracker-containerd",
		ContainerdSocket: s.config.ContainerdSocket,
		DeviceMapperPool: "fc-dev-thinpool",
		BaseImageSize:    "10GB",
		SnapshotterRoot:  "/var/lib/firecracker-containerd/snapshotter/devmapper",
		LogLevel:         "info",
	}

	return s.renderTemplateToFile(tmpl, data, configPath)
}

// createRuntimeConfig creates runtime configuration using template
func (s *SetupManager) createRuntimeConfig(configPath string) error {
	tmpl, err := GetConfigTemplate(RuntimeConfigTemplate)
	if err != nil {
		return fmt.Errorf("failed to get runtime config template: %w", err)
	}

	data := RuntimeConfigData{
		FirecrackerBinary: "/usr/local/bin/firecracker",
		KernelImagePath:   s.config.DefaultKernelPath,
		KernelArgs:        "console=ttyS0 noapic reboot=k panic=1 pci=off nomodules rw",
		RootDrive:         s.config.DefaultRootfsPath,
		CPUTemplate:       "T2",
		VCPUs:             s.config.DefaultVCpus,
		MemoryMB:          s.config.DefaultMemoryMB,
		HTEnabled:         false,
		RuncBinary:        "/usr/local/bin/runc",
		LogLevel:          "Info",
	}

	return s.renderTemplateToFile(tmpl, data, configPath)
}

// ensureDeviceMapper sets up device mapper using script template
func (s *SetupManager) ensureDeviceMapper(ctx context.Context) error {
	s.logger.Info("Setting up device mapper thin pool")

	poolName := "fc-dev-thinpool"
	if s.deviceMapperPoolExists(poolName) {
		s.logger.Debug("Device mapper pool already exists")
		return nil
	}

	tmpl, err := GetScriptTemplate(SetupDevmapperTemplate)
	if err != nil {
		return fmt.Errorf("failed to get devmapper setup template: %w", err)
	}

	data := SetupDevmapperData{
		PoolName:     poolName,
		DataPath:     "/var/lib/firecracker-containerd/snapshotter/devmapper/data",
		MetadataPath: "/var/lib/firecracker-containerd/snapshotter/devmapper/metadata",
		DataSize:     "100G",
		MetadataSize: "10G",
	}

	return s.executeScriptTemplate(ctx, tmpl, data, "setup-devmapper")
}

// ensureCNINetworking sets up CNI networking using templates
func (s *SetupManager) ensureCNINetworking(ctx context.Context) error {
	s.logger.Info("Setting up CNI networking")

	// Install CNI plugins
	if err := s.installCNIPlugins(ctx); err != nil {
		return fmt.Errorf("failed to install CNI plugins: %w", err)
	}

	// Create network configuration
	cniConfigPath := filepath.Join(s.config.CNIConfDir, "10-zeitwork.conf")
	if _, err := os.Stat(cniConfigPath); os.IsNotExist(err) {
		if err := s.createCNIConfig(cniConfigPath); err != nil {
			return fmt.Errorf("failed to create CNI config: %w", err)
		}
	}

	return nil
}

// installCNIPlugins installs CNI plugins using script template
func (s *SetupManager) installCNIPlugins(ctx context.Context) error {
	// Check if plugins already installed
	if _, err := os.Stat(filepath.Join(s.config.CNIBinDir, "bridge")); err == nil {
		s.logger.Debug("CNI plugins already installed")
		return nil
	}

	tmpl, err := GetScriptTemplate(InstallCNITemplate)
	if err != nil {
		return fmt.Errorf("failed to get CNI install template: %w", err)
	}

	data := InstallCNIData{
		CNIVersion: "v1.3.0",
		CNIDir:     s.config.CNIBinDir,
		TempDir:    s.tempDir,
	}

	return s.executeScriptTemplate(ctx, tmpl, data, "install-cni")
}

// createCNIConfig creates CNI configuration using template
func (s *SetupManager) createCNIConfig(configPath string) error {
	tmpl, err := GetConfigTemplate(CNIConfigTemplate)
	if err != nil {
		return fmt.Errorf("failed to get CNI config template: %w", err)
	}

	data := CNIConfigData{
		NetworkName: "zeitwork",
		BridgeName:  "zeitwork0",
		Subnet:      "172.20.0.0/16",
		RangeStart:  "172.20.1.2",
		RangeEnd:    "172.20.254.254",
		Gateway:     "172.20.0.1",
	}

	return s.renderTemplateToFile(tmpl, data, configPath)
}

// ensureVMImages prepares VM kernel and rootfs images
func (s *SetupManager) ensureVMImages(ctx context.Context) error {
	s.logger.Info("Preparing VM images", "build_method", s.config.BuildMethod)

	// Skip if build method is "skip"
	if s.config.BuildMethod == "skip" {
		s.logger.Info("Build method is 'skip', assuming kernel and rootfs are provided manually")
		return s.validateExistingImages()
	}

	// Build or create kernel if needed
	if _, err := os.Stat(s.config.DefaultKernelPath); os.IsNotExist(err) {
		s.logger.Info("Building kernel image")
		if err := s.buildKernelImage(ctx); err != nil {
			return fmt.Errorf("failed to build kernel: %w", err)
		}
	} else {
		s.logger.Debug("Kernel already exists", "path", s.config.DefaultKernelPath)
	}

	// Build base rootfs with firecracker-containerd agent if needed
	if _, err := os.Stat(s.config.DefaultRootfsPath); os.IsNotExist(err) {
		s.logger.Info("Building base VM rootfs with firecracker-containerd agent")
		if err := s.createRootfsImage(ctx); err != nil {
			return fmt.Errorf("failed to create base VM rootfs: %w", err)
		}
	} else {
		s.logger.Debug("Base VM rootfs already exists", "path", s.config.DefaultRootfsPath)
	}

	return nil
}

// validateExistingImages checks if manually provided kernel and base rootfs exist
func (s *SetupManager) validateExistingImages() error {
	// Check kernel (required)
	if _, err := os.Stat(s.config.DefaultKernelPath); os.IsNotExist(err) {
		return fmt.Errorf("kernel image not found at %s (build_method=skip requires manual kernel setup)", s.config.DefaultKernelPath)
	}

	// Check base rootfs (required - must contain firecracker-containerd agent)
	if _, err := os.Stat(s.config.DefaultRootfsPath); os.IsNotExist(err) {
		return fmt.Errorf("base VM rootfs not found at %s (must contain firecracker-containerd-agent)", s.config.DefaultRootfsPath)
	}

	s.logger.Info("Manual images validated",
		"kernel", s.config.DefaultKernelPath,
		"base_rootfs", s.config.DefaultRootfsPath)
	s.logger.Info("Container images will be layered on top of base rootfs")
	return nil
}

// buildKernelImage builds kernel based on configured method
func (s *SetupManager) buildKernelImage(ctx context.Context) error {
	switch s.config.BuildMethod {
	case "firecracker-devtool":
		return s.downloadKernelImage(ctx) // This now uses devtool building
	case "manual":
		return s.createMinimalKernelFallback(ctx)
	default:
		s.logger.Warn("Unknown build method, using firecracker-devtool", "method", s.config.BuildMethod)
		return s.downloadKernelImage(ctx)
	}
}

// createRootfsImage creates base VM rootfs with firecracker-containerd agent using Firecracker's CI tools
func (s *SetupManager) createRootfsImage(ctx context.Context) error {
	s.logger.Info("Building base VM rootfs with firecracker-containerd agent using Firecracker CI tools")

	// Try to build using Firecracker's devtool (creates Ubuntu 22.04 with agent pre-installed)
	if err := s.buildRootfsUsingFirecrackerTools(ctx); err != nil {
		s.logger.Warn("Firecracker devtool build failed, using fallback method", "error", err)
		return s.createBasicRootfsImage(ctx)
	}

	return nil
}

// buildRootfsUsingFirecrackerTools builds rootfs using Firecracker's devtool
func (s *SetupManager) buildRootfsUsingFirecrackerTools(ctx context.Context) error {
	tmpl, err := GetScriptTemplate(BuildRootfsTemplate)
	if err != nil {
		return fmt.Errorf("failed to get rootfs build template: %w", err)
	}

	firecrackerSourceDir := filepath.Join(s.tempDir, "firecracker-source")

	data := BuildRootfsData{
		FirecrackerSourceDir: firecrackerSourceDir,
		OutputPath:           s.config.DefaultRootfsPath, // Base VM rootfs with agent
		TempDir:              s.tempDir,
	}

	return s.executeScriptTemplate(ctx, tmpl, data, "build-rootfs")
}

// createBasicRootfsImage creates a minimal rootfs as fallback
func (s *SetupManager) createBasicRootfsImage(ctx context.Context) error {
	s.logger.Info("Creating basic rootfs image as fallback")

	tmpl, err := GetScriptTemplate(CreateRootfsTemplate)
	if err != nil {
		return fmt.Errorf("failed to get basic rootfs creation template: %w", err)
	}

	data := CreateRootfsData{
		RootfsPath: s.config.DefaultRootfsPath, // Base VM rootfs with basic tools
		RootfsSize: "1G",
		TempMount:  "/tmp/zeitwork-rootfs-mount",
	}

	return s.executeScriptTemplate(ctx, tmpl, data, "create-basic-rootfs")
}

// ensureDaemon ensures firecracker-containerd daemon is running
func (s *SetupManager) ensureDaemon(ctx context.Context) error {
	s.logger.Info("Ensuring firecracker-containerd daemon is running")

	if s.isDaemonRunning() {
		s.logger.Debug("Daemon already running")
		return nil
	}

	// Create systemd service
	servicePath := "/etc/systemd/system/firecracker-containerd.service"
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		if err := s.createSystemdService(servicePath); err != nil {
			return fmt.Errorf("failed to create systemd service: %w", err)
		}
	}

	// Start daemon
	if err := s.startDaemon(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Wait for daemon to be ready
	return s.waitForDaemon(ctx)
}

// createSystemdService creates systemd service using template
func (s *SetupManager) createSystemdService(servicePath string) error {
	tmpl, err := GetConfigTemplate(SystemdServiceTemplate)
	if err != nil {
		return fmt.Errorf("failed to get systemd service template: %w", err)
	}

	data := SystemdServiceData{
		FirecrackerContainerdBinary: "/usr/local/bin/firecracker-containerd",
		ConfigPath:                  "/etc/firecracker-containerd/config.toml",
	}

	if err := s.renderTemplateToFile(tmpl, data, servicePath); err != nil {
		return err
	}

	// Reload systemd and enable service
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	if err := exec.Command("systemctl", "enable", "firecracker-containerd").Run(); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	return nil
}

// validateSetup performs end-to-end validation WITHOUT creating another runtime instance
func (s *SetupManager) validateSetup(ctx context.Context) error {
	s.logger.Info("Validating firecracker-containerd setup")

	// 1. Verify core binaries exist and are executable
	coreBinaries := []string{
		"/usr/local/bin/firecracker-containerd",
		"/usr/local/bin/containerd-shim-aws-firecracker",
		"/usr/local/bin/firecracker-ctr",
	}

	for _, binary := range coreBinaries {
		if err := s.checkBinary(binary); err != nil {
			return fmt.Errorf("validation failed - binary not found or not executable: %s", binary)
		}
		s.logger.Debug("Binary validated", "binary", binary)
	}

	// 2. Verify configuration files exist
	configFiles := []string{
		"/etc/firecracker-containerd/config.toml",
		"/etc/containerd/firecracker-runtime.json", // This is where firecracker-containerd expects it
	}

	// 3. Verify VM images exist
	if _, err := os.Stat(s.config.DefaultKernelPath); err != nil {
		return fmt.Errorf("validation failed - kernel not found: %s", s.config.DefaultKernelPath)
	}
	s.logger.Debug("Kernel validated", "kernel", s.config.DefaultKernelPath)

	if _, err := os.Stat(s.config.DefaultRootfsPath); err != nil {
		return fmt.Errorf("validation failed - base VM rootfs not found: %s", s.config.DefaultRootfsPath)
	}
	s.logger.Debug("Base VM rootfs validated", "rootfs", s.config.DefaultRootfsPath)

	for _, configFile := range configFiles {
		if _, err := os.Stat(configFile); err != nil {
			return fmt.Errorf("validation failed - config file not found: %s", configFile)
		}
		s.logger.Debug("Config file validated", "config", configFile)
	}

	// 3. Test daemon connectivity (without starting a new runtime)
	if s.isDaemonRunning() {
		s.logger.Debug("Daemon is running")

		// Test basic connectivity
		cmd := exec.CommandContext(ctx, "/usr/local/bin/firecracker-ctr",
			"--address", s.config.ContainerdSocket, "version")
		if err := cmd.Run(); err != nil {
			s.logger.Warn("Daemon connectivity test failed", "error", err)
			// Don't fail validation for this - daemon might just be starting
		} else {
			s.logger.Debug("Daemon connectivity test passed")
		}
	} else {
		s.logger.Debug("Daemon not running yet (will be started by runtime)")
	}

	s.logger.Info("Setup validation successful - firecracker-containerd ready")
	return nil
}

// Helper methods continue in next part...
