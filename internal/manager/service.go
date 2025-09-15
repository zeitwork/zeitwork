package manager

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"

	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	natsClient "github.com/zeitwork/zeitwork/internal/shared/nats"
	zuuid "github.com/zeitwork/zeitwork/internal/shared/uuid"
	pb "github.com/zeitwork/zeitwork/proto"
)

// Service represents the manager service with type-safe event handling
type Service struct {
	logger *slog.Logger
	config *config.ManagerConfig

	// Infrastructure
	db         *pgxpool.Pool
	queries    *database.Queries
	natsClient *natsClient.Client

	instanceID string

	// Reconciliation
	ticker *time.Ticker
	done   chan struct{}

	// Synchronization
	mu     sync.Mutex
	closed bool
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

	natsClient, err := natsClient.NewClient(cfg.NATS, "manager")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize NATS client: %w", err)
	}

	instanceID := uuid.Must(uuid.NewV7()).String()

	service := &Service{
		logger:     logger,
		config:     cfg,
		db:         db,
		queries:    queries,
		natsClient: natsClient,
		instanceID: instanceID,
		done:       make(chan struct{}),
	}

	logger.Info("Manager service created successfully",
		"instance_id", instanceID)

	return service, nil
}

// Start starts the manager service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting manager service", "instance_id", s.instanceID)

	// Subscribe directly to core event subjects
	if err := s.subscribeToCoreEvents(ctx); err != nil {
		return fmt.Errorf("failed to subscribe to core events: %w", err)
	}

	// Initial reconciliation
	if err := s.reconcileAll(ctx); err != nil {
		s.logger.Error("Initial reconciliation failed", "error", err)
	}

	// Start periodic reconciliation (simple 30s interval)
	s.ticker = time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				if err := s.reconcileAll(ctx); err != nil {
					s.logger.Error("Reconciliation failed", "error", err)
				}
			case <-s.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	s.logger.Info("Manager service started successfully", "instance_id", s.instanceID)

	// Wait for shutdown
	<-ctx.Done()
	s.logger.Info("Shutting down manager service", "instance_id", s.instanceID)

	return s.Close()
}

// subscribeToCoreEvents subscribes directly to the core subjects needed by the manager
func (s *Service) subscribeToCoreEvents(ctx context.Context) error {
	queueGroup := "manager-workers"
	cc := s.natsClient.WithContext(ctx)

	// Map subjects to handlers
	subscriptions := []struct {
		subject string
		handler func(context.Context, *nats.Msg)
	}{
		{"image_build.updated", s.handleImageBuildUpdated},
		{"deployment.created", s.handleDeploymentCreated},
		{"instance.updated", s.handleInstanceUpdated},
	}

	results := lo.Map(subscriptions, func(su struct {
		subject string
		handler func(context.Context, *nats.Msg)
	}, _ int) error {
		_, err := cc.QueueSubscribe(su.subject, queueGroup, func(msg *nats.Msg) {
			su.handler(ctx, msg)
		})
		if err == nil {
			s.logger.Info("Subscribed to subject", "subject", su.subject, "queue_group", queueGroup, "instance_id", s.instanceID)
		}
		return err
	})

	errs := lo.Filter(results, func(err error, _ int) bool { return err != nil })
	if len(errs) > 0 {
		return fmt.Errorf("failed to subscribe to some subjects: %v", errs)
	}

	return nil
}

// --- Event Handlers ---

func (s *Service) handleImageBuildUpdated(ctx context.Context, msg *nats.Msg) {
	var event pb.ImageBuildUpdated
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		s.logger.Error("Failed to unmarshal ImageBuildUpdated", "error", err)
		return
	}

	buildID := zuuid.MustParseUUID(event.Id)

	deployment, err := s.queries.DeploymentsGetByImageBuildId(ctx, buildID)
	if err != nil {
		s.logger.Error("Failed to get deployment by image_build_id", "build_id", buildID, "error", err)
		return
	}

	build, err := s.queries.ImageBuildsGetById(ctx, buildID)
	if err != nil {
		s.logger.Error("Failed to get image build", "build_id", buildID, "error", err)
		return
	}

	if build.Status != "completed" || !build.ImageID.Valid {
		return
	}

	if _, err := s.queries.DeploymentsUpdateImageId(ctx, &database.DeploymentsUpdateImageIdParams{ID: deployment.ID, ImageID: build.ImageID}); err != nil {
		s.logger.Error("Failed to update deployment image_id", "deployment_id", deployment.ID, "error", err)
		return
	}
	if _, err := s.queries.DeploymentsUpdateStatus(ctx, &database.DeploymentsUpdateStatusParams{ID: deployment.ID, Status: "deploying"}); err != nil {
		s.logger.Error("Failed to update deployment status to deploying", "deployment_id", deployment.ID, "error", err)
		return
	}

	s.logger.Info("Deployment moved to deploying", "deployment_id", deployment.ID, "build_id", buildID)
}

