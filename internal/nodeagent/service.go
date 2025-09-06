package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
	"github.com/zeitwork/zeitwork/internal/nodeagent/events"
	"github.com/zeitwork/zeitwork/internal/nodeagent/monitoring"
	"github.com/zeitwork/zeitwork/internal/nodeagent/runtime"
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
	rt, err := runtime.NewRuntime(cfg.Runtime, logger)
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
