package monitoring

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
)

// Reporter handles reporting node and instance status back to the database
type Reporter struct {
	logger         *slog.Logger
	nodeID         uuid.UUID
	db             *database.DB
	healthMonitor  *HealthMonitor
	statsCollector *StatsCollector

	// Reporting configuration
	reportInterval time.Duration
}

// NewReporter creates a new status reporter
func NewReporter(logger *slog.Logger, nodeID uuid.UUID, db *database.DB, healthMonitor *HealthMonitor, statsCollector *StatsCollector) *Reporter {
	return &Reporter{
		logger:         logger,
		nodeID:         nodeID,
		db:             db,
		healthMonitor:  healthMonitor,
		statsCollector: statsCollector,
		reportInterval: 60 * time.Second, // Report every minute
	}
}

// SetReportInterval sets the reporting interval
func (r *Reporter) SetReportInterval(interval time.Duration) {
	r.reportInterval = interval
}

// Start starts the status reporting loop
func (r *Reporter) Start(ctx context.Context) {
	r.logger.Info("Starting status reporting", "interval", r.reportInterval)

	ticker := time.NewTicker(r.reportInterval)
	defer ticker.Stop()

	// Initial report
	r.reportStatus(ctx)

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Stopping status reporting")
			return
		case <-ticker.C:
			r.reportStatus(ctx)
		}
	}
}

// reportStatus reports current status to the database
func (r *Reporter) reportStatus(ctx context.Context) {
	r.logger.Debug("Reporting status to database")

	// Report node status
	if err := r.reportNodeStatus(ctx); err != nil {
		r.logger.Error("Failed to report node status", "error", err)
	}

	// Report instance statuses
	if err := r.reportInstanceStatuses(ctx); err != nil {
		r.logger.Error("Failed to report instance statuses", "error", err)
	}

	r.logger.Debug("Status reporting completed")
}

// reportNodeStatus reports the current node status
func (r *Reporter) reportNodeStatus(ctx context.Context) error {
	// Get node statistics
	nodeStats := r.statsCollector.GetNodeStats()
	healthSummary := r.healthMonitor.GetHealthSummary()

	// Determine node state based on health
	nodeState := "healthy"
	if healthSummary.UnhealthyInstances > 0 {
		if healthSummary.HealthyInstances == 0 {
			nodeState = "unhealthy"
		} else {
			nodeState = "degraded"
		}
	}

	// Prepare node resources data (for future use)
	_ = map[string]interface{}{
		"total_instances":     nodeStats.TotalInstances,
		"running_instances":   nodeStats.RunningInstances,
		"healthy_instances":   healthSummary.HealthyInstances,
		"unhealthy_instances": healthSummary.UnhealthyInstances,
		"total_cpu_percent":   nodeStats.TotalCPUPercent,
		"total_memory_used":   nodeStats.TotalMemoryUsed,
		"total_memory_limit":  nodeStats.TotalMemoryLimit,
		"avg_cpu_percent":     nodeStats.AvgCPUPercent,
		"avg_memory_percent":  nodeStats.AvgMemoryPercent,
		"last_updated":        time.Now().Unix(),
	}

	// Convert to pgtype.UUID (for future use)
	_ = pgtype.UUID{
		Bytes: r.nodeID,
		Valid: true,
	}

	// Update node state in database
	// Note: This assumes there's a NodesUpdateState query - you might need to add it
	r.logger.Debug("Updating node state",
		"node_id", r.nodeID,
		"state", nodeState,
		"instances", nodeStats.TotalInstances)

	// For now, we'll log what we would update
	// In a full implementation, you'd add the appropriate database queries
	r.logger.Info("Node status",
		"node_id", r.nodeID,
		"state", nodeState,
		"total_instances", nodeStats.TotalInstances,
		"running_instances", nodeStats.RunningInstances,
		"healthy_instances", healthSummary.HealthyInstances)

	return nil
}

// reportInstanceStatuses reports the current status of all instances
func (r *Reporter) reportInstanceStatuses(ctx context.Context) error {
	// Get health information for all instances
	allHealth := r.healthMonitor.GetAllInstanceHealth()

	// Report status for each instance
	for instanceID, health := range allHealth {
		if err := r.reportInstanceStatus(ctx, instanceID, health); err != nil {
			r.logger.Error("Failed to report instance status",
				"instance_id", instanceID,
				"error", err)
			// Continue with other instances
		}
	}

	return nil
}