func (s *Service) handleDeploymentCreated(ctx context.Context, msg *nats.Msg) {
	var event pb.DeploymentCreated
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		s.logger.Error("Failed to unmarshal DeploymentCreated", "error", err)
		return
	}

	deploymentID := zuuid.MustParseUUID(event.Id)
	deployment, err := s.queries.DeploymentsGetById(ctx, deploymentID)
	if err != nil {
		s.logger.Error("Failed to get deployment", "deployment_id", deploymentID, "error", err)
		return
	}
	if deployment.Status != "pending" {
		return
	}

	if !deployment.ImageBuildID.Valid {
		if err := s.createImageBuildForDeployment(ctx, deployment.ID); err != nil {
			s.logger.Error("Failed to create image build for deployment", "deployment_id", deployment.ID, "error", err)
		}
	}
}

func (s *Service) handleInstanceUpdated(ctx context.Context, msg *nats.Msg) {
	var event pb.InstanceUpdated
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		s.logger.Error("Failed to unmarshal InstanceUpdated", "error", err)
		return
	}

	instanceID := zuuid.MustParseUUID(event.Id)
	if err := s.checkDeploymentReadyForActive(ctx, instanceID); err != nil {
		s.logger.Error("Failed to process instance update", "instance_id", instanceID, "error", err)
	}
}

// --- Reconciliation ---

