package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	Port           string
	DatabaseURL    string
	GitHubClientID string
	GitHubSecret   string
	JWTSecret      string
	BaseURL        string
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

	// Authentication
	mux.HandleFunc("GET /v1/auth/github", s.handleGitHubAuth)
	mux.HandleFunc("GET /v1/auth/github/callback", s.handleGitHubCallback)
	mux.HandleFunc("POST /v1/auth/logout", s.handleLogout)

	// Waitlist
	mux.HandleFunc("POST /v1/waitlist", s.handleWaitlistSignup)

	// Projects (authenticated)
	mux.HandleFunc("GET /v1/projects", s.withAuth(s.handleListProjects))
	mux.HandleFunc("POST /v1/projects", s.withAuth(s.handleCreateProject))
	mux.HandleFunc("GET /v1/projects/{id}", s.withAuth(s.handleGetProject))
	mux.HandleFunc("PUT /v1/projects/{id}", s.withAuth(s.handleUpdateProject))
	mux.HandleFunc("DELETE /v1/projects/{id}", s.withAuth(s.handleDeleteProject))

	// Deployments (authenticated)
	mux.HandleFunc("GET /v1/deployments", s.withAuth(s.handleListDeployments))
	mux.HandleFunc("POST /v1/deployments", s.withAuth(s.handleCreateDeployment))
	mux.HandleFunc("GET /v1/deployments/{id}", s.withAuth(s.handleGetDeployment))
	mux.HandleFunc("PUT /v1/deployments/{id}/status", s.withAuth(s.handleUpdateDeploymentStatus))

	// GitHub webhook
	mux.HandleFunc("POST /v1/webhook/github", s.handleGitHubWebhook)

	// Organizations (authenticated)
	mux.HandleFunc("GET /v1/organizations", s.withAuth(s.handleListOrganizations))
	mux.HandleFunc("POST /v1/organizations", s.withAuth(s.handleCreateOrganization))
	mux.HandleFunc("GET /v1/organizations/{id}", s.withAuth(s.handleGetOrganization))
}

// handleHealth handles health check requests
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check database connection
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	healthy := true
	var dbStatus string

	if err := s.db.Ping(ctx); err != nil {
		healthy = false
		dbStatus = fmt.Sprintf("database error: %v", err)
	} else {
		dbStatus = "connected"
	}

	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}

	response := map[string]interface{}{
		"status":   "healthy",
		"database": dbStatus,
		"version":  "1.0.0",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}

// withCORS adds CORS headers to responses
func (s *Service) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*") // Configure this properly in production
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// withAuth middleware checks for valid authentication
func (s *Service) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Validate token
		token := authHeader[7:] // Remove "Bearer " prefix
		userID, err := s.validateToken(r.Context(), token)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Add user ID to context
		ctx := context.WithValue(r.Context(), "userID", userID)
		next(w, r.WithContext(ctx))
	}
}
