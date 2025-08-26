package operator

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zeitwork/zeitwork/internal/database"
)

// Service represents the operator service that orchestrates the cluster
type Service struct {
	db     *database.DB
	logger *slog.Logger
	config *Config

	// HTTP client for communicating with node agents
	httpClient *http.Client
}

// Config holds the configuration for the operator service
type Config struct {
	Port          string
	DatabaseURL   string
	NodeAgentPort string // Default port for node agents
}

// NewService creates a new operator service
func NewService(config *Config, logger *slog.Logger) (*Service, error) {
	// Connect to database
	db, err := database.NewDB(config.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Service{
		db:         db,
		logger:     logger,
		config:     config,
		httpClient: &http.Client{},
	}, nil
}

// Start starts the operator service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting operator service", "port", s.config.Port)

	// Create HTTP server
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

	// Start build queue processor
	go s.processBuildQueue(ctx)

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown server
	s.logger.Info("Shutting down operator service")
	return server.Shutdown(context.Background())
}

// setupRoutes sets up the HTTP routes for the operator using Go 1.22 enhanced routing
func (s *Service) setupRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("GET /health", s.handleHealth)

	// Node management - using Go 1.22 pattern matching
	mux.HandleFunc("GET /api/v1/nodes", s.listNodes)
	mux.HandleFunc("POST /api/v1/nodes", s.createNode)
	mux.HandleFunc("GET /api/v1/nodes/{id}", s.getNode)
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", s.deleteNode)
	mux.HandleFunc("PUT /api/v1/nodes/{id}/state", s.updateNodeState)

	// Instance management
	mux.HandleFunc("GET /api/v1/instances", s.listInstances)
	mux.HandleFunc("POST /api/v1/instances", s.createInstance)
	mux.HandleFunc("GET /api/v1/instances/{id}", s.getInstance)
	mux.HandleFunc("PUT /api/v1/instances/{id}/state", s.updateInstanceState)
	mux.HandleFunc("DELETE /api/v1/instances/{id}", s.deleteInstance)

	// Image management
	mux.HandleFunc("GET /api/v1/images", s.listImages)
	mux.HandleFunc("POST /api/v1/images", s.createImage)
	mux.HandleFunc("GET /api/v1/images/{id}", s.getImage)
	mux.HandleFunc("PUT /api/v1/images/{id}/status", s.updateImageStatus)
	mux.HandleFunc("DELETE /api/v1/images/{id}", s.deleteImage)

	// Deployment management
	mux.HandleFunc("GET /api/v1/deployments", s.listDeployments)
	mux.HandleFunc("POST /api/v1/deployments", s.createDeployment)
	mux.HandleFunc("GET /api/v1/deployments/{id}", s.getDeployment)
	mux.HandleFunc("PUT /api/v1/deployments/{id}/status", s.updateDeploymentStatus)
}

// handleHealth handles health check requests
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy"}`))
}

func (s *Service) processBuildQueue(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Stopping build queue processor")
			return
		default:
			err := s.processOneBuild(ctx)
			if err != nil {
				s.logger.Error("Failed to process build", "error", err)
			}
			time.Sleep(5 * time.Second)
		}
	}
}

func (s *Service) processOneBuild(ctx context.Context) error {
	return s.db.WithTx(ctx, func(q *database.Queries) error {
		image, err := q.ImageDequeuePending(ctx)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil
			}
			return err
		}

		params := database.ImageUpdateStatusParams{
			ID:        image.ID,
			Status:    "building",
			ImageSize: image.ImageSize,
		}
		_, err = q.ImageUpdateStatus(ctx, &params)
		if err != nil {
			return err
		}

		// Process the build in background
		go s.buildImage(image)

		return nil
	})
}

func (s *Service) buildImage(image *database.Image) {
	// TODO: Implement actual build logic using Docker in Firecracker VM

	// Stub implementation
	time.Sleep(10 * time.Second)

	params := database.ImageUpdateStatusParams{
		ID:        image.ID,
		Status:    "ready",
		ImageSize: pgtype.Int4{Int32: 512, Valid: true}, // 512 MB
	}

	_, err := s.db.Queries().ImageUpdateStatus(context.Background(), &params)
	if err != nil {
		s.logger.Error("Failed to update image status after build", "error", err, "image_id", image.ID)
	}

	s.logger.Info("Image build completed", "image_id", image.ID, "name", image.Name)
}

// Close closes the operator service
func (s *Service) Close() error {
	if s.db != nil {
		s.db.Close()
	}
	return nil
}
