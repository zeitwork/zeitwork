package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zeitwork/zeitwork/internal/database"
)

// ScalingManager manages instance scaling for deployments
type ScalingManager struct {
	service *Service
	logger  *slog.Logger
	mu      sync.RWMutex

	// Scaling policies by deployment ID
	policies map[string]*ScalingPolicy

	// Track scaling operations
	activeScaling map[string]time.Time
}

// ScalingPolicy defines scaling rules for a deployment
type ScalingPolicy struct {
	DeploymentID       string
	MinInstances       int
	MaxInstances       int
	TargetCPU          float64 // Target CPU utilization percentage
	ScaleUpThreshold   float64 // CPU % to trigger scale up
	ScaleDownThreshold float64 // CPU % to trigger scale down
	CooldownPeriod     time.Duration
	LastScaleTime      time.Time
}

// InstanceMetrics holds metrics for an instance
type InstanceMetrics struct {
	InstanceID   string
	CPUUsage     float64
	MemoryUsage  float64
	RequestCount int64
	Healthy      bool
}

// NewScalingManager creates a new scaling manager
func NewScalingManager(service *Service, logger *slog.Logger) *ScalingManager {
	return &ScalingManager{
		service:       service,
		logger:        logger,
		policies:      make(map[string]*ScalingPolicy),
		activeScaling: make(map[string]time.Time),
	}
}

// Start starts the scaling manager
func (sm *ScalingManager) Start(ctx context.Context) {
	sm.logger.Info("Starting scaling manager")

	// Start monitoring loop
	go sm.monitoringLoop(ctx)

	// Start health check loop
	go sm.healthCheckLoop(ctx)
}

// monitoringLoop continuously monitors deployments and scales as needed
func (sm *ScalingManager) monitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sm.checkAndScale(ctx)
		}
	}
}

// checkAndScale checks all deployments and scales as needed
func (sm *ScalingManager) checkAndScale(ctx context.Context) {
	db, err := database.NewDB(sm.service.config.DatabaseURL)
	if err != nil {
		sm.logger.Error("Failed to connect to database", "error", err)
		return
	}
	defer db.Close()

	// Get all active deployments
	deployments, err := db.Queries().DeploymentFindByStatus(ctx, "active")
	if err != nil {
		sm.logger.Error("Failed to get active deployments", "error", err)
		return
	}

	for _, deployment := range deployments {
		deploymentID := uuid.UUID(deployment.ID.Bytes).String()

		// Get or create scaling policy
		policy := sm.getOrCreatePolicy(deploymentID, deployment.MinInstances)

		// Check instance count
		instances, err := db.Queries().InstanceFindByDeployment(ctx, deployment.ID)
		if err != nil {
			sm.logger.Error("Failed to get instances", "deploymentID", deploymentID, "error", err)
			continue
		}

		// Count healthy instances per region
		regionCounts := make(map[string]int)
		healthyInstances := 0
		for _, inst := range instances {
			if inst.State == "running" {
				healthyInstances++
				// Get region from instance
				if inst.RegionID.Valid {
					region, err := db.Queries().RegionFindById(ctx, inst.RegionID)
					if err == nil {
						regionCounts[region.Code]++
					}
				}
			}
		}

		// Check minimum instances per region
		minPerRegion := policy.MinInstances / 3 // Distribute across 3 regions
		if minPerRegion < 1 {
			minPerRegion = 1
		}

		regionsToScale := []string{"eu-central-1", "us-east-1", "asia-southeast-1"}
		for _, region := range regionsToScale {
			currentCount := regionCounts[region]
			if currentCount < minPerRegion {
				sm.logger.Info("Scaling up instances in region",
					"deploymentID", deploymentID,
					"region", region,
					"current", currentCount,
					"target", minPerRegion)

				// Scale up
				toAdd := minPerRegion - currentCount
				for i := 0; i < toAdd; i++ {
					if err := sm.createInstance(ctx, deployment, region); err != nil {
						sm.logger.Error("Failed to create instance",
							"deploymentID", deploymentID,
							"region", region,
							"error", err)
						break
					}
				}
			}
		}

		// Check for load-based scaling
		if healthyInstances < int(policy.MinInstances) {
			// Scale up to minimum
			toAdd := int(policy.MinInstances) - healthyInstances
			sm.logger.Info("Scaling up to minimum instances",
				"deploymentID", deploymentID,
				"current", healthyInstances,
				"target", policy.MinInstances)

			for i := 0; i < toAdd; i++ {
				// Pick region with least instances
				minRegion := sm.getRegionWithLeastInstances(regionCounts, regionsToScale)
				if err := sm.createInstance(ctx, deployment, minRegion); err != nil {
					sm.logger.Error("Failed to create instance", "error", err)
					break
				}
				regionCounts[minRegion]++
			}
		} else if healthyInstances > int(policy.MaxInstances) && policy.MaxInstances > 0 {
			// Scale down to maximum
			toRemove := healthyInstances - int(policy.MaxInstances)
			sm.logger.Info("Scaling down to maximum instances",
				"deploymentID", deploymentID,
				"current", healthyInstances,
				"target", policy.MaxInstances)

			// Remove oldest instances first
			for i := 0; i < toRemove && i < len(instances); i++ {
				inst := instances[i]
				if inst.State == "running" {
					if err := sm.terminateInstance(ctx, inst); err != nil {
						sm.logger.Error("Failed to terminate instance", "error", err)
						break
					}
				}
			}
		}

		// Check CPU-based scaling if we have metrics
		metrics := sm.getInstanceMetrics(ctx, instances)
		if len(metrics) > 0 {
			avgCPU := sm.calculateAverageCPU(metrics)

			// Check if we need to scale based on CPU
			if avgCPU > policy.ScaleUpThreshold && sm.canScale(deploymentID, policy) {
				// Scale up
				sm.logger.Info("Scaling up based on CPU usage",
					"deploymentID", deploymentID,
					"avgCPU", avgCPU,
					"threshold", policy.ScaleUpThreshold)

				minRegion := sm.getRegionWithLeastInstances(regionCounts, regionsToScale)
				if err := sm.createInstance(ctx, deployment, minRegion); err != nil {
					sm.logger.Error("Failed to create instance", "error", err)
				} else {
					sm.recordScaling(deploymentID, policy)
				}
			} else if avgCPU < policy.ScaleDownThreshold && healthyInstances > int(policy.MinInstances) && sm.canScale(deploymentID, policy) {
				// Scale down
				sm.logger.Info("Scaling down based on CPU usage",
					"deploymentID", deploymentID,
					"avgCPU", avgCPU,
					"threshold", policy.ScaleDownThreshold)

				// Find instance to terminate (prefer least loaded)
				if inst := sm.findLeastLoadedInstance(instances, metrics); inst != nil {
					if err := sm.terminateInstance(ctx, inst); err != nil {
						sm.logger.Error("Failed to terminate instance", "error", err)
					} else {
						sm.recordScaling(deploymentID, policy)
					}
				}
			}
		}
	}
}

