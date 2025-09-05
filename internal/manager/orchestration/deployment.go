package orchestration

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/samber/lo"

	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

// DeploymentOrchestrator handles deployment business logic
type DeploymentOrchestrator struct {
	db     *database.Queries
	logger *slog.Logger
}

// NewDeploymentOrchestrator creates a new deployment orchestrator
func NewDeploymentOrchestrator(db *database.Queries, logger *slog.Logger) *DeploymentOrchestrator {
	return &DeploymentOrchestrator{
		db:     db,
		logger: logger,
	}
}

// HandleBuildCompletion processes build completion events (legacy method - can be removed)
func (o *DeploymentOrchestrator) HandleBuildCompletion(ctx context.Context, buildID string) error {
	o.logger.Info("Processing build completion", "build_id", buildID)

	// Just delegate to the ID-based method
	buildUUID := uuid.MustParseUUID(buildID)
	return o.HandleBuildCompletedByID(ctx, buildUUID)
}

// HandleBuildCompletedByID processes build completion events using just the build ID
func (o *DeploymentOrchestrator) HandleBuildCompletedByID(ctx context.Context, buildID pgtype.UUID) error {
	o.logger.Info("Processing build completion by ID", "build_id", buildID)

	// Find deployments ready for this build
	readyDeployments := lo.Must(o.db.DeploymentsGetReadyForDeployment(ctx))

	matchingDeployment, found := lo.Find(readyDeployments, func(d *database.DeploymentsGetReadyForDeploymentRow) bool {
		return d.BuildID == buildID
	})

	if !found {
		o.logger.Debug("No deployment found ready for this build", "build_id", buildID)
		return nil
	}

	o.logger.Info("Found deployment ready for deployment",
		"deployment_id", matchingDeployment.ID,
		"build_id", buildID)

	return o.CreateInstancesForDeployment(ctx, matchingDeployment)
}

// HandleBuildCreatedByID processes build creation events using just the build ID
func (o *DeploymentOrchestrator) HandleBuildCreatedByID(ctx context.Context, buildID pgtype.UUID) error {
	o.logger.Info("Processing build creation by ID", "build_id", buildID)

	// For build created events, we typically just log and potentially update tracking
	// The actual deployment happens when the build is completed
	o.logger.Info("Build started", "build_id", buildID)

	return nil
}

// HandleDeploymentUpdatedByID processes deployment update events using just the deployment ID
func (o *DeploymentOrchestrator) HandleDeploymentUpdatedByID(ctx context.Context, deploymentID pgtype.UUID) error {
	o.logger.Info("Processing deployment update by ID", "deployment_id", deploymentID)

	// Fetch the current deployment record
	deployment := lo.Must(o.db.DeploymentsGetById(ctx, deploymentID))

	switch deployment.Status {
	case "failed":
		return o.handleDeploymentFailed(ctx, deployment)
	case "cancelled":
		return o.handleDeploymentCancelled(ctx, deployment)
	default:
		o.logger.Debug("No specific handler for deployment status",
			"deployment_id", deploymentID,
			"status", deployment.Status)
	}

	return nil
}

// HandleDeploymentCreatedByID processes deployment creation events using just the deployment ID
func (o *DeploymentOrchestrator) HandleDeploymentCreatedByID(ctx context.Context, deploymentID pgtype.UUID) error {
	o.logger.Info("Processing deployment creation by ID", "deployment_id", deploymentID)

	// For deployment created events, we typically just log and track
	// The actual instance creation happens when the build completes
	o.logger.Info("Deployment created", "deployment_id", deploymentID)

	return nil
}

// HandleDeploymentChange processes deployment state changes (legacy method - can be removed)
func (o *DeploymentOrchestrator) HandleDeploymentChange(ctx context.Context, deploymentID string, status string) error {
	o.logger.Info("Processing deployment change", "deployment_id", deploymentID, "status", status)

	// Just delegate to the ID-based method
	deploymentUUID := uuid.MustParseUUID(deploymentID)
	return o.HandleDeploymentUpdatedByID(ctx, deploymentUUID)
}

// CreateInstancesForDeployment creates instances for a deployment
func (o *DeploymentOrchestrator) CreateInstancesForDeployment(ctx context.Context, deployment *database.DeploymentsGetReadyForDeploymentRow) error {
	o.logger.Info("Creating instances for deployment", "deployment_id", deployment.ID)

	// Get available nodes
	nodes := lo.Must(o.db.NodesGetAll(ctx))
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes available for deployment")
	}

	// Calculate instance count
	instanceCount := o.calculateInstanceCount(len(nodes))
	selectedNodes := lo.Slice(nodes, 0, instanceCount)

	o.logger.Info("Selected nodes for deployment",
		"deployment_id", deployment.ID,
		"instance_count", instanceCount,
		"available_nodes", len(nodes))

	// Create instances
	instanceIDs := lo.Map(selectedNodes, func(node *database.NodesGetAllRow, i int) pgtype.UUID {
		return o.createSingleInstance(ctx, deployment, node)
	})

	// Create deployment-instance relationships
	lo.ForEach(instanceIDs, func(instanceID pgtype.UUID, _ int) {
		o.createDeploymentInstanceRelation(ctx, deployment.ID, instanceID)
	})

	// Update deployment status to active
	lo.Must0(o.db.DeploymentsUpdateStatus(ctx, &database.DeploymentsUpdateStatusParams{
		ID:     deployment.ID,
		Status: "active",
	}))

	o.logger.Info("Successfully created instances for deployment",
		"deployment_id", deployment.ID,
		"instances_created", len(instanceIDs))

	return nil
}

