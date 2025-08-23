package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InstanceManager handles VM instance operations
type InstanceManager struct {
	server *Server
}

// NewInstanceManager creates a new instance manager
func NewInstanceManager(server *Server) *InstanceManager {
	return &InstanceManager{
		server: server,
	}
}

// CreateInstance creates a new VM instance on a node
func (im *InstanceManager) CreateInstance(node *Node, image *Image, vcpuCount int, memoryMiB int, defaultPort int) (*Instance, error) {
	// Create instance object
	instance := &Instance{
		ID:          generateID("instance"),
		NodeID:      node.ID,
		ImageID:     image.ID,
		Status:      "creating",
		VCPUCount:   vcpuCount,
		MemoryMiB:   memoryMiB,
		DefaultPort: defaultPort,
		CreatedAt:   time.Now(),
	}

	// Add to server state
	im.server.mu.Lock()
	im.server.instances[instance.ID] = instance
	im.server.mu.Unlock()

	// Start creation in background
	go im.createInstanceAsync(instance, node, image)

	return instance, nil
}

// createInstanceAsync creates the instance asynchronously
func (im *InstanceManager) createInstanceAsync(instance *Instance, node *Node, image *Image) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in createInstanceAsync for instance %s: %v", instance.ID, r)
			instance.Status = "error"
		}
	}()

	// Check KVM availability
	if err := im.checkKVM(node); err != nil {
		log.Printf("KVM check failed for instance %s: %v", instance.ID, err)
		instance.Status = "error"
		return
	}

	// Setup Firecracker VM
	if err := im.setupFirecrackerVM(instance, node, image); err != nil {
		log.Printf("Failed to setup Firecracker VM for instance %s: %v", instance.ID, err)
		instance.Status = "error"
		return
	}

	instance.Status = "running"
	log.Printf("Instance %s created successfully on node %s", instance.ID, node.ID)
}

// checkKVM checks if KVM is available on the node
func (im *InstanceManager) checkKVM(node *Node) error {
	kvmCheckCmd := "if [ -e /dev/kvm ]; then echo 'KVM available'; else echo 'KVM not available'; fi"
	output, err := im.server.nodeManager.RunCommand(node.ID, kvmCheckCmd)
	if err != nil {
		return fmt.Errorf("failed to check KVM: %v", err)
	}

	if !strings.Contains(output, "KVM available") {
		// Try to enable KVM
		enableKvmCmd := `
			if grep -E -q '(vmx|svm)' /proc/cpuinfo 2>/dev/null; then
				if grep -q "Intel" /proc/cpuinfo; then
					sudo modprobe kvm_intel 2>/dev/null || true
				else
					sudo modprobe kvm_amd 2>/dev/null || true
				fi
				sudo modprobe kvm 2>/dev/null || true
				sudo chmod 666 /dev/kvm 2>/dev/null || true
				
				if [ -e /dev/kvm ]; then
					echo "KVM enabled successfully"
				else
					echo "Failed to enable KVM"
				fi
			else
				echo "CPU does not support virtualization"
			fi
		`

		kvmEnableOutput, _ := im.server.nodeManager.RunCommand(node.ID, enableKvmCmd)

		// Check again
		output, _ = im.server.nodeManager.RunCommand(node.ID, kvmCheckCmd)
		if !strings.Contains(output, "KVM available") {
			return fmt.Errorf("KVM is not available on node %s: %s", node.ID, kvmEnableOutput)
		}
	}

	return nil
}

