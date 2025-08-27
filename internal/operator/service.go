package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/health"
)

// Service represents the operator service that orchestrates the cluster
type Service struct {
	db     *database.DB
	logger *slog.Logger
	config *Config

	// HTTP client for communicating with node agents
	httpClient *http.Client

	// Deployment manager for handling deployments
	deploymentManager *DeploymentManager

	// Scaling manager for auto-scaling
	scalingManager *ScalingManager

	// Health monitoring
	healthHandler *health.Handler
	healthMonitor *health.Monitor
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

	s := &Service{
		db:         db,
		logger:     logger,
		config:     config,
		httpClient: &http.Client{},
	}

	// Initialize deployment manager
	s.deploymentManager = NewDeploymentManager(s)

	// Initialize scaling manager
	s.scalingManager = NewScalingManager(s, logger)

	// Initialize health monitoring
	s.healthHandler = health.NewHandler()

	// Add health checks
	s.healthHandler.AddCheck("database", health.DatabaseCheck(db))
	s.healthHandler.AddCheck("scaling", func(ctx context.Context) error {
		// Check if scaling manager is running
		if s.scalingManager == nil {
			return fmt.Errorf("scaling manager not initialized")
		}
		return nil
	})

	// Add readiness check
	s.healthHandler.AddReadinessCheck(func(ctx context.Context) error {
		// Service is ready when database is available
		return db.Ping(ctx)
	})

	// Add liveness check
	s.healthHandler.AddLivenessCheck(func(ctx context.Context) error {
		// Service is alive if it can respond
		return nil
	})

	// Initialize health monitor
	if config.DatabaseURL != "" {
		monitorConfig := &health.Config{
			DatabaseURL:        config.DatabaseURL,
			CheckInterval:      30 * time.Second,
			UnhealthyThreshold: 3,
		}
		monitor, err := health.NewMonitor(monitorConfig, logger)
		if err != nil {
			logger.Warn("Failed to create health monitor", "error", err)
		} else {
			s.healthMonitor = monitor
			s.healthHandler.SetMonitor(monitor)
		}
	}

	return s, nil
}

// Start starts the operator service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting operator service", "port", s.config.Port)

	// Start scaling manager
	go s.scalingManager.Start(ctx)

	// Start health monitor
	if s.healthMonitor != nil {
		go s.healthMonitor.Start(ctx)
	}

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
	// Health monitoring endpoints
	if s.healthHandler != nil {
		mux.HandleFunc("GET /health", s.healthHandler.HandleHealth)
		mux.HandleFunc("GET /ready", s.healthHandler.HandleReady)
		mux.HandleFunc("GET /live", s.healthHandler.HandleLive)
		mux.HandleFunc("GET /metrics", s.healthHandler.HandleMetrics)
		mux.HandleFunc("GET /status", s.healthHandler.HandleStatus)
	} else {
		// Fallback simple health check
		mux.HandleFunc("GET /health", s.handleHealth)
	}

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
	mux.HandleFunc("POST /api/v1/deployments/{id}/deploy", s.startDeployment)
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
	ctx := context.Background()

	// Parse repository
	var repo struct {
		Type string `json:"type"`
		Repo string `json:"repo"`
	}
	if err := json.Unmarshal(image.Repository, &repo); err != nil || repo.Type != "github" {
		s.logger.Error("Invalid repository", "image_id", image.ID)
		s.failBuild(image.ID, "Invalid repository")
		return
	}

	// Select node for building (with at least 2 vCPU and 4GB memory available)
	nodes, err := s.db.Queries().NodeFindByState(ctx, "ready")
	if err != nil {
		s.logger.Error("Failed to find nodes", "error", err)
		s.failBuild(image.ID, "No available nodes")
		return
	}

	var selectedNode *database.Node
	for _, node := range nodes {
		var res struct {
			Available struct {
				VCPU   int `json:"vcpu"`
				Memory int `json:"memory"`
			} `json:"available"`
		}
		if err := json.Unmarshal(node.Resources, &res); err == nil {
			if res.Available.VCPU >= 2 && res.Available.Memory >= 4096 {
				selectedNode = node
				break
			}
		}
	}

	if selectedNode == nil {
		s.logger.Error("No suitable node found", "image_id", image.ID)
		s.failBuild(image.ID, "No suitable build node")
		return
	}

	// Send build request to node agent
	buildReq := map[string]string{
		"image_id":    uuid.UUID(image.ID.Bytes).String(),
		"github_repo": repo.Repo,
	}
	body, _ := json.Marshal(buildReq)

	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%s/api/v1/build", selectedNode.IpAddress, s.config.NodeAgentPort), strings.NewReader(string(body)))
	if err != nil {
		s.failBuild(image.ID, "Failed to create request")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusAccepted {
		s.logger.Error("Build request failed", "error", err, "status", resp.StatusCode)
		s.failBuild(image.ID, "Build initiation failed")
		return
	}

	// Monitor build status (poll or wait for callback)
	// For now, stub with sleep and success
	time.Sleep(30 * time.Second)

	params := database.ImageUpdateStatusParams{
		ID:        image.ID,
		Status:    "ready",
		ImageSize: pgtype.Int4{Int32: 512, Valid: true},
	}
	_, err = s.db.Queries().ImageUpdateStatus(ctx, &params)
	if err != nil {
		s.logger.Error("Failed to update status", "error", err)
	}

	s.logger.Info("Build completed", "image_id", image.ID)
}

