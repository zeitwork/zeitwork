package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/samber/lo"
	"github.com/zeitwork/zeitwork/internal/database"
	container "github.com/zeitwork/zeitwork/internal/nodeagent/runtime"
	"github.com/zeitwork/zeitwork/internal/nodeagent/runtime/docker"
	"github.com/zeitwork/zeitwork/internal/nodeagent/utils"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type Config struct {
	NodeID          string `env:"NODE_ID"`
	NodeRegionID    string `env:"NODE_REGION_ID"`
	NodeDatabaseURL string `env:"NODE_DATABASE_URL"`
	NodeRuntimeMode string `env:"NODE_RUNTIME_MODE" envDefault:"docker"`
	NodeIPAddress   string `env:"NODE_IP_ADDRESS"` // External IP address of the node
}

type Service struct {
	cfg      Config
	db       *database.DB
	runtime  container.Runtime
	logger   *slog.Logger
	nodeID   pgtype.UUID
	regionID pgtype.UUID
}

func NewService(cfg Config, logger *slog.Logger) (*Service, error) {
	// Parse node UUID
	nodeUUID, err := uuid.Parse(cfg.NodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid node id: %w", err)
	}

	// Parse region UUID
	regionUUID, err := uuid.Parse(cfg.NodeRegionID)
	if err != nil {
		return nil, fmt.Errorf("invalid region id: %w", err)
	}

	// Initialize runtime based on mode
	var rt container.Runtime
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

	svc := &Service{
		cfg:      cfg,
		db:       db,
		runtime:  rt,
		logger:   logger,
		nodeID:   nodeUUID,
		regionID: regionUUID,
	}

	// Register node in database
	if err := svc.registerNode(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to register node: %w", err)
	}

	return svc, nil
}

func (s *Service) registerNode(ctx context.Context) error {
	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Get IP address from config or default to empty
	ipAddress := s.cfg.NodeIPAddress
	if ipAddress == "" {
		ipAddress = "0.0.0.0" // Placeholder, should be set via config
	}

	// Get system resources
	numCPU := runtime.NumCPU()

	// Get total system memory
	memoryMB, err := utils.GetSystemMemoryMB()
	if err != nil {
		return fmt.Errorf("failed to get system memory: %w", err)
	}

	resources := map[string]interface{}{
		"vcpu":   numCPU,
		"memory": memoryMB,
	}
	resourcesJSON, err := json.Marshal(resources)
	if err != nil {
		return fmt.Errorf("failed to marshal resources: %w", err)
	}

	s.logger.Info("registering node",
		"node_id", s.cfg.NodeID,
		"region_id", s.cfg.NodeRegionID,
		"hostname", hostname,
		"ip", ipAddress,
		"vcpu", numCPU,
		"memory_mb", memoryMB,
	)

	// Upsert node in database
	err = s.db.Queries().UpsertNode(ctx, &database.UpsertNodeParams{
		ID:        s.nodeID,
		RegionID:  s.regionID,
		Hostname:  hostname,
		IpAddress: ipAddress,
		State:     "ready",
		Resources: resourcesJSON,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert node: %w", err)
	}

	s.logger.Info("node registered successfully")
	return nil
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

		// Sleep 10s +/- 5s random offset
		offset := time.Duration(rand.Intn(6)-5) * time.Second
		sleepDuration := 10*time.Second + offset
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

	// Log running containers with their IPs
	for _, c := range runningContainers {
		s.logger.Debug("running container",
			"instance_id", c.InstanceID,
			"container_id", c.ID,
			"ip_address", c.IPAddress,
			"state", c.State,
		)
	}

	// 3. Build maps for comparison
	desiredMap := make(map[string]*database.GetInstancesByNodeIDRow)
	stoppingInstances := make([]*database.GetInstancesByNodeIDRow, 0)

	for _, inst := range desiredInstances {
		// Handle instances that should be stopped
		if inst.State == database.InstanceStatusesStopping {
			stoppingInstances = append(stoppingInstances, inst)
			continue
		}

		// Only include instances that should be running
		if inst.State == database.InstanceStatusesPending ||
			inst.State == database.InstanceStatusesStarting ||
			inst.State == database.InstanceStatusesRunning {

			idStr := uuid.ToString(inst.ID)
			desiredMap[idStr] = inst
		}
	}

	runningMap := make(map[string]container.Container)
	for _, c := range runningContainers {
		runningMap[c.InstanceID] = c
	}

	// 4. Compute differences
	// Instances to start: in desired but not running
	toStart := lo.Filter(lo.Values(desiredMap), func(inst *database.GetInstancesByNodeIDRow, _ int) bool {
		idStr := uuid.ToString(inst.ID)
		_, exists := runningMap[idStr]
		return !exists
	})

	// Instances to stop: running but not in desired state or should be stopped
	toStop := lo.Filter(runningContainers, func(c container.Container, _ int) bool {
		_, exists := desiredMap[c.InstanceID]
		return !exists
	})

	s.logger.Info("computed reconciliation actions",
		"to_start", len(toStart),
		"to_stop", len(toStop),
		"to_shutdown", len(stoppingInstances),
	)

	// 5. Apply changes
	// Handle instances marked as stopping - ensure they are stopped
	for _, inst := range stoppingInstances {
		instanceID := uuid.ToString(inst.ID)
		s.logger.Info("shutting down instance", "instance_id", instanceID)

		// Check if container is running
		if _, exists := runningMap[instanceID]; exists {
			if err := s.runtime.Stop(ctx, instanceID); err != nil {
				s.logger.Error("failed to stop container",
					"instance_id", instanceID,
					"error", err,
				)
				continue
			}
		}

		// Update state to stopped
		if err := s.db.Queries().UpdateInstanceState(ctx, &database.UpdateInstanceStateParams{
			ID:    inst.ID,
			State: database.InstanceStatusesStopped,
		}); err != nil {
			s.logger.Error("failed to update instance state",
				"instance_id", instanceID,
				"error", err,
			)
		} else {
			s.logger.Info("instance stopped", "instance_id", instanceID)
		}
	}

	// Stop containers that should not be running
	for _, c := range toStop {
		s.logger.Info("stopping instance", "instance_id", c.InstanceID)

		if err := s.runtime.Stop(ctx, c.InstanceID); err != nil {
			s.logger.Error("failed to stop container",
				"instance_id", c.InstanceID,
				"error", err,
			)
			continue
		}

		// Update state in database
		instanceUUID, _ := uuid.Parse(c.InstanceID)
		if err := s.db.Queries().UpdateInstanceState(ctx, &database.UpdateInstanceStateParams{
			ID:    instanceUUID,
			State: database.InstanceStatusesStopped,
		}); err != nil {
			s.logger.Error("failed to update instance state",
				"instance_id", c.InstanceID,
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
			"ip_address", inst.IpAddress,
			"vcpus", inst.Vcpus,
			"memory_mb", inst.Memory,
			"port", inst.DefaultPort,
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
