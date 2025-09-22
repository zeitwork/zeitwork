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
	logger        *slog.Logger
	nodeID        uuid.UUID
	db            *database.DB
	imageRegistry string
}

// NewLoader creates a new state loader
func NewLoader(logger *slog.Logger, nodeID uuid.UUID, db *database.DB, imageRegistry string) *Loader {
	return &Loader{
		logger:        logger,
		nodeID:        nodeID,
		db:            db,
		imageRegistry: imageRegistry,
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

	l.logger.Debug("Querying database for instances",
		"node_id", l.nodeID.String(),
		"pg_node_id_valid", pgNodeID.Valid)

	// Query instances for this node
	dbInstances, err := l.db.Queries().InstancesFindByNode(ctx, pgNodeID)
	if err != nil {
		l.logger.Error("Database query failed",
			"node_id", l.nodeID.String(),
			"error", err)
		return nil, fmt.Errorf("failed to query instances: %w", err)
	}

	l.logger.Debug("Database query completed",
		"node_id", l.nodeID.String(),
		"raw_instances_count", len(dbInstances))

	// Filter valid instances and convert to runtime format
	validInstances := lo.Filter(dbInstances, func(dbInstance *database.InstancesFindByNodeRow, _ int) bool {
		isValid := dbInstance.ID.Valid && dbInstance.ImageID.Valid
		if !isValid {
			l.logger.Warn("Filtering out invalid instance",
				"id_valid", dbInstance.ID.Valid,
				"image_id_valid", dbInstance.ImageID.Valid,
				"instance_id", dbInstance.ID)
		}
		return isValid
	})

	l.logger.Debug("Instance filtering completed",
		"raw_instances", len(dbInstances),
		"valid_instances", len(validInstances))

	instances := lo.Map(validInstances, func(dbInstance *database.InstancesFindByNodeRow, _ int) *types.Instance {
		instance := l.dbInstanceToRuntime(ctx, dbInstance)
		l.logger.Debug("Converted instance",
			"instance_id", instance.ID,
			"state", instance.State,
			"ip_address", instance.NetworkInfo.IPAddress,
			"default_port", instance.NetworkInfo.DefaultPort)
		return instance
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

	instance := l.dbInstanceToRuntimeFromGetById(ctx, *dbInstance)
	l.logger.Debug("Instance loaded", "instance_id", instanceID)

	return instance, nil
}

// dbInstanceToRuntime converts database instance to runtime instance
func (l *Loader) dbInstanceToRuntime(ctx context.Context, dbInstance *database.InstancesFindByNodeRow) *types.Instance {
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

	// Get the actual image tag from database
	imageTag, err := l.getImageTag(ctx, imageID)
	if err != nil {
		l.logger.Error("Failed to get image tag",
			"instance_id", instanceID,
			"image_id", imageID,
			"error", err)
		// Use fallback format
		imageTag = fmt.Sprintf("%s/zeitwork/%s:latest", l.imageRegistry, imageID[:8])
	}

	// Use database enum directly as runtime state
	runtimeState := types.InstanceState(dbInstance.State)

	// Create resource specification
	resources := &types.ResourceSpec{
		VCPUs:  dbInstance.Vcpus,
		Memory: dbInstance.Memory,
	}

	instance := &types.Instance{
		ID:        instanceID,
		ImageID:   imageID,
		ImageTag:  imageTag,
		State:     runtimeState,
		Resources: resources,
		EnvVars:   envVars,
		NetworkInfo: &types.NetworkInfo{
			IPAddress:   dbInstance.IpAddress,
			DefaultPort: dbInstance.DefaultPort,
		},
		CreatedAt: dbInstance.CreatedAt.Time,
	}

	return instance
}

// dbInstanceToRuntimeFromGetById converts database instance from GetById query to runtime instance
func (l *Loader) dbInstanceToRuntimeFromGetById(ctx context.Context, dbInstance database.InstancesGetByIdRow) *types.Instance {
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

	// Get the actual image tag from database
	imageTag, err := l.getImageTag(ctx, imageID)
	if err != nil {
		l.logger.Error("Failed to get image tag",
			"instance_id", instanceID,
			"image_id", imageID,
			"error", err)
		// Use fallback format
		imageTag = fmt.Sprintf("%s/zeitwork/%s:latest", l.imageRegistry, imageID[:8])
	}

	// Use database enum directly as runtime state
	runtimeState := types.InstanceState(dbInstance.State)

	// Create resource specification
	resources := &types.ResourceSpec{
		VCPUs:  dbInstance.Vcpus,
		Memory: dbInstance.Memory,
	}

	instance := &types.Instance{
		ID:        instanceID,
		ImageID:   imageID,
		ImageTag:  imageTag,
		State:     runtimeState,
		Resources: resources,
		EnvVars:   envVars,
		NetworkInfo: &types.NetworkInfo{
			IPAddress:   dbInstance.IpAddress,
			DefaultPort: dbInstance.DefaultPort,
		},
		CreatedAt: dbInstance.CreatedAt.Time,
	}

	return instance
}

// getImageTag queries the images table to get the actual image tag
func (l *Loader) getImageTag(ctx context.Context, imageID string) (string, error) {
	// Parse image ID to UUID
	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return "", fmt.Errorf("invalid image ID: %w", err)
	}

	pgImageID := pgtype.UUID{
		Bytes: imageUUID,
		Valid: true,
	}

	// Query the image from database
	image, err := l.db.Queries().ImagesGetById(ctx, pgImageID)
	if err != nil {
		if isNotFoundError(err) {
			l.logger.Warn("Image not found in database, using fallback format",
				"image_id", imageID)
			// Fallback to placeholder format if image not found
			return fmt.Sprintf("%s/zeitwork/%s:latest", l.imageRegistry, imageID[:8]), nil
		}
		return "", fmt.Errorf("failed to query image: %w", err)
	}

	return image.Name, nil
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