// setupFirecrackerVM sets up and starts a Firecracker VM
func (im *InstanceManager) setupFirecrackerVM(instance *Instance, node *Node, image *Image) error {
	// Create VM configuration
	vmConfig := im.createVMConfig(instance, image)

	// Create the VM directory first
	vmDir := fmt.Sprintf("/var/lib/firecracker/vms/%s", instance.ID)
	createDirCmd := fmt.Sprintf("mkdir -p %s", vmDir)
	if _, err := im.server.nodeManager.RunCommand(node.ID, createDirCmd); err != nil {
		return fmt.Errorf("failed to create VM directory: %v", err)
	}

	// Upload VM configuration to the correct location
	configPath := fmt.Sprintf("%s/config.json", vmDir)
	configJSON, err := json.MarshalIndent(vmConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal VM config: %v", err)
	}

	if err := im.server.nodeManager.UploadFile(node.ID, configJSON, configPath); err != nil {
		return fmt.Errorf("failed to upload VM config: %v", err)
	}

	// Ensure Firecracker is installed
	if err := im.ensureFirecracker(node); err != nil {
		return fmt.Errorf("failed to setup Firecracker: %v", err)
	}

	// Ensure kernel is present
	if err := im.ensureKernel(node); err != nil {
		return fmt.Errorf("failed to setup kernel: %v", err)
	}

	// Setup TAP device and networking before creating rootfs
	// Note: We do this BEFORE starting Firecracker to ensure the TAP device exists
	if err := im.setupNetworking(instance, node, image); err != nil {
		log.Printf("Warning: Failed to setup networking: %v", err)
		// Continue without networking for now
	}

	// Create rootfs from image
	rootfsPath := fmt.Sprintf("%s/rootfs.ext4", vmDir)
	if err := im.createRootFS(node, image, rootfsPath); err != nil {
		return fmt.Errorf("failed to create rootfs: %v", err)
	}

	// Start Firecracker
	// Note: socketPath and logPath are embedded in the script now
	// socketPath := fmt.Sprintf("%s/firecracker.sock", vmDir)
	// logPath := fmt.Sprintf("%s/firecracker.log", vmDir)

	// First, verify Firecracker is installed
	checkFcCmd := "which firecracker && firecracker --version || echo 'Firecracker not found'"
	fcCheck, _ := im.server.nodeManager.RunCommand(node.ID, checkFcCmd)
	log.Printf("Firecracker check: %s", strings.TrimSpace(fcCheck))

	// Verify the config file exists and is valid
	validateCmd := fmt.Sprintf(`
		CONFIG_PATH="%s/config.json"
		if [ ! -f "$CONFIG_PATH" ]; then
			echo "ERROR: Config file not found at $CONFIG_PATH"
			echo "Checking directory contents:"
			ls -la %s
			exit 1
		fi
		echo "Config file exists at $CONFIG_PATH"
		echo "Config content:"
		cat "$CONFIG_PATH"
	`, vmDir, vmDir)

	validateOutput, err := im.server.nodeManager.RunCommand(node.ID, validateCmd)
	if err != nil {
		return fmt.Errorf("config validation failed: %v, output: %s", err, validateOutput)
	}
	log.Printf("Config validation: %s", validateOutput)

	// Start Firecracker with better error handling
	startCmd := fmt.Sprintf(`
		#!/bin/bash
		
		# Function to cleanup old processes
		cleanup_old_firecracker() {
			# Only kill Firecracker processes for this specific instance
			local INSTANCE_ID="%s"
			local VM_DIR="/var/lib/firecracker/vms/$INSTANCE_ID"
			
			# Look for Firecracker processes that are using this specific instance's config or socket
			# Be very specific to avoid killing unrelated processes
			echo "Looking for Firecracker processes for instance $INSTANCE_ID..."
			
			# Method 1: Find by config file path
			OLD_PIDS=""
			for PID in $(pgrep -x firecracker 2>/dev/null || true); do
				# Check if this PID is using our config file
				if ps -p $PID -o args= 2>/dev/null | grep -q "$VM_DIR/config.json"; then
					OLD_PIDS="$OLD_PIDS $PID"
				fi
			done
			
			if [ -n "$OLD_PIDS" ]; then
				echo "Found Firecracker processes for this instance: $OLD_PIDS"
				for PID in $OLD_PIDS; do
					echo "Stopping Firecracker PID $PID for instance $INSTANCE_ID"
					kill -TERM $PID 2>/dev/null || true
				done
				sleep 1
				# Force kill if still running
				for PID in $OLD_PIDS; do
					if ps -p $PID > /dev/null 2>&1; then
						echo "Force killing PID $PID"
						kill -KILL $PID 2>/dev/null || true
					fi
				done
			else
				echo "No Firecracker processes found for instance $INSTANCE_ID"
			fi
			
			# Cleanup socket and log files
			rm -f "$VM_DIR/firecracker.sock" 2>/dev/null || true
			rm -f "$VM_DIR/firecracker.log" 2>/dev/null || true
		}
		
		# Main script starts here
		echo "=== Starting Firecracker setup for instance %s ==="
		
		# Cleanup old processes
		cleanup_old_firecracker
		
		# Setup directories and files
		VM_DIR="/var/lib/firecracker/vms/%s"
		SOCKET_PATH="$VM_DIR/firecracker.sock"
		LOG_PATH="$VM_DIR/firecracker.log"
		CONFIG_PATH="$VM_DIR/config.json"
		
		echo "Creating VM directory..."
		mkdir -p "$VM_DIR"
		touch "$LOG_PATH"
		
		# Remove old socket if exists
		rm -f "$SOCKET_PATH"
		
		echo "Configuration:"
		echo "  Socket: $SOCKET_PATH"
		echo "  Config: $CONFIG_PATH"
		echo "  Log: $LOG_PATH"
		
		# Verify config exists (it should have been created earlier)
		if [ ! -f "$CONFIG_PATH" ]; then
			echo "WARNING: Config file not found at $CONFIG_PATH"
			echo "This might be a timing issue. Checking parent directory..."
			ls -la "$VM_DIR" 2>/dev/null || echo "VM directory not accessible"
			
			# Check if the config.json exists with a different name or location
			find /var/lib/firecracker/vms -name "*.json" -type f 2>/dev/null | head -5
			
			echo "ERROR: Cannot proceed without config file"
			exit 1
		fi
		
		# Start Firecracker using nohup to detach from terminal
		echo "Starting Firecracker..."
		nohup firecracker --api-sock "$SOCKET_PATH" --config-file "$CONFIG_PATH" > "$LOG_PATH" 2>&1 &
		FC_PID=$!
		
		echo "Started Firecracker with PID: $FC_PID"
		
		# Wait a moment for process to initialize
		sleep 3
		
		# Check if process is running
		if ! ps -p $FC_PID > /dev/null 2>&1; then
			echo "ERROR: Firecracker process (PID $FC_PID) died immediately"
			echo "Log contents:"
			tail -50 "$LOG_PATH" 2>/dev/null || echo "No log available"
			exit 1
		fi
		
		# Wait for socket with timeout
		echo "Waiting for API socket..."
		TIMEOUT=15
		ELAPSED=0
		while [ $ELAPSED -lt $TIMEOUT ]; do
			if [ -e "$SOCKET_PATH" ]; then
				echo "Socket created after $ELAPSED seconds"
				break
			fi
			
			# Check if process is still alive
			if ! ps -p $FC_PID > /dev/null 2>&1; then
				echo "ERROR: Firecracker process died while waiting for socket"
				echo "Log contents:"
				tail -50 "$LOG_PATH" 2>/dev/null || echo "No log available"
				exit 1
			fi
			
			sleep 1
			ELAPSED=$((ELAPSED + 1))
		done
		
		if [ ! -e "$SOCKET_PATH" ]; then
			echo "ERROR: Socket not created after $TIMEOUT seconds"
			echo "Firecracker may still be initializing or there may be an error"
			echo "Log contents:"
			tail -50 "$LOG_PATH" 2>/dev/null || echo "No log available"
			
			# Try to kill the process
			kill -TERM $FC_PID 2>/dev/null || true
			exit 1
		fi
		
		# Final verification
		if ps -p $FC_PID > /dev/null 2>&1; then
			echo "SUCCESS: Firecracker is running with PID $FC_PID"
			echo "First few lines of log:"
			head -10 "$LOG_PATH" 2>/dev/null || true
		else
			echo "ERROR: Firecracker process not running"
			echo "Full log contents:"
			cat "$LOG_PATH" 2>/dev/null || echo "No log available"
			exit 1
		fi
		
		echo "=== Firecracker started successfully ==="
		echo "PID: $FC_PID"
	`, instance.ID, instance.ID, instance.ID)

	output, err := im.server.nodeManager.RunCommand(node.ID, startCmd)
	log.Printf("Firecracker start output:\n%s", output)

	if err != nil {
		// Try to get more diagnostic information
		logPath := fmt.Sprintf("%s/firecracker.log", vmDir)
		diagnosticCmd := fmt.Sprintf(`
			echo "=== Diagnostic Information ==="
			echo "VM Directory contents:"
			ls -la %s 2>/dev/null || echo "Directory not found"
			echo ""
			echo "Firecracker processes:"
			ps aux | grep firecracker | grep -v grep || echo "No Firecracker processes"
			echo ""
			echo "Log file contents:"
			if [ -f %s ]; then
				tail -n 100 %s
			else
				echo "Log file not found at %s"
			fi
			echo ""
			echo "Kernel file check:"
			ls -la /var/lib/firecracker/kernels/ 2>/dev/null || echo "Kernel directory not found"
			echo ""
			echo "Rootfs check:"
			ls -la %s/rootfs.ext4 2>/dev/null || echo "Rootfs not found"
		`, vmDir, logPath, logPath, logPath, vmDir)

		diagnosticOutput, _ := im.server.nodeManager.RunCommand(node.ID, diagnosticCmd)
		return fmt.Errorf("failed to start Firecracker: %v\nOutput: %s\nDiagnostics:\n%s", err, output, diagnosticOutput)
	}

	log.Printf("Firecracker started for instance %s: %s", instance.ID, output)

	// Get instance IP (simplified - in production you'd use proper networking)
	instance.IPAddress = "10.0.0.2" // Placeholder

	return nil
}

