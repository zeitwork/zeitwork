package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samber/lo"

	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

// DeploymentReconciler handles periodic reconciliation of deployment state
type DeploymentReconciler struct {
	queries *database.Queries
	db      *pgxpool.Pool
	logger  *slog.Logger
	ticker  *time.Ticker
	stopCh  chan struct{}

	// Synchronization for graceful shutdown
	mu      sync.Mutex
	stopped bool
	wg      sync.WaitGroup
}

// NewDeploymentReconciler creates a new deployment reconciler
func NewDeploymentReconciler(queries *database.Queries, db *pgxpool.Pool, logger *slog.Logger) *DeploymentReconciler {
	return &DeploymentReconciler{
		queries: queries,
		db:      db,
		logger:  logger.With("component", "deployment_reconciler"),
		stopCh:  make(chan struct{}),
	}
}

// Start starts the reconciler loop
func (r *DeploymentReconciler) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stopped {
		return fmt.Errorf("reconciler has been stopped and cannot be restarted")
	}

	r.logger.Info("Starting deployment reconciler")

	// Run initial reconciliation
	if err := r.reconcileDeployments(ctx); err != nil {
		r.logger.Error("Initial reconciliation failed", "error", err)
	}

	// Start periodic reconciliation
	r.ticker = time.NewTicker(30 * time.Second)
	r.wg.Add(1)
	go r.reconcileLoop(ctx)

	r.logger.Info("Deployment reconciler started")
	return nil
}

// Stop stops the reconciler gracefully
func (r *DeploymentReconciler) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stopped {
		r.logger.Debug("Reconciler already stopped")
		return
	}

	r.logger.Info("Stopping deployment reconciler")
	r.stopped = true

	// Stop the ticker first to prevent new cycles
	if r.ticker != nil {
		r.ticker.Stop()
		r.logger.Debug("Ticker stopped")
	}

	// Close the stop channel to signal the goroutine to exit
	close(r.stopCh)

	// Wait for the reconcile loop goroutine to finish
	r.logger.Debug("Waiting for reconciler goroutine to finish")
	r.wg.Wait()

	r.logger.Info("Deployment reconciler stopped")
}

// reconcileLoop runs the periodic reconciliation
func (r *DeploymentReconciler) reconcileLoop(ctx context.Context) {
	defer r.wg.Done()
	r.logger.Debug("Reconciler loop started")

	for {
		select {
		case <-r.ticker.C:
			if err := r.reconcileDeployments(ctx); err != nil {
				r.logger.Error("Reconciliation cycle failed", "error", err)
			}
		case <-r.stopCh:
			r.logger.Debug("Reconciler loop stopped")
			return
		case <-ctx.Done():
			r.logger.Debug("Reconciler loop cancelled")
			return
		}
	}
}

