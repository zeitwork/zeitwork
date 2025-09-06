package internal

import (
	"fmt"
	"time"

	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// InstanceConverter provides utilities for converting between different instance representations
type InstanceConverter struct{}

// NewInstanceConverter creates a new instance converter
func NewInstanceConverter() *InstanceConverter {
	return &InstanceConverter{}
}

// DatabaseStateToRuntimeState converts database state to runtime state
func (c *InstanceConverter) DatabaseStateToRuntimeState(dbState string) types.InstanceState {
	switch dbState {
	case "pending":
		return types.InstanceStatePending
	case "creating":
		return types.InstanceStateCreating
	case "starting":
		return types.InstanceStateStarting
	case "running":
		return types.InstanceStateRunning
	case "stopping":
		return types.InstanceStateStopping
	case "stopped":
		return types.InstanceStateStopped
	case "failed":
		return types.InstanceStateFailed
	case "terminated":
		return types.InstanceStateTerminated
	default:
		return types.InstanceStatePending
	}
}

// RuntimeStateToDatabase converts runtime state to database state
func (c *InstanceConverter) RuntimeStateToDatabase(runtimeState types.InstanceState) string {
	switch runtimeState {
	case types.InstanceStatePending:
		return "pending"
	case types.InstanceStateCreating:
		return "creating"
	case types.InstanceStateStarting:
		return "starting"
	case types.InstanceStateRunning:
		return "running"
	case types.InstanceStateStopping:
		return "stopping"
	case types.InstanceStateStopped:
		return "stopped"
	case types.InstanceStateFailed:
		return "failed"
	case types.InstanceStateTerminated:
		return "terminated"
	default:
		return "pending"
	}
}

// IsValidStateTransition checks if a state transition is valid
func (c *InstanceConverter) IsValidStateTransition(from, to types.InstanceState) bool {
	validTransitions := map[types.InstanceState][]types.InstanceState{
		types.InstanceStatePending: {
			types.InstanceStateCreating,
			types.InstanceStateFailed,
			types.InstanceStateTerminated,
		},
		types.InstanceStateCreating: {
			types.InstanceStateStarting,
			types.InstanceStateFailed,
			types.InstanceStateTerminated,
		},
		types.InstanceStateStarting: {
			types.InstanceStateRunning,
			types.InstanceStateFailed,
			types.InstanceStateTerminated,
		},
		types.InstanceStateRunning: {
			types.InstanceStateStopping,
			types.InstanceStateFailed,
			types.InstanceStateTerminated,
		},
		types.InstanceStateStopping: {
			types.InstanceStateStopped,
			types.InstanceStateFailed,
			types.InstanceStateTerminated,
		},
		types.InstanceStateStopped: {
			types.InstanceStateStarting,
			types.InstanceStateTerminated,
		},
		types.InstanceStateFailed: {
			types.InstanceStateStarting,
			types.InstanceStateTerminated,
		},
		types.InstanceStateTerminated: {
			// Terminal state - no transitions allowed
		},
	}

	allowedTransitions, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, allowed := range allowedTransitions {
		if allowed == to {
			return true
		}
	}

	return false
}

// InstanceLifecycleManager provides utilities for managing instance lifecycle
type InstanceLifecycleManager struct {
	converter *InstanceConverter
}

// NewInstanceLifecycleManager creates a new instance lifecycle manager
func NewInstanceLifecycleManager() *InstanceLifecycleManager {
	return &InstanceLifecycleManager{
		converter: NewInstanceConverter(),
	}
}

// CalculateInstanceAge calculates the age of an instance
func (m *InstanceLifecycleManager) CalculateInstanceAge(instance *types.Instance) time.Duration {
	return time.Since(instance.CreatedAt)
}

// CalculateInstanceUptime calculates the uptime of a running instance
func (m *InstanceLifecycleManager) CalculateInstanceUptime(instance *types.Instance) time.Duration {
	if instance.StartedAt == nil {
		return 0
	}
	return time.Since(*instance.StartedAt)
}

// ShouldRestartInstance determines if an instance should be restarted based on its state and history
func (m *InstanceLifecycleManager) ShouldRestartInstance(instance *types.Instance, maxRestartAge time.Duration) bool {
	// Don't restart if not in failed state
	if instance.State != types.InstanceStateFailed {
		return false
	}

	// Don't restart very old failed instances
	if m.CalculateInstanceAge(instance) > maxRestartAge {
		return false
	}

	return true
}

// FormatInstanceSummary creates a human-readable summary of an instance
func (m *InstanceLifecycleManager) FormatInstanceSummary(instance *types.Instance) string {
	age := m.CalculateInstanceAge(instance)
	uptime := m.CalculateInstanceUptime(instance)

	summary := fmt.Sprintf("Instance %s: state=%s, age=%s",
		instance.ID[:8], instance.State, age.Round(time.Second))

	if instance.StartedAt != nil {
		summary += fmt.Sprintf(", uptime=%s", uptime.Round(time.Second))
	}

	if instance.Resources != nil {
		summary += fmt.Sprintf(", vcpus=%d, memory=%dMB",
			instance.Resources.VCPUs, instance.Resources.Memory)
	}

	return summary
}
