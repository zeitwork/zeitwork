package nodeagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/samber/lo"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/nodeagent/proxy"
	container "github.com/zeitwork/zeitwork/internal/nodeagent/runtime"
	"github.com/zeitwork/zeitwork/internal/nodeagent/runtime/docker"
	"github.com/zeitwork/zeitwork/internal/nodeagent/runtime/firecracker"
	"github.com/zeitwork/zeitwork/internal/nodeagent/utils"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type Config struct {
	NodeID           string `env:"NODE_ID"`
	NodeRegionID     string `env:"NODE_REGION_ID"`
	NodeDatabaseURL  string `env:"NODE_DATABASE_URL"`
	NodeRuntimeMode  string `env:"NODE_RUNTIME_MODE" envDefault:"docker"`
	NodeIPAddress    string `env:"NODE_IP_ADDRESS"`                    // External IP address of the node
	NodeProxyAddr    string `env:"NODE_PROXY_ADDR" envDefault:":8080"` // Reverse proxy listen address
	NodeRegistryURL  string `env:"NODE_REGISTRY_URL"`                  // Registry URL for pulling images (e.g., "ghcr.io/yourorg")
	NodeRegistryUser string `env:"NODE_REGISTRY_USER"`                 // Registry username for authentication
	NodeRegistryPass string `env:"NODE_REGISTRY_PASS"`                 // Registry password or token
}

type logEntry struct {
	level    string
	message  string
	loggedAt time.Time
}

type logBuffer struct {
	mu          sync.Mutex
	logs        []logEntry
	instanceID  pgtype.UUID
	flushThresh int
	db          *database.DB
	ctx         context.Context
	cancel      context.CancelFunc
}

type Service struct {
	cfg          Config
	db           *database.DB
	runtime      container.Runtime
	proxy        *proxy.Proxy
	logger       *slog.Logger
	nodeID       pgtype.UUID
	regionID     pgtype.UUID
	logStreams   map[string]context.CancelFunc // track log streaming goroutines
	logStreamsMu sync.Mutex
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
		dockerCfg := docker.Config{
			RegistryURL:  cfg.NodeRegistryURL,
			RegistryUser: cfg.NodeRegistryUser,
			RegistryPass: cfg.NodeRegistryPass,
		}
		rt, err = docker.NewDockerRuntime(dockerCfg, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create docker runtime: %w", err)
		}
	case "firecracker":
		fcCfg := firecracker.Config{
			FirecrackerBinary: "/opt/firecracker/firecracker",
			KernelImagePath:   "/opt/firecracker/vmlinux",
			KernelArgs:        "console=ttyS0 reboot=k panic=1 pci=off",
			ConfigDir:         "/var/lib/firecracker-runtime/configs",
			RootfsDir:         "/var/lib/firecracker-runtime/rootfs",
			SocketDir:         "/var/lib/firecracker-runtime/sockets",
			RegistryURL:       cfg.NodeRegistryURL,
			RegistryUser:      cfg.NodeRegistryUser,
			RegistryPass:      cfg.NodeRegistryPass,
		}
		rt, err = firecracker.NewFirecrackerRuntime(fcCfg, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create firecracker runtime: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown runtime mode: %s", cfg.NodeRuntimeMode)
	}

	// Initialize database
	db, err := database.NewDB(cfg.NodeDatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize proxy
	proxyCfg := proxy.Config{
		ListenAddr:     cfg.NodeProxyAddr,
		UpdateInterval: 10 * time.Second,
	}
	prx := proxy.NewProxy(proxyCfg, db, logger.With("component", "proxy"))

	svc := &Service{
		cfg:        cfg,
		db:         db,
		runtime:    rt,
		proxy:      prx,
		logger:     logger,
		nodeID:     nodeUUID,
		regionID:   regionUUID,
		logStreams: make(map[string]context.CancelFunc),
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

	// Start the reverse proxy
	if err := s.proxy.Start(context.Background()); err != nil {
		s.logger.Error("failed to start proxy", "error", err)
	}

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

	// Stop all log streaming
	s.logStreamsMu.Lock()
	for instanceID, cancel := range s.logStreams {
		s.logger.Info("stopping log streaming during shutdown", "instance_id", instanceID)
		cancel()
	}
	s.logStreams = make(map[string]context.CancelFunc)
	s.logStreamsMu.Unlock()

	// Stop proxy
	if s.proxy != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.proxy.Stop(ctx); err != nil {
			s.logger.Error("failed to stop proxy", "error", err)
		}
	}

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

		// Stop log streaming
		s.stopLogStreaming(instanceID)

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

		// Stop log streaming
		s.stopLogStreaming(c.InstanceID)

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

		// Get the actual IP address assigned by the runtime
		status, err := s.runtime.GetStatus(ctx, instanceID)
		if err != nil {
			s.logger.Error("failed to get container status",
				"instance_id", instanceID,
				"error", err,
			)
		} else if status != nil && status.IPAddress != inst.IpAddress {
			// Update IP address in database if it changed
			s.logger.Info("updating instance IP address",
				"instance_id", instanceID,
				"old_ip", inst.IpAddress,
				"new_ip", status.IPAddress,
			)

			if err := s.db.Queries().UpdateInstanceIPAddress(ctx, &database.UpdateInstanceIPAddressParams{
				ID:        inst.ID,
				IpAddress: status.IPAddress,
			}); err != nil {
				s.logger.Error("failed to update instance IP address",
					"instance_id", instanceID,
					"error", err,
				)
			}
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

		// Start log streaming for this instance
		s.startLogStreaming(instanceID, inst.ID)
	}

	s.logger.Info("reconciliation completed")
	return nil
}

// newLogBuffer creates a new log buffer for the given instance
func newLogBuffer(ctx context.Context, db *database.DB, instanceID pgtype.UUID) *logBuffer {
	ctx, cancel := context.WithCancel(ctx)
	lb := &logBuffer{
		logs:        make([]logEntry, 0, 100),
		instanceID:  instanceID,
		flushThresh: 100,
		db:          db,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start ticker to flush logs every 5 seconds
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				lb.flush()
			case <-ctx.Done():
				lb.flush() // Final flush on shutdown
				return
			}
		}
	}()

	return lb
}