// ProcessReadyDeployments processes all deployments ready for deployment
func (o *DeploymentOrchestrator) ProcessReadyDeployments(ctx context.Context) error {
	readyDeployments := lo.Must(o.db.DeploymentsGetReadyForDeployment(ctx))

	if len(readyDeployments) == 0 {
		o.logger.Debug("No deployments ready for deployment")
		return nil
	}

	o.logger.Info("Processing ready deployments", "count", len(readyDeployments))

	// Process each deployment, collecting any errors
	errors := lo.FilterMap(readyDeployments, func(deployment *database.DeploymentsGetReadyForDeploymentRow, _ int) (error, bool) {
		if err := o.CreateInstancesForDeployment(ctx, deployment); err != nil {
			o.logger.Error("Failed to create instances for deployment",
				"deployment_id", deployment.ID,
				"error", err)
			return err, true
		}
		return nil, false
	})

	if len(errors) > 0 {
		return fmt.Errorf("failed to process %d deployments: %v", len(errors), errors)
	}

	return nil
}

// Helper methods
func (o *DeploymentOrchestrator) calculateInstanceCount(availableNodes int) int {
	// TODO: Get instance count from deployment configuration or database
	// For now, use simple logic: 1 instance per node, max 3
	instanceCount := availableNodes
	if instanceCount > 3 {
		instanceCount = 3
	}
	if instanceCount < 1 {
		instanceCount = 1
	}

	o.logger.Info("Calculated instance count",
		"available_nodes", availableNodes,
		"instance_count", instanceCount)

	return instanceCount
}

func (o *DeploymentOrchestrator) createSingleInstance(ctx context.Context, deployment *database.DeploymentsGetReadyForDeploymentRow, node *database.NodesGetAllRow) pgtype.UUID {
	instanceUUID := uuid.GeneratePgUUID()
	imageUUID := uuid.GeneratePgUUID() // Mock image ID

	createParams := &database.InstancesCreateParams{
		ID:                   instanceUUID,
		RegionID:             node.RegionID,
		NodeID:               node.ID,
		ImageID:              imageUUID,
		State:                "pending",
		Vcpus:                1,
		Memory:               1024,
		DefaultPort:          8080,
		Ipv6Address:          "",
		EnvironmentVariables: "{}",
	}

	instance := lo.Must(o.db.InstancesCreate(ctx, createParams))

	o.logger.Info("Created instance",
		"instance_id", instance.ID,
		"node_id", instance.NodeID,
		"deployment_id", deployment.ID)

	return instance.ID
}

func (o *DeploymentOrchestrator) createDeploymentInstanceRelation(ctx context.Context, deploymentID, instanceID pgtype.UUID) {
	relationUUID := uuid.GeneratePgUUID()

	createParams := &database.DeploymentInstancesCreateParams{
		ID:           relationUUID,
		DeploymentID: deploymentID,
		InstanceID:   instanceID,
	}

	lo.Must(o.db.DeploymentInstancesCreate(ctx, createParams))

	o.logger.Info("Created deployment instance relationship",
		"deployment_id", deploymentID,
		"instance_id", instanceID)
}

func (o *DeploymentOrchestrator) handleDeploymentFailed(ctx context.Context, deployment *database.DeploymentsGetByIdRow) error {
	o.logger.Info("Handling failed deployment cleanup", "deployment_id", deployment.ID)

	instances := lo.Must(o.db.InstancesGetByDeployment(ctx, deployment.ID))

	// Mark all instances for termination
	lo.ForEach(instances, func(instance *database.InstancesGetByDeploymentRow, _ int) {
		lo.Must(o.db.InstancesUpdateState(ctx, &database.InstancesUpdateStateParams{
			ID:    instance.ID,
			State: "terminating",
		}))

		o.logger.Info("Marked instance for termination",
			"instance_id", instance.ID,
			"deployment_id", deployment.ID)
	})

	return nil
}

func (o *DeploymentOrchestrator) handleDeploymentCancelled(ctx context.Context, deployment *database.DeploymentsGetByIdRow) error {
	o.logger.Info("Handling cancelled deployment cleanup", "deployment_id", deployment.ID)
	return o.handleDeploymentFailed(ctx, deployment)
}
