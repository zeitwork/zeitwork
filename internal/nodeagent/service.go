package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
	"github.com/zeitwork/zeitwork/internal/nodeagent/events"
	"github.com/zeitwork/zeitwork/internal/nodeagent/monitoring"
	nodeagentRuntime "github.com/zeitwork/zeitwork/internal/nodeagent/runtime"
	"github.com/zeitwork/zeitwork/internal/nodeagent/state"
	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
	sharedConfig "github.com/zeitwork/zeitwork/internal/shared/config"
	sharedNats "github.com/zeitwork/zeitwork/internal/shared/nats"
)

// Service represents the node agent service
type Service struct {
	logger *slog.Logger
	config *config.Config
	nodeID uuid.UUID

	// Core components
	db              *database.DB
	natsClient      *sharedNats.Client
	runtime         types.Runtime
	stateManager    *state.Manager
	eventSubscriber *events.Subscriber

	// Monitoring components
	healthMonitor  *monitoring.HealthMonitor
	statsCollector *monitoring.StatsCollector
	reporter       *monitoring.Reporter
}

// NewService creates a new node agent service
func NewService(cfg *config.Config, logger *slog.Logger) (*Service, error) {
	// Parse node ID
	nodeID, err := uuid.Parse(cfg.NodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid node ID: %w", err)
	}

	// Connect to database
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize NATS client
	natsConfig, err := sharedConfig.LoadNATSConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load NATS config: %w", err)
	}

	natsClient, err := sharedNats.NewClient(natsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Initialize runtime
	rt, err := nodeagentRuntime.NewRuntime(cfg.Runtime, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize runtime: %w", err)
	}

	// Initialize state manager
	stateManager := state.NewManager(logger, nodeID, db, rt)

	// Initialize event handlers
	instanceCreatedHandler := events.NewInstanceCreatedHandler(stateManager, logger)
	instanceUpdatedHandler := events.NewInstanceUpdatedHandler(stateManager, logger)
	nodeUpdatedHandler := events.NewNodeUpdatedHandler(stateManager, logger)

	// Initialize event subscriber
	eventSubscriber := events.NewSubscriber(logger, nodeID, natsClient,
		instanceCreatedHandler, instanceUpdatedHandler, nodeUpdatedHandler)

	// Initialize monitoring components
	healthMonitor := monitoring.NewHealthMonitor(logger, rt)
	statsCollector := monitoring.NewStatsCollector(logger, rt)
	reporter := monitoring.NewReporter(logger, nodeID, db, healthMonitor, statsCollector)

	service := &Service{
		logger:          logger,
		config:          cfg,
		nodeID:          nodeID,
		db:              db,
		natsClient:      natsClient,
		runtime:         rt,
		stateManager:    stateManager,
		eventSubscriber: eventSubscriber,
		healthMonitor:   healthMonitor,
		statsCollector:  statsCollector,
		reporter:        reporter,
	}

	// Setup health monitoring callbacks
	service.setupHealthMonitoring()

	return service, nil
}

// Start starts the node agent service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting node agent service",
		"node_id", s.nodeID,
		"runtime_mode", s.config.Runtime.Mode)

	// Ensure node is registered in database
	if err := s.ensureNodeRegistration(ctx); err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}

	// Load initial state
	s.logger.Info("Loading initial state")
	if err := s.stateManager.LoadInitialState(ctx); err != nil {
		return fmt.Errorf("failed to load initial state: %w", err)
	}

	// Perform initial reconciliation
	s.logger.Info("Performing initial reconciliation")
	if err := s.stateManager.Reconcile(ctx); err != nil {
		s.logger.Error("Initial reconciliation failed", "error", err)
		// Continue anyway - we'll retry via events and periodic reconciliation
	}

	// Start all components
	var wg sync.WaitGroup

	// Start NATS event subscriber
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.eventSubscriber.Subscribe(ctx); err != nil {
			s.logger.Error("Event subscriber failed", "error", err)
		}
	}()

	// Start periodic state reconciliation
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.stateManager.StartPeriodicReconciliation(ctx)
	}()

	// Start health monitoring
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.healthMonitor.Start(ctx)
	}()

	// Start statistics collection
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.statsCollector.Start(ctx)
	}()

	// Start status reporting
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.reporter.Start(ctx)
	}()

	s.logger.Info("Node agent service started successfully")

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown
	s.logger.Info("Shutting down node agent service")

	// Mark node as terminated in database
	if err := s.markNodeAsTerminated(context.Background()); err != nil {
		s.logger.Error("Failed to mark node as terminated", "error", err)
		// Continue with shutdown anyway
	}

	// Stop event subscriptions
	s.eventSubscriber.Unsubscribe()

	// Close connections
	s.natsClient.Close()
	s.db.Close()

	// Wait for all goroutines to finish
	wg.Wait()

	s.logger.Info("Node agent service stopped")
	return nil
}

