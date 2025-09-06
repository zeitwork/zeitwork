package state

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// Manager handles state synchronization and reconciliation
type Manager struct {
	logger     *slog.Logger
	nodeID     uuid.UUID
	db         *database.DB
	runtime    types.Runtime
	loader     *Loader
	reconciler *Reconciler

	// State tracking
	desiredState map[string]*types.Instance
	actualState  map[string]*types.Instance
	stateMu      sync.RWMutex

	// Configuration
	reconcileInterval time.Duration
}

// NewManager creates a new state manager
func NewManager(logger *slog.Logger, nodeID uuid.UUID, db *database.DB, rt types.Runtime, imageRegistry string) *Manager {
	loader := NewLoader(logger, nodeID, db, imageRegistry)
	reconciler := NewReconciler(logger, rt, db.Queries())

	return &Manager{
		logger:            logger,
		nodeID:            nodeID,
		db:                db,
		runtime:           rt,
		loader:            loader,
		reconciler:        reconciler,
		desiredState:      make(map[string]*types.Instance),
		actualState:       make(map[string]*types.Instance),
		reconcileInterval: 5 * time.Minute, // More frequent reconciliation to detect stopped containers
	}
}

// LoadInitialState loads the initial desired state from database and actual state from runtime
func (m *Manager) LoadInitialState(ctx context.Context) error {
	m.logger.Info("Loading initial state")

	// Load desired state from database
	desired, err := m.loader.LoadDesiredState(ctx)
	if err != nil {
		return fmt.Errorf("failed to load desired state: %w", err)
	}

	// Load actual state from runtime
	actual, err := m.runtime.ListInstances(ctx)
	if err != nil {
		return fmt.Errorf("failed to load actual state: %w", err)
	}

	// Update state maps
	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	m.desiredState = make(map[string]*types.Instance)
	for _, instance := range desired {
		m.desiredState[instance.ID] = instance
	}

	m.actualState = make(map[string]*types.Instance)
	for _, instance := range actual {
		m.actualState[instance.ID] = instance
	}

	m.logger.Info("Initial state loaded",
		"desired_instances", len(m.desiredState),
		"actual_instances", len(m.actualState))

	return nil
}

// UpdateDesiredState updates the desired state for a specific instance
func (m *Manager) UpdateDesiredState(ctx context.Context, instanceID string) error {
	m.logger.Debug("Updating desired state", "instance_id", instanceID)

	// Load updated instance from database
	instance, err := m.loader.LoadInstance(ctx, instanceID)
	if err != nil {
		if IsInstanceNotFound(err) {
			// Instance was deleted
			m.stateMu.Lock()
			delete(m.desiredState, instanceID)
			m.stateMu.Unlock()
			m.logger.Debug("Instance removed from desired state", "instance_id", instanceID)
			return nil
		}
		return fmt.Errorf("failed to load instance: %w", err)
	}

	// Update desired state
	m.stateMu.Lock()
	m.desiredState[instanceID] = instance
	m.stateMu.Unlock()

	m.logger.Debug("Desired state updated", "instance_id", instanceID)
	return nil
}

// RefreshDesiredState reloads all desired state from database
func (m *Manager) RefreshDesiredState(ctx context.Context) error {
	m.logger.Debug("Refreshing desired state")

	desired, err := m.loader.LoadDesiredState(ctx)
	if err != nil {
		return fmt.Errorf("failed to refresh desired state: %w", err)
	}

	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	m.desiredState = make(map[string]*types.Instance)
	for _, instance := range desired {
		m.desiredState[instance.ID] = instance
	}

	m.logger.Debug("Desired state refreshed", "instances", len(m.desiredState))
	return nil
}

// RefreshActualState reloads actual state from runtime
func (m *Manager) RefreshActualState(ctx context.Context) error {
	m.logger.Debug("Refreshing actual state")

	actual, err := m.runtime.ListInstances(ctx)
	if err != nil {
		return fmt.Errorf("failed to refresh actual state: %w", err)
	}

	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	m.actualState = make(map[string]*types.Instance)
	for _, instance := range actual {
		m.actualState[instance.ID] = instance
	}

	m.logger.Debug("Actual state refreshed", "instances", len(m.actualState))
	return nil
}