// reconcileDeployments performs a full reconciliation cycle
func (r *DeploymentReconciler) reconcileDeployments(ctx context.Context) error {
	r.logger.Debug("Starting reconciliation cycle")

	// Find deployments ready for instance creation
	readyDeployments, err := r.queries.DeploymentsGetReadyForDeployment(ctx)
	if err != nil {
		return fmt.Errorf("failed to get ready deployments: %w", err)
	}

	if len(readyDeployments) == 0 {
		r.logger.Debug("No deployments ready for reconciliation")
		return nil
	}

	r.logger.Info("Found deployments ready for reconciliation", "count", len(readyDeployments))

	// Process each deployment
	var errors []error
	for _, deployment := range readyDeployments {
		if err := r.reconcileDeployment(ctx, deployment); err != nil {
			r.logger.Error("Failed to reconcile deployment",
				"deployment_id", deployment.ID,
				"error", err)
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("reconciliation completed with %d errors: %v", len(errors), errors)
	}

	r.logger.Debug("Reconciliation cycle completed successfully")
	return nil
}

// reconcileDeployment reconciles a single deployment
func (r *DeploymentReconciler) reconcileDeployment(ctx context.Context, deployment *database.DeploymentsGetReadyForDeploymentRow) error {
	r.logger.Info("Reconciling deployment",
		"deployment_id", deployment.ID,
		"status", deployment.Status)

	// Check if deployment already has instances
	existingInstances, err := r.queries.InstancesGetByDeployment(ctx, deployment.ID)
	if err != nil {
		return fmt.Errorf("failed to get existing instances: %w", err)
	}

	if len(existingInstances) > 0 {
		r.logger.Debug("Deployment already has instances, checking if reconciliation needed",
			"deployment_id", deployment.ID,
			"instance_count", len(existingInstances))
		return r.reconcileExistingDeployment(ctx, deployment, existingInstances)
	}

	// Create new instances for deployment
	return r.createInstancesForDeployment(ctx, deployment)
}

// reconcileExistingDeployment handles deployments that already have instances
func (r *DeploymentReconciler) reconcileExistingDeployment(ctx context.Context, deployment *database.DeploymentsGetReadyForDeploymentRow, instances []*database.InstancesGetByDeploymentRow) error {
	r.logger.Debug("Reconciling existing deployment",
		"deployment_id", deployment.ID,
		"instance_count", len(instances))

	// For now, just ensure deployment status is correct
	// Future: Check instance health, scale up/down, etc.

	if deployment.Status != "active" {
		r.logger.Info("Updating deployment status to active",
			"deployment_id", deployment.ID,
			"current_status", deployment.Status)

		_, err := r.queries.DeploymentsUpdateStatus(ctx, &database.DeploymentsUpdateStatusParams{
			ID:     deployment.ID,
			Status: "active",
		})
		if err != nil {
			return fmt.Errorf("failed to update deployment status: %w", err)
		}
	}

	return nil
}

// createInstancesForDeployment creates instances for a deployment
func (r *DeploymentReconciler) createInstancesForDeployment(ctx context.Context, deployment *database.DeploymentsGetReadyForDeploymentRow) error {
	r.logger.Info("Creating instances for deployment", "deployment_id", deployment.ID)

	// Get available nodes
	allNodes, err := r.queries.NodesGetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	// Filter for ready nodes
	readyNodes := lo.Filter(allNodes, func(node *database.NodesGetAllRow, _ int) bool {
		return node.State == "ready"
	})

	if len(readyNodes) == 0 {
		return fmt.Errorf("no ready nodes available for deployment")
	}

	// Calculate instance count and select nodes
	instanceCount := r.calculateInstanceCount(len(readyNodes))
	selectedNodes := r.selectNodes(readyNodes, instanceCount)

	r.logger.Info("Selected nodes for deployment",
		"deployment_id", deployment.ID,
		"instance_count", instanceCount,
		"available_nodes", len(readyNodes),
		"selected_nodes", len(selectedNodes))

	// Create instances in a transaction
	return r.createInstancesTransaction(ctx, deployment, selectedNodes)
}

// calculateInstanceCount determines how many instances to create
func (r *DeploymentReconciler) calculateInstanceCount(availableNodes int) int {
	// Default: 3 instances for high availability
	desiredCount := 3

	// Scale down if not enough nodes
	if availableNodes < desiredCount {
		desiredCount = availableNodes
	}

	// Minimum 1 instance
	if desiredCount < 1 {
		desiredCount = 1
	}

	r.logger.Debug("Calculated instance count",
		"available_nodes", availableNodes,
		"desired_count", desiredCount)

	return desiredCount
}

// selectNodes selects nodes for instance placement with multi-region distribution
func (r *DeploymentReconciler) selectNodes(nodes []*database.NodesGetAllRow, instanceCount int) []*database.NodesGetAllRow {
	if instanceCount >= len(nodes) {
		return nodes
	}

	// Group nodes by region
	nodesByRegion := lo.GroupBy(nodes, func(node *database.NodesGetAllRow) pgtype.UUID {
		return node.RegionID
	})

	// Distribute instances across regions using round-robin
	return r.distributeAcrossRegions(nodesByRegion, instanceCount)
}

// distributeAcrossRegions distributes instances across regions for better availability
func (r *DeploymentReconciler) distributeAcrossRegions(nodesByRegion map[pgtype.UUID][]*database.NodesGetAllRow, instanceCount int) []*database.NodesGetAllRow {
	regions := lo.Keys(nodesByRegion)
	selectedNodes := make([]*database.NodesGetAllRow, 0, instanceCount)

	r.logger.Debug("Distributing instances across regions",
		"region_count", len(regions),
		"instance_count", instanceCount)

	// Round-robin selection across regions
	regionIndex := 0
	for len(selectedNodes) < instanceCount {
		region := regions[regionIndex%len(regions)]
		regionNodes := nodesByRegion[region]

		if len(regionNodes) > 0 {
			// Take first available node from region
			selectedNode := regionNodes[0]
			selectedNodes = append(selectedNodes, selectedNode)

			// Remove selected node to avoid duplicates
			nodesByRegion[region] = regionNodes[1:]

			r.logger.Debug("Selected node from region",
				"region_id", region,
				"node_id", selectedNode.ID,
				"hostname", selectedNode.Hostname)
		}

		regionIndex++

		// Safety check: if no more nodes available, break
		totalAvailable := lo.SumBy(lo.Values(nodesByRegion), func(regionNodes []*database.NodesGetAllRow) int {
			return len(regionNodes)
		})
		if totalAvailable == 0 {
			break
		}
	}

	r.logger.Info("Node selection completed",
		"selected_count", len(selectedNodes),
		"requested_count", instanceCount)

	return selectedNodes
}

// createInstancesTransaction creates instances and relationships in a single transaction
func (r *DeploymentReconciler) createInstancesTransaction(ctx context.Context, deployment *database.DeploymentsGetReadyForDeploymentRow, nodes []*database.NodesGetAllRow) error {
	r.logger.Debug("Creating instances in transaction",
		"deployment_id", deployment.ID,
		"node_count", len(nodes))

	// Use a transaction to ensure consistency
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := r.queries.WithTx(tx)
	instanceIDs := make([]pgtype.UUID, 0, len(nodes))

	// Create instances
	for i, node := range nodes {
		instanceID, err := r.createSingleInstance(ctx, txQueries, deployment, node, i)
		if err != nil {
			return fmt.Errorf("failed to create instance on node %s: %w", node.Hostname, err)
		}
		instanceIDs = append(instanceIDs, instanceID)
	}

	// Create deployment-instance relationships
	for _, instanceID := range instanceIDs {
		if err := r.createDeploymentInstanceRelation(ctx, txQueries, deployment.ID, instanceID, deployment.OrganisationID); err != nil {
			return fmt.Errorf("failed to create deployment-instance relation: %w", err)
		}
	}

	// Update deployment status to active
	_, err = txQueries.DeploymentsUpdateStatus(ctx, &database.DeploymentsUpdateStatusParams{
		ID:     deployment.ID,
		Status: "active",
	})
	if err != nil {
		return fmt.Errorf("failed to update deployment status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Info("Successfully created instances for deployment",
		"deployment_id", deployment.ID,
		"instances_created", len(instanceIDs))

	return nil
}

// createSingleInstance creates a single VM instance
func (r *DeploymentReconciler) createSingleInstance(ctx context.Context, queries *database.Queries, deployment *database.DeploymentsGetReadyForDeploymentRow, node *database.NodesGetAllRow, instanceIndex int) (pgtype.UUID, error) {
	instanceUUID := uuid.GeneratePgUUID()

	// TODO: Get actual image ID from deployment
	// For now, use the image_id from deployment if available, otherwise generate one
	imageID := deployment.ImageID
	if !imageID.Valid {
		imageID = uuid.GeneratePgUUID()
	}

	// Generate IPv6 address in format: fd00:00:regionid:nodeid:vmid/64
	ipv6Address, err := r.generateIPv6Address(node.RegionID, node.ID, instanceUUID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("failed to generate IPv6 address: %w", err)
	}

	createParams := &database.InstancesCreateParams{
		ID:                   instanceUUID,
		RegionID:             node.RegionID,
		NodeID:               node.ID,
		ImageID:              imageID,
		State:                "pending",
		Vcpus:                2,    // Standard allocation as per architecture
		Memory:               2048, // 2GB RAM as per architecture
		DefaultPort:          8080,
		IpAddress:            ipv6Address,
		EnvironmentVariables: "{}",
	}

	instance, err := queries.InstancesCreate(ctx, createParams)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("failed to create instance: %w", err)
	}

	r.logger.Info("Created instance",
		"instance_id", instance.ID,
		"node_id", instance.NodeID,
		"region_id", instance.RegionID,
		"deployment_id", deployment.ID)

	return instance.ID, nil
}

// createDeploymentInstanceRelation creates the deployment-instance relationship
func (r *DeploymentReconciler) createDeploymentInstanceRelation(ctx context.Context, queries *database.Queries, deploymentID, instanceID, organisationID pgtype.UUID) error {
	relationUUID := uuid.GeneratePgUUID()

	createParams := &database.DeploymentInstancesCreateParams{
		ID:             relationUUID,
		DeploymentID:   deploymentID,
		InstanceID:     instanceID,
		OrganisationID: organisationID,
	}

	_, err := queries.DeploymentInstancesCreate(ctx, createParams)
	if err != nil {
		return fmt.Errorf("failed to create deployment-instance relation: %w", err)
	}

	r.logger.Debug("Created deployment instance relationship",
		"deployment_id", deploymentID,
		"instance_id", instanceID,
		"organisation_id", organisationID)

	return nil
}

// generateIPv6Address generates an IPv6 address in the format fd00:00:regionid:nodeid:vmid/64
func (r *DeploymentReconciler) generateIPv6Address(regionID, nodeID, instanceID pgtype.UUID) (string, error) {
	// Validate UUIDs are valid
	if !regionID.Valid || !nodeID.Valid || !instanceID.Valid {
		return "", fmt.Errorf("invalid UUID provided for IPv6 generation")
	}

	// Extract first 2 bytes from each UUID for the IPv6 address
	// Format as fd00:00:regionid:nodeid:vmid/64
	ipv6Address := fmt.Sprintf("fd00:00:%02x%02x:%02x%02x:%02x%02x/64",
		regionID.Bytes[0], regionID.Bytes[1],
		nodeID.Bytes[0], nodeID.Bytes[1],
		instanceID.Bytes[0], instanceID.Bytes[1])

	r.logger.Debug("Generated IPv6 address",
		"region_id", regionID,
		"node_id", nodeID,
		"instance_id", instanceID,
		"ipv6_address", ipv6Address)

	return ipv6Address, nil
}
