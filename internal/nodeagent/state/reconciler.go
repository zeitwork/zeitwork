package state

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/samber/lo"
	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// Reconciler handles state reconciliation between desired and actual state
type Reconciler struct {
	logger  *slog.Logger
	runtime types.Runtime
}

// NewReconciler creates a new state reconciler
func NewReconciler(logger *slog.Logger, rt types.Runtime) *Reconciler {
	return &Reconciler{
		logger:  logger,
		runtime: rt,
	}
}

// ReconciliationActions represents the actions needed to reconcile state
type ReconciliationActions struct {
	ToCreate []*types.Instance
	ToUpdate []*InstanceUpdate
	ToDelete []*types.Instance
}

// InstanceUpdate represents an instance that needs updating
type InstanceUpdate struct {
	Current *types.Instance
	Desired *types.Instance
	Changes []ChangeType
}

// ChangeType represents the type of change needed
type ChangeType string

const (
	ChangeTypeState   ChangeType = "state"
	ChangeTypeEnvVars ChangeType = "env_vars"
)

// PlanReconciliation analyzes desired vs actual state and plans reconciliation actions
func (r *Reconciler) PlanReconciliation(desired, actual []*types.Instance) (*ReconciliationActions, error) {
	r.logger.Debug("Planning reconciliation",
		"desired_count", len(desired),
		"actual_count", len(actual))

	// Create maps for efficient lookups
	desiredMap := lo.SliceToMap(desired, func(instance *types.Instance) (string, *types.Instance) {
		return instance.ID, instance
	})

	actualMap := lo.SliceToMap(actual, func(instance *types.Instance) (string, *types.Instance) {
		return instance.ID, instance
	})

	actions := &ReconciliationActions{
		ToCreate: []*types.Instance{},
		ToUpdate: []*InstanceUpdate{},
		ToDelete: []*types.Instance{},
	}

	// Find instances to create (in desired but not in actual)
	for instanceID, desiredInstance := range desiredMap {
		if _, exists := actualMap[instanceID]; !exists {
			actions.ToCreate = append(actions.ToCreate, desiredInstance)
		}
	}

	// Find instances to delete (in actual but not in desired)
	for instanceID, actualInstance := range actualMap {
		if _, exists := desiredMap[instanceID]; !exists {
			actions.ToDelete = append(actions.ToDelete, actualInstance)
		}
	}

	// Find instances to update (exist in both but have differences)
	for instanceID, desiredInstance := range desiredMap {
		if actualInstance, exists := actualMap[instanceID]; exists {
			update := r.detectChanges(actualInstance, desiredInstance)
			if update != nil {
				actions.ToUpdate = append(actions.ToUpdate, update)
			}
		}
	}

	r.logger.Debug("Reconciliation plan created",
		"to_create", len(actions.ToCreate),
		"to_update", len(actions.ToUpdate),
		"to_delete", len(actions.ToDelete))

	return actions, nil
}

// ExecuteReconciliation executes the planned reconciliation actions
func (r *Reconciler) ExecuteReconciliation(ctx context.Context, actions *ReconciliationActions) error {
	r.logger.Info("Executing reconciliation actions")

	// Execute deletions first
	for _, instance := range actions.ToDelete {
		if err := r.executeDelete(ctx, instance); err != nil {
			r.logger.Error("Failed to delete instance",
				"instance_id", instance.ID,
				"error", err)
			// Continue with other deletions
		}
	}

	// Execute updates
	for _, update := range actions.ToUpdate {
		if err := r.executeUpdate(ctx, update); err != nil {
			r.logger.Error("Failed to update instance",
				"instance_id", update.Current.ID,
				"error", err)
			// Continue with other updates
		}
	}

	// Execute creations last
	for _, instance := range actions.ToCreate {
		if err := r.executeCreate(ctx, instance); err != nil {
			r.logger.Error("Failed to create instance",
				"instance_id", instance.ID,
				"error", err)
			// Continue with other creations
		}
	}

	r.logger.Info("Reconciliation actions executed")
	return nil
}

// detectChanges compares actual and desired instances to detect differences
func (r *Reconciler) detectChanges(actual, desired *types.Instance) *InstanceUpdate {
	var changes []ChangeType

	// Check state changes
	if r.shouldUpdateState(actual, desired) {
		changes = append(changes, ChangeTypeState)
	}

	// Check environment variable changes
	if r.hasEnvVarChanges(actual.EnvVars, desired.EnvVars) {
		changes = append(changes, ChangeTypeEnvVars)
	}

	// If no changes needed, return nil
	if len(changes) == 0 {
		return nil
	}

	return &InstanceUpdate{
		Current: actual,
		Desired: desired,
		Changes: changes,
	}
}

// shouldUpdateState determines if instance state should be updated
func (r *Reconciler) shouldUpdateState(actual, desired *types.Instance) bool {
	// State reconciliation logic
	switch desired.State {
	case types.InstanceStateRunning:
		return actual.State != types.InstanceStateRunning
	case types.InstanceStatePending:
		// Pending instances should be started if they're not already running
		return actual.State != types.InstanceStateRunning
	case types.InstanceStateStopped:
		return actual.State == types.InstanceStateRunning
	case types.InstanceStateTerminated:
		return actual.State != types.InstanceStateTerminated
	default:
		return false
	}
}