func (s *Service) reconcileAll(ctx context.Context) error {
	if err := s.reconcilePendingDeployments(ctx); err != nil {
		return err
	}
	if err := s.reconcileDeployingDeployments(ctx); err != nil {
		return err
	}
	if err := s.reconcilePotentiallyActiveDeployments(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) reconcilePendingDeployments(ctx context.Context) error {
	pending, err := s.queries.DeploymentsGetPendingWithoutBuilds(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending deployments without builds: %w", err)
	}
	for _, d := range pending {
		if err := s.createImageBuildForDeployment(ctx, d.ID); err != nil {
			s.logger.Error("Failed to create image build during reconciliation", "deployment_id", d.ID, "error", err)
		}
	}
	return nil
}

func (s *Service) reconcileDeployingDeployments(ctx context.Context) error {
	ready, err := s.queries.DeploymentsGetReadyForDeployment(ctx)
	if err != nil {
		return fmt.Errorf("failed to get ready deployments: %w", err)
	}
	for _, d := range ready {
		instances, err := s.queries.InstancesGetByDeployment(ctx, d.ID)
		if err != nil {
			s.logger.Error("Failed to get instances for deployment", "deployment_id", d.ID, "error", err)
			continue
		}
		if len(instances) == 0 {
			if err := s.createInstancesForDeployment(ctx, d); err != nil {
				s.logger.Error("Failed to create instances for deployment", "deployment_id", d.ID, "error", err)
			}
		}
	}
	return nil
}

func (s *Service) reconcilePotentiallyActiveDeployments(ctx context.Context) error {
	// instance.updated events will typically drive activation; nothing to do here for now.
	return nil
}

// --- Helpers ---

func (s *Service) createImageBuildForDeployment(ctx context.Context, deploymentID pgtype.UUID) error {
	deployment, err := s.queries.DeploymentsGetById(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	buildUUID := zuuid.GeneratePgUUID()
	project, err := s.queries.ProjectsGetById(ctx, deployment.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	if _, err := s.queries.ImageBuildsCreate(ctx, &database.ImageBuildsCreateParams{
		ID:               buildUUID,
		GithubRepository: project.GithubRepository,
		GithubCommit:     deployment.GithubCommit,
	}); err != nil {
		return fmt.Errorf("failed to create image build: %w", err)
	}

	if _, err := s.queries.DeploymentsUpdateImageBuildId(ctx, &database.DeploymentsUpdateImageBuildIdParams{
		ID:           deployment.ID,
		ImageBuildID: pgtype.UUID{Valid: true, Bytes: buildUUID.Bytes},
	}); err != nil {
		return fmt.Errorf("failed to set deployment image_build_id: %w", err)
	}

	if _, err := s.queries.DeploymentsUpdateStatus(ctx, &database.DeploymentsUpdateStatusParams{
		ID:     deployment.ID,
		Status: "building",
	}); err != nil {
		return fmt.Errorf("failed to update deployment status to building: %w", err)
	}

	s.logger.Info("Created image build for deployment", "deployment_id", deployment.ID, "build_id", buildUUID)
	return nil
}

func (s *Service) createInstancesForDeployment(ctx context.Context, d *database.DeploymentsGetReadyForDeploymentRow) error {
	nodes, err := s.queries.NodesGetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}
	var selected *database.NodesGetAllRow
	for _, n := range nodes {
		if n.State == "ready" {
			selected = n
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("no ready nodes available for deployment")
	}

	instanceID := zuuid.GeneratePgUUID()
	if _, err := s.queries.InstancesCreate(ctx, &database.InstancesCreateParams{
		ID:                   instanceID,
		RegionID:             selected.RegionID,
		NodeID:               selected.ID,
		ImageID:              d.ImageID,
		State:                "pending",
		Vcpus:                2,
		Memory:               2048,
		DefaultPort:          3000,
		IpAddress:            "",
		EnvironmentVariables: "{}",
	}); err != nil {
		return fmt.Errorf("failed to create instance: %w", err)
	}

	relationID := zuuid.GeneratePgUUID()
	if _, err := s.queries.DeploymentInstancesCreate(ctx, &database.DeploymentInstancesCreateParams{
		ID:             relationID,
		DeploymentID:   d.ID,
		InstanceID:     instanceID,
		OrganisationID: d.OrganisationID,
	}); err != nil {
		return fmt.Errorf("failed to create deployment-instance relation: %w", err)
	}

	s.logger.Info("Created instance for deployment", "deployment_id", d.ID, "instance_id", instanceID)
	return nil
}

func (s *Service) checkDeploymentReadyForActive(ctx context.Context, instanceID pgtype.UUID) error {
	di, err := s.queries.DeploymentInstancesGetByInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("failed to get deployment instance relation: %w", err)
	}

	instances, err := s.queries.InstancesGetByDeployment(ctx, di.DeploymentID)
	if err != nil {
		return fmt.Errorf("failed to get instances for deployment: %w", err)
	}

	allRunning := true
	for _, inst := range instances {
		if inst.State != "running" {
			allRunning = false
			break
		}
	}
	if !allRunning {
		return nil
	}

	if _, err := s.queries.DeploymentsUpdateStatus(ctx, &database.DeploymentsUpdateStatusParams{ID: di.DeploymentID, Status: "active"}); err != nil {
		return fmt.Errorf("failed to update deployment status to active: %w", err)
	}

	deployment, err := s.queries.DeploymentsGetById(ctx, di.DeploymentID)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	if _, err := s.queries.DomainsRepointToDeploymentForProjectEnv(ctx, &database.DomainsRepointToDeploymentForProjectEnvParams{
		ProjectID:     deployment.ProjectID,
		EnvironmentID: deployment.EnvironmentID,
		DeploymentID:  deployment.ID,
	}); err != nil {
		return fmt.Errorf("failed to repoint domains: %w", err)
	}

	s.logger.Info("Deployment activated and domains repointed", "deployment_id", di.DeploymentID)
	return nil
}

// Close closes the manager service and cleans up resources
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already closed
	if s.closed {
		s.logger.Debug("Service already closed", "instance_id", s.instanceID)
		return nil
	}

	s.logger.Info("Closing manager service", "instance_id", s.instanceID)

	// Stop reconciliation ticker
	if s.ticker != nil {
		s.ticker.Stop()
		close(s.done)
		s.logger.Debug("Reconciliation ticker stopped", "instance_id", s.instanceID)
	}

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

	s.closed = true

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