// createInstance creates a new instance for a deployment in a specific region
func (sm *ScalingManager) createInstance(ctx context.Context, deployment *database.Deployment, regionCode string) error {
	db, err := database.NewDB(sm.service.config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	// Get region
	region, err := db.Queries().RegionFindByCode(ctx, regionCode)
	if err != nil {
		return fmt.Errorf("region not found: %s", regionCode)
	}

	// Find available node in region
	nodes, err := db.Queries().NodeFindByRegion(ctx, region.ID)
	if err != nil {
		return err
	}

	var selectedNode *database.Node
	for _, node := range nodes {
		if node.State == "ready" {
			selectedNode = node
			break
		}
	}

	if selectedNode == nil {
		return fmt.Errorf("no available nodes in region %s", regionCode)
	}

	// Create instance
	instance, err := db.Queries().InstanceCreate(ctx, &database.InstanceCreateParams{
		RegionID:             region.ID,
		NodeID:               selectedNode.ID,
		ImageID:              deployment.ImageID,
		State:                "creating",
		Resources:            json.RawMessage(`{"vcpu": 1, "memory": 512}`),
		DefaultPort:          8080,
		IpAddress:            fmt.Sprintf("fc00::%x", time.Now().Unix()),
		EnvironmentVariables: `{"PORT": "8080"}`,
	})

	if err != nil {
		return fmt.Errorf("failed to create instance: %w", err)
	}

	// Create deployment-instance mapping
	_, err = db.Queries().DeploymentInstanceCreate(ctx, &database.DeploymentInstanceCreateParams{
		DeploymentID: deployment.ID,
		InstanceID:   instance.ID,
	})

	if err != nil {
		return fmt.Errorf("failed to create deployment-instance mapping: %w", err)
	}

	sm.logger.Info("Created new instance",
		"deploymentID", uuid.UUID(deployment.ID.Bytes).String(),
		"instanceID", uuid.UUID(instance.ID.Bytes).String(),
		"region", regionCode,
		"node", selectedNode.Hostname)

	// TODO: Send request to node agent to start the instance

	return nil
}

// terminateInstance terminates an instance
func (sm *ScalingManager) terminateInstance(ctx context.Context, instance *database.Instance) error {
	db, err := database.NewDB(sm.service.config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	// Update instance state
	_, err = db.Queries().InstanceUpdateState(ctx, &database.InstanceUpdateStateParams{
		ID:    instance.ID,
		State: "terminating",
	})

	if err != nil {
		return fmt.Errorf("failed to update instance state: %w", err)
	}

	sm.logger.Info("Terminating instance",
		"instanceID", uuid.UUID(instance.ID.Bytes).String())

	// TODO: Send termination request to node agent

	// Schedule deletion after grace period
	go func() {
		time.Sleep(30 * time.Second)
		db.Queries().InstanceDelete(context.Background(), instance.ID)
	}()

	return nil
}

// healthCheckLoop monitors instance health
func (sm *ScalingManager) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sm.checkInstanceHealth(ctx)
		}
	}
}