// ensureNodeRegistration ensures this node is registered in the database
func (s *Service) ensureNodeRegistration(ctx context.Context) error {
	queries := s.db.Queries()

	// Parse region ID
	regionID, err := uuid.Parse(s.config.RegionID)
	if err != nil {
		return fmt.Errorf("invalid region ID: %w", err)
	}

	// Ensure region exists (panic if it doesn't)
	if err := s.ensureRegionExists(ctx, regionID); err != nil {
		panic(fmt.Sprintf("Region %s does not exist in database: %v", regionID, err))
	}

	// Check if node already exists
	_, err = queries.NodesGetById(ctx, pgtype.UUID{Bytes: s.nodeID, Valid: true})
	if err == nil {
		// Node already exists, just update its state to "ready"
		s.logger.Info("Node already registered, updating state to ready")
		_, err = queries.NodesUpdateState(ctx, &database.NodesUpdateStateParams{
			ID:    pgtype.UUID{Bytes: s.nodeID, Valid: true},
			State: "ready",
		})
		if err != nil {
			return fmt.Errorf("failed to update node state: %w", err)
		}
		return nil
	}

	// Node doesn't exist, create it
	s.logger.Info("Registering new node in database")

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("node-%s", s.nodeID.String()[:8])
		s.logger.Warn("Failed to get hostname, using generated name", "hostname", hostname)
	}

	// Get IP address (best effort)
	ipAddress := s.getLocalIPAddress()

	// Get system resources
	resources := s.getSystemResources()
	resourcesJSON, err := json.Marshal(resources)
	if err != nil {
		return fmt.Errorf("failed to marshal resources: %w", err)
	}

	// Create node record
	createParams := &database.NodesCreateParams{
		ID:        pgtype.UUID{Bytes: s.nodeID, Valid: true},
		RegionID:  pgtype.UUID{Bytes: regionID, Valid: true},
		Hostname:  hostname,
		IpAddress: ipAddress,
		State:     "ready",
		Resources: resourcesJSON,
	}

	node, err := queries.NodesCreate(ctx, createParams)
	if err != nil {
		return fmt.Errorf("failed to create node record: %w", err)
	}

	s.logger.Info("Node registered successfully",
		"node_id", node.ID,
		"hostname", node.Hostname,
		"ip_address", node.IpAddress,
		"region_id", node.RegionID)

	return nil
}

// ensureRegionExists checks that the region exists in the database
func (s *Service) ensureRegionExists(ctx context.Context, regionID uuid.UUID) error {
	queries := s.db.Queries()

	// Check if region exists
	_, err := queries.RegionsGetById(ctx, pgtype.UUID{Bytes: regionID, Valid: true})
	if err != nil {
		return fmt.Errorf("region does not exist in database")
	}

	s.logger.Debug("Region exists", "region_id", regionID)
	return nil
}

