package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nats-io/nats.go"
	"github.com/samber/lo"
	"github.com/zeitwork/zeitwork/internal/database"
	sharedConfig "github.com/zeitwork/zeitwork/internal/shared/config"
	sharedNats "github.com/zeitwork/zeitwork/internal/shared/nats"
)

// Service represents the node agent service that runs on each compute node
type Service struct {
	logger     *slog.Logger
	config     *Config
	httpClient *http.Client
	nodeID     uuid.UUID
	db         *database.DB
	natsClient *sharedNats.Client

	// Instance management
	instanceManager *InstanceManager
	instances       map[string]*Instance
	instancesMu     sync.RWMutex

	// Polling configuration
	pollInterval time.Duration
}

// Config holds the configuration for the node agent service
type Config struct {
	Port         string
	NodeID       string
	DatabaseURL  string
	PollInterval time.Duration
}

// Instance represents a running VM instance
type Instance struct {
	ID        string
	ImageID   string
	State     string
	Resources map[string]interface{}
	EnvVars   map[string]string
}

// NewService creates a new node agent service
func NewService(config *Config, logger *slog.Logger) (*Service, error) {
	// Parse or generate node ID
	var nodeID uuid.UUID
	var err error
	if config.NodeID != "" {
		nodeID, err = uuid.Parse(config.NodeID)
		if err != nil {
			return nil, fmt.Errorf("invalid node ID: %w", err)
		}
	} else {
		return nil, fmt.Errorf("node ID is required")
	}

	// Connect to database
	db, err := database.NewDB(config.DatabaseURL)
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

	service := &Service{
		logger:          logger,
		config:          config,
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		nodeID:          nodeID,
		db:              db,
		natsClient:      natsClient,
		instanceManager: NewInstanceManager(logger, nodeID),
		instances:       make(map[string]*Instance),
		pollInterval:    config.PollInterval,
	}

	// Load initial state from database
	if err := service.loadStateFromDB(context.Background()); err != nil {
		logger.Error("Failed to load initial state from database", "error", err)
		// Continue anyway - we'll sync on first poll
	}

	return service, nil
}

// Start starts the node agent service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting node agent service",
		"port", s.config.Port,
		"node_id", s.nodeID,
	)

	// Create a wait group for goroutines
	var wg sync.WaitGroup

	wg.Add(2) // Add count for both goroutines

	// Start database poller
	go func() {
		defer wg.Done()
		s.runPoller(ctx)
	}()

	// Start NATS subscriber
	go func() {
		defer wg.Done()
		s.runNATSSubscriber(ctx)
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown server
	s.logger.Info("Shutting down node agent service")

	// Wait for goroutines to finish
	wg.Wait()

	// Close connections
	s.natsClient.Close()
	s.db.Close()

	return nil
}

// loadStateFromDB loads the current instances from the database
func (s *Service) loadStateFromDB(ctx context.Context) error {
	// Convert UUID to pgtype.UUID for database query
	pgNodeID := pgtype.UUID{
		Bytes: s.nodeID,
		Valid: true,
	}

	instances, err := s.db.Queries().InstancesFindByNode(ctx, pgNodeID)
	if err != nil {
		return fmt.Errorf("failed to query instances: %w", err)
	}

	s.instancesMu.Lock()
	defer s.instancesMu.Unlock()

	// Filter valid instances and convert to internal format using lo
	validInstances := lo.Filter(instances, func(dbInstance *database.InstancesFindByNodeRow, _ int) bool {
		return dbInstance.ID.Valid
	})

	s.instances = lo.SliceToMap(validInstances, func(dbInstance *database.InstancesFindByNodeRow) (string, *Instance) {
		// Convert pgtype.UUID to string
		instanceID := uuid.UUID(dbInstance.ID.Bytes)
		imageID := uuid.UUID(dbInstance.ImageID.Bytes)

		instance := &Instance{
			ID:        instanceID.String(),
			ImageID:   imageID.String(),
			State:     dbInstance.State,
			Resources: make(map[string]interface{}),
			EnvVars:   make(map[string]string),
		}

		// Resources are stored at node level, not instance level
		instance.Resources = make(map[string]interface{})

		// Parse environment variables if available
		if dbInstance.EnvironmentVariables != "" {
			if err := json.Unmarshal([]byte(dbInstance.EnvironmentVariables), &instance.EnvVars); err != nil {
				s.logger.Warn("Failed to parse instance env vars", "instance_id", instance.ID, "error", err)
			}
		}

		return instance.ID, instance
	})

	s.logger.Info("Loaded instances from database", "count", len(s.instances))
	return nil
}

// runPoller periodically polls the database for state updates
func (s *Service) runPoller(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	s.logger.Info("Starting database poller", "interval", s.pollInterval)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Stopping database poller")
			return
		case <-ticker.C:
			if err := s.syncWithDatabase(ctx); err != nil {
				s.logger.Error("Failed to sync with database", "error", err)
			}
		}
	}
}

// syncWithDatabase loads state from database and reconciles with actual state
func (s *Service) syncWithDatabase(ctx context.Context) error {
	// Load desired state from database
	if err := s.loadStateFromDB(ctx); err != nil {
		return fmt.Errorf("failed to load state from database: %w", err)
	}

	// Reconcile with actual state
	if err := s.reconcileState(ctx); err != nil {
		return fmt.Errorf("failed to reconcile state: %w", err)
	}

	return nil
}