// append adds a log entry to the buffer
func (lb *logBuffer) append(level, message string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.logs = append(lb.logs, logEntry{
		level:    level,
		message:  message,
		loggedAt: time.Now(),
	})

	// Flush if threshold reached
	if len(lb.logs) >= lb.flushThresh {
		go lb.flush()
	}
}

// flush writes all buffered logs to the database
func (lb *logBuffer) flush() {
	lb.mu.Lock()
	if len(lb.logs) == 0 {
		lb.mu.Unlock()
		return
	}

	// Copy logs to insert
	logsToInsert := make([]logEntry, len(lb.logs))
	copy(logsToInsert, lb.logs)
	lb.logs = lb.logs[:0] // Clear buffer
	lb.mu.Unlock()

	// Prepare batch insert
	params := make([]*database.InsertLogsParams, len(logsToInsert))
	for i, log := range logsToInsert {
		params[i] = &database.InsertLogsParams{
			ID:           uuid.New(),
			ImageBuildID: pgtype.UUID{Valid: false},
			InstanceID:   lb.instanceID,
			Level:        pgtype.Text{String: log.level, Valid: log.level != ""},
			Message:      log.message,
			LoggedAt:     pgtype.Timestamptz{Time: log.loggedAt, Valid: true},
		}
	}

	// Insert logs
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := lb.db.Queries().InsertLogs(ctx, params)
	if err != nil {
		// Log error but don't fail
		fmt.Fprintf(os.Stderr, "failed to insert instance logs: %v\n", err)
	}
}

// close stops the log buffer and flushes remaining logs
func (lb *logBuffer) close() {
	if lb.cancel != nil {
		lb.cancel()
	}
	lb.flush()
}

// startLogStreaming starts streaming logs for an instance
func (s *Service) startLogStreaming(instanceID string, instanceUUID pgtype.UUID) {
	s.logger.Info("starting log streaming", "instance_id", instanceID)

	// Check if already streaming
	s.logStreamsMu.Lock()
	if _, exists := s.logStreams[instanceID]; exists {
		s.logStreamsMu.Unlock()
		s.logger.Info("log streaming already active", "instance_id", instanceID)
		return
	}

	// Create context for this log stream
	ctx, cancel := context.WithCancel(context.Background())
	s.logStreams[instanceID] = cancel
	s.logStreamsMu.Unlock()

	// Create log buffer
	logBuf := newLogBuffer(ctx, s.db, instanceUUID)

	go func() {
		defer logBuf.close()

		// Try to stream logs
		logStream, err := s.runtime.StreamLogs(ctx, instanceID, true)
		if err != nil {
			s.logger.Warn("failed to start log streaming",
				"instance_id", instanceID,
				"error", err,
			)
			return
		}
		defer logStream.Close()

		// Read logs line by line
		scanner := bufio.NewScanner(logStream)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				line := scanner.Text()
				if line != "" {
					logBuf.append("info", line)
				}
			}
		}

		if err := scanner.Err(); err != nil {
			s.logger.Error("log streaming error",
				"instance_id", instanceID,
				"error", err,
			)
		}

		s.logger.Info("log streaming ended", "instance_id", instanceID)
	}()
}

// stopLogStreaming stops streaming logs for an instance
func (s *Service) stopLogStreaming(instanceID string) {
	s.logStreamsMu.Lock()
	defer s.logStreamsMu.Unlock()

	if cancel, exists := s.logStreams[instanceID]; exists {
		s.logger.Info("stopping log streaming", "instance_id", instanceID)
		cancel()
		delete(s.logStreams, instanceID)
	}
}
