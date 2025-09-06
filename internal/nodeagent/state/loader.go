package state

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/samber/lo"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// Loader handles loading state from the database
type Loader struct {
	logger *slog.Logger
	nodeID uuid.UUID
	db     *database.DB
}

// NewLoader creates a new state loader
func NewLoader(logger *slog.Logger, nodeID uuid.UUID, db *database.DB) *Loader {
	return &Loader{
		logger: logger,
		nodeID: nodeID,
		db:     db,
	}
}

// LoadDesiredState loads all desired instances for this node from the database
func (l *Loader) LoadDesiredState(ctx context.Context) ([]*types.Instance, error) {
	l.logger.Debug("Loading desired state from database", "node_id", l.nodeID)

	// Convert UUID to pgtype.UUID for database query
	pgNodeID := pgtype.UUID{
		Bytes: l.nodeID,
		Valid: true,
	}

	// Query instances for this node
	dbInstances, err := l.db.Queries().InstancesFindByNode(ctx, pgNodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to query instances: %w", err)
	}

	// Filter valid instances and convert to runtime format
	validInstances := lo.Filter(dbInstances, func(dbInstance *database.InstancesFindByNodeRow, _ int) bool {
		return dbInstance.ID.Valid && dbInstance.ImageID.Valid
	})

	instances := lo.Map(validInstances, func(dbInstance *database.InstancesFindByNodeRow, _ int) *types.Instance {
		return l.dbInstanceToRuntime(dbInstance)
	})

	l.logger.Debug("Desired state loaded",
		"total_db_instances", len(dbInstances),
		"valid_instances", len(instances))

	return instances, nil
}

// LoadInstance loads a specific instance from the database
func (l *Loader) LoadInstance(ctx context.Context, instanceID string) (*types.Instance, error) {
	l.logger.Debug("Loading instance from database", "instance_id", instanceID)

	// Parse instance ID
	instanceUUID, err := uuid.Parse(instanceID)
	if err != nil {
		return nil, fmt.Errorf("invalid instance ID: %w", err)
	}

	pgInstanceID := pgtype.UUID{
		Bytes: instanceUUID,
		Valid: true,
	}

	// Query the specific instance
	dbInstance, err := l.db.Queries().InstancesGetById(ctx, pgInstanceID)
	if err != nil {
		if isNotFoundError(err) {
			return nil, NewInstanceNotFoundError(instanceID)
		}
		return nil, fmt.Errorf("failed to query instance: %w", err)
	}

	// Verify the instance belongs to this node
	nodeUUID := uuid.UUID(dbInstance.NodeID.Bytes)
	if nodeUUID != l.nodeID {
		return nil, fmt.Errorf("instance %s does not belong to node %s", instanceID, l.nodeID)
	}

	instance := l.dbInstanceToRuntimeFromGetById(*dbInstance)
	l.logger.Debug("Instance loaded", "instance_id", instanceID)

	return instance, nil
}

// dbInstanceToRuntime converts database instance to runtime instance
func (l *Loader) dbInstanceToRuntime(dbInstance *database.InstancesFindByNodeRow) *types.Instance {
	// Convert UUIDs
	instanceID := uuid.UUID(dbInstance.ID.Bytes).String()
	imageID := uuid.UUID(dbInstance.ImageID.Bytes).String()

	// Parse environment variables
	envVars := make(map[string]string)
	if dbInstance.EnvironmentVariables != "" {
		if err := json.Unmarshal([]byte(dbInstance.EnvironmentVariables), &envVars); err != nil {
			l.logger.Warn("Failed to parse instance env vars",
				"instance_id", instanceID,
				"error", err)
		}
	}

	// Map database state to runtime state
	runtimeState := l.mapDatabaseState(dbInstance.State)

	// Create resource specification
	resources := &types.ResourceSpec{
		VCPUs:  dbInstance.Vcpus,
		Memory: dbInstance.Memory,
	}

	instance := &types.Instance{
		ID:        instanceID,
		ImageID:   imageID,
		ImageTag:  l.buildImageTag(imageID), // TODO: Get actual image tag from images table
		State:     runtimeState,
		Resources: resources,
		EnvVars:   envVars,
		NetworkInfo: &types.NetworkInfo{
			IPv6Address: dbInstance.Ipv6Address,
		},
		CreatedAt: dbInstance.CreatedAt.Time,
	}

	return instance
}

// dbInstanceToRuntimeFromGetById converts database instance from GetById query to runtime instance
func (l *Loader) dbInstanceToRuntimeFromGetById(dbInstance database.InstancesGetByIdRow) *types.Instance {
	// Convert UUIDs
	instanceID := uuid.UUID(dbInstance.ID.Bytes).String()
	imageID := uuid.UUID(dbInstance.ImageID.Bytes).String()

	// Parse environment variables
	envVars := make(map[string]string)
	if dbInstance.EnvironmentVariables != "" {
		if err := json.Unmarshal([]byte(dbInstance.EnvironmentVariables), &envVars); err != nil {
			l.logger.Warn("Failed to parse instance env vars",
				"instance_id", instanceID,
				"error", err)
		}
	}

	// Map database state to runtime state
	runtimeState := l.mapDatabaseState(dbInstance.State)

	// Create resource specification
	resources := &types.ResourceSpec{
		VCPUs:  dbInstance.Vcpus,
		Memory: dbInstance.Memory,
	}

	instance := &types.Instance{
		ID:        instanceID,
		ImageID:   imageID,
		ImageTag:  l.buildImageTag(imageID), // TODO: Get actual image tag from images table
		State:     runtimeState,
		Resources: resources,
		EnvVars:   envVars,
		NetworkInfo: &types.NetworkInfo{
			IPv6Address: dbInstance.Ipv6Address,
		},
		CreatedAt: dbInstance.CreatedAt.Time,
	}

	return instance
}

// mapDatabaseState maps database instance state to runtime state
func (l *Loader) mapDatabaseState(dbState string) types.InstanceState {
	switch dbState {
	case "pending":
		return types.InstanceStatePending
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
		l.logger.Warn("Unknown database state", "state", dbState)
		return types.InstanceStatePending
	}
}

// buildImageTag constructs an image tag from image ID
// TODO: This should query the images table to get the actual tag
func (l *Loader) buildImageTag(imageID string) string {
	// For now, use a placeholder format
	// In production, this should query the images table to get registry/name:tag
	return fmt.Sprintf("localhost:5000/zeitwork/%s:latest", imageID[:8])
}

// Helper functions

// isNotFoundError checks if the error is a "not found" error
func isNotFoundError(err error) bool {
	// This is a simplified check - in practice you'd check for specific PostgreSQL error codes
	return err != nil && (err.Error() == "no rows in result set" ||
		err.Error() == "sql: no rows in result set")
}

// InstanceNotFoundError represents an instance not found error
type InstanceNotFoundError struct {
	InstanceID string
}

func (e *InstanceNotFoundError) Error() string {
	return fmt.Sprintf("instance not found: %s", e.InstanceID)
}

// NewInstanceNotFoundError creates a new instance not found error
func NewInstanceNotFoundError(instanceID string) *InstanceNotFoundError {
	return &InstanceNotFoundError{InstanceID: instanceID}
}

// IsInstanceNotFound checks if an error is an instance not found error
func IsInstanceNotFound(err error) bool {
	_, ok := err.(*InstanceNotFoundError)
	return ok
}