// reconcileState ensures actual VM state matches desired state from database
func (s *Service) reconcileState(ctx context.Context) error {
	s.instancesMu.RLock()
	instances := lo.Values(s.instances)
	s.instancesMu.RUnlock()

	// Group instances by state for more efficient processing
	instancesByState := lo.GroupBy(instances, func(instance *Instance) string {
		return instance.State
	})

	// Process each state group
	s.processInstanceGroup(ctx, instancesByState["pending"], s.startInstance)
	s.processInstanceGroup(ctx, instancesByState["starting"], s.startInstance)
	s.processInstanceGroup(ctx, instancesByState["stopping"], s.stopInstance)

	// Handle running instances (need status check)
	lo.ForEach(instancesByState["running"], func(instance *Instance, _ int) {
		s.reconcileRunningInstance(ctx, instance)
	})

	// Handle stopped instances (need status check)
	lo.ForEach(lo.Flatten([]([]*Instance){
		instancesByState["stopped"],
		instancesByState["terminated"],
	}), func(instance *Instance, _ int) {
		s.reconcileStoppedInstance(ctx, instance)
	})

	return nil
}

// processInstanceGroup processes a group of instances with the same action
func (s *Service) processInstanceGroup(ctx context.Context, instances []*Instance, action func(context.Context, *Instance) error) {
	lo.ForEach(instances, func(instance *Instance, _ int) {
		s.logger.Debug("Reconciling instance", "id", instance.ID, "state", instance.State)
		if err := action(ctx, instance); err != nil {
			s.logger.Error("Failed to process instance", "id", instance.ID, "error", err)
		}
	})
}

// reconcileRunningInstance ensures a running instance is actually running
func (s *Service) reconcileRunningInstance(ctx context.Context, instance *Instance) {
	running, err := s.instanceManager.IsInstanceRunning(ctx, instance)
	if err != nil {
		s.logger.Error("Failed to check instance status", "id", instance.ID, "error", err)
		return
	}

	if !running {
		s.logger.Warn("VM should be running but isn't", "id", instance.ID)
		if err := s.startInstance(ctx, instance); err != nil {
			s.logger.Error("Failed to restart VM", "id", instance.ID, "error", err)
		}
	}
}

// reconcileStoppedInstance ensures a stopped instance is actually stopped
func (s *Service) reconcileStoppedInstance(ctx context.Context, instance *Instance) {
	running, err := s.instanceManager.IsInstanceRunning(ctx, instance)
	if err != nil {
		s.logger.Error("Failed to check instance status", "id", instance.ID, "error", err)
		return
	}

	if running {
		if err := s.stopInstance(ctx, instance); err != nil {
			s.logger.Error("Failed to ensure VM is stopped", "id", instance.ID, "error", err)
		}
	}
}

// runNATSSubscriber subscribes to NATS topics for real-time updates
func (s *Service) runNATSSubscriber(ctx context.Context) {
	// Subscribe to node-specific topics
	nodeSubject := fmt.Sprintf("nodes.%s.instances", s.nodeID.String())

	sub, err := s.natsClient.WithContext(ctx).Subscribe(nodeSubject, func(msg *nats.Msg) {
		s.logger.Debug("Received NATS message", "subject", msg.Subject, "data", string(msg.Data))

		// Trigger immediate sync
		if err := s.syncWithDatabase(ctx); err != nil {
			s.logger.Error("Failed to sync after NATS message", "error", err)
		}
	})

	if err != nil {
		s.logger.Error("Failed to subscribe to NATS", "subject", nodeSubject, "error", err)
		return
	}
	defer sub.Unsubscribe()

	s.logger.Info("NATS subscriber started", "subject", nodeSubject)

	// Wait for context cancellation
	<-ctx.Done()
	s.logger.Info("Stopping NATS subscriber")
}

// startInstance starts an instance and updates the database state
func (s *Service) startInstance(ctx context.Context, instance *Instance) error {
	s.logger.Info("Starting instance", "id", instance.ID, "image", instance.ImageID)

	// Use instance manager to start the VM
	if err := s.instanceManager.StartInstance(ctx, instance); err != nil {
		return fmt.Errorf("failed to start instance: %w", err)
	}

	// Update state in database to "running"
	return s.updateInstanceState(ctx, instance.ID, "running")
}

// stopInstance stops an instance and updates the database state
func (s *Service) stopInstance(ctx context.Context, instance *Instance) error {
	s.logger.Info("Stopping instance", "id", instance.ID)

	// Use instance manager to stop the VM
	if err := s.instanceManager.StopInstance(ctx, instance); err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}

	// Update state in database to "stopped"
	return s.updateInstanceState(ctx, instance.ID, "stopped")
}

// updateInstanceState updates the instance state in the database
func (s *Service) updateInstanceState(ctx context.Context, instanceID, state string) error {
	instanceUUID, err := uuid.Parse(instanceID)
	if err != nil {
		return fmt.Errorf("invalid instance ID: %w", err)
	}

	pgInstanceID := pgtype.UUID{
		Bytes: instanceUUID,
		Valid: true,
	}

	_, err = s.db.Queries().InstancesUpdateState(ctx, &database.InstancesUpdateStateParams{
		ID:    pgInstanceID,
		State: state,
	})

	return err
}
