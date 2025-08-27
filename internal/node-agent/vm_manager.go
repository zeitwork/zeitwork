package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/health"
)

// VMManager manages the lifecycle of Firecracker VMs
type VMManager struct {
	logger *slog.Logger
	db     *database.DB
	config *VMManagerConfig

	// VM tracking
	mu  sync.RWMutex
	vms map[string]*VM

	// Resource management
	resourceTracker *ResourceTracker

	// Health monitoring
	healthMonitor *health.Monitor

	// Shutdown channel
	shutdown chan struct{}
}

// VMManagerConfig holds VM manager configuration
type VMManagerConfig struct {
	VMDir                 string
	FirecrackerBin        string
	JailerBin             string
	KernelPath            string
	MaxVMs                int
	MaxVCPUs              int
	MaxMemoryMB           int
	HealthCheckInterval   time.Duration
	ResourceCheckInterval time.Duration
}

// VM represents a managed Firecracker VM
type VM struct {
	ID              string
	InstanceID      string
	State           VMState
	Config          *VMConfig
	Process         *os.Process
	SocketPath      string
	LogPath         string
	MetricsPath     string
	CreatedAt       time.Time
	StartedAt       time.Time
	LastHealthCheck time.Time

	// Resource usage
	Resources *VMResources

	// Health status
	HealthStatus health.HealthStatus
	HealthError  string

	// Lifecycle hooks
	OnStart       func(*VM) error
	OnStop        func(*VM) error
	OnHealthCheck func(*VM) error
}

// VMState represents the state of a VM
type VMState string

const (
	VMStatePending    VMState = "pending"
	VMStateStarting   VMState = "starting"
	VMStateRunning    VMState = "running"
	VMStateStopping   VMState = "stopping"
	VMStateStopped    VMState = "stopped"
	VMStateError      VMState = "error"
	VMStateRestarting VMState = "restarting"
)

// VMConfig holds VM configuration
type VMConfig struct {
	VCPUs         int            `json:"vcpus"`
	MemoryMB      int            `json:"memory_mb"`
	KernelPath    string         `json:"kernel_path"`
	RootfsPath    string         `json:"rootfs_path"`
	NetworkConfig *NetworkConfig `json:"network_config"`
	BootArgs      string         `json:"boot_args"`

	// Resource limits
	CPUTemplate string             `json:"cpu_template,omitempty"` // C3, T2, etc
	RateLimiter *RateLimiterConfig `json:"rate_limiter,omitempty"`
}

// NetworkConfig holds network configuration
type NetworkConfig struct {
	InterfaceID string `json:"interface_id"`
	GuestMAC    string `json:"guest_mac"`
	HostDevName string `json:"host_dev_name"`
	GuestIPv6   string `json:"guest_ipv6"`
}

// RateLimiterConfig holds rate limiting configuration
type RateLimiterConfig struct {
	Bandwidth  int `json:"bandwidth"`   // Mbps
	PacketRate int `json:"packet_rate"` // pps
}

// VMResources holds VM resource usage
type VMResources struct {
	CPUUsage       float64   `json:"cpu_usage"`
	MemoryUsage    int64     `json:"memory_usage"`
	DiskReadBytes  int64     `json:"disk_read_bytes"`
	DiskWriteBytes int64     `json:"disk_write_bytes"`
	NetworkRxBytes int64     `json:"network_rx_bytes"`
	NetworkTxBytes int64     `json:"network_tx_bytes"`
	Timestamp      time.Time `json:"timestamp"`
}

// NewVMManager creates a new VM manager
func NewVMManager(config *VMManagerConfig, logger *slog.Logger, db *database.DB) *VMManager {
	return &VMManager{
		logger:          logger,
		db:              db,
		config:          config,
		vms:             make(map[string]*VM),
		shutdown:        make(chan struct{}),
		resourceTracker: NewResourceTracker(config.MaxVCPUs, config.MaxMemoryMB),
	}
}