// Reconcile performs state reconciliation
func (m *Manager) Reconcile(ctx context.Context) error {
	m.logger.Debug("Starting state reconciliation")

	// Get current state snapshots
	m.stateMu.RLock()
	desired := make([]*types.Instance, 0, len(m.desiredState))
	for _, instance := range m.desiredState {
		desired = append(desired, instance)
	}

	actual := make([]*types.Instance, 0, len(m.actualState))
	for _, instance := range m.actualState {
		actual = append(actual, instance)
	}
	m.stateMu.RUnlock()

	// Perform reconciliation
	actions, err := m.reconciler.PlanReconciliation(desired, actual)
	if err != nil {
		return fmt.Errorf("failed to plan reconciliation: %w", err)
	}

	if len(actions.ToCreate) == 0 && len(actions.ToUpdate) == 0 && len(actions.ToDelete) == 0 {
		m.logger.Debug("No reconciliation actions needed")
		return nil
	}

	m.logger.Info("Executing reconciliation actions",
		"to_create", len(actions.ToCreate),
		"to_update", len(actions.ToUpdate),
		"to_delete", len(actions.ToDelete))

	// Execute reconciliation actions
	if err := m.reconciler.ExecuteReconciliation(ctx, actions); err != nil {
		return fmt.Errorf("failed to execute reconciliation: %w", err)
	}

	// Refresh actual state after reconciliation
	if err := m.RefreshActualState(ctx); err != nil {
		m.logger.Warn("Failed to refresh actual state after reconciliation", "error", err)
	}

	m.logger.Info("State reconciliation completed")
	return nil
}

// StartPeriodicReconciliation starts the periodic full reconciliation loop
func (m *Manager) StartPeriodicReconciliation(ctx context.Context) {
	ticker := time.NewTicker(m.reconcileInterval)
	defer ticker.Stop()

	m.logger.Info("Starting periodic reconciliation", "interval", m.reconcileInterval)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Stopping periodic reconciliation")
			return
		case <-ticker.C:
			m.logger.Info("Performing periodic full reconciliation")

			// Refresh both desired and actual state
			if err := m.RefreshDesiredState(ctx); err != nil {
				m.logger.Error("Failed to refresh desired state during periodic reconciliation", "error", err)
				continue
			}

			if err := m.RefreshActualState(ctx); err != nil {
				m.logger.Error("Failed to refresh actual state during periodic reconciliation", "error", err)
				continue
			}

			// Perform reconciliation
			if err := m.Reconcile(ctx); err != nil {
				m.logger.Error("Failed to perform periodic reconciliation", "error", err)
			}
		}
	}
}

// GetDesiredInstances returns a copy of the current desired state
func (m *Manager) GetDesiredInstances() []*types.Instance {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()

	instances := make([]*types.Instance, 0, len(m.desiredState))
	for _, instance := range m.desiredState {
		instances = append(instances, instance)
	}
	return instances
}

// GetActualInstances returns a copy of the current actual state
func (m *Manager) GetActualInstances() []*types.Instance {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()

	instances := make([]*types.Instance, 0, len(m.actualState))
	for _, instance := range m.actualState {
		instances = append(instances, instance)
	}
	return instances
}

// GetInstance returns a specific instance from desired state
func (m *Manager) GetInstance(instanceID string) (*types.Instance, bool) {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()

	instance, exists := m.desiredState[instanceID]
	return instance, exists
}

// GetStateStats returns statistics about current state
func (m *Manager) GetStateStats() StateStats {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()

	stats := StateStats{
		DesiredInstances: len(m.desiredState),
		ActualInstances:  len(m.actualState),
		StatesByDesired:  make(map[string]int),
		StatesByActual:   make(map[string]int),
	}

	// Count desired states
	for _, instance := range m.desiredState {
		stats.StatesByDesired[string(instance.State)]++
	}

	// Count actual states
	for _, instance := range m.actualState {
		stats.StatesByActual[string(instance.State)]++
	}

	return stats
}

// StateStats provides statistics about the current state
type StateStats struct {
	DesiredInstances int            `json:"desired_instances"`
	ActualInstances  int            `json:"actual_instances"`
	StatesByDesired  map[string]int `json:"states_by_desired"`
	StatesByActual   map[string]int `json:"states_by_actual"`
}
