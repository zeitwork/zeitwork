package nodeagent

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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

	// Create instance directory
	instanceDir := filepath.Join(s.config.VMWorkDir, instance.ID)
	if err := os.MkdirAll(instanceDir, 0755); err != nil {
		s.logger.Error("Failed to create instance directory", "error", err)
		instance.State = "failed"
		return
	}

	// TODO: Download and prepare the image
	// TODO: Create Firecracker configuration
	// TODO: Start Firecracker process

	// For now, just mark as running
	instance.State = "running"
	instance.Process = &FirecrackerProcess{
		PID:        12345, // TODO: Get actual PID
		SocketPath: filepath.Join(instanceDir, "firecracker.sock"),
		LogFile:    filepath.Join(instanceDir, "firecracker.log"),
	}

	s.logger.Info("Instance started", "instance_id", instance.ID)
}

// stopInstance stops a VM instance
func (s *Service) stopInstance(instance *Instance) {
	s.logger.Info("Stopping instance", "instance_id", instance.ID)

	if instance.Process != nil {
		// TODO: Send shutdown command to Firecracker
		// TODO: Wait for graceful shutdown
		// TODO: Force kill if necessary
	}

	instance.State = "stopped"
	instance.Process = nil

	s.logger.Info("Instance stopped", "instance_id", instance.ID)
}