// Start starts the VM manager
func (m *VMManager) Start(ctx context.Context) error {
	m.logger.Info("Starting VM manager")

	// Start monitoring goroutines
	go m.healthCheckLoop(ctx)
	go m.resourceMonitorLoop(ctx)
	go m.stateReconciliationLoop(ctx)

	return nil
}

// CreateVM creates a new VM
func (m *VMManager) CreateVM(instanceID string, config *VMConfig) (*VM, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check resource availability
	if !m.resourceTracker.CanAllocate(config.VCPUs, config.MemoryMB) {
		return nil, fmt.Errorf("insufficient resources: need %d vCPUs, %d MB memory",
			config.VCPUs, config.MemoryMB)
	}

	// Create VM object
	vm := &VM{
		ID:           uuid.New().String(),
		InstanceID:   instanceID,
		State:        VMStatePending,
		Config:       config,
		CreatedAt:    time.Now(),
		HealthStatus: health.HealthUnknown,
		Resources:    &VMResources{},
	}

	// Set up VM directory
	vmDir := filepath.Join(m.config.VMDir, vm.ID)
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create VM directory: %w", err)
	}

	// Set paths
	vm.SocketPath = filepath.Join(vmDir, "firecracker.sock")
	vm.LogPath = filepath.Join(vmDir, "firecracker.log")
	vm.MetricsPath = filepath.Join(vmDir, "metrics.json")

	// Reserve resources
	if err := m.resourceTracker.Allocate(vm.ID, config.VCPUs, config.MemoryMB); err != nil {
		return nil, fmt.Errorf("failed to allocate resources: %w", err)
	}

	// Store VM
	m.vms[vm.ID] = vm

	m.logger.Info("Created VM", "id", vm.ID, "instance", instanceID)

	return vm, nil
}

