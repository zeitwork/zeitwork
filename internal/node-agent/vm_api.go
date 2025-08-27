package nodeagent

import (
	"encoding/json"
	"net/http"
	"time"
)

// VM Lifecycle API handlers

// handleVMRestart handles VM restart requests
func (s *Service) handleVMRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	instanceID := r.PathValue("id")
	if instanceID == "" {
		http.Error(w, "Instance ID required", http.StatusBadRequest)
		return
	}

	// Parse request body for restart options
	var req struct {
		Graceful bool `json:"graceful"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Graceful = true // Default to graceful restart
	}

	s.logger.Info("Restarting VM instance", "instance_id", instanceID, "graceful", req.Graceful)

	// Find VM by instance ID
	s.mu.RLock()
	var vmID string
	for id, vm := range s.vmManager.vms {
		if vm.InstanceID == instanceID {
			vmID = id
			break
		}
	}
	s.mu.RUnlock()

	if vmID == "" {
		http.Error(w, "VM not found for instance", http.StatusNotFound)
		return
	}

	// Restart VM
	if err := s.vmManager.RestartVM(vmID, req.Graceful); err != nil {
		s.logger.Error("Failed to restart VM", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "success",
		"instance_id": instanceID,
		"vm_id":       vmID,
		"action":      "restarted",
	})
}

// handleVMHealth handles VM health check requests
func (s *Service) handleVMHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	instanceID := r.PathValue("id")
	if instanceID == "" {
		http.Error(w, "Instance ID required", http.StatusBadRequest)
		return
	}

	// Find VM by instance ID
	s.mu.RLock()
	var vm *VM
	for _, v := range s.vmManager.vms {
		if v.InstanceID == instanceID {
			vm = v
			break
		}
	}
	s.mu.RUnlock()

	if vm == nil {
		http.Error(w, "VM not found for instance", http.StatusNotFound)
		return
	}

	// Prepare health response
	health := map[string]interface{}{
		"instance_id":       instanceID,
		"vm_id":             vm.ID,
		"state":             string(vm.State),
		"health_status":     string(vm.HealthStatus),
		"created_at":        vm.CreatedAt.Format(time.RFC3339),
		"started_at":        vm.StartedAt.Format(time.RFC3339),
		"last_health_check": vm.LastHealthCheck.Format(time.RFC3339),
		"uptime_seconds":    int(time.Since(vm.StartedAt).Seconds()),
	}

	// Add resource usage if available
	if vm.Resources != nil {
		health["resources"] = map[string]interface{}{
			"cpu_usage":        vm.Resources.CPUUsage,
			"memory_usage":     vm.Resources.MemoryUsage,
			"disk_read_bytes":  vm.Resources.DiskReadBytes,
			"disk_write_bytes": vm.Resources.DiskWriteBytes,
			"network_rx_bytes": vm.Resources.NetworkRxBytes,
			"network_tx_bytes": vm.Resources.NetworkTxBytes,
		}
	}

	// Add error if unhealthy
	if vm.HealthError != "" {
		health["error"] = vm.HealthError
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// handleVMMetrics handles VM metrics requests
func (s *Service) handleVMMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get all VM metrics
	vms := s.vmManager.ListVMs()

	metrics := make([]map[string]interface{}, 0, len(vms))
	for _, vm := range vms {
		vmMetrics := map[string]interface{}{
			"vm_id":         vm.ID,
			"instance_id":   vm.InstanceID,
			"state":         string(vm.State),
			"health_status": string(vm.HealthStatus),
			"vcpus":         vm.Config.VCPUs,
			"memory_mb":     vm.Config.MemoryMB,
		}

		if vm.Resources != nil {
			vmMetrics["cpu_usage"] = vm.Resources.CPUUsage
			vmMetrics["memory_usage"] = vm.Resources.MemoryUsage
			vmMetrics["network_rx_bytes"] = vm.Resources.NetworkRxBytes
			vmMetrics["network_tx_bytes"] = vm.Resources.NetworkTxBytes
		}

		metrics = append(metrics, vmMetrics)
	}

	// Get resource tracker stats
	resourceStats := s.vmManager.resourceTracker.GetStats()

	response := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"node_id":   s.nodeID.String(),
		"resources": resourceStats,
		"vms":       metrics,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleVMList handles VM list requests
func (s *Service) handleVMList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vms := s.vmManager.ListVMs()

	vmList := make([]map[string]interface{}, 0, len(vms))
	for _, vm := range vms {
		vmInfo := map[string]interface{}{
			"vm_id":             vm.ID,
			"instance_id":       vm.InstanceID,
			"state":             string(vm.State),
			"health_status":     string(vm.HealthStatus),
			"vcpus":             vm.Config.VCPUs,
			"memory_mb":         vm.Config.MemoryMB,
			"created_at":        vm.CreatedAt.Format(time.RFC3339),
			"started_at":        vm.StartedAt.Format(time.RFC3339),
			"last_health_check": vm.LastHealthCheck.Format(time.RFC3339),
		}

		if vm.Config.NetworkConfig != nil {
			vmInfo["ipv6_address"] = vm.Config.NetworkConfig.GuestIPv6
		}

		vmList = append(vmList, vmInfo)
	}

	response := map[string]interface{}{
		"node_id": s.nodeID.String(),
		"count":   len(vmList),
		"vms":     vmList,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleVMResources handles resource availability requests
func (s *Service) handleVMResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get resource statistics
	stats := s.vmManager.resourceTracker.GetStats()

	// Check if over threshold
	overThreshold, message := s.vmManager.resourceTracker.IsOverThreshold()

	// Predict capacity exhaustion
	timeToExhaustion := s.vmManager.resourceTracker.PredictCapacity()

	response := map[string]interface{}{
		"node_id":            s.nodeID.String(),
		"resources":          stats,
		"over_threshold":     overThreshold,
		"threshold_message":  message,
		"time_to_exhaustion": timeToExhaustion.String(),
		"healthy":            !overThreshold,
	}

	// Get history if requested
	if r.URL.Query().Get("history") == "true" {
		duration := 1 * time.Hour // Default 1 hour
		if d := r.URL.Query().Get("duration"); d != "" {
			if parsed, err := time.ParseDuration(d); err == nil {
				duration = parsed
			}
		}

		history := s.vmManager.resourceTracker.GetHistory(duration)
		response["history"] = history
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// setupVMRoutes sets up VM lifecycle management routes
func (s *Service) setupVMRoutes(mux *http.ServeMux) {
	// VM lifecycle operations
	mux.HandleFunc("POST /api/v1/vms/{id}/restart", s.handleVMRestart)

	// VM health and metrics
	mux.HandleFunc("GET /api/v1/vms/{id}/health", s.handleVMHealth)
	mux.HandleFunc("GET /api/v1/vms/metrics", s.handleVMMetrics)
	mux.HandleFunc("GET /api/v1/vms", s.handleVMList)

	// Resource management
	mux.HandleFunc("GET /api/v1/resources", s.handleVMResources)
}