// checkInstanceHealth checks the health of all instances
func (sm *ScalingManager) checkInstanceHealth(ctx context.Context) {
	db, err := database.NewDB(sm.service.config.DatabaseURL)
	if err != nil {
		sm.logger.Error("Failed to connect to database", "error", err)
		return
	}
	defer db.Close()

	// Get all running instances
	instances, err := db.Queries().InstanceFindByState(ctx, "running")
	if err != nil {
		sm.logger.Error("Failed to get instances", "error", err)
		return
	}

	for _, instance := range instances {
		// TODO: Implement actual health check (HTTP request to instance)
		// For now, mark as healthy based on last update time
		if instance.UpdatedAt.Time.Before(time.Now().Add(-5 * time.Minute)) {
			// Instance hasn't been updated in 5 minutes, might be unhealthy
			sm.logger.Warn("Instance may be unhealthy",
				"instanceID", uuid.UUID(instance.ID.Bytes).String(),
				"lastUpdate", instance.UpdatedAt.Time)

			// Replace unhealthy instance
			// TODO: Get deployment ID and create replacement
		}
	}
}

// getOrCreatePolicy gets or creates a scaling policy for a deployment
func (sm *ScalingManager) getOrCreatePolicy(deploymentID string, minInstances int32) *ScalingPolicy {
	sm.mu.RLock()
	policy, exists := sm.policies[deploymentID]
	sm.mu.RUnlock()

	if !exists {
		if minInstances < 3 {
			minInstances = 3 // Minimum 3 for HA
		}

		policy = &ScalingPolicy{
			DeploymentID:       deploymentID,
			MinInstances:       int(minInstances),
			MaxInstances:       int(minInstances * 3), // 3x for max
			TargetCPU:          70.0,
			ScaleUpThreshold:   80.0,
			ScaleDownThreshold: 30.0,
			CooldownPeriod:     5 * time.Minute,
		}

		sm.mu.Lock()
		sm.policies[deploymentID] = policy
		sm.mu.Unlock()
	}

	return policy
}

// getRegionWithLeastInstances finds the region with the least instances
func (sm *ScalingManager) getRegionWithLeastInstances(counts map[string]int, regions []string) string {
	minRegion := regions[0]
	minCount := counts[minRegion]

	for _, region := range regions[1:] {
		if counts[region] < minCount {
			minRegion = region
			minCount = counts[region]
		}
	}

	return minRegion
}

// canScale checks if we can scale (cooldown period)
func (sm *ScalingManager) canScale(deploymentID string, policy *ScalingPolicy) bool {
	return time.Since(policy.LastScaleTime) > policy.CooldownPeriod
}

// recordScaling records a scaling event
func (sm *ScalingManager) recordScaling(deploymentID string, policy *ScalingPolicy) {
	policy.LastScaleTime = time.Now()
	sm.mu.Lock()
	sm.activeScaling[deploymentID] = time.Now()
	sm.mu.Unlock()
}

// getInstanceMetrics gets metrics for instances (placeholder)
func (sm *ScalingManager) getInstanceMetrics(ctx context.Context, instances []*database.Instance) []InstanceMetrics {
	// TODO: Implement actual metrics collection from monitoring system
	var metrics []InstanceMetrics
	for _, inst := range instances {
		metrics = append(metrics, InstanceMetrics{
			InstanceID:   uuid.UUID(inst.ID.Bytes).String(),
			CPUUsage:     50.0, // Placeholder
			MemoryUsage:  60.0, // Placeholder
			RequestCount: 100,  // Placeholder
			Healthy:      inst.State == "running",
		})
	}
	return metrics
}

// calculateAverageCPU calculates average CPU usage across instances
func (sm *ScalingManager) calculateAverageCPU(metrics []InstanceMetrics) float64 {
	if len(metrics) == 0 {
		return 0
	}

	var total float64
	for _, m := range metrics {
		total += m.CPUUsage
	}
	return total / float64(len(metrics))
}

// findLeastLoadedInstance finds the instance with lowest load
func (sm *ScalingManager) findLeastLoadedInstance(instances []*database.Instance, metrics []InstanceMetrics) *database.Instance {
	if len(instances) == 0 || len(metrics) == 0 {
		return nil
	}

	minLoad := metrics[0].CPUUsage
	minIndex := 0

	for i, m := range metrics[1:] {
		if m.CPUUsage < minLoad {
			minLoad = m.CPUUsage
			minIndex = i + 1
		}
	}

	if minIndex < len(instances) {
		return instances[minIndex]
	}
	return nil
}
