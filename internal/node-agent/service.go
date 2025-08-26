package nodeagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service represents the node agent service that runs on each compute node
type Service struct {
	logger     *slog.Logger
	config     *Config
	httpClient *http.Client
	nodeID     uuid.UUID

	// Firecracker VM management
	instances map[string]*Instance // instance_id -> instance
}

// Config holds the configuration for the node agent service
type Config struct {
	Port              string
	OperatorURL       string
	NodeID            string
	FirecrackerBin    string
	FirecrackerSocket string
	VMWorkDir         string
	KernelImagePath   string
	BuilderRootfsPath string
}

// Instance represents a running VM instance
type Instance struct {
	ID        string
	ImageID   string
	State     string
	Resources struct {
		VCPU   int `json:"vcpu"`
		Memory int `json:"memory"`
	}
	IPAddress string
	Process   *FirecrackerProcess
}

// FirecrackerProcess represents a running Firecracker process
type FirecrackerProcess struct {
	PID        int
	SocketPath string
	LogFile    string
}

// NewService creates a new node agent service
func NewService(config *Config, logger *slog.Logger) (*Service, error) {
	// Parse or generate node ID
	var nodeID uuid.UUID
	var err error
	if config.NodeID != "" {
		nodeID, err = uuid.Parse(config.NodeID)
		if err != nil {
			return nil, fmt.Errorf("invalid node ID: %w", err)
		}
	} else {
		// Generate a new node ID if not provided
		nodeID = uuid.New()
		logger.Info("Generated new node ID", "node_id", nodeID)
	}

	return &Service{
		logger:     logger,
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		nodeID:     nodeID,
		instances:  make(map[string]*Instance),
	}, nil
}

// Start starts the node agent service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting node agent service",
		"port", s.config.Port,
		"node_id", s.nodeID,
		"operator_url", s.config.OperatorURL,
	)

	// Register with operator
	if err := s.registerWithOperator(ctx); err != nil {
		return fmt.Errorf("failed to register with operator: %w", err)
	}

	// Start health reporting goroutine
	go s.reportHealthPeriodically(ctx)

	// Create HTTP server for receiving commands from operator
	mux := http.NewServeMux()
	s.setupRoutes(mux)

	server := &http.Server{
		Addr:    ":" + s.config.Port,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Failed to start HTTP server", "error", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown server
	s.logger.Info("Shutting down node agent service")

	// Stop all running instances
	s.stopAllInstances()

	// Deregister from operator
	s.deregisterFromOperator()

	return server.Shutdown(context.Background())
}

// setupRoutes sets up the HTTP routes for the node agent
func (s *Service) setupRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("GET /health", s.handleHealth)

	// Instance management endpoints (called by operator)
	mux.HandleFunc("POST /instances", s.handleCreateInstance)
	mux.HandleFunc("GET /instances/{id}", s.handleGetInstance)
	mux.HandleFunc("PUT /instances/{id}/state", s.handleUpdateInstanceState)
	mux.HandleFunc("DELETE /instances/{id}", s.handleDeleteInstance)

	// Node information
	mux.HandleFunc("GET /node/info", s.handleNodeInfo)
	mux.HandleFunc("GET /node/resources", s.handleNodeResources)

	// Build management
	mux.HandleFunc("POST /api/v1/build", s.handleBuildImage)
}

// handleHealth handles health check requests
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"node_id":   s.nodeID,
		"instances": len(s.instances),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// registerWithOperator registers this node with the operator
func (s *Service) registerWithOperator(ctx context.Context) error {
	// Get system information
	hostname := "node-" + s.nodeID.String()[:8]
	ipAddress := s.getNodeIPAddress()

	// Prepare registration request
	registration := map[string]interface{}{
		"hostname":   hostname,
		"ip_address": ipAddress,
		"resources": map[string]int{
			"vcpu":   4,    // TODO: Get actual CPU count
			"memory": 8192, // TODO: Get actual memory
		},
	}

	body, err := json.Marshal(registration)
	if err != nil {
		return err
	}

	// Send registration request to operator
	req, err := http.NewRequestWithContext(ctx, "POST",
		s.config.OperatorURL+"/api/v1/nodes", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-ID", s.nodeID.String())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send registration: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registration failed with status %d", resp.StatusCode)
	}

	s.logger.Info("Successfully registered with operator")
	return nil
}

// reportHealthPeriodically sends periodic health reports to the operator
func (s *Service) reportHealthPeriodically(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reportHealth(ctx)
		}
	}
}

