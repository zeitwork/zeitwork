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
)

// Service represents the node agent service that runs on each compute node
type Service struct {
	logger     *slog.Logger
	config     *Config
	httpClient *http.Client
	nodeID     uuid.UUID
	mu         sync.RWMutex

	// Simple VM instance tracking
	instances map[string]*Instance
}

// Config holds the configuration for the node agent service
type Config struct {
	Port        string
	OperatorURL string
	NodeID      string
}

// Instance represents a running VM instance
type Instance struct {
	ID      string
	ImageID string
	State   string
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
	)

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
	return server.Shutdown(context.Background())
}

// setupRoutes sets up the HTTP routes for the node agent
func (s *Service) setupRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("GET /health", s.handleHealth)

	// Instance management endpoints
	mux.HandleFunc("POST /instances", s.handleCreateInstance)
	mux.HandleFunc("GET /instances/{id}", s.handleGetInstance)
	mux.HandleFunc("DELETE /instances/{id}", s.handleDeleteInstance)

	// Node information
	mux.HandleFunc("GET /node/info", s.handleNodeInfo)
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

// handleCreateInstance handles instance creation requests
func (s *Service) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InstanceID string `json:"instance_id"`
		ImageID    string `json:"image_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.instances[req.InstanceID] = &Instance{
		ID:      req.InstanceID,
		ImageID: req.ImageID,
		State:   "running",
	}
	s.mu.Unlock()

	s.logger.Info("Created instance", "instance_id", req.InstanceID, "image_id", req.ImageID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}

// handleGetInstance handles instance retrieval requests
func (s *Service) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")

	s.mu.RLock()
	instance, exists := s.instances[instanceID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(instance)
}

// handleDeleteInstance handles instance deletion requests
func (s *Service) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")

	s.mu.Lock()
	if _, exists := s.instances[instanceID]; !exists {
		s.mu.Unlock()
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}
	delete(s.instances, instanceID)
	s.mu.Unlock()

	s.logger.Info("Deleted instance", "instance_id", instanceID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// handleNodeInfo handles node information requests
func (s *Service) handleNodeInfo(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"node_id":   s.nodeID,
		"instances": len(s.instances),
		"status":    "ready",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Close closes the node agent service
func (s *Service) Close() error {
	return nil
}