// reportInstanceStatus reports the status of a single instance
func (r *Reporter) reportInstanceStatus(ctx context.Context, instanceID string, health *InstanceHealthState) error {
	// Parse instance ID
	instanceUUID, err := uuid.Parse(instanceID)
	if err != nil {
		return fmt.Errorf("invalid instance ID: %w", err)
	}

	pgInstanceID := pgtype.UUID{
		Bytes: instanceUUID,
		Valid: true,
	}

	// Determine instance state based on health
	var instanceState string
	if health.IsHealthy {
		instanceState = "running"
	} else {
		instanceState = "failed"
	}

	// Check current state to avoid unnecessary updates
	currentInstance, err := r.db.Queries().InstancesGetById(ctx, pgInstanceID)
	if err != nil {
		r.logger.Warn("Failed to get current instance state",
			"instance_id", instanceID,
			"error", err)
		// Continue with update anyway
	} else if string(currentInstance.State) == instanceState {
		// State hasn't changed, skip update to avoid triggering events
		return nil
	}

	// Update instance state in database
	_, err = r.db.Queries().InstancesUpdateState(ctx, &database.InstancesUpdateStateParams{
		ID:    pgInstanceID,
		State: database.InstanceStatuses(instanceState),
	})

	if err != nil {
		return fmt.Errorf("failed to update instance state: %w", err)
	}

	r.logger.Debug("Instance status updated",
		"instance_id", instanceID,
		"state", instanceState,
		"healthy", health.IsHealthy,
		"consecutive_fails", health.ConsecutiveFails)

	return nil
}

// ReportInstanceStateChange reports an immediate instance state change
func (r *Reporter) ReportInstanceStateChange(ctx context.Context, instanceID, newState string) error {
	r.logger.Info("Reporting instance state change",
		"instance_id", instanceID,
		"new_state", newState)

	// Parse instance ID
	instanceUUID, err := uuid.Parse(instanceID)
	if err != nil {
		return fmt.Errorf("invalid instance ID: %w", err)
	}

	pgInstanceID := pgtype.UUID{
		Bytes: instanceUUID,
		Valid: true,
	}

	// Update instance state in database
	_, err = r.db.Queries().InstancesUpdateState(ctx, &database.InstancesUpdateStateParams{
		ID:    pgInstanceID,
		State: database.InstanceStatuses(newState),
	})

	if err != nil {
		return fmt.Errorf("failed to update instance state: %w", err)
	}

	r.logger.Info("Instance state change reported",
		"instance_id", instanceID,
		"state", newState)

	return nil
}

// GetNodeStatusSummary returns a summary of the current node status
func (r *Reporter) GetNodeStatusSummary() NodeStatusSummary {
	nodeStats := r.statsCollector.GetNodeStats()
	healthSummary := r.healthMonitor.GetHealthSummary()

	return NodeStatusSummary{
		NodeID:             r.nodeID.String(),
		Timestamp:          time.Now(),
		TotalInstances:     nodeStats.TotalInstances,
		RunningInstances:   nodeStats.RunningInstances,
		HealthyInstances:   healthSummary.HealthyInstances,
		UnhealthyInstances: healthSummary.UnhealthyInstances,
		TotalCPUPercent:    nodeStats.TotalCPUPercent,
		TotalMemoryUsed:    nodeStats.TotalMemoryUsed,
		TotalMemoryLimit:   nodeStats.TotalMemoryLimit,
		AvgCPUPercent:      nodeStats.AvgCPUPercent,
		AvgMemoryPercent:   nodeStats.AvgMemoryPercent,
	}
}

// NodeStatusSummary provides a comprehensive summary of node status
type NodeStatusSummary struct {
	NodeID             string    `json:"node_id"`
	Timestamp          time.Time `json:"timestamp"`
	TotalInstances     int       `json:"total_instances"`
	RunningInstances   int       `json:"running_instances"`
	HealthyInstances   int       `json:"healthy_instances"`
	UnhealthyInstances int       `json:"unhealthy_instances"`
	TotalCPUPercent    float64   `json:"total_cpu_percent"`
	TotalMemoryUsed    uint64    `json:"total_memory_used"`
	TotalMemoryLimit   uint64    `json:"total_memory_limit"`
	AvgCPUPercent      float64   `json:"avg_cpu_percent"`
	AvgMemoryPercent   float64   `json:"avg_memory_percent"`
}