// StartVM starts a VM
func (m *VMManager) StartVM(vmID string) error {
	m.mu.Lock()
	vm, exists := m.vms[vmID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("VM not found: %s", vmID)
	}

	if vm.State != VMStatePending && vm.State != VMStateStopped {
		m.mu.Unlock()
		return fmt.Errorf("VM cannot be started from state: %s", vm.State)
	}

	vm.State = VMStateStarting
	m.mu.Unlock()

	// Create Firecracker configuration
	fcConfig := m.createFirecrackerConfig(vm)
	configPath := filepath.Join(m.config.VMDir, vm.ID, "config.json")

	configData, err := json.MarshalIndent(fcConfig, "", "  ")
	if err != nil {
		m.setVMState(vm.ID, VMStateError)
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := ioutil.WriteFile(configPath, configData, 0644); err != nil {
		m.setVMState(vm.ID, VMStateError)
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Start Firecracker process
	cmd := exec.Command(m.config.FirecrackerBin,
		"--api-sock", vm.SocketPath,
		"--config-file", configPath,
		"--log-path", vm.LogPath,
		"--metrics-path", vm.MetricsPath,
		"--level", "Info",
	)

	// Set up log file
	logFile, err := os.Create(vm.LogPath)
	if err != nil {
		m.setVMState(vm.ID, VMStateError)
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Start the process
	if err := cmd.Start(); err != nil {
		m.setVMState(vm.ID, VMStateError)
		return fmt.Errorf("failed to start Firecracker: %w", err)
	}

	vm.Process = cmd.Process
	vm.StartedAt = time.Now()

	// Run start hook if defined
	if vm.OnStart != nil {
		if err := vm.OnStart(vm); err != nil {
			m.logger.Error("Start hook failed", "vm", vm.ID, "error", err)
			// Don't fail the start, just log
		}
	}

	// Update state
	m.setVMState(vm.ID, VMStateRunning)

	// Update database
	if err := m.updateInstanceState(vm.InstanceID, "running"); err != nil {
		m.logger.Error("Failed to update instance state", "error", err)
	}

	m.logger.Info("Started VM", "id", vm.ID)

	return nil
}

// StopVM stops a VM
func (m *VMManager) StopVM(vmID string, graceful bool) error {
	m.mu.Lock()
	vm, exists := m.vms[vmID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("VM not found: %s", vmID)
	}

	if vm.State != VMStateRunning {
		m.mu.Unlock()
		return fmt.Errorf("VM is not running: %s", vm.State)
	}

	vm.State = VMStateStopping
	m.mu.Unlock()

	// Run stop hook if defined
	if vm.OnStop != nil {
		if err := vm.OnStop(vm); err != nil {
			m.logger.Error("Stop hook failed", "vm", vm.ID, "error", err)
		}
	}

	if graceful {
		// Send shutdown signal via API
		if err := m.sendShutdownSignal(vm); err != nil {
			m.logger.Warn("Failed to send shutdown signal", "vm", vm.ID, "error", err)
		}

		// Wait for graceful shutdown (max 30s)
		timeout := time.After(30 * time.Second)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				m.logger.Warn("Graceful shutdown timeout, forcing stop", "vm", vm.ID)
				goto force_stop
			case <-ticker.C:
				if vm.Process != nil {
					if err := vm.Process.Signal(syscall.Signal(0)); err != nil {
						// Process has exited
						goto cleanup
					}
				}
			}
		}
	}

force_stop:
	// Force stop the VM
	if vm.Process != nil {
		if err := vm.Process.Kill(); err != nil {
			m.logger.Error("Failed to kill VM process", "vm", vm.ID, "error", err)
		}
		vm.Process.Wait()
	}

cleanup:
	// Clean up resources
	m.resourceTracker.Release(vm.ID)

	// Update state
	m.setVMState(vm.ID, VMStateStopped)

	// Update database
	if err := m.updateInstanceState(vm.InstanceID, "stopped"); err != nil {
		m.logger.Error("Failed to update instance state", "error", err)
	}

	m.logger.Info("Stopped VM", "id", vm.ID, "graceful", graceful)

	return nil
}

// RestartVM restarts a VM
func (m *VMManager) RestartVM(vmID string, graceful bool) error {
	m.mu.RLock()
	vm, exists := m.vms[vmID]
	if !exists {
		m.mu.RUnlock()
		return fmt.Errorf("VM not found: %s", vmID)
	}
	m.mu.RUnlock()

	m.logger.Info("Restarting VM", "id", vmID, "graceful", graceful)

	// Save original state
	originalState := vm.State

	// Update state
	m.setVMState(vmID, VMStateRestarting)

	// Stop the VM if running
	if originalState == VMStateRunning {
		if err := m.StopVM(vmID, graceful); err != nil {
			m.logger.Error("Failed to stop VM for restart", "vm", vmID, "error", err)
			m.setVMState(vmID, originalState)
			return err
		}
	}

	// Wait a moment for cleanup
	time.Sleep(2 * time.Second)

	// Start the VM
	if err := m.StartVM(vmID); err != nil {
		m.logger.Error("Failed to start VM after restart", "vm", vmID, "error", err)
		m.setVMState(vmID, VMStateError)
		return err
	}

	m.logger.Info("Restarted VM successfully", "id", vmID)

	return nil
}

// GetVM returns VM information
func (m *VMManager) GetVM(vmID string) (*VM, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vm, exists := m.vms[vmID]
	if !exists {
		return nil, fmt.Errorf("VM not found: %s", vmID)
	}

	return vm, nil
}

// ListVMs lists all VMs
func (m *VMManager) ListVMs() []*VM {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vms := make([]*VM, 0, len(m.vms))
	for _, vm := range m.vms {
		vms = append(vms, vm)
	}

	return vms
}

// DeleteVM deletes a VM
func (m *VMManager) DeleteVM(vmID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.vms[vmID]
	if !exists {
		return fmt.Errorf("VM not found: %s", vmID)
	}

	// Stop if running
	if vm.State == VMStateRunning {
		m.mu.Unlock()
		if err := m.StopVM(vmID, false); err != nil {
			return err
		}
		m.mu.Lock()
	}

	// Clean up VM directory
	vmDir := filepath.Join(m.config.VMDir, vm.ID)
	if err := os.RemoveAll(vmDir); err != nil {
		m.logger.Error("Failed to remove VM directory", "path", vmDir, "error", err)
	}

	// Release resources
	m.resourceTracker.Release(vm.ID)

	// Remove from tracking
	delete(m.vms, vmID)

	m.logger.Info("Deleted VM", "id", vmID)

	return nil
}

// healthCheckLoop periodically checks VM health
func (m *VMManager) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkVMHealth()
		}
	}
}