// hasEnvVarChanges checks if environment variables have changed
func (r *Reconciler) hasEnvVarChanges(actual, desired map[string]string) bool {
	if len(actual) != len(desired) {
		return true
	}

	for key, desiredValue := range desired {
		if actualValue, exists := actual[key]; !exists || actualValue != desiredValue {
			return true
		}
	}

	return false
}

// executeCreate creates a new instance
func (r *Reconciler) executeCreate(ctx context.Context, instance *types.Instance) error {
	r.logger.Info("Creating instance", "instance_id", instance.ID)

	// Convert to instance spec
	spec := r.instanceToSpec(instance)

	// Create the instance
	createdInstance, err := r.runtime.CreateInstance(ctx, spec)
	if err != nil {
		return fmt.Errorf("failed to create instance: %w", err)
	}

	// Start the instance if it should be running
	// Instances in "pending" or "running" state should be started
	if instance.State == types.InstanceStateRunning || instance.State == types.InstanceStatePending {
		if err := r.runtime.StartInstance(ctx, createdInstance); err != nil {
			// Try to clean up the created instance
			if deleteErr := r.runtime.DeleteInstance(ctx, createdInstance); deleteErr != nil {
				r.logger.Error("Failed to cleanup instance after start failure",
					"instance_id", instance.ID,
					"error", deleteErr)
			}
			return fmt.Errorf("failed to start instance: %w", err)
		}
	}

	r.logger.Info("Instance created successfully", "instance_id", instance.ID)
	return nil
}

// executeUpdate updates an existing instance
func (r *Reconciler) executeUpdate(ctx context.Context, update *InstanceUpdate) error {
	r.logger.Info("Updating instance",
		"instance_id", update.Current.ID,
		"changes", update.Changes)

	// Handle state changes
	if lo.Contains(update.Changes, ChangeTypeState) {
		if err := r.updateInstanceState(ctx, update); err != nil {
			return fmt.Errorf("failed to update instance state: %w", err)
		}
	}

	// Handle environment variable changes
	if lo.Contains(update.Changes, ChangeTypeEnvVars) {
		// For env var changes, we typically need to recreate the instance
		r.logger.Info("Environment variables changed, recreating instance",
			"instance_id", update.Current.ID)

		if err := r.executeRecreate(ctx, update); err != nil {
			return fmt.Errorf("failed to recreate instance for env var changes: %w", err)
		}
	}

	r.logger.Info("Instance updated successfully", "instance_id", update.Current.ID)
	return nil
}

// executeDelete deletes an instance
func (r *Reconciler) executeDelete(ctx context.Context, instance *types.Instance) error {
	r.logger.Info("Deleting instance", "instance_id", instance.ID)

	if err := r.runtime.DeleteInstance(ctx, instance); err != nil {
		return fmt.Errorf("failed to delete instance: %w", err)
	}

	r.logger.Info("Instance deleted successfully", "instance_id", instance.ID)
	return nil
}

// updateInstanceState updates the state of an instance
func (r *Reconciler) updateInstanceState(ctx context.Context, update *InstanceUpdate) error {
	switch update.Desired.State {
	case types.InstanceStateRunning:
		return r.runtime.StartInstance(ctx, update.Current)
	case types.InstanceStatePending:
		// Pending instances should be started
		return r.runtime.StartInstance(ctx, update.Current)
	case types.InstanceStateStopped:
		return r.runtime.StopInstance(ctx, update.Current)
	case types.InstanceStateTerminated:
		return r.runtime.DeleteInstance(ctx, update.Current)
	default:
		return fmt.Errorf("unsupported desired state: %s", update.Desired.State)
	}
}

// executeRecreate recreates an instance (delete then create)
func (r *Reconciler) executeRecreate(ctx context.Context, update *InstanceUpdate) error {
	r.logger.Info("Recreating instance", "instance_id", update.Current.ID)

	// Delete current instance
	if err := r.runtime.DeleteInstance(ctx, update.Current); err != nil {
		return fmt.Errorf("failed to delete instance for recreation: %w", err)
	}

	// Create new instance with desired configuration
	if err := r.executeCreate(ctx, update.Desired); err != nil {
		return fmt.Errorf("failed to create instance during recreation: %w", err)
	}

	return nil
}

// instanceToSpec converts a runtime instance to an instance spec
func (r *Reconciler) instanceToSpec(instance *types.Instance) *types.InstanceSpec {
	return &types.InstanceSpec{
		ID:                   instance.ID,
		ImageID:              instance.ImageID,
		ImageTag:             instance.ImageTag,
		Resources:            instance.Resources,
		EnvironmentVariables: instance.EnvVars,
		NetworkConfig: &types.NetworkConfig{
			IPv6Address: instance.NetworkInfo.IPv6Address,
			EnableIPv6:  instance.NetworkInfo.IPv6Address != "",
		},
	}
}
