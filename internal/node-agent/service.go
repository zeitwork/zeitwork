package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Service represents the node agent service that runs on each compute node
type Service struct {
	logger     *slog.Logger
	config     *Config
	httpClient *http.Client
	nodeID     uuid.UUID

	// Firecracker VM management
	instances map[string]*Instance // instance_id -> instance
}

// Config holds the configuration for the node agent service
type Config struct {
	Port              string
	OperatorURL       string
	NodeID            string
	FirecrackerBin    string
	FirecrackerSocket string
	VMWorkDir         string
}

// Instance represents a running VM instance
type Instance struct {
	ID        string
	ImageID   string
	State     string
	Resources struct {
		VCPU   int `json:"vcpu"`
		Memory int `json:"memory"`
	}
	IPAddress string
	Process   *FirecrackerProcess
}

// FirecrackerProcess represents a running Firecracker process
type FirecrackerProcess struct {
	PID        int
	SocketPath string
	LogFile    string
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
		// Generate a new node ID if not provided
		nodeID = uuid.New()
		logger.Info("Generated new node ID", "node_id", nodeID)
	}

	return &Service{
		logger:     logger,
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		nodeID:     nodeID,
		instances:  make(map[string]*Instance),
	}, nil
}

// Start starts the node agent service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting node agent service",
		"port", s.config.Port,
		"node_id", s.nodeID,
		"operator_url", s.config.OperatorURL,
	)

	// Register with operator
	if err := s.registerWithOperator(ctx); err != nil {
		return fmt.Errorf("failed to register with operator: %w", err)
	}

	// Start health reporting goroutine
	go s.reportHealthPeriodically(ctx)

	// Create HTTP server for receiving commands from operator
	mux := http.NewServeMux()
	s.setupRoutes(mux)

	server := &http.Server{
		Addr:    ":" + s.config.Port,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Failed to start HTTP server", "error", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown server
	s.logger.Info("Shutting down node agent service")

	// Stop all running instances
	s.stopAllInstances()

	// Deregister from operator
	s.deregisterFromOperator()

	return server.Shutdown(context.Background())
}

// setupRoutes sets up the HTTP routes for the node agent
func (s *Service) setupRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("GET /health", s.handleHealth)

	// Instance management endpoints (called by operator)
	mux.HandleFunc("POST /instances", s.handleCreateInstance)
	mux.HandleFunc("GET /instances/{id}", s.handleGetInstance)
	mux.HandleFunc("PUT /instances/{id}/state", s.handleUpdateInstanceState)
	mux.HandleFunc("DELETE /instances/{id}", s.handleDeleteInstance)

	// Node information
	mux.HandleFunc("GET /node/info", s.handleNodeInfo)
	mux.HandleFunc("GET /node/resources", s.handleNodeResources)
}

// handleHealth handles health check requests
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"node_id":   s.nodeID,
		"instances": len(s.instances),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// registerWithOperator registers this node with the operator
func (s *Service) registerWithOperator(ctx context.Context) error {
	// Get system information
	hostname := "node-" + s.nodeID.String()[:8]
	ipAddress := s.getNodeIPAddress()

	// Prepare registration request
	registration := map[string]interface{}{
		"hostname":   hostname,
		"ip_address": ipAddress,
		"resources": map[string]int{
			"vcpu":   4,    // TODO: Get actual CPU count
			"memory": 8192, // TODO: Get actual memory
		},
	}

	body, err := json.Marshal(registration)
	if err != nil {
		return err
	}

	// Send registration request to operator
	req, err := http.NewRequestWithContext(ctx, "POST",
		s.config.OperatorURL+"/api/v1/nodes", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-ID", s.nodeID.String())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send registration: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registration failed with status %d", resp.StatusCode)
	}

	s.logger.Info("Successfully registered with operator")
	return nil
}

// reportHealthPeriodically sends periodic health reports to the operator
func (s *Service) reportHealthPeriodically(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reportHealth(ctx)
		}
	}
}

// reportHealth sends a health report to the operator
func (s *Service) reportHealth(ctx context.Context) {
	// Prepare health report
	health := map[string]interface{}{
		"state": "ready",
		"resources": map[string]interface{}{
			"vcpu_available":   4,    // TODO: Calculate available resources
			"memory_available": 4096, // TODO: Calculate available memory
		},
	}

	body, err := json.Marshal(health)
	if err != nil {
		s.logger.Error("Failed to marshal health report", "error", err)
		return
	}

	// Send health report to operator
	req, err := http.NewRequestWithContext(ctx, "PUT",
		fmt.Sprintf("%s/api/v1/nodes/%s/state", s.config.OperatorURL, s.nodeID),
		bytes.NewReader(body))
	if err != nil {
		s.logger.Error("Failed to create health report request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error("Failed to send health report", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.logger.Warn("Health report failed", "status", resp.StatusCode)
	}
}

// deregisterFromOperator deregisters this node from the operator
func (s *Service) deregisterFromOperator() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("%s/api/v1/nodes/%s", s.config.OperatorURL, s.nodeID), nil)
	if err != nil {
		s.logger.Error("Failed to create deregistration request", "error", err)
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error("Failed to deregister from operator", "error", err)
		return
	}
	defer resp.Body.Close()

	s.logger.Info("Deregistered from operator")
}

// stopAllInstances stops all running VM instances
func (s *Service) stopAllInstances() {
	s.logger.Info("Stopping all instances", "count", len(s.instances))

	for id, instance := range s.instances {
		s.logger.Info("Stopping instance", "instance_id", id)
		// TODO: Implement actual instance stopping
		instance.State = "stopped"
	}
}

// getNodeIPAddress gets the IP address of this node
func (s *Service) getNodeIPAddress() string {
	// TODO: Implement actual IP address detection
	return "10.0.1.1"
}

// Close closes the node agent service
func (s *Service) Close() error {
	s.stopAllInstances()
	return nil
}