// ensureFirecracker ensures Firecracker is installed on the node
func (im *InstanceManager) ensureFirecracker(node *Node) error {
	// Check if Firecracker is already installed
	checkCmd := "which firecracker && firecracker --version"
	checkOutput, err := im.server.nodeManager.RunCommand(node.ID, checkCmd)

	if err == nil && strings.Contains(checkOutput, "Firecracker") {
		log.Printf("Firecracker already installed on node %s: %s", node.ID, strings.TrimSpace(checkOutput))
		return nil
	}

	log.Printf("Installing Firecracker on node %s", node.ID)

	// Try to use the installation script
	scriptLocalPath := im.getFirecrackerInstallScriptPath()
	scriptRemotePath := "/tmp/install_firecracker.sh"

	// Read the installation script
	scriptContent, err := os.ReadFile(scriptLocalPath)
	if err != nil {
		// Fallback to inline installation
		log.Printf("Warning: Could not read Firecracker install script, using inline version")
		return im.installFirecrackerInline(node)
	}

	// Upload the script to the node
	if err := im.server.nodeManager.UploadFile(node.ID, scriptContent, scriptRemotePath); err != nil {
		return fmt.Errorf("failed to upload Firecracker install script: %v", err)
	}

	// Make executable and run
	installCmd := fmt.Sprintf("chmod +x %s && %s", scriptRemotePath, scriptRemotePath)
	installOutput, err := im.server.nodeManager.RunCommand(node.ID, installCmd)
	if err != nil {
		log.Printf("Firecracker installation output: %s", installOutput)
		return fmt.Errorf("failed to install Firecracker: %v", err)
	}

	log.Printf("Firecracker successfully installed on node %s", node.ID)

	// Clean up
	cleanupCmd := fmt.Sprintf("rm -f %s", scriptRemotePath)
	im.server.nodeManager.RunCommand(node.ID, cleanupCmd)

	return nil
}

// getFirecrackerInstallScriptPath returns the path to the Firecracker installation script
func (im *InstanceManager) getFirecrackerInstallScriptPath() string {
	possiblePaths := []string{
		"internal/manager/scripts/install_firecracker.sh",
		"../internal/manager/scripts/install_firecracker.sh",
		"./internal/manager/scripts/install_firecracker.sh",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}

	wd, err := os.Getwd()
	if err == nil {
		scriptPath := filepath.Join(wd, "internal", "manager", "scripts", "install_firecracker.sh")
		if _, err := os.Stat(scriptPath); err == nil {
			return scriptPath
		}
	}

	return "internal/manager/scripts/install_firecracker.sh"
}

// installFirecrackerInline installs Firecracker using inline commands (fallback)
func (im *InstanceManager) installFirecrackerInline(node *Node) error {
	installCmd := `
		set -e
		
		# Check if already installed
		if command -v firecracker &> /dev/null; then
			echo "Firecracker is already installed"
			firecracker --version
			exit 0
		fi
		
		echo "Installing Firecracker v1.12.1..."
		
		# Detect architecture
		ARCH=$(uname -m)
		FC_VERSION="v1.12.1"
		
		# Download and install
		cd /tmp
		curl -fsSL -o firecracker.tgz "https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-${ARCH}.tgz"
		tar -xzf firecracker.tgz
		
		RELEASE_DIR="release-${FC_VERSION}-${ARCH}"
		if [ -d "$RELEASE_DIR" ]; then
			# Install firecracker
			cp "$RELEASE_DIR/firecracker-${FC_VERSION}-${ARCH}" /usr/local/bin/firecracker
			chmod +x /usr/local/bin/firecracker
			
			# Install jailer (optional)
			if [ -f "$RELEASE_DIR/jailer-${FC_VERSION}-${ARCH}" ]; then
				cp "$RELEASE_DIR/jailer-${FC_VERSION}-${ARCH}" /usr/local/bin/jailer
				chmod +x /usr/local/bin/jailer
			fi
			
			# Clean up
			rm -rf "$RELEASE_DIR" firecracker.tgz
			
			# Verify
			firecracker --version
			echo "Firecracker installed successfully"
		else
			echo "Failed to extract Firecracker"
			exit 1
		fi
		
		# Check KVM
		if [ ! -e /dev/kvm ]; then
			echo "WARNING: /dev/kvm not found. Firecracker requires KVM support."
		fi
	`

	output, err := im.server.nodeManager.RunCommand(node.ID, installCmd)
	if err != nil {
		return fmt.Errorf("failed to install Firecracker: %v, output: %s", err, output)
	}

	return nil
}

