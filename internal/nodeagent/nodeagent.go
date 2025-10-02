package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/samber/lo"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/nodeagent/runtime"
	"github.com/zeitwork/zeitwork/internal/nodeagent/runtime/docker"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type Config struct {
	NodeID          string `env:"NODE_ID"`
	NodeRegionID    string `env:"NODE_REGION_ID"`
	NodeDatabaseURL string `env:"NODE_DATABASE_URL"`
	NodeRuntimeMode string `env:"NODE_RUNTIME_MODE" envDefault:"docker"`
}

type Service struct {
	cfg     Config
	db      *database.DB
	runtime runtime.Runtime
	logger  *slog.Logger
	nodeID  pgtype.UUID
}

func NewService(cfg Config, logger *slog.Logger) (*Service, error) {
	// Parse node UUID
	nodeUUID, err := uuid.Parse(cfg.NodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid node id: %w", err)
	}

	// Initialize runtime based on mode
	var rt runtime.Runtime
	switch cfg.NodeRuntimeMode {
	case "docker":
		rt, err = docker.NewDockerRuntime(logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create docker runtime: %w", err)
		}
	case "kata":
		// TODO: implement kata runtime
		return nil, fmt.Errorf("kata runtime not yet implemented")
	default:
		return nil, fmt.Errorf("unknown runtime mode: %s", cfg.NodeRuntimeMode)
	}

	// Initialize database
	db, err := database.NewDB(cfg.NodeDatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Service{
		cfg:     cfg,
		db:      db,
		runtime: rt,
		logger:  logger,
		nodeID:  nodeUUID,
	}, nil
}

func (s *Service) Start() {
	s.logger.Info("nodeagent started",
		"node_id", s.cfg.NodeID,
		"runtime", s.cfg.NodeRuntimeMode,
	)

	for {
		s.logger.Info("starting reconciliation loop")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := s.reconcile(ctx); err != nil {
			s.logger.Error("reconciliation failed", "error", err)
		}
		cancel()

		// Sleep 60s +/- 15s random offset
		offset := time.Duration(rand.Intn(31)-15) * time.Second
		sleepDuration := 60*time.Second + offset
		s.logger.Info("sleeping", "duration", sleepDuration)
		time.Sleep(sleepDuration)
	}
}

func (s *Service) Close() {
	s.logger.Info("shutting down nodeagent")

	if s.runtime != nil {
		if err := s.runtime.Close(); err != nil {
			s.logger.Error("failed to close runtime", "error", err)
		}
	}

	if s.db != nil {
		s.db.Close()
	}
}

func (s *Service) reconcile(ctx context.Context) error {
	// 1. Fetch desired state from database
	desiredInstances, err := s.db.Queries().GetInstancesByNodeID(ctx, s.nodeID)
	if err != nil {
		return fmt.Errorf("failed to get instances: %w", err)
	}

	s.logger.Info("fetched desired state",
		"instance_count", len(desiredInstances),
	)

	// 2. Get current state from runtime
	runningContainers, err := s.runtime.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	s.logger.Info("fetched current state",
		"container_count", len(runningContainers),
	)

	// 3. Build maps for comparison
	desiredMap := make(map[string]*database.GetInstancesByNodeIDRow)
	for _, inst := range desiredInstances {
		// Only include instances that should be running
		if inst.State == database.InstanceStatusesPending ||
			inst.State == database.InstanceStatusesStarting ||
			inst.State == database.InstanceStatusesRunning {

			idStr := uuid.ToString(inst.ID)
			desiredMap[idStr] = inst
		}
	}

	runningMap := make(map[string]runtime.Container)
	for _, container := range runningContainers {
		runningMap[container.InstanceID] = container
	}

	// 4. Compute differences
	// Instances to start: in desired but not running
	toStart := lo.Filter(lo.Values(desiredMap), func(inst *database.GetInstancesByNodeIDRow, _ int) bool {
		idStr := uuid.ToString(inst.ID)
		_, exists := runningMap[idStr]
		return !exists
	})

	// Instances to stop: running but not in desired state or should be stopped
	toStop := lo.Filter(runningContainers, func(container runtime.Container, _ int) bool {
		_, exists := desiredMap[container.InstanceID]
		return !exists
	})

	s.logger.Info("computed reconciliation actions",
		"to_start", len(toStart),
		"to_stop", len(toStop),
	)

	// 5. Apply changes
	// Stop containers that should not be running
	for _, container := range toStop {
		s.logger.Info("stopping instance", "instance_id", container.InstanceID)

		if err := s.runtime.Stop(ctx, container.InstanceID); err != nil {
			s.logger.Error("failed to stop container",
				"instance_id", container.InstanceID,
				"error", err,
			)
			continue
		}

		// Update state in database
		instanceUUID, _ := uuid.Parse(container.InstanceID)
		if err := s.db.Queries().UpdateInstanceState(ctx, &database.UpdateInstanceStateParams{
			ID:    instanceUUID,
			State: database.InstanceStatusesStopped,
		}); err != nil {
			s.logger.Error("failed to update instance state",
				"instance_id", container.InstanceID,
				"error", err,
			)
		}
	}

	// Start containers that should be running
	for _, inst := range toStart {
		instanceID := uuid.ToString(inst.ID)
		s.logger.Info("starting instance",
			"instance_id", instanceID,
			"image", inst.ImageName,
		)

		// Update state to starting
		if err := s.db.Queries().UpdateInstanceState(ctx, &database.UpdateInstanceStateParams{
			ID:    inst.ID,
			State: database.InstanceStatusesStarting,
		}); err != nil {
			s.logger.Error("failed to update instance state",
				"instance_id", instanceID,
				"error", err,
			)
		}

		// Parse environment variables (stored as JSON string)
		envVars := make(map[string]string)
		if inst.EnvironmentVariables != "" {
			if err := json.Unmarshal([]byte(inst.EnvironmentVariables), &envVars); err != nil {
				s.logger.Error("failed to parse environment variables",
					"instance_id", instanceID,
					"error", err,
				)
			}
		}

		// Start the container
		if err := s.runtime.Start(
			ctx,
			instanceID,
			inst.ImageName,
			inst.IpAddress,
			int(inst.Vcpus),
			int(inst.Memory),
			int(inst.DefaultPort),
			envVars,
		); err != nil {
			s.logger.Error("failed to start container",
				"instance_id", instanceID,
				"error", err,
			)

			// Update state to failed
			if err := s.db.Queries().UpdateInstanceState(ctx, &database.UpdateInstanceStateParams{
				ID:    inst.ID,
				State: database.InstanceStatusesFailed,
			}); err != nil {
				s.logger.Error("failed to update instance state",
					"instance_id", instanceID,
					"error", err,
				)
			}
			continue
		}

		// Update state to running
		if err := s.db.Queries().UpdateInstanceState(ctx, &database.UpdateInstanceStateParams{
			ID:    inst.ID,
			State: database.InstanceStatusesRunning,
		}); err != nil {
			s.logger.Error("failed to update instance state",
				"instance_id", instanceID,
				"error", err,
			)
		}
	}

	s.logger.Info("reconciliation completed")
	return nil
}
