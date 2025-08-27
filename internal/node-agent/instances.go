package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// CreateInstanceRequest represents a request to create a new instance
type CreateInstanceRequest struct {
	InstanceID  string          `json:"instance_id"`
	ImageID     string          `json:"image_id"`
	Resources   json.RawMessage `json:"resources"`
	IPAddress   string          `json:"ip_address"`
	DefaultPort int             `json:"default_port"`
}

// UpdateInstanceStateRequest represents a request to update instance state
type UpdateInstanceStateRequest struct {
	State string `json:"state"`
}

// handleCreateInstance handles requests to create a new VM instance
func (s *Service) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	var req CreateInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.InstanceID == "" || req.ImageID == "" {
		http.Error(w, "instance_id and image_id are required", http.StatusBadRequest)
		return
	}

	// Check if instance already exists
	if _, exists := s.instances[req.InstanceID]; exists {
		http.Error(w, "Instance already exists", http.StatusConflict)
		return
	}

	// Parse resources
	var resources struct {
		VCPU   int `json:"vcpu"`
		Memory int `json:"memory"`
	}
	if err := json.Unmarshal(req.Resources, &resources); err != nil {
		resources.VCPU = 1
		resources.Memory = 512
	}

	// Create instance
	instance := &Instance{
		ID:        req.InstanceID,
		ImageID:   req.ImageID,
		State:     "creating",
		Resources: resources,
		IPAddress: req.IPAddress,
	}

	// Store instance
	s.instances[req.InstanceID] = instance

	// Start the instance asynchronously
	go s.startInstance(instance)

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(instance)
}

// handleGetInstance handles requests to get instance information
func (s *Service) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")

	instance, exists := s.instances[instanceID]
	if !exists {
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(instance)
}