func (s *Service) failBuild(imageID pgtype.UUID, reason string) {
	params := database.ImageUpdateStatusParams{
		ID:        imageID,
		Status:    "failed",
		ImageSize: pgtype.Int4{Int32: 0, Valid: true},
	}
	s.db.Queries().ImageUpdateStatus(context.Background(), &params)
}

// Close closes the operator service
func (s *Service) Close() error {
	if s.db != nil {
		s.db.Close()
	}
	return nil
}

// listDeployments handles GET /api/v1/deployments
func (s *Service) listDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get query parameters for filtering
	projectID := r.URL.Query().Get("project_id")
	status := r.URL.Query().Get("status")

	var deployments []*database.Deployment
	var err error

	if projectID != "" {
		// Filter by project
		projectUUID, _ := uuid.Parse(projectID)
		deployments, err = s.db.Queries().DeploymentFindByProject(ctx, pgtype.UUID{
			Bytes: projectUUID,
			Valid: true,
		})
	} else {
		// Get all deployments
		deployments, err = s.db.Queries().DeploymentList(ctx)
	}

	if err != nil {
		http.Error(w, "Failed to list deployments", http.StatusInternalServerError)
		return
	}

	// Filter by status if provided
	if status != "" {
		var filtered []*database.Deployment
		for _, d := range deployments {
			if d.Status == status {
				filtered = append(filtered, d)
			}
		}
		deployments = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deployments)
}

// createDeployment handles POST /api/v1/deployments
func (s *Service) createDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		ProjectID    string `json:"project_id"`
		CommitHash   string `json:"commit_hash"`
		ImageID      string `json:"image_id"`
		MinInstances int    `json:"min_instances"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Create deployment in database
	projectUUID, _ := uuid.Parse(req.ProjectID)
	imageUUID, _ := uuid.Parse(req.ImageID)

	deployment, err := s.db.Queries().DeploymentCreate(ctx, &database.DeploymentCreateParams{
		ProjectID:    pgtype.UUID{Bytes: projectUUID, Valid: true},
		CommitHash:   req.CommitHash,
		ImageID:      pgtype.UUID{Bytes: imageUUID, Valid: true},
		Status:       "pending",
		MinInstances: int32(req.MinInstances),
	})

	if err != nil {
		http.Error(w, "Failed to create deployment", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(deployment)
}

// getDeployment handles GET /api/v1/deployments/{id}
func (s *Service) getDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	deploymentID := r.PathValue("id")

	deploymentUUID, err := uuid.Parse(deploymentID)
	if err != nil {
		http.Error(w, "Invalid deployment ID", http.StatusBadRequest)
		return
	}

	deployment, err := s.db.Queries().DeploymentFindById(ctx, pgtype.UUID{
		Bytes: deploymentUUID,
		Valid: true,
	})

	if err != nil {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deployment)
}

// updateDeploymentStatus handles PUT /api/v1/deployments/{id}/status
func (s *Service) updateDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	deploymentID := r.PathValue("id")

	var req struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	deploymentUUID, err := uuid.Parse(deploymentID)
	if err != nil {
		http.Error(w, "Invalid deployment ID", http.StatusBadRequest)
		return
	}

	// Update deployment status
	deployment, err := s.db.Queries().DeploymentUpdateStatus(ctx, &database.DeploymentUpdateStatusParams{
		ID:          pgtype.UUID{Bytes: deploymentUUID, Valid: true},
		Status:      req.Status,
		ActivatedAt: pgtype.Timestamptz{},
	})

	if err != nil {
		http.Error(w, "Failed to update deployment status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deployment)
}

// startDeployment handles POST /api/v1/deployments/{id}/deploy
func (s *Service) startDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	deploymentID := r.PathValue("id")

	deploymentUUID, err := uuid.Parse(deploymentID)
	if err != nil {
		http.Error(w, "Invalid deployment ID", http.StatusBadRequest)
		return
	}

	// Get deployment details
	deployment, err := s.db.Queries().DeploymentFindById(ctx, pgtype.UUID{
		Bytes: deploymentUUID,
		Valid: true,
	})

	if err != nil {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	// Start deployment workflow
	err = s.deploymentManager.StartDeploymentWorkflow(ctx, deploymentID, uuid.UUID(deployment.ImageID.Bytes).String())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start deployment: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"message":       "Deployment started",
		"deployment_id": deploymentID,
	})
}