// ensureKernel ensures a Firecracker kernel is available on the node
func (im *InstanceManager) ensureKernel(node *Node) error {
	kernelPath := "/var/lib/firecracker/kernels/vmlinux"

	// Check if kernel already exists
	kernelCheckCmd := fmt.Sprintf("[ -f %s ] && echo 'exists' || echo 'missing'", kernelPath)
	kernelStatus, _ := im.server.nodeManager.RunCommand(node.ID, kernelCheckCmd)

	if strings.Contains(kernelStatus, "exists") {
		log.Printf("Kernel already present on node %s", node.ID)
		return nil
	}

	log.Printf("Downloading kernel for node %s", node.ID)

	// Get the kernel download script path
	scriptLocalPath := im.getKernelScriptPath()
	scriptRemotePath := "/tmp/download_kernel.sh"

	// Read the kernel download script
	scriptContent, err := os.ReadFile(scriptLocalPath)
	if err != nil {
		// Fallback to inline script if external script not found
		log.Printf("Warning: Could not read kernel download script, using inline version")
		return im.downloadKernelInline(node)
	}

	// Upload the script to the node
	if err := im.server.nodeManager.UploadFile(node.ID, scriptContent, scriptRemotePath); err != nil {
		return fmt.Errorf("failed to upload kernel download script: %v", err)
	}

	// Make executable and run
	makeExecCmd := fmt.Sprintf("chmod +x %s", scriptRemotePath)
	if _, err := im.server.nodeManager.RunCommand(node.ID, makeExecCmd); err != nil {
		return fmt.Errorf("failed to make kernel script executable: %v", err)
	}

	// Run the kernel download script
	kernelOutput, err := im.server.nodeManager.RunCommand(node.ID, scriptRemotePath)
	if err != nil {
		log.Printf("Kernel download output: %s", kernelOutput)
		return fmt.Errorf("failed to download kernel: %v", err)
	}

	log.Printf("Kernel successfully downloaded to node %s", node.ID)

	// Clean up the script
	cleanupCmd := fmt.Sprintf("rm -f %s", scriptRemotePath)
	im.server.nodeManager.RunCommand(node.ID, cleanupCmd)

	return nil
}

// getKernelScriptPath returns the path to the kernel download script
func (im *InstanceManager) getKernelScriptPath() string {
	// Try multiple strategies to find the script
	possiblePaths := []string{
		"internal/manager/scripts/download_kernel.sh",
		"../internal/manager/scripts/download_kernel.sh",
		"./internal/manager/scripts/download_kernel.sh",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}

	// Try based on working directory
	wd, err := os.Getwd()
	if err == nil {
		scriptPath := filepath.Join(wd, "internal", "manager", "scripts", "download_kernel.sh")
		if _, err := os.Stat(scriptPath); err == nil {
			return scriptPath
		}
	}

	// Fallback
	return "internal/manager/scripts/download_kernel.sh"
}

// downloadKernelInline downloads kernel using inline commands (fallback)
func (im *InstanceManager) downloadKernelInline(node *Node) error {
	downloadCmd := `
		set -e
		KERNEL_DIR="/var/lib/firecracker/kernels"
		mkdir -p "$KERNEL_DIR"
		
		echo "Downloading Firecracker kernel..."
		
		# Try the official Firecracker quickstart kernel (most reliable)
		QUICKSTART_KERNEL="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
		echo "Downloading from: $QUICKSTART_KERNEL"
		
		if curl -fsSL -o "$KERNEL_DIR/vmlinux" "$QUICKSTART_KERNEL"; then
			# Verify the download
			if [ -f "$KERNEL_DIR/vmlinux" ] && [ -s "$KERNEL_DIR/vmlinux" ]; then
				SIZE=$(stat -c%s "$KERNEL_DIR/vmlinux" 2>/dev/null || stat -f%z "$KERNEL_DIR/vmlinux" 2>/dev/null)
				echo "Kernel downloaded successfully (size: $SIZE bytes)"
				exit 0
			else
				echo "Downloaded file is empty or invalid"
				rm -f "$KERNEL_DIR/vmlinux"
			fi
		fi
		
		# Try alternative sources
		echo "Trying alternative kernel sources..."
		ALTERNATIVE_KERNELS=(
			"https://s3.amazonaws.com/spec.ccfc.min/ci-artifacts/kernels/x86_64/vmlinux-5.10.bin"
			"https://github.com/firecracker-microvm/firecracker/releases/download/v1.0.0/vmlinux.bin"
		)
		
		for KERNEL_URL in "${ALTERNATIVE_KERNELS[@]}"; do
			echo "Trying: $KERNEL_URL"
			if curl -fsSL -o "$KERNEL_DIR/vmlinux" "$KERNEL_URL" 2>/dev/null; then
				if [ -f "$KERNEL_DIR/vmlinux" ] && [ -s "$KERNEL_DIR/vmlinux" ]; then
					echo "Kernel downloaded from alternative source"
					exit 0
				fi
				rm -f "$KERNEL_DIR/vmlinux"
			fi
		done
		
		echo "Failed to download kernel from any source"
		exit 1
	`

	output, err := im.server.nodeManager.RunCommand(node.ID, downloadCmd)
	if err != nil {
		return fmt.Errorf("failed to download kernel: %v, output: %s", err, output)
	}

	return nil
}