// markNodeAsTerminated marks this node as terminated in the database
func (s *Service) markNodeAsTerminated(ctx context.Context) error {
	queries := s.db.Queries()

	s.logger.Info("Marking node as terminated in database")

	// TODO: Implement proper node draining process
	// - Wait for running instances to complete or migrate them
	// - Set node state to "draining" first, then "terminated" after instances are handled
	// - Coordinate with manager service for graceful instance migration
	// - Add configurable drain timeout
	// For now, we just mark as terminated immediately

	_, err := queries.NodesUpdateState(ctx, &database.NodesUpdateStateParams{
		ID:    pgtype.UUID{Bytes: s.nodeID, Valid: true},
		State: "terminated",
	})
	if err != nil {
		return fmt.Errorf("failed to update node state to terminated: %w", err)
	}

	s.logger.Info("Node marked as terminated successfully", "node_id", s.nodeID)
	return nil
}

// getLocalIPAddress attempts to get the local IP address
func (s *Service) getLocalIPAddress() string {
	// Try to get outbound IP by connecting to a remote address
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		s.logger.Warn("Failed to determine local IP address", "error", err)
		return "unknown"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// getSystemResources gets system resource information
func (s *Service) getSystemResources() map[string]interface{} {
	// Get basic system info
	numCPU := runtime.NumCPU()

	// For now, use static values - in production we'd get actual memory info
	// TODO: Use proper system info library to get actual memory, disk, etc.
	resources := map[string]interface{}{
		"vcpu":   numCPU,
		"memory": 8192, // 8GB default - should be detected properly
		"disk":   100,  // 100GB default - should be detected properly
	}

	return resources
}

// setupHealthMonitoring configures health monitoring callbacks
func (s *Service) setupHealthMonitoring() {
	// Set callback for health changes
	s.healthMonitor.SetHealthChangeCallback(func(instanceID string, healthy bool, reason string) {
		s.logger.Info("Instance health changed",
			"instance_id", instanceID,
			"healthy", healthy,
			"reason", reason)

		// Report state change to database
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var newState string
		if healthy {
			newState = "running"
		} else {
			newState = "failed"
		}

		if err := s.reporter.ReportInstanceStateChange(ctx, instanceID, newState); err != nil {
			s.logger.Error("Failed to report instance state change",
				"instance_id", instanceID,
				"error", err)
		}
	})

	s.logger.Debug("Health monitoring configured")
}

// GetNodeStatus returns the current status of the node
func (s *Service) GetNodeStatus() NodeStatus {
	stateStats := s.stateManager.GetStateStats()
	healthSummary := s.healthMonitor.GetHealthSummary()
	nodeStats := s.statsCollector.GetNodeStats()
	runtimeInfo := s.runtime.GetRuntimeInfo()

	return NodeStatus{
		NodeID:           s.nodeID.String(),
		RuntimeInfo:      runtimeInfo,
		StateStats:       stateStats,
		HealthSummary:    healthSummary,
		NodeStats:        nodeStats,
		DesiredInstances: s.stateManager.GetDesiredInstances(),
		ActualInstances:  s.stateManager.GetActualInstances(),
	}
}

// ForceReconciliation forces an immediate state reconciliation
func (s *Service) ForceReconciliation(ctx context.Context) error {
	s.logger.Info("Forcing immediate reconciliation")

	// Refresh both desired and actual state
	if err := s.stateManager.RefreshDesiredState(ctx); err != nil {
		return fmt.Errorf("failed to refresh desired state: %w", err)
	}

	if err := s.stateManager.RefreshActualState(ctx); err != nil {
		return fmt.Errorf("failed to refresh actual state: %w", err)
	}

	// Perform reconciliation
	if err := s.stateManager.Reconcile(ctx); err != nil {
		return fmt.Errorf("failed to perform reconciliation: %w", err)
	}

	s.logger.Info("Forced reconciliation completed")
	return nil
}

// NodeStatus represents the current status of the node
type NodeStatus struct {
	NodeID           string                   `json:"node_id"`
	RuntimeInfo      *types.RuntimeInfo       `json:"runtime_info"`
	StateStats       state.StateStats         `json:"state_stats"`
	HealthSummary    monitoring.HealthSummary `json:"health_summary"`
	NodeStats        *monitoring.NodeStats    `json:"node_stats"`
	DesiredInstances []*types.Instance        `json:"desired_instances"`
	ActualInstances  []*types.Instance        `json:"actual_instances"`
}
