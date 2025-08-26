package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
)

// DeploymentManager manages the deployment workflow
type DeploymentManager struct {
	service *Service
	mu      sync.RWMutex
	active  map[string]*DeploymentState
}

// DeploymentState tracks the state of a deployment
type DeploymentState struct {
	DeploymentID string
	ImageID      string
	Status       string
	Regions      []string
	Instances    map[string][]string // region -> instance IDs
	StartedAt    time.Time
	UpdatedAt    time.Time
}

// NewDeploymentManager creates a new deployment manager
func NewDeploymentManager(service *Service) *DeploymentManager {
	return &DeploymentManager{
		service: service,
		active:  make(map[string]*DeploymentState),
	}
}

// StartDeploymentWorkflow starts the deployment workflow for a deployment
func (dm *DeploymentManager) StartDeploymentWorkflow(ctx context.Context, deploymentID, imageID string) error {
	dm.service.logger.Info("Starting deployment workflow",
		"deploymentID", deploymentID,
		"imageID", imageID)

	// Create deployment state
	state := &DeploymentState{
		DeploymentID: deploymentID,
		ImageID:      imageID,
		Status:       "deploying",
		Regions:      []string{"eu-central-1", "us-east-1", "asia-southeast-1"},
		Instances:    make(map[string][]string),
		StartedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	dm.mu.Lock()
	dm.active[deploymentID] = state
	dm.mu.Unlock()

	// Update deployment status in database
	if err := dm.updateDeploymentStatus(ctx, deploymentID, "deploying"); err != nil {
		return fmt.Errorf("failed to update deployment status: %w", err)
	}

	// Start async deployment
	go dm.deployToAllRegions(deploymentID, state)

	return nil
}

// deployToAllRegions deploys to all regions
func (dm *DeploymentManager) deployToAllRegions(deploymentID string, state *DeploymentState) {
	ctx := context.Background()

	// Get deployment details from database
	db, err := database.NewDB(dm.service.config.DatabaseURL)
	if err != nil {
		dm.service.logger.Error("Failed to connect to database", "error", err)
		dm.failDeployment(ctx, deploymentID, "database connection failed")
		return
	}
	defer db.Close()

	deployment, err := db.Queries().DeploymentFindById(ctx, pgtype.UUID{
		Bytes: uuid.MustParse(deploymentID),
		Valid: true,
	})
	if err != nil {
		dm.service.logger.Error("Failed to find deployment", "error", err)
		dm.failDeployment(ctx, deploymentID, "deployment not found")
		return
	}

	// Deploy to each region
	var wg sync.WaitGroup
	successCount := 0
	var successMu sync.Mutex

	for _, region := range state.Regions {
		wg.Add(1)
		go func(region string) {
			defer wg.Done()

			// Deploy to region
			instances, err := dm.deployToRegion(ctx, deployment, state.ImageID, region)
			if err != nil {
				dm.service.logger.Error("Failed to deploy to region",
					"region", region,
					"error", err)
				return
			}

			// Store instance IDs
			dm.mu.Lock()
			state.Instances[region] = instances
			state.UpdatedAt = time.Now()
			dm.mu.Unlock()

			successMu.Lock()
			successCount++
			successMu.Unlock()

			dm.service.logger.Info("Deployed to region",
				"region", region,
				"instances", len(instances))
		}(region)
	}

	wg.Wait()

	// Check if deployment was successful
	if successCount == 0 {
		dm.failDeployment(ctx, deploymentID, "failed to deploy to any region")
		return
	}

	// Update DNS records for the deployment
	if err := dm.updateDNSRecords(ctx, deployment); err != nil {
		dm.service.logger.Error("Failed to update DNS records", "error", err)
		// Don't fail deployment for DNS issues
	}

	// Perform graceful rollover if there's an active deployment
	if err := dm.performGracefulRollover(ctx, deployment); err != nil {
		dm.service.logger.Error("Failed to perform graceful rollover", "error", err)
	}

	// Mark deployment as active
	if err := dm.activateDeployment(ctx, deploymentID); err != nil {
		dm.service.logger.Error("Failed to activate deployment", "error", err)
		return
	}

	dm.service.logger.Info("Deployment workflow completed",
		"deploymentID", deploymentID,
		"regions", successCount)
}

// deployToRegion deploys instances to a specific region
func (dm *DeploymentManager) deployToRegion(ctx context.Context, deployment *database.Deployment, imageID, region string) ([]string, error) {
	// Get nodes in the region
	nodes, err := dm.getNodesInRegion(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available in region %s", region)
	}

	// Calculate instances to deploy (minimum 3 per region)
	minInstances := 3
	if deployment.MinInstances > 0 {
		minInstances = int(deployment.MinInstances)
	}

	// Deploy instances
	var instances []string
	for i := 0; i < minInstances && i < len(nodes); i++ {
		node := nodes[i%len(nodes)]

		// Create instance in database
		db, _ := database.NewDB(dm.service.config.DatabaseURL)
		defer db.Close()

		instance, err := db.Queries().InstanceCreate(ctx, &database.InstanceCreateParams{
			RegionID:             node.RegionID,
			NodeID:               node.ID,
			ImageID:              pgtype.UUID{Bytes: uuid.MustParse(imageID), Valid: true},
			State:                "creating",
			Resources:            json.RawMessage(`{"vcpu": 1, "memory": 512}`),
			DefaultPort:          8080,
			IpAddress:            fmt.Sprintf("fc00::%x", i+1), // IPv6 address
			EnvironmentVariables: `{"PORT": "8080"}`,
		})
		if err != nil {
			dm.service.logger.Error("Failed to create instance", "error", err)
			continue
		}

		// Request node agent to start the instance
		instanceIDStr := uuid.UUID(instance.ID.Bytes).String()
		if err := dm.requestInstanceStart(node.IpAddress, instanceIDStr, imageID); err != nil {
			dm.service.logger.Error("Failed to start instance", "error", err)
			continue
		}

		instances = append(instances, instanceIDStr)

		// Create deployment-instance mapping
		_, err = db.Queries().DeploymentInstanceCreate(ctx, &database.DeploymentInstanceCreateParams{
			DeploymentID: deployment.ID,
			InstanceID:   instance.ID,
		})
		if err != nil {
			dm.service.logger.Error("Failed to create deployment-instance mapping", "error", err)
		}
	}

	return instances, nil
}

// getNodesInRegion gets available nodes in a region
func (dm *DeploymentManager) getNodesInRegion(ctx context.Context, regionCode string) ([]*database.Node, error) {
	db, err := database.NewDB(dm.service.config.DatabaseURL)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Get region
	region, err := db.Queries().RegionFindByCode(ctx, regionCode)
	if err != nil {
		return nil, fmt.Errorf("region not found: %s", regionCode)
	}

	// Get nodes in region
	nodes, err := db.Queries().NodeFindByRegion(ctx, region.ID)
	if err != nil {
		return nil, err
	}

	// Filter for ready worker nodes
	var workerNodes []*database.Node
	for _, node := range nodes {
		if node.State == "ready" && strings.Contains(node.Hostname, "worker") {
			workerNodes = append(workerNodes, node)
		}
	}

	return workerNodes, nil
}

// requestInstanceStart requests a node agent to start an instance
func (dm *DeploymentManager) requestInstanceStart(nodeIP, instanceID, imageID string) error {
	req := map[string]interface{}{
		"instance_id": instanceID,
		"image_id":    imageID,
		"resources": map[string]int{
			"vcpu":   1,
			"memory": 512,
		},
		"default_port": 8080,
	}

	body, _ := json.Marshal(req)
	url := fmt.Sprintf("http://%s:8081/api/v1/instances", nodeIP)

	resp, err := dm.service.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create instance: status %d", resp.StatusCode)
	}

	return nil
}