// createVMConfig creates the Firecracker VM configuration
func (im *InstanceManager) createVMConfig(instance *Instance, image *Image) map[string]interface{} {
	vmDir := fmt.Sprintf("/var/lib/firecracker/vms/%s", instance.ID)

	// Use last 8 chars of instance ID for TAP name to match setupNetworking
	tapName := fmt.Sprintf("tap%s", instance.ID[len(instance.ID)-8:])

	// Configuration with networking enabled
	config := map[string]interface{}{
		"boot-source": map[string]interface{}{
			"kernel_image_path": "/var/lib/firecracker/kernels/vmlinux",
			"boot_args":         "console=ttyS0 reboot=k panic=1 pci=off init=/init",
		},
		"drives": []map[string]interface{}{
			{
				"drive_id":       "rootfs",
				"path_on_host":   fmt.Sprintf("%s/rootfs.ext4", vmDir),
				"is_root_device": true,
				"is_read_only":   false,
			},
		},
		"machine-config": map[string]interface{}{
			"vcpu_count":   instance.VCPUCount,
			"mem_size_mib": instance.MemoryMiB,
			"smt":          false,
		},
		"network-interfaces": []map[string]interface{}{
			{
				"iface_id":      "eth0",
				"guest_mac":     generateMACAddress(instance.ID),
				"host_dev_name": tapName,
			},
		},
	}

	return config
}