// reportHealth sends a health report to the operator
func (s *Service) reportHealth(ctx context.Context) {
	// Prepare health report
	health := map[string]interface{}{
		"state": "ready",
		"resources": map[string]interface{}{
			"vcpu_available":   4,    // TODO: Calculate available resources
			"memory_available": 4096, // TODO: Calculate available memory
		},
	}

	body, err := json.Marshal(health)
	if err != nil {
		s.logger.Error("Failed to marshal health report", "error", err)
		return
	}

	// Send health report to operator
	req, err := http.NewRequestWithContext(ctx, "PUT",
		fmt.Sprintf("%s/api/v1/nodes/%s/state", s.config.OperatorURL, s.nodeID),
		bytes.NewReader(body))
	if err != nil {
		s.logger.Error("Failed to create health report request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error("Failed to send health report", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.logger.Warn("Health report failed", "status", resp.StatusCode)
	}
}

// deregisterFromOperator deregisters this node from the operator
func (s *Service) deregisterFromOperator() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("%s/api/v1/nodes/%s", s.config.OperatorURL, s.nodeID), nil)
	if err != nil {
		s.logger.Error("Failed to create deregistration request", "error", err)
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error("Failed to deregister from operator", "error", err)
		return
	}
	defer resp.Body.Close()

	s.logger.Info("Deregistered from operator")
}

// stopAllInstances stops all running VM instances
func (s *Service) stopAllInstances() {
	s.logger.Info("Stopping all instances", "count", len(s.instances))

	for id, instance := range s.instances {
		s.logger.Info("Stopping instance", "instance_id", id)
		// TODO: Implement actual instance stopping
		instance.State = "stopped"
	}
}

// getNodeIPAddress gets the IP address of this node
func (s *Service) getNodeIPAddress() string {
	// TODO: Implement actual IP address detection
	return "10.0.1.1"
}