// handleUpdateInstanceState handles requests to update instance state
func (s *Service) handleUpdateInstanceState(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")

	instance, exists := s.instances[instanceID]
	if !exists {
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	var req UpdateInstanceStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update instance state based on request
	switch req.State {
	case "stop":
		s.stopInstance(instance)
	case "start":
		go s.startInstance(instance)
	default:
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(instance)
}

// handleDeleteInstance handles requests to delete an instance
func (s *Service) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")

	instance, exists := s.instances[instanceID]
	if !exists {
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	// Stop the instance if running
	if instance.State == "running" {
		s.stopInstance(instance)
	}

	// Remove instance from map
	delete(s.instances, instanceID)

	// Clean up instance directory
	instanceDir := filepath.Join(s.config.VMWorkDir, instanceID)
	os.RemoveAll(instanceDir)

	w.WriteHeader(http.StatusNoContent)
}

// handleNodeInfo returns information about this node
func (s *Service) handleNodeInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"node_id":   s.nodeID,
		"instances": len(s.instances),
		"state":     "ready",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// handleNodeResources returns resource information for this node
func (s *Service) handleNodeResources(w http.ResponseWriter, r *http.Request) {
	// Calculate used resources
	usedVCPU := 0
	usedMemory := 0
	for _, instance := range s.instances {
		if instance.State == "running" {
			usedVCPU += instance.Resources.VCPU
			usedMemory += instance.Resources.Memory
		}
	}

	resources := map[string]interface{}{
		"total": map[string]int{
			"vcpu":   4,    // TODO: Get actual CPU count
			"memory": 8192, // TODO: Get actual memory
		},
		"used": map[string]int{
			"vcpu":   usedVCPU,
			"memory": usedMemory,
		},
		"available": map[string]int{
			"vcpu":   4 - usedVCPU,
			"memory": 8192 - usedMemory,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resources)
}

// startInstance starts a VM instance
func (s *Service) startInstance(instance *Instance) {
	s.logger.Info("Starting instance", "instance_id", instance.ID)

	instanceDir := filepath.Join(s.config.VMWorkDir, instance.ID)
	if err := os.MkdirAll(instanceDir, 0755); err != nil {
		s.logger.Error("Failed to create dir", "error", err)
		instance.State = "failed"
		return
	}

	// Download/prepare image
	rootfsPath := fmt.Sprintf("/var/lib/zeitwork/images/%s.ext4", instance.ImageID)

	// Check if image exists locally
	if _, err := os.Stat(rootfsPath); os.IsNotExist(err) {
		// Download from S3 if available
		if s.s3Client != nil {
			s.logger.Info("Downloading image from S3", "imageID", instance.ImageID)

			// Create directory if needed
			os.MkdirAll(filepath.Dir(rootfsPath), 0755)

			// Download image
			file, err := os.Create(rootfsPath)
			if err != nil {
				s.logger.Error("Failed to create image file", "error", err)
				instance.State = "failed"
				return
			}
			defer file.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			if err := s.s3Client.DownloadImage(ctx, instance.ImageID, file); err != nil {
				s.logger.Error("Failed to download image from S3", "error", err)
				os.Remove(rootfsPath) // Clean up partial download
				instance.State = "failed"
				return
			}

			s.logger.Info("Downloaded image from S3", "imageID", instance.ImageID)
		} else {
			s.logger.Error("Image not found and S3 not configured", "imageID", instance.ImageID)
			instance.State = "failed"
			return
		}
	}

	// Allocate IPv6 address if not already assigned
	var vmIPv6 net.IP
	if instance.IPAddress != "" {
		// Use provided IPv6 address
		vmIPv6 = net.ParseIP(instance.IPAddress)
	} else if s.ipv6Allocator != nil {
		// Allocate a new IPv6 address
		var err error
		vmIPv6, err = s.ipv6Allocator.AllocateAddress()
		if err != nil {
			s.logger.Error("Failed to allocate IPv6 address", "error", err)
			instance.State = "failed"
			return
		}
		instance.IPAddress = vmIPv6.String()
	} else {
		// Fallback to IPv4 or link-local
		instance.IPAddress = "10.0.0.2"
	}

	// Create TAP interface
	var tapName string
	if vmIPv6 != nil && s.networkManager != nil {
		var err error
		tapName, err = s.networkManager.CreateTAPInterface(instance.ID, vmIPv6)
		if err != nil {
			s.logger.Error("Failed to create TAP interface", "error", err)
			instance.State = "failed"
			return
		}
	} else {
		tapName = "tap0" // Fallback
	}

	// Generate MAC address
	macAddr := GenerateMAC(instance.ID)

	// Generate config
	config := map[string]interface{}{
		"boot-source": map[string]string{
			"kernel_image_path": s.config.KernelImagePath,
			"boot_args": fmt.Sprintf(
				"console=ttyS0 reboot=k panic=1 pci=off init=/init ip=%s:::64::eth0:off",
				instance.IPAddress,
			),
		},
		"drives": []map[string]interface{}{
			{
				"drive_id":       "rootfs",
				"path_on_host":   rootfsPath,
				"is_root_device": true,
				"is_read_only":   false,
			},
		},
		"machine-config": map[string]int{
			"vcpu_count":   instance.Resources.VCPU,
			"mem_size_mib": instance.Resources.Memory,
		},
		"network-interfaces": []map[string]string{
			{
				"guest_mac":     macAddr,
				"host_dev_name": tapName,
			},
		},
	}

	configPath := filepath.Join(instanceDir, "config.json")
	jsonData, _ := json.Marshal(config)
	os.WriteFile(configPath, jsonData, 0644)

	socketPath := filepath.Join(instanceDir, "firecracker.sock")
	logPath := filepath.Join(instanceDir, "firecracker.log")

	cmd := exec.Command(s.config.FirecrackerBin, "--api-sock", socketPath, "--config-file", configPath, "--log-path", logPath)
	if err := cmd.Start(); err != nil {
		s.logger.Error("Failed to start Firecracker", "error", err)
		instance.State = "failed"
		return
	}

	instance.Process = &FirecrackerProcess{
		PID:        cmd.Process.Pid,
		SocketPath: socketPath,
		LogFile:    logPath,
	}
	instance.State = "running"

	s.logger.Info("Instance started", "pid", cmd.Process.Pid)
}

// stopInstance stops a VM instance
func (s *Service) stopInstance(instance *Instance) {
	s.logger.Info("Stopping instance", "instance_id", instance.ID)

	if instance.Process != nil {
		// Try graceful shutdown first
		// TODO: Send shutdown command via Firecracker API

		// Force kill after timeout
		if process, err := os.FindProcess(instance.Process.PID); err == nil {
			process.Kill()
		}
	}

	// Clean up network resources
	if s.networkManager != nil && instance.IPAddress != "" {
		vmIPv6 := net.ParseIP(instance.IPAddress)
		if vmIPv6 != nil {
			if err := s.networkManager.CleanupInstance(instance.ID, vmIPv6); err != nil {
				s.logger.Error("Failed to cleanup network", "error", err)
			}
		}
	}

	// Release IPv6 address
	if s.ipv6Allocator != nil && instance.IPAddress != "" {
		vmIPv6 := net.ParseIP(instance.IPAddress)
		if vmIPv6 != nil {
			s.ipv6Allocator.ReleaseAddress(vmIPv6)
		}
	}

	instance.State = "stopped"
	instance.Process = nil

	s.logger.Info("Instance stopped", "instance_id", instance.ID)
}