// createRootFS creates a root filesystem from an image
func (im *InstanceManager) createRootFS(node *Node, image *Image, rootfsPath string) error {
	// Extract the Docker image and create a proper rootfs from it
	imageTarPath := fmt.Sprintf("/var/lib/firecracker/images/%s.tar", image.ID)

	createRootfsCmd := fmt.Sprintf(`
		set -e
		
		echo "Creating rootfs from Docker image at %s"
		
		IMAGE_TAR="%s"
		ROOTFS_PATH="%s"
		IMAGE_NAME="%s"
		
		# Check if Docker image tar exists
		if [ ! -f "$IMAGE_TAR" ]; then
			echo "ERROR: Docker image tar not found at $IMAGE_TAR"
			exit 1
		fi
		
		# Create a larger rootfs file (1GB to accommodate the Docker image)
		echo "Creating rootfs file..."
		dd if=/dev/zero of="$ROOTFS_PATH" bs=1M count=1024 2>/dev/null
		
		# Format as ext4
		echo "Formatting rootfs as ext4..."
		mkfs.ext4 -F "$ROOTFS_PATH" 2>/dev/null
		
		# Mount the rootfs
		MOUNT_DIR=$(mktemp -d)
		echo "Mounting rootfs at $MOUNT_DIR..."
		if ! mount "$ROOTFS_PATH" "$MOUNT_DIR"; then
			echo "Failed to mount rootfs"
			rm -f "$ROOTFS_PATH"
			exit 1
		fi
		
		# Extract Docker image layers
		echo "Extracting Docker image..."
		EXTRACT_DIR=$(mktemp -d)
		cd "$EXTRACT_DIR"
		
		# Load the Docker image and extract its filesystem
		echo "Loading Docker image..."
		LOAD_OUTPUT=$(docker load -i "$IMAGE_TAR" 2>&1)
		echo "Docker load output: $LOAD_OUTPUT"
		
		# Extract the image name from the load output
		# Format is usually "Loaded image: <image>:<tag>" or "Loaded image ID: sha256:..."
		IMAGE_ID=$(echo "$LOAD_OUTPUT" | grep -E "Loaded image(:| ID:)" | sed -E 's/.*Loaded image(:| ID:) *//' | tail -1)
		
		if [ -z "$IMAGE_ID" ]; then
			echo "ERROR: Failed to extract image ID from docker load"
			echo "Load output was: $LOAD_OUTPUT"
			umount "$MOUNT_DIR"
			rm -f "$ROOTFS_PATH"
			exit 1
		fi
		
		echo "Docker image loaded with ID/tag: $IMAGE_ID"
		
		# Verify the image exists
		if ! docker images --format "{{.Repository}}:{{.Tag}} {{.ID}}" | grep -q "$IMAGE_ID"; then
			# If not found by name, try to find by ID prefix
			echo "Image not found by name, checking by ID..."
			IMAGE_ID=$(docker images -q | head -1)
			if [ -z "$IMAGE_ID" ]; then
				echo "ERROR: No Docker images found after load"
				docker images
				umount "$MOUNT_DIR"
				rm -f "$ROOTFS_PATH"
				exit 1
			fi
			echo "Using image ID: $IMAGE_ID"
		fi
		
		# Create a container from the image and export its filesystem
		echo "Creating container from image..."
		CONTAINER_ID=$(docker create "$IMAGE_ID")
		if [ -z "$CONTAINER_ID" ]; then
			echo "ERROR: Failed to create container from image"
			umount "$MOUNT_DIR"
			rm -f "$ROOTFS_PATH"
			exit 1
		fi
		
		echo "Exporting container filesystem..."
		docker export "$CONTAINER_ID" | tar -xC "$MOUNT_DIR"
		
		# Clean up the container
		docker rm "$CONTAINER_ID" >/dev/null 2>&1
		
		# Create essential device nodes if they don't exist
		echo "Setting up device nodes..."
		mkdir -p "$MOUNT_DIR/dev"
		mknod -m 666 "$MOUNT_DIR/dev/null" c 1 3 2>/dev/null || true
		mknod -m 666 "$MOUNT_DIR/dev/zero" c 1 5 2>/dev/null || true
		mknod -m 666 "$MOUNT_DIR/dev/random" c 1 8 2>/dev/null || true
		mknod -m 666 "$MOUNT_DIR/dev/urandom" c 1 9 2>/dev/null || true
		mknod -m 666 "$MOUNT_DIR/dev/tty" c 5 0 2>/dev/null || true
		mknod -m 622 "$MOUNT_DIR/dev/console" c 5 1 2>/dev/null || true
		mknod -m 666 "$MOUNT_DIR/dev/ttyS0" c 4 64 2>/dev/null || true
		
		# Create proc, sys directories if they don't exist
		mkdir -p "$MOUNT_DIR/proc" "$MOUNT_DIR/sys" "$MOUNT_DIR/tmp"
		chmod 1777 "$MOUNT_DIR/tmp"
		
		# Check what's in the Docker image for the entrypoint
		echo "Checking Docker image configuration..."
		ENTRYPOINT=$(docker inspect "$IMAGE_ID" --format='{{json .Config.Entrypoint}}' 2>/dev/null | sed 's/\[//g' | sed 's/\]//g' | sed 's/"//g' | sed 's/,/ /g')
		CMD=$(docker inspect "$IMAGE_ID" --format='{{json .Config.Cmd}}' 2>/dev/null | sed 's/\[//g' | sed 's/\]//g' | sed 's/"//g' | sed 's/,/ /g')
		WORKDIR=$(docker inspect "$IMAGE_ID" --format='{{.Config.WorkingDir}}' 2>/dev/null)
		
		# Get exposed ports
		EXPOSED_PORTS=$(docker inspect "$IMAGE_ID" --format='{{json .Config.ExposedPorts}}' 2>/dev/null)
		
		echo "Docker image configuration:"
		echo "  Entrypoint: $ENTRYPOINT"
		echo "  Cmd: $CMD"
		echo "  WorkDir: $WORKDIR"
		echo "  Exposed Ports: $EXPOSED_PORTS"
		
		# Create an init script that will run the container's entrypoint/cmd
		cat > "$MOUNT_DIR/init" << 'INIT_SCRIPT'
#!/bin/sh
echo "Firecracker VM booting..."
echo "Image: IMAGE_NAME_PLACEHOLDER"

# Mount essential filesystems
mount -t proc proc /proc 2>/dev/null || true
mount -t sysfs sys /sys 2>/dev/null || true
mount -t devtmpfs devtmpfs /dev 2>/dev/null || true

# Set up networking (if interface exists)
if [ -e /sys/class/net/eth0 ]; then
    echo "Configuring network interface eth0..."
    
    # Try to use ip command first, fall back to ifconfig
    if command -v ip >/dev/null 2>&1; then
        ip addr add 10.0.0.2/24 dev eth0
        ip link set eth0 up
        ip route add default via 10.0.0.1
    elif command -v ifconfig >/dev/null 2>&1; then
        ifconfig eth0 10.0.0.2 netmask 255.255.255.0 up
        if command -v route >/dev/null 2>&1; then
            route add default gw 10.0.0.1
        fi
    else
        echo "Warning: Neither ip nor ifconfig found, network configuration skipped"
        echo "Network interface eth0 exists but cannot be configured"
    fi
    
    echo "Network configured: 10.0.0.2/24"
fi

# Change to workdir if specified
WORKDIR_PLACEHOLDER
if [ -n "$WORKDIR" ] && [ -d "$WORKDIR" ]; then
    cd "$WORKDIR"
fi

# Set environment variables
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Force applications to bind to all interfaces instead of just localhost
export HOST=0.0.0.0
export HOSTNAME=0.0.0.0
export NUXT_HOST=0.0.0.0
export NITRO_HOST=0.0.0.0

echo "System initialized"
echo "Starting application..."

# Run the Docker image's entrypoint/cmd
ENTRYPOINT_PLACEHOLDER
CMD_PLACEHOLDER

# If no entrypoint/cmd, fall back to shell
if [ -z "$ENTRYPOINT" ] && [ -z "$CMD" ]; then
    echo "No entrypoint or cmd specified, starting shell..."
    exec /bin/sh
else
    # Execute the entrypoint and/or cmd
    if [ -n "$ENTRYPOINT" ] && [ -n "$CMD" ]; then
        exec $ENTRYPOINT $CMD
    elif [ -n "$ENTRYPOINT" ]; then
        exec $ENTRYPOINT
    elif [ -n "$CMD" ]; then
        exec $CMD
    fi
fi
INIT_SCRIPT
		
		# Replace placeholders in init script
		sed -i "s|IMAGE_NAME_PLACEHOLDER|$IMAGE_NAME|g" "$MOUNT_DIR/init"
		sed -i "s|WORKDIR_PLACEHOLDER|WORKDIR=\"$WORKDIR\"|g" "$MOUNT_DIR/init"
		sed -i "s|ENTRYPOINT_PLACEHOLDER|ENTRYPOINT=\"$ENTRYPOINT\"|g" "$MOUNT_DIR/init"
		sed -i "s|CMD_PLACEHOLDER|CMD=\"$CMD\"|g" "$MOUNT_DIR/init"
		
		chmod +x "$MOUNT_DIR/init"
		
		# Ensure we have a shell
		if [ ! -f "$MOUNT_DIR/bin/sh" ]; then
			echo "Warning: No shell found in Docker image, copying from host..."
			cp /bin/sh "$MOUNT_DIR/bin/sh" 2>/dev/null || true
		fi
		
		# Install busybox for networking tools if not present
		if [ ! -f "$MOUNT_DIR/bin/busybox" ] && [ ! -f "$MOUNT_DIR/sbin/ifconfig" ] && [ ! -f "$MOUNT_DIR/bin/ip" ]; then
			echo "No networking tools found, installing busybox..."
			# Download and install a static busybox binary
			if command -v busybox >/dev/null 2>&1; then
				# Copy from host if available
				cp $(which busybox) "$MOUNT_DIR/bin/busybox" 2>/dev/null || true
			else
				# Download a static busybox binary (for x86_64)
				wget -q -O "$MOUNT_DIR/bin/busybox" "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox" 2>/dev/null || \
				curl -sL -o "$MOUNT_DIR/bin/busybox" "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox" 2>/dev/null || true
			fi
			
			if [ -f "$MOUNT_DIR/bin/busybox" ]; then
				chmod +x "$MOUNT_DIR/bin/busybox"
				# Create symlinks for networking commands
				mkdir -p "$MOUNT_DIR/sbin"
				for cmd in ifconfig route ping ip; do
					ln -sf /bin/busybox "$MOUNT_DIR/sbin/$cmd" 2>/dev/null || true
					ln -sf /bin/busybox "$MOUNT_DIR/bin/$cmd" 2>/dev/null || true
				done
				echo "Busybox installed with networking tools"
			fi
		fi
		
		# If we have busybox, ensure network command symlinks exist
		if [ -f "$MOUNT_DIR/bin/busybox" ]; then
			echo "Ensuring busybox network command symlinks..."
			for cmd in ifconfig route ping; do
				if [ ! -f "$MOUNT_DIR/sbin/$cmd" ] && [ ! -f "$MOUNT_DIR/bin/$cmd" ]; then
					ln -sf /bin/busybox "$MOUNT_DIR/sbin/$cmd" 2>/dev/null || true
				fi
			done
		fi
		
		# Clean up Docker image from local storage to save space
		echo "Cleaning up Docker image from local storage..."
		docker rmi $IMAGE_ID 2>/dev/null || true
		
		# Sync and unmount
		echo "Finalizing rootfs..."
		sync
		umount "$MOUNT_DIR"
		rmdir "$MOUNT_DIR"
		rm -rf "$EXTRACT_DIR"
		
		echo "Rootfs created successfully at $ROOTFS_PATH"
		echo "The VM will run: ${ENTRYPOINT:-} ${CMD:-}"
		echo "Exposed ports: $EXPOSED_PORTS"
	`, rootfsPath, imageTarPath, rootfsPath, image.Name)

	output, err := im.server.nodeManager.RunCommand(node.ID, createRootfsCmd)
	if err != nil {
		return fmt.Errorf("failed to create rootfs: %v, output: %s", err, output)
	}

	return nil
}