// handleBuildImage handles build requests
func (s *Service) handleBuildImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ImageID    string `json:"image_id"`
		GitHubRepo string `json:"github_repo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.ImageID == "" || req.GitHubRepo == "" {
		http.Error(w, "image_id and github_repo are required", http.StatusBadRequest)
		return
	}

	go s.buildInVM(req.ImageID, req.GitHubRepo)
	w.WriteHeader(http.StatusAccepted)
}

// buildInVM spins up an ephemeral Firecracker VM (builder image) and performs the docker build inside it
func (s *Service) buildInVM(imageID string, githubRepo string) {
	tmpDir, err := os.MkdirTemp("", "zw-build-*")
	if err != nil {
		s.logger.Error("Failed to create temp dir", "error", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	// Prepare drives: src (repo), output (build artifacts), and use configured builder rootfs
	srcDrive := filepath.Join(tmpDir, "src.ext4")
	outDrive := filepath.Join(tmpDir, "out.ext4")
	if err := s.createExt4(srcDrive, 2048); err != nil { // 2GiB for sources
		s.notifyBuildFailed(imageID, "failed to create src drive")
		return
	}
	if err := s.createExt4(outDrive, 2048); err != nil { // 2GiB for output
		s.notifyBuildFailed(imageID, "failed to create output drive")
		return
	}

	// Mount src drive, clone repo into it, unmount
	srcMount := filepath.Join(tmpDir, "mnt-src")
	if err := os.MkdirAll(srcMount, 0755); err != nil {
		s.notifyBuildFailed(imageID, "failed to create src mount")
		return
	}
	if err := s.runCmd(tmpDir, "bash", "-lc", fmt.Sprintf("sudo mount %q %q && git clone https://github.com/%s.git %q/repo && sudo umount %q", srcDrive, srcMount, githubRepo, srcMount, srcMount)); err != nil {
		s.notifyBuildFailed(imageID, fmt.Sprintf("git clone into src drive failed: %v", err))
		return
	}

	// Generate builder VM config
	instanceDir := filepath.Join(s.config.VMWorkDir, "builder-"+imageID)
	if err := os.MkdirAll(instanceDir, 0755); err != nil {
		s.notifyBuildFailed(imageID, "failed to create instance dir")
		return
	}
	config := map[string]interface{}{
		"boot-source": map[string]string{
			"kernel_image_path": s.config.KernelImagePath,
			"boot_args":         "console=ttyS0 reboot=k panic=1 pci=off init=/sbin/init",
		},
		"drives": []map[string]interface{}{
			{
				"drive_id":       "rootfs",
				"path_on_host":   s.config.BuilderRootfsPath,
				"is_root_device": true,
				"is_read_only":   false,
			},
			{
				"drive_id":       "src",
				"path_on_host":   srcDrive,
				"is_root_device": false,
				"is_read_only":   false,
			},
			{
				"drive_id":       "output",
				"path_on_host":   outDrive,
				"is_root_device": false,
				"is_read_only":   false,
			},
		},
		"machine-config": map[string]int{
			"vcpu_count":   2,
			"mem_size_mib": 4096,
		},
	}
	configPath := filepath.Join(instanceDir, "config.json")
	if err := s.writeJSON(configPath, config); err != nil {
		s.notifyBuildFailed(imageID, "failed to write builder config")
		return
	}

	// Start firecracker
	sock := filepath.Join(instanceDir, "firecracker.sock")
	logPath := filepath.Join(instanceDir, "firecracker.log")
	cmd := exec.Command(s.config.FirecrackerBin, "--api-sock", sock, "--config-file", configPath, "--log-path", logPath)
	if err := cmd.Start(); err != nil {
		s.notifyBuildFailed(imageID, fmt.Sprintf("firecracker failed: %v", err))
		return
	}
	pid := cmd.Process.Pid
	s.logger.Info("Builder VM started", "pid", pid, "sock", sock)

	// Wait a bit for boot, then exec build inside VM via vsock/agent or a simple userdata/init hook
	// Simpler approach: have builder rootfs init script watch /dev/vdb (src) and /dev/vdc (out), run docker build and write image.tar to output.
	// We just sleep with a generous timeout and then collect results.
	time.Sleep(3 * time.Minute)

	// Stop VM (best-effort)
	_ = cmd.Process.Kill()

	// Mount output drive and check for image.tar
	outMount := filepath.Join(tmpDir, "mnt-out")
	_ = os.MkdirAll(outMount, 0755)
	if err := s.runCmd(tmpDir, "bash", "-lc", fmt.Sprintf("sudo mount %q %q", outDrive, outMount)); err != nil {
		s.notifyBuildFailed(imageID, "failed to mount output")
		return
	}
	defer s.runCmd(tmpDir, "bash", "-lc", fmt.Sprintf("sudo umount %q", outMount))

	imageTar := filepath.Join(outMount, "image.tar")
	if _, err := os.Stat(imageTar); err != nil {
		s.notifyBuildFailed(imageID, "image.tar not found from builder")
		return
	}

	// Convert docker image tar to Firecracker rootfs
	imagesDir := "/var/lib/zeitwork/images"
	_ = os.MkdirAll(imagesDir, 0755)
	rootfsPath := filepath.Join(imagesDir, fmt.Sprintf("%s.ext4", imageID))
	if err := s.createRootfsFromDockerTar(imageTar, rootfsPath, imageID); err != nil {
		s.notifyBuildFailed(imageID, fmt.Sprintf("rootfs creation failed: %v", err))
		return
	}

	// Hash and size
	fi, err := os.Stat(rootfsPath)
	if err != nil {
		s.notifyBuildFailed(imageID, "stat rootfs failed")
		return
	}
	sizeMB := int32(fi.Size() / 1024 / 1024)
	f, _ := os.Open(rootfsPath)
	defer f.Close()
	h := sha256.New()
	io.Copy(h, f)
	hash := hex.EncodeToString(h.Sum(nil))

	// Notify operator
	notify := map[string]interface{}{
		"status": "ready",
		"hash":   hash,
		"size":   sizeMB,
	}
	body, _ := json.Marshal(notify)
	url := fmt.Sprintf("%s/api/v1/images/%s/status", s.config.OperatorURL, imageID)
	_, _ = s.httpClient.Post(url, "application/json", bytes.NewReader(body))
	s.logger.Info("Build completed in VM", "image_id", imageID)
}

func (s *Service) notifyBuildFailed(imageID string, reason string) {
	s.logger.Error("Build failed", "image_id", imageID, "reason", reason)
	notify := map[string]interface{}{
		"status": "failed",
		"hash":   "",
		"size":   0,
	}
	body, _ := json.Marshal(notify)
	url := fmt.Sprintf("%s/api/v1/images/%s/status", s.config.OperatorURL, imageID)
	s.httpClient.Post(url, "application/json", bytes.NewReader(body))
}

func (s *Service) runCmd(workingDir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = workingDir
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		s.logger.Error("Command failed", "cmd", name+" "+strings.Join(args, " "), "output", out.String())
		return fmt.Errorf("%s: %w", out.String(), err)
	}
	return nil
}

// createRootfsFromDockerTar converts a docker image tar to a bootable ext4 rootfs for Firecracker
func (s *Service) createRootfsFromDockerTar(imageTarPath string, rootfsPath string, imageName string) error {
	// Heavily inspired by _archive_/cmd/instance_manager.go:createRootFS
	script := fmt.Sprintf(`
		set -e
		IMAGE_TAR=%q
		ROOTFS_PATH=%q
		IMAGE_NAME=%q
		MOUNT_DIR=$(mktemp -d)
		EXTRACT_DIR=$(mktemp -d)

		# Create 1GiB rootfs file
		dd if=/dev/zero of="$ROOTFS_PATH" bs=1M count=1024 2>/dev/null
		mkfs.ext4 -F "$ROOTFS_PATH" 2>/dev/null
		mount "$ROOTFS_PATH" "$MOUNT_DIR"

		# Load the Docker image and get ID
		LOAD_OUTPUT=$(docker load -i "$IMAGE_TAR" 2>&1)
		IMAGE_ID=$(echo "$LOAD_OUTPUT" | grep -E "Loaded image(:| ID:)" | sed -E 's/.*Loaded image(:| ID:) *//' | tail -1)
		if [ -z "$IMAGE_ID" ]; then
			IMAGE_ID=$(docker images -q | head -1)
		fi
		[ -n "$IMAGE_ID" ]

		# Export container filesystem
		CONTAINER_ID=$(docker create "$IMAGE_ID")
		docker export "$CONTAINER_ID" | tar -xC "$MOUNT_DIR"
		docker rm "$CONTAINER_ID" >/dev/null 2>&1 || true

		# Provide init script
		cat > "$MOUNT_DIR/init" << 'INIT_SCRIPT'
		#!/bin/sh
		echo "Firecracker VM booting..."
		mount -t proc proc /proc 2>/dev/null || true
		mount -t sysfs sys /sys 2>/dev/null || true
		mount -t devtmpfs devtmpfs /dev 2>/dev/null || true
		# Network best-effort
		if [ -e /sys/class/net/eth0 ]; then
			if command -v ip >/dev/null 2>&1; then
				ip addr add 10.0.0.2/24 dev eth0
				ip link set eth0 up
				ip route add default via 10.0.0.1
			fi
		fi
		export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
		export HOST=0.0.0.0 HOSTNAME=0.0.0.0 NUXT_HOST=0.0.0.0 NITRO_HOST=0.0.0.0
		ENTRYPOINT=$(cat /etc/entrypoint 2>/dev/null || echo "")
		CMD=$(cat /etc/cmd 2>/dev/null || echo "")
		if [ -n "$ENTRYPOINT" ] && [ -n "$CMD" ]; then
			exec $ENTRYPOINT $CMD
		elif [ -n "$ENTRYPOINT" ]; then
			exec $ENTRYPOINT
		elif [ -n "$CMD" ]; then
			exec $CMD
		else
			exec /bin/sh
		fi
		INIT_SCRIPT
		chmod +x "$MOUNT_DIR/init"

		# Try to capture image config as hints
		ENTRYPOINT=$(docker inspect "$IMAGE_ID" --format='{{json .Config.Entrypoint}}' | sed 's/[\[\]"]//g')
		CMD=$(docker inspect "$IMAGE_ID" --format='{{json .Config.Cmd}}' | sed 's/[\[\]"]//g')
		echo "$ENTRYPOINT" > "$MOUNT_DIR/etc/entrypoint" || true
		echo "$CMD" > "$MOUNT_DIR/etc/cmd" || true

		sync
		umount "$MOUNT_DIR"
		rmdir "$MOUNT_DIR"
		rm -rf "$EXTRACT_DIR"
	`, imageTarPath, rootfsPath, imageName)

	cmd := exec.Command("sh", "-c", script)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		s.logger.Error("rootfs script failed", "output", out.String())
		return fmt.Errorf("rootfs creation failed: %w", err)
	}
	return nil
}

func (s *Service) createExt4(path string, sizeMiB int) error {
	if err := s.runCmd("", "bash", "-lc", fmt.Sprintf("dd if=/dev/zero of=%q bs=1M count=%d && mkfs.ext4 -F %q", path, sizeMiB, path)); err != nil {
		return err
	}
	return nil
}

func (s *Service) writeJSON(path string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Close closes the node agent service
func (s *Service) Close() error {
	s.stopAllInstances()
	return nil
}
