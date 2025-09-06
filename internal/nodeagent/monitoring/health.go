package monitoring

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// HealthMonitor monitors the health of instances and the runtime
type HealthMonitor struct {
	logger  *slog.Logger
	runtime types.Runtime

	// Health check configuration
	checkInterval      time.Duration
	healthTimeout      time.Duration
	unhealthyThreshold int

	// Health state tracking
	instanceHealth map[string]*InstanceHealthState
	healthMu       sync.RWMutex

	// Callbacks
	onHealthChange HealthChangeCallback
}

// InstanceHealthState tracks the health state of an instance
type InstanceHealthState struct {
	InstanceID       string
	IsHealthy        bool
	LastCheckTime    time.Time
	LastHealthyTime  time.Time
	ConsecutiveFails int
	LastError        error
}

// HealthChangeCallback is called when instance health changes
type HealthChangeCallback func(instanceID string, healthy bool, reason string)

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(logger *slog.Logger, rt types.Runtime) *HealthMonitor {
	return &HealthMonitor{
		logger:             logger,
		runtime:            rt,
		checkInterval:      30 * time.Second,
		healthTimeout:      10 * time.Second,
		unhealthyThreshold: 3,
		instanceHealth:     make(map[string]*InstanceHealthState),
	}
}

// SetHealthChangeCallback sets the callback for health changes
func (h *HealthMonitor) SetHealthChangeCallback(callback HealthChangeCallback) {
	h.onHealthChange = callback
}

// SetCheckInterval sets the health check interval
func (h *HealthMonitor) SetCheckInterval(interval time.Duration) {
	h.checkInterval = interval
}

// SetUnhealthyThreshold sets the number of consecutive failures before marking unhealthy
func (h *HealthMonitor) SetUnhealthyThreshold(threshold int) {
	h.unhealthyThreshold = threshold
}

// Start starts the health monitoring loop
func (h *HealthMonitor) Start(ctx context.Context) {
	h.logger.Info("Starting health monitoring", "interval", h.checkInterval)

	ticker := time.NewTicker(h.checkInterval)
	defer ticker.Stop()

	// Initial health check
	h.performHealthChecks(ctx)

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Stopping health monitoring")
			return
		case <-ticker.C:
			h.performHealthChecks(ctx)
		}
	}
}

// performHealthChecks performs health checks on all instances
func (h *HealthMonitor) performHealthChecks(ctx context.Context) {
	h.logger.Debug("Performing health checks")

	// Get current instances
	instances, err := h.runtime.ListInstances(ctx)
	if err != nil {
		h.logger.Error("Failed to list instances for health check", "error", err)
		return
	}

	// Check health of each instance
	var wg sync.WaitGroup
	for _, instance := range instances {
		wg.Add(1)
		go func(inst *types.Instance) {
			defer wg.Done()
			h.checkInstanceHealth(ctx, inst)
		}(instance)
	}

	// Clean up health state for instances that no longer exist
	h.cleanupHealthState(instances)

	wg.Wait()
	h.logger.Debug("Health checks completed", "instances_checked", len(instances))
}

// checkInstanceHealth checks the health of a single instance
func (h *HealthMonitor) checkInstanceHealth(ctx context.Context, instance *types.Instance) {
	checkCtx, cancel := context.WithTimeout(ctx, h.healthTimeout)
	defer cancel()

	h.logger.Debug("Checking instance health", "instance_id", instance.ID)

	// Perform health check
	healthy, err := h.performInstanceHealthCheck(checkCtx, instance)

	// Update health state
	h.updateHealthState(instance.ID, healthy, err)
}

// performInstanceHealthCheck performs the actual health check for an instance
func (h *HealthMonitor) performInstanceHealthCheck(ctx context.Context, instance *types.Instance) (bool, error) {
	// Check if instance is running
	running, err := h.runtime.IsInstanceRunning(ctx, instance)
	if err != nil {
		return false, fmt.Errorf("failed to check if instance is running: %w", err)
	}

	if !running {
		return false, fmt.Errorf("instance is not running")
	}

	// Additional health checks could be added here:
	// - TCP port connectivity
	// - HTTP health endpoints
	// - Custom health check commands

	// For now, if the instance is running, consider it healthy
	return true, nil
}