// StopInstance stops a running instance
func (im *InstanceManager) StopInstance(instanceID string) error {
	im.server.mu.RLock()
	instance, exists := im.server.instances[instanceID]
	im.server.mu.RUnlock()

	if !exists {
		return fmt.Errorf("instance %s not found", instanceID)
	}

	// Kill the Firecracker process
	stopCmd := fmt.Sprintf("pkill -f 'firecracker.*%s' 2>/dev/null || true", instanceID)
	if _, err := im.server.nodeManager.RunCommand(instance.NodeID, stopCmd); err != nil {
		return fmt.Errorf("failed to stop instance: %v", err)
	}

	instance.Status = "stopped"
	return nil
}

// DeleteInstance deletes an instance
func (im *InstanceManager) DeleteInstance(instanceID string) error {
	im.server.mu.Lock()
	instance, exists := im.server.instances[instanceID]
	if !exists {
		im.server.mu.Unlock()
		return fmt.Errorf("instance %s not found", instanceID)
	}
	delete(im.server.instances, instanceID)
	im.server.mu.Unlock()

	// Stop the instance if running
	if instance.Status == "running" {
		im.StopInstance(instanceID)
	}

	// Clean up TAP device
	tapName := fmt.Sprintf("tap%s", instanceID[len(instanceID)-8:])
	cleanupNetCmd := fmt.Sprintf(`
		# Clean up TAP device
		TAP_NAME="%s"
		if ip link show $TAP_NAME 2>/dev/null; then
			echo "Cleaning up TAP device $TAP_NAME"
			ip link set $TAP_NAME down 2>/dev/null || true
			ip link delete $TAP_NAME 2>/dev/null || true
		fi
		
		# Clean up any iptables rules for this instance
		# Remove DNAT rules
		iptables-save | grep "10.0.0.2" | grep DNAT | while read -r rule; do
			echo "Removing rule: $rule"
			iptables -t nat -D PREROUTING $(echo "$rule" | sed 's/^-A PREROUTING//')
		done 2>/dev/null || true
		
		# Remove FORWARD rules
		iptables-save | grep "10.0.0.2" | grep FORWARD | while read -r rule; do
			iptables -D FORWARD $(echo "$rule" | sed 's/^-A FORWARD//')
		done 2>/dev/null || true
	`, tapName)

	if _, err := im.server.nodeManager.RunCommand(instance.NodeID, cleanupNetCmd); err != nil {
		log.Printf("Failed to cleanup networking for instance %s: %v", instanceID, err)
	}

	// Clean up VM directory
	vmDir := fmt.Sprintf("/var/lib/firecracker/vms/%s", instanceID)
	cleanupCmd := fmt.Sprintf("rm -rf %s", vmDir)
	if _, err := im.server.nodeManager.RunCommand(instance.NodeID, cleanupCmd); err != nil {
		log.Printf("Failed to cleanup VM directory for instance %s: %v", instanceID, err)
	}

	return nil
}

// GetInstanceLogs gets the logs for an instance
func (im *InstanceManager) GetInstanceLogs(instanceID string) (string, error) {
	im.server.mu.RLock()
	instance, exists := im.server.instances[instanceID]
	im.server.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("instance %s not found", instanceID)
	}

	logPath := fmt.Sprintf("/var/lib/firecracker/vms/%s/firecracker.log", instanceID)
	logs, err := im.server.nodeManager.RunCommand(instance.NodeID, fmt.Sprintf("cat %s 2>/dev/null || echo 'No logs available'", logPath))
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %v", err)
	}

	return logs, nil
}

// UploadUtils uploads utility scripts needed for VM operations
func (im *InstanceManager) UploadUtils(nodeID string) error {
	// Upload utils.sh
	utilsPath := filepath.Join("internal", "scripts", "utils.sh")
	utilsContent, err := os.ReadFile(utilsPath)
	if err != nil {
		// If utils.sh doesn't exist, create a minimal one
		utilsContent = []byte(`#!/bin/bash
# Minimal utils for VM operations
log_info() { echo "[INFO] $*"; }
log_error() { echo "[ERROR] $*" >&2; }
`)
	}

	if err := im.server.nodeManager.UploadFile(nodeID, utilsContent, "/tmp/utils.sh"); err != nil {
		return fmt.Errorf("failed to upload utils.sh: %v", err)
	}

	// Make executable
	if _, err := im.server.nodeManager.RunCommand(nodeID, "chmod +x /tmp/utils.sh"); err != nil {
		return fmt.Errorf("failed to make utils.sh executable: %v", err)
	}

	return nil
}