// checkVMHealth checks health of all VMs
func (m *VMManager) checkVMHealth() {
	m.mu.RLock()
	vms := make([]*VM, 0, len(m.vms))
	for _, vm := range m.vms {
		vms = append(vms, vm)
	}
	m.mu.RUnlock()

	for _, vm := range vms {
		if vm.State != VMStateRunning {
			continue
		}

		// Check process is alive
		if vm.Process != nil {
			if err := vm.Process.Signal(syscall.Signal(0)); err != nil {
				m.logger.Error("VM process died unexpectedly", "vm", vm.ID)
				m.setVMState(vm.ID, VMStateError)
				vm.HealthStatus = health.HealthUnhealthy
				vm.HealthError = "Process died"
				continue
			}
		}

		// Check VM responsiveness via socket API
		healthy := m.checkVMSocket(vm)

		if healthy {
			vm.HealthStatus = health.HealthHealthy
			vm.HealthError = ""
		} else {
			vm.HealthStatus = health.HealthUnhealthy
			vm.HealthError = "Socket unresponsive"
		}

		vm.LastHealthCheck = time.Now()

		// Run health check hook
		if vm.OnHealthCheck != nil {
			if err := vm.OnHealthCheck(vm); err != nil {
				m.logger.Error("Health check hook failed", "vm", vm.ID, "error", err)
			}
		}
	}
}

// resourceMonitorLoop monitors VM resource usage
func (m *VMManager) resourceMonitorLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.ResourceCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.collectResourceMetrics()
		}
	}
}

// collectResourceMetrics collects resource metrics from VMs
func (m *VMManager) collectResourceMetrics() {
	m.mu.RLock()
	vms := make([]*VM, 0, len(m.vms))
	for _, vm := range m.vms {
		if vm.State == VMStateRunning {
			vms = append(vms, vm)
		}
	}
	m.mu.RUnlock()

	for _, vm := range vms {
		// Read metrics file if available
		if vm.MetricsPath != "" {
			if data, err := ioutil.ReadFile(vm.MetricsPath); err == nil {
				var metrics map[string]interface{}
				if err := json.Unmarshal(data, &metrics); err == nil {
					// Parse and update resource usage
					vm.Resources = m.parseMetrics(metrics)
					vm.Resources.Timestamp = time.Now()
				}
			}
		}
	}
}

// stateReconciliationLoop ensures VM states are consistent
func (m *VMManager) stateReconciliationLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.reconcileStates()
		}
	}
}

// reconcileStates reconciles VM states with actual state
func (m *VMManager) reconcileStates() {
	m.mu.RLock()
	vms := make([]*VM, 0, len(m.vms))
	for _, vm := range m.vms {
		vms = append(vms, vm)
	}
	m.mu.RUnlock()

	for _, vm := range vms {
		// Check stuck states
		switch vm.State {
		case VMStateStarting:
			// If starting for more than 5 minutes, mark as error
			if time.Since(vm.StartedAt) > 5*time.Minute {
				m.logger.Error("VM stuck in starting state", "vm", vm.ID)
				m.setVMState(vm.ID, VMStateError)
			}
		case VMStateStopping:
			// If stopping for more than 2 minutes, force stop
			if time.Since(vm.LastHealthCheck) > 2*time.Minute {
				m.logger.Warn("VM stuck in stopping state, forcing stop", "vm", vm.ID)
				if vm.Process != nil {
					vm.Process.Kill()
				}
				m.setVMState(vm.ID, VMStateStopped)
			}
		}
	}
}

// Helper functions