// updateDNSRecords updates DNS records for the deployment
func (dm *DeploymentManager) updateDNSRecords(ctx context.Context, deployment *database.Deployment) error {
	// In a real implementation, this would update Route53 or similar
	// For now, just update the routing cache in the database

	if !deployment.DeploymentUrl.Valid {
		return nil
	}

	domain := deployment.DeploymentUrl.String

	// Get all instances for this deployment
	db, err := database.NewDB(dm.service.config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	instances, err := db.Queries().InstanceFindByDeployment(ctx, deployment.ID)
	if err != nil {
		return err
	}

	// Extract IP addresses
	var ips []string
	for _, inst := range instances {
		if inst.IpAddress != "" {
			ips = append(ips, inst.IpAddress)
		}
	}

	if len(ips) == 0 {
		return fmt.Errorf("no instances with IP addresses found")
	}

	// Update routing cache
	instancesJSON, _ := json.Marshal(ips)
	_, err = db.Queries().RoutingCacheUpsert(ctx, &database.RoutingCacheUpsertParams{
		Domain:       domain,
		DeploymentID: deployment.ID,
		Instances:    instancesJSON,
	})

	return err
}

// performGracefulRollover performs a graceful rollover from old deployment
func (dm *DeploymentManager) performGracefulRollover(ctx context.Context, newDeployment *database.Deployment) error {
	db, err := database.NewDB(dm.service.config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	// Find previous active deployment for the same project
	deployments, err := db.Queries().DeploymentFindByProject(ctx, newDeployment.ProjectID)
	if err != nil {
		return err
	}

	for _, dep := range deployments {
		if dep.ID == newDeployment.ID {
			continue // Skip self
		}

		if dep.Status == "active" {
			// Deactivate old deployment
			dm.service.logger.Info("Deactivating old deployment", "deploymentID", dep.ID)

			// Update status
			_, err := db.Queries().DeploymentUpdateStatus(ctx, &database.DeploymentUpdateStatusParams{
				ID:          dep.ID,
				Status:      "inactive",
				ActivatedAt: pgtype.Timestamptz{}, // Clear activation time
			})
			if err != nil {
				dm.service.logger.Error("Failed to deactivate old deployment", "error", err)
			}

			// Schedule instance termination after grace period
			go dm.scheduleInstanceTermination(uuid.UUID(dep.ID.Bytes).String(), 5*time.Minute)
		}
	}

	return nil
}

// scheduleInstanceTermination schedules termination of instances after grace period
func (dm *DeploymentManager) scheduleInstanceTermination(deploymentID string, gracePeriod time.Duration) {
	time.Sleep(gracePeriod)

	ctx := context.Background()
	db, err := database.NewDB(dm.service.config.DatabaseURL)
	if err != nil {
		return
	}
	defer db.Close()

	// Get all instances for deployment
	instances, err := db.Queries().InstanceFindByDeployment(ctx, pgtype.UUID{
		Bytes: uuid.MustParse(deploymentID),
		Valid: true,
	})
	if err != nil {
		return
	}

	// Terminate each instance
	for _, inst := range instances {
		// Update instance state
		_, err := db.Queries().InstanceUpdateState(ctx, &database.InstanceUpdateStateParams{
			ID:    inst.ID,
			State: "terminated",
		})
		if err != nil {
			dm.service.logger.Error("Failed to terminate instance", "instanceID", inst.ID, "error", err)
		}

		// TODO: Send termination request to node agent
	}

	dm.service.logger.Info("Terminated instances for old deployment",
		"deploymentID", deploymentID,
		"count", len(instances))
}

// updateDeploymentStatus updates the deployment status in the database
func (dm *DeploymentManager) updateDeploymentStatus(ctx context.Context, deploymentID, status string) error {
	db, err := database.NewDB(dm.service.config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Queries().DeploymentUpdateStatus(ctx, &database.DeploymentUpdateStatusParams{
		ID:          pgtype.UUID{Bytes: uuid.MustParse(deploymentID), Valid: true},
		Status:      status,
		ActivatedAt: pgtype.Timestamptz{},
	})
	return err
}

// activateDeployment marks a deployment as active
func (dm *DeploymentManager) activateDeployment(ctx context.Context, deploymentID string) error {
	db, err := database.NewDB(dm.service.config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Queries().DeploymentUpdateStatus(ctx, &database.DeploymentUpdateStatusParams{
		ID:          pgtype.UUID{Bytes: uuid.MustParse(deploymentID), Valid: true},
		Status:      "active",
		ActivatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})

	dm.mu.Lock()
	if state, exists := dm.active[deploymentID]; exists {
		state.Status = "active"
		state.UpdatedAt = time.Now()
	}
	dm.mu.Unlock()

	return err
}

// failDeployment marks a deployment as failed
func (dm *DeploymentManager) failDeployment(ctx context.Context, deploymentID, reason string) {
	dm.service.logger.Error("Deployment failed",
		"deploymentID", deploymentID,
		"reason", reason)

	dm.updateDeploymentStatus(ctx, deploymentID, "failed")

	dm.mu.Lock()
	delete(dm.active, deploymentID)
	dm.mu.Unlock()
}

// GetDeploymentState returns the current state of a deployment
func (dm *DeploymentManager) GetDeploymentState(deploymentID string) (*DeploymentState, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	state, exists := dm.active[deploymentID]
	return state, exists
}