// updateHealthState updates the health state for an instance
func (h *HealthMonitor) updateHealthState(instanceID string, healthy bool, checkErr error) {
	h.healthMu.Lock()
	defer h.healthMu.Unlock()

	now := time.Now()
	state, exists := h.instanceHealth[instanceID]

	if !exists {
		state = &InstanceHealthState{
			InstanceID:      instanceID,
			IsHealthy:       true, // Start optimistic
			LastHealthyTime: now,
		}
		h.instanceHealth[instanceID] = state
	}

	state.LastCheckTime = now
	state.LastError = checkErr

	wasHealthy := state.IsHealthy

	if healthy {
		// Instance is healthy
		state.ConsecutiveFails = 0
		state.LastHealthyTime = now
		if !state.IsHealthy {
			state.IsHealthy = true
			h.logger.Info("Instance became healthy", "instance_id", instanceID)
			if h.onHealthChange != nil {
				h.onHealthChange(instanceID, true, "health check passed")
			}
		}
	} else {
		// Instance failed health check
		state.ConsecutiveFails++

		// Mark as unhealthy if threshold reached
		if state.IsHealthy && state.ConsecutiveFails >= h.unhealthyThreshold {
			state.IsHealthy = false
			reason := "health check failed"
			if checkErr != nil {
				reason = fmt.Sprintf("health check failed: %v", checkErr)
			}

			h.logger.Warn("Instance became unhealthy",
				"instance_id", instanceID,
				"consecutive_fails", state.ConsecutiveFails,
				"error", checkErr)

			if h.onHealthChange != nil {
				h.onHealthChange(instanceID, false, reason)
			}
		}
	}

	// Log health state changes
	if wasHealthy != state.IsHealthy {
		h.logger.Info("Instance health changed",
			"instance_id", instanceID,
			"healthy", state.IsHealthy,
			"consecutive_fails", state.ConsecutiveFails)
	}
}

// cleanupHealthState removes health state for instances that no longer exist
func (h *HealthMonitor) cleanupHealthState(currentInstances []*types.Instance) {
	h.healthMu.Lock()
	defer h.healthMu.Unlock()

	// Create set of current instance IDs
	currentIDs := make(map[string]bool)
	for _, instance := range currentInstances {
		currentIDs[instance.ID] = true
	}

	// Remove health state for instances that no longer exist
	for instanceID := range h.instanceHealth {
		if !currentIDs[instanceID] {
			h.logger.Debug("Cleaning up health state for removed instance", "instance_id", instanceID)
			delete(h.instanceHealth, instanceID)
		}
	}
}

// GetInstanceHealth returns the health state of a specific instance
func (h *HealthMonitor) GetInstanceHealth(instanceID string) (*InstanceHealthState, bool) {
	h.healthMu.RLock()
	defer h.healthMu.RUnlock()

	state, exists := h.instanceHealth[instanceID]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid race conditions
	return &InstanceHealthState{
		InstanceID:       state.InstanceID,
		IsHealthy:        state.IsHealthy,
		LastCheckTime:    state.LastCheckTime,
		LastHealthyTime:  state.LastHealthyTime,
		ConsecutiveFails: state.ConsecutiveFails,
		LastError:        state.LastError,
	}, true
}

// GetAllInstanceHealth returns health state for all instances
func (h *HealthMonitor) GetAllInstanceHealth() map[string]*InstanceHealthState {
	h.healthMu.RLock()
	defer h.healthMu.RUnlock()

	result := make(map[string]*InstanceHealthState)
	for id, state := range h.instanceHealth {
		result[id] = &InstanceHealthState{
			InstanceID:       state.InstanceID,
			IsHealthy:        state.IsHealthy,
			LastCheckTime:    state.LastCheckTime,
			LastHealthyTime:  state.LastHealthyTime,
			ConsecutiveFails: state.ConsecutiveFails,
			LastError:        state.LastError,
		}
	}

	return result
}

// GetHealthSummary returns a summary of health statistics
func (h *HealthMonitor) GetHealthSummary() HealthSummary {
	h.healthMu.RLock()
	defer h.healthMu.RUnlock()

	summary := HealthSummary{
		TotalInstances:     len(h.instanceHealth),
		HealthyInstances:   0,
		UnhealthyInstances: 0,
	}

	for _, state := range h.instanceHealth {
		if state.IsHealthy {
			summary.HealthyInstances++
		} else {
			summary.UnhealthyInstances++
		}
	}

	return summary
}

// HealthSummary provides a summary of instance health
type HealthSummary struct {
	TotalInstances     int `json:"total_instances"`
	HealthyInstances   int `json:"healthy_instances"`
	UnhealthyInstances int `json:"unhealthy_instances"`
}
