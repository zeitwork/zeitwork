package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/zeitwork/zeitwork/internal/database"
)

// Service represents the public API service
type Service struct {
	logger *slog.Logger
	config *Config
	db     *database.DB
	server *http.Server
}

// Config holds the configuration for the API service
type Config struct {
	Port        string
	DatabaseURL string
}

// NewService creates a new API service
func NewService(config *Config, db *database.DB, logger *slog.Logger) (*Service, error) {
	s := &Service{
		logger: logger,
		config: config,
		db:     db,
	}

	return s, nil
}

// Start starts the API service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting public API service",
		"port", s.config.Port,
		"base_url", s.config.BaseURL,
	)

	// Create HTTP server
	mux := http.NewServeMux()
	s.setupRoutes(mux)

	// Add CORS middleware
	handler := s.withCORS(mux)

	s.server = &http.Server{
		Addr:    ":" + s.config.Port,
		Handler: handler,
	}

	// Start server in goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Failed to start HTTP server", "error", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown server
	s.logger.Info("Shutting down API service")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return s.server.Shutdown(shutdownCtx)
}

// setupRoutes sets up the HTTP routes for the API
func (s *Service) setupRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("GET /health", s.handleHealth)

	// Basic CRUD operations - simplified
	mux.HandleFunc("GET /v1/projects", s.handleListProjects)
	mux.HandleFunc("POST /v1/projects", s.handleCreateProject)
	mux.HandleFunc("GET /v1/projects/{id}", s.handleGetProject)

	mux.HandleFunc("GET /v1/deployments", s.handleListDeployments)
	mux.HandleFunc("POST /v1/deployments", s.handleCreateDeployment)
	mux.HandleFunc("GET /v1/deployments/{id}", s.handleGetDeployment)
}

// handleHealth handles health check requests
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status": "healthy",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Simplified handlers - basic CRUD operations only
func (s *Service) handleListProjects(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement basic project listing
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]interface{}{})
}

func (s *Service) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement basic project creation
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}

func (s *Service) handleGetProject(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement basic project retrieval
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": r.PathValue("id")})
}

func (s *Service) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement basic deployment listing
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]interface{}{})
}

func (s *Service) handleCreateDeployment(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement basic deployment creation
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}

func (s *Service) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement basic deployment retrieval
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": r.PathValue("id")})
}