func (m *VMManager) setVMState(vmID string, state VMState) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if vm, exists := m.vms[vmID]; exists {
		oldState := vm.State
		vm.State = state
		m.logger.Debug("VM state changed", "vm", vmID, "from", oldState, "to", state)
	}
}

func (m *VMManager) createFirecrackerConfig(vm *VM) map[string]interface{} {
	config := map[string]interface{}{
		"boot-source": map[string]interface{}{
			"kernel_image_path": vm.Config.KernelPath,
			"boot_args":         vm.Config.BootArgs,
		},
		"drives": []map[string]interface{}{
			{
				"drive_id":       "rootfs",
				"path_on_host":   vm.Config.RootfsPath,
				"is_root_device": true,
				"is_read_only":   false,
			},
		},
		"machine-config": map[string]interface{}{
			"vcpu_count":        vm.Config.VCPUs,
			"mem_size_mib":      vm.Config.MemoryMB,
			"track_dirty_pages": false,
		},
	}

	// Add network configuration
	if vm.Config.NetworkConfig != nil {
		config["network-interfaces"] = []map[string]interface{}{
			{
				"iface_id":      vm.Config.NetworkConfig.InterfaceID,
				"guest_mac":     vm.Config.NetworkConfig.GuestMAC,
				"host_dev_name": vm.Config.NetworkConfig.HostDevName,
			},
		}
	}

	// Add rate limiter if configured
	if vm.Config.RateLimiter != nil {
		config["rate-limiter"] = map[string]interface{}{
			"bandwidth": map[string]interface{}{
				"size":        vm.Config.RateLimiter.Bandwidth * 1024 * 1024 / 8, // Mbps to bytes/s
				"refill_time": 100,                                               // ms
			},
		}
	}

	// Add CPU template if specified
	if vm.Config.CPUTemplate != "" {
		config["cpu-config"] = map[string]interface{}{
			"cpu_template": vm.Config.CPUTemplate,
		}
	}

	return config
}

func (m *VMManager) sendShutdownSignal(vm *VM) error {
	// Send Ctrl+Alt+Del via Firecracker API
	// This would use the Firecracker API client
	// For now, just return nil
	return nil
}

func (m *VMManager) checkVMSocket(vm *VM) bool {
	// Check if socket exists and is responsive
	if _, err := os.Stat(vm.SocketPath); err != nil {
		return false
	}

	// In production, would make an API call to check VM status
	return true
}

func (m *VMManager) parseMetrics(metrics map[string]interface{}) *VMResources {
	resources := &VMResources{}

	// Parse CPU usage
	if cpu, ok := metrics["cpu"].(map[string]interface{}); ok {
		if usage, ok := cpu["usage_percent"].(float64); ok {
			resources.CPUUsage = usage
		}
	}

	// Parse memory usage
	if mem, ok := metrics["memory"].(map[string]interface{}); ok {
		if usage, ok := mem["used_bytes"].(float64); ok {
			resources.MemoryUsage = int64(usage)
		}
	}

	// Parse network metrics
	if net, ok := metrics["network"].(map[string]interface{}); ok {
		if rx, ok := net["rx_bytes"].(float64); ok {
			resources.NetworkRxBytes = int64(rx)
		}
		if tx, ok := net["tx_bytes"].(float64); ok {
			resources.NetworkTxBytes = int64(tx)
		}
	}

	// Parse disk metrics
	if disk, ok := metrics["disk"].(map[string]interface{}); ok {
		if read, ok := disk["read_bytes"].(float64); ok {
			resources.DiskReadBytes = int64(read)
		}
		if write, ok := disk["write_bytes"].(float64); ok {
			resources.DiskWriteBytes = int64(write)
		}
	}

	return resources
}

func (m *VMManager) updateInstanceState(instanceID string, state string) error {
	// Update instance state in database
	// This would use the database client
	return nil
}

// GetResourceUsage returns current resource usage
func (m *VMManager) GetResourceUsage() (int, int) {
	return m.resourceTracker.GetUsage()
}

// GetAvailableResources returns available resources
func (m *VMManager) GetAvailableResources() (int, int) {
	return m.resourceTracker.GetAvailable()
}
