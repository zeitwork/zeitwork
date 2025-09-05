package manager

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/samber/lo"

	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/manager/events"
	"github.com/zeitwork/zeitwork/internal/manager/handlers"
	"github.com/zeitwork/zeitwork/internal/manager/orchestration"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	natsClient "github.com/zeitwork/zeitwork/internal/shared/nats"
)

// Service represents the manager service with type-safe event handling
type Service struct {
	logger *slog.Logger
	config *config.ManagerConfig

	// Infrastructure
	db         *pgxpool.Pool
	queries    *database.Queries
	natsClient *natsClient.Client

	// Components
	orchestrator  *orchestration.DeploymentOrchestrator
	eventRegistry *events.Registry

	instanceID string
	stopChan   chan struct{}
}

// NewService creates a new manager service
func NewService(cfg *config.ManagerConfig, logger *slog.Logger) (*Service, error) {
	logger.Info("Creating manager service")

	// Initialize infrastructure
	db, err := initializeDatabase(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	queries := database.New(db)

	natsClient, err := natsClient.NewSimpleClient()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize NATS client: %w", err)
	}

	// Initialize orchestration layer
	orchestrator := orchestration.NewDeploymentOrchestrator(queries, logger)

	// Initialize event handling
	eventRegistry := events.NewRegistry(logger)

	// Create simple, direct protobuf handlers
	buildCreatedHandler := handlers.NewBuildCreatedHandler(orchestrator, logger)
	buildCompletedHandler := handlers.NewBuildCompletedHandler(orchestrator, logger)
	deploymentCreatedHandler := handlers.NewDeploymentCreatedHandler(orchestrator, logger)
	deploymentUpdatedHandler := handlers.NewDeploymentUpdatedHandler(orchestrator, logger)

	eventRegistry.MustRegister(buildCreatedHandler)
	eventRegistry.MustRegister(buildCompletedHandler)
	eventRegistry.MustRegister(deploymentCreatedHandler)
	eventRegistry.MustRegister(deploymentUpdatedHandler)

	instanceID := uuid.Must(uuid.NewV7()).String()

	service := &Service{
		logger:        logger,
		config:        cfg,
		db:            db,
		queries:       queries,
		natsClient:    natsClient,
		orchestrator:  orchestrator,
		eventRegistry: eventRegistry,
		instanceID:    instanceID,
		stopChan:      make(chan struct{}),
	}

	logger.Info("Manager service created successfully",
		"instance_id", instanceID,
		"registered_handlers", eventRegistry.GetHandlerCount())

	return service, nil
}

// Start starts the manager service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting manager service", "instance_id", s.instanceID)

	// TODO: Add startup check and recovery logic here
	// This should handle first-manager detection and system recovery

	// Subscribe to all registered event types
	if err := s.subscribeToEvents(ctx); err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	s.logger.Info("Manager service started successfully", "instance_id", s.instanceID)

	// Wait for shutdown
	<-ctx.Done()
	s.logger.Info("Shutting down manager service", "instance_id", s.instanceID)

	return s.Close()
}

// subscribeToEvents subscribes to all registered event types using type-safe handlers
func (s *Service) subscribeToEvents(ctx context.Context) error {
	eventTypes := s.eventRegistry.GetAllEventTypes()

	s.logger.Info("Subscribing to event types",
		"count", len(eventTypes),
		"types", lo.Map(eventTypes, func(et events.EventType, _ int) string { return string(et) }),
		"instance_id", s.instanceID)

	// Subscribe to each event type with comprehensive error handling
	subscriptionResults := lo.Map(eventTypes, func(eventType events.EventType, _ int) struct {
		eventType events.EventType
		err       error
	} {
		err := s.subscribeToEventType(ctx, eventType)
		return struct {
			eventType events.EventType
			err       error
		}{eventType, err}
	})

	// Collect any subscription errors
	errors := lo.FilterMap(subscriptionResults, func(result struct {
		eventType events.EventType
		err       error
	}, _ int) (error, bool) {
		if result.err != nil {
			s.logger.Error("Failed to subscribe to event type",
				"event_type", result.eventType,
				"error", result.err,
				"instance_id", s.instanceID)
			return fmt.Errorf("failed to subscribe to %s: %w", result.eventType, result.err), true
		}
		return nil, false
	})

	if len(errors) > 0 {
		return fmt.Errorf("failed to subscribe to %d event types: %v", len(errors), errors)
	}

	s.logger.Info("Successfully subscribed to all event types",
		"count", len(eventTypes),
		"instance_id", s.instanceID)

	return nil
}

// subscribeToEventType subscribes to a specific event type with type-safe routing
func (s *Service) subscribeToEventType(ctx context.Context, eventType events.EventType) error {
	subject := string(eventType)
	queueGroup := "manager-workers"

	_, err := s.natsClient.QueueSubscribe(subject, queueGroup, func(msg *nats.Msg) {
		s.handleNATSMessage(ctx, eventType, msg)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	s.logger.Info("Subscribed to event type",
		"event_type", eventType,
		"subject", subject,
		"queue_group", queueGroup,
		"instance_id", s.instanceID)

	return nil
}

// handleNATSMessage handles incoming NATS messages with type-safe event routing
func (s *Service) handleNATSMessage(ctx context.Context, eventType events.EventType, msg *nats.Msg) {
	s.logger.Debug("Received NATS message",
		"event_type", eventType,
		"data_size", len(msg.Data),
		"subject", msg.Subject,
		"instance_id", s.instanceID)

	// Route to appropriate handler using the registry
	if err := s.eventRegistry.HandleEvent(ctx, eventType, msg.Data); err != nil {
		s.logger.Error("Failed to handle event",
			"event_type", eventType,
			"error", err,
			"subject", msg.Subject,
			"instance_id", s.instanceID)
	} else {
		s.logger.Debug("Successfully handled event",
			"event_type", eventType,
			"instance_id", s.instanceID)
	}
}

// Close closes the manager service and cleans up resources
func (s *Service) Close() error {
	s.logger.Info("Closing manager service", "instance_id", s.instanceID)

	// Close NATS client
	if s.natsClient != nil {
		s.natsClient.Close()
		s.logger.Debug("NATS client closed", "instance_id", s.instanceID)
	}

	// Close database connections
	if s.db != nil {
		s.db.Close()
		s.logger.Debug("Database connections closed", "instance_id", s.instanceID)
	}

	close(s.stopChan)
	s.logger.Info("Manager service closed successfully", "instance_id", s.instanceID)

	return nil
}

// Helper function to initialize database with proper error handling
func initializeDatabase(cfg *config.ManagerConfig) (*pgxpool.Pool, error) {
	dbConfig := lo.Must(pgxpool.ParseConfig(cfg.DatabaseURL))
	db := lo.Must(pgxpool.NewWithConfig(context.Background(), dbConfig))
	lo.Must0(db.Ping(context.Background()))
	return db, nil
}