// setupNetworking sets up TAP device and networking for the VM
func (im *InstanceManager) setupNetworking(instance *Instance, node *Node, image *Image) error {
	// Use a shorter, unique TAP name (max 15 chars for Linux interface names)
	// Use last 8 chars of instance ID to ensure uniqueness
	tapName := fmt.Sprintf("tap%s", instance.ID[len(instance.ID)-8:])

	// Get exposed ports from the image to set up port forwarding
	getPortsCmd := fmt.Sprintf(`
		IMAGE_TAR="/var/lib/firecracker/images/%s.tar"
		if [ -f "$IMAGE_TAR" ]; then
			# Load image temporarily to get exposed ports
			LOAD_OUTPUT=$(docker load -i "$IMAGE_TAR" 2>&1)
			IMAGE_ID=$(echo "$LOAD_OUTPUT" | grep -E "Loaded image(:| ID:)" | sed -E 's/.*Loaded image(:| ID:) *//' | tail -1)
			
			if [ -n "$IMAGE_ID" ]; then
				docker inspect "$IMAGE_ID" --format='{{range $p, $conf := .Config.ExposedPorts}}{{$p}} {{end}}' 2>/dev/null
				docker rmi "$IMAGE_ID" 2>/dev/null || true
			fi
		fi
	`, image.ID)

	exposedPorts, _ := im.server.nodeManager.RunCommand(node.ID, getPortsCmd)
	exposedPorts = strings.TrimSpace(exposedPorts)

	// Setup TAP device and networking
	setupNetCmd := fmt.Sprintf(`
		set -e
		
		TAP_NAME="%s"
		INSTANCE_ID="%s"
		
		echo "Setting up networking for instance $INSTANCE_ID"
		
		# Check if TAP device already exists and clean it up
		if ip link show $TAP_NAME 2>/dev/null; then
			echo "TAP device $TAP_NAME already exists, cleaning up..."
			ip link set $TAP_NAME down 2>/dev/null || true
			ip link delete $TAP_NAME 2>/dev/null || true
			sleep 1
		fi
		
		# Create TAP device
		echo "Creating TAP device $TAP_NAME..."
		if ! ip tuntap add dev $TAP_NAME mode tap; then
			echo "ERROR: Failed to create TAP device $TAP_NAME"
			# List existing TAP devices for debugging
			echo "Existing TAP devices:"
			ip link show type tap
			exit 1
		fi
		ip link set $TAP_NAME up
		
		# Create bridge if it doesn't exist
		BRIDGE_NAME="fcbr0"
		if ! ip link show $BRIDGE_NAME 2>/dev/null; then
			echo "Creating bridge $BRIDGE_NAME..."
			ip link add name $BRIDGE_NAME type bridge
			ip addr add 10.0.0.1/24 dev $BRIDGE_NAME
			ip link set $BRIDGE_NAME up
			
			# Enable IP forwarding
			echo 1 > /proc/sys/net/ipv4/ip_forward
			
			# Setup NAT for outbound traffic
			iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE 2>/dev/null || true
			iptables -t nat -A POSTROUTING -o ens3 -j MASQUERADE 2>/dev/null || true
			iptables -t nat -A POSTROUTING -o enp0s3 -j MASQUERADE 2>/dev/null || true
		fi
		
		# Add TAP to bridge
		echo "Adding $TAP_NAME to bridge $BRIDGE_NAME..."
		ip link set $TAP_NAME master $BRIDGE_NAME
		
		# Parse and setup port forwarding for exposed ports
		EXPOSED_PORTS="%s"
		if [ -n "$EXPOSED_PORTS" ]; then
			echo "Setting up port forwarding for: $EXPOSED_PORTS"
			for PORT_SPEC in $EXPOSED_PORTS; do
				# Extract port number (format is usually "8080/tcp")
				PORT=$(echo $PORT_SPEC | cut -d'/' -f1)
				if [ -n "$PORT" ]; then
					echo "Forwarding host port $PORT to VM port $PORT"
					# Forward from host to VM (10.0.0.2 is the VM IP)
					iptables -t nat -A PREROUTING -p tcp --dport $PORT -j DNAT --to-destination 10.0.0.2:$PORT 2>/dev/null || true
					iptables -A FORWARD -p tcp -d 10.0.0.2 --dport $PORT -j ACCEPT 2>/dev/null || true
				fi
			done
		else
			# Default: forward common ports if no ports specified
			echo "No exposed ports found, setting up default port 8080"
			iptables -t nat -A PREROUTING -p tcp --dport 8080 -j DNAT --to-destination 10.0.0.2:8080 2>/dev/null || true
			iptables -A FORWARD -p tcp -d 10.0.0.2 --dport 8080 -j ACCEPT 2>/dev/null || true
		fi
		
		echo "Networking setup complete for instance $INSTANCE_ID"
		echo "VM will have IP: 10.0.0.2"
		echo "Exposed ports forwarded: ${EXPOSED_PORTS:-8080}"
	`, tapName, instance.ID, exposedPorts)

	output, err := im.server.nodeManager.RunCommand(node.ID, setupNetCmd)
	if err != nil {
		return fmt.Errorf("failed to setup networking: %v, output: %s", err, output)
	}

	log.Printf("Networking setup for instance %s: %s", instance.ID, output)

	// Store the exposed ports info in the instance
	if exposedPorts != "" {
		instance.IPAddress = "10.0.0.2"
		log.Printf("Instance %s will expose ports: %s", instance.ID, exposedPorts)
	}

	return nil
}

// generateMACAddress generates a MAC address for the VM
func generateMACAddress(instanceID string) string {
	// Use a fixed prefix for Firecracker VMs and derive the rest from instance ID
	// AA:FC is a locally administered MAC address prefix
	hash := 0
	for _, c := range instanceID[:8] {
		hash = hash*31 + int(c)
	}

	return fmt.Sprintf("AA:FC:%02X:%02X:%02X:%02X",
		(hash>>24)&0xFF,
		(hash>>16)&0xFF,
		(hash>>8)&0xFF,
		hash&0xFF,
	)
}
