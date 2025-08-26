package api

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
)

type Deployment struct {
	ID                   string  `json:"id"`
	ProjectID            string  `json:"project_id"`
	ProjectEnvironmentID string  `json:"project_environment_id"`
	Status               string  `json:"status"`
	CommitHash           string  `json:"commit_hash"`
	ImageID              string  `json:"image_id,omitempty"`
	DeploymentURL        string  `json:"deployment_url"`
	NanoID               string  `json:"nano_id"`
	MinInstances         int     `json:"min_instances"`
	ActivatedAt          *string `json:"activated_at,omitempty"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

type CreateDeploymentRequest struct {
	ProjectID            string `json:"project_id"`
	ProjectEnvironmentID string `json:"project_environment_id,omitempty"`
	CommitHash           string `json:"commit_hash"`
	Branch               string `json:"branch"`
}

type UpdateDeploymentStatusRequest struct {
	Status string `json:"status"`
}

// handleListDeployments lists deployments for the authenticated user
func (s *Service) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("userID").(pgtype.UUID)

	// Get optional project filter
	projectID := r.URL.Query().Get("project_id")

	// Get user's organizations
	memberships, err := s.db.Queries().OrganisationMemberFindByUser(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user organizations", "error", err)
		http.Error(w, "Failed to get deployments", http.StatusInternalServerError)
		return
	}

	var allDeployments []*database.Deployment

	if projectID != "" {
		// Get deployments for specific project
		pid, err := uuid.Parse(projectID)
		if err != nil {
			http.Error(w, "Invalid project ID", http.StatusBadRequest)
			return
		}
		pgProjectID := pgtype.UUID{Bytes: pid, Valid: true}

		// Verify user has access to project
		project, err := s.db.Queries().ProjectFindById(ctx, pgProjectID)
		if err != nil {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}

		// Check if user is member of org
		hasAccess := false
		for _, m := range memberships {
			if m.OrganisationID == project.OrganisationID {
				hasAccess = true
				break
			}
		}

		if !hasAccess {
			http.Error(w, "Not authorized to view these deployments", http.StatusForbidden)
			return
		}

		deployments, err := s.db.Queries().DeploymentFindByProject(ctx, pgProjectID)
		if err != nil {
			s.logger.Error("Failed to get deployments", "error", err)
			http.Error(w, "Failed to get deployments", http.StatusInternalServerError)
			return
		}
		allDeployments = deployments
	} else {
		// Get all deployments for user's organizations
		for _, membership := range memberships {
			deployments, err := s.db.Queries().DeploymentFindByOrganisation(ctx, membership.OrganisationID)
			if err != nil {
				s.logger.Error("Failed to get deployments", "error", err, "orgID", membership.OrganisationID)
				continue
			}
			allDeployments = append(allDeployments, deployments...)
		}
	}

	// Convert to response format
	response := make([]Deployment, len(allDeployments))
	for i, d := range allDeployments {
		dep := Deployment{
			ID:            uuid.UUID(d.ID.Bytes).String(),
			ProjectID:     uuid.UUID(d.ProjectID.Bytes).String(),
			Status:        d.Status,
			CommitHash:    d.CommitHash,
			DeploymentURL: d.DeploymentUrl.String,
			NanoID:        d.Nanoid.String,
			MinInstances:  int(d.MinInstances),
			CreatedAt:     d.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:     d.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
		}

		if d.ProjectEnvironmentID.Valid {
			envID := uuid.UUID(d.ProjectEnvironmentID.Bytes).String()
			dep.ProjectEnvironmentID = envID
		}

		if d.ImageID.Valid {
			imgID := uuid.UUID(d.ImageID.Bytes).String()
			dep.ImageID = imgID
		}

		if d.ActivatedAt.Valid {
			activatedAt := d.ActivatedAt.Time.Format("2006-01-02T15:04:05Z")
			dep.ActivatedAt = &activatedAt
		}

		response[i] = dep
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCreateDeployment creates a new deployment
func (s *Service) handleCreateDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("userID").(pgtype.UUID)

	var req CreateDeploymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ProjectID == "" || req.CommitHash == "" {
		http.Error(w, "project_id and commit_hash are required", http.StatusBadRequest)
		return
	}

	// Parse project ID
	projectID, err := uuid.Parse(req.ProjectID)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}
	pgProjectID := pgtype.UUID{Bytes: projectID, Valid: true}

	// Get project to verify access
	project, err := s.db.Queries().ProjectFindById(ctx, pgProjectID)
	if err != nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// Verify user has access
	membership, err := s.db.Queries().OrganisationMemberFindByUserAndOrg(ctx, &database.OrganisationMemberFindByUserAndOrgParams{
		UserID:         userID,
		OrganisationID: project.OrganisationID,
	})
	if err != nil || membership == nil {
		http.Error(w, "Not authorized to create deployments for this project", http.StatusForbidden)
		return
	}

	// Get or create production environment
	var envID pgtype.UUID
	if req.ProjectEnvironmentID != "" {
		eid, err := uuid.Parse(req.ProjectEnvironmentID)
		if err != nil {
			http.Error(w, "Invalid environment ID", http.StatusBadRequest)
			return
		}
		envID = pgtype.UUID{Bytes: eid, Valid: true}
	} else {
		// Get production environment
		envs, err := s.db.Queries().ProjectEnvironmentFindByProject(ctx, pgProjectID)
		if err != nil {
			s.logger.Error("Failed to get environments", "error", err)
			http.Error(w, "Failed to get environments", http.StatusInternalServerError)
			return
		}

		for _, env := range envs {
			if env.Name == "production" {
				envID = env.ID
				break
			}
		}

		if !envID.Valid {
			// Create production environment
			env, err := s.db.Queries().ProjectEnvironmentCreate(ctx, &database.ProjectEnvironmentCreateParams{
				ProjectID:      pgProjectID,
				Name:           "production",
				OrganisationID: project.OrganisationID,
			})
			if err != nil {
				s.logger.Error("Failed to create environment", "error", err)
				http.Error(w, "Failed to create environment", http.StatusInternalServerError)
				return
			}
			envID = env.ID
		}
	}

	// Generate nano ID for deployment URL
	nanoID := generateNanoID(7)

	// Get organization for deployment URL
	org, err := s.db.Queries().OrganisationFindById(ctx, project.OrganisationID)
	if err != nil {
		s.logger.Error("Failed to get organization", "error", err)
		http.Error(w, "Failed to create deployment", http.StatusInternalServerError)
		return
	}

	deploymentURL := fmt.Sprintf("%s-%s-%s.zeitwork.app", project.Slug, nanoID, org.Slug)

	// Create deployment
	params := database.DeploymentCreateParams{
		ProjectID:            pgProjectID,
		ProjectEnvironmentID: envID,
		Status:               "pending",
		CommitHash:           req.CommitHash,
		ImageID:              pgtype.UUID{}, // Will be set later when image is built
		OrganisationID:       project.OrganisationID,
		DeploymentUrl:        pgtype.Text{String: deploymentURL, Valid: true},
		Nanoid:               pgtype.Text{String: nanoID, Valid: true},
		MinInstances:         3, // Default 3 instances per region
		RolloutStrategy:      "blue-green",
	}

	deployment, err := s.db.Queries().DeploymentCreate(ctx, &params)
	if err != nil {
		s.logger.Error("Failed to create deployment", "error", err)
		http.Error(w, "Failed to create deployment", http.StatusInternalServerError)
		return
	}

	// Trigger build process
	// TODO: Add to build queue

	// Return created deployment
	response := Deployment{
		ID:                   uuid.UUID(deployment.ID.Bytes).String(),
		ProjectID:            uuid.UUID(deployment.ProjectID.Bytes).String(),
		ProjectEnvironmentID: uuid.UUID(deployment.ProjectEnvironmentID.Bytes).String(),
		Status:               deployment.Status,
		CommitHash:           deployment.CommitHash,
		DeploymentURL:        deployment.DeploymentUrl.String,
		NanoID:               deployment.Nanoid.String,
		MinInstances:         int(deployment.MinInstances),
		CreatedAt:            deployment.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:            deployment.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// handleGetDeployment gets a specific deployment
func (s *Service) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("userID").(pgtype.UUID)

	// Get deployment ID from path
	deploymentIDStr := r.PathValue("id")
	deploymentID, err := uuid.Parse(deploymentIDStr)
	if err != nil {
		http.Error(w, "Invalid deployment ID", http.StatusBadRequest)
		return
	}

	pgDeploymentID := pgtype.UUID{Bytes: deploymentID, Valid: true}

	// Get deployment
	deployment, err := s.db.Queries().DeploymentFindById(ctx, pgDeploymentID)
	if err != nil {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	// Verify user has access
	membership, err := s.db.Queries().OrganisationMemberFindByUserAndOrg(ctx, &database.OrganisationMemberFindByUserAndOrgParams{
		UserID:         userID,
		OrganisationID: deployment.OrganisationID,
	})
	if err != nil || membership == nil {
		http.Error(w, "Not authorized to access this deployment", http.StatusForbidden)
		return
	}

	// Return deployment
	response := Deployment{
		ID:            uuid.UUID(deployment.ID.Bytes).String(),
		ProjectID:     uuid.UUID(deployment.ProjectID.Bytes).String(),
		Status:        deployment.Status,
		CommitHash:    deployment.CommitHash,
		DeploymentURL: deployment.DeploymentUrl.String,
		NanoID:        deployment.Nanoid.String,
		MinInstances:  int(deployment.MinInstances),
		CreatedAt:     deployment.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     deployment.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}

	if deployment.ProjectEnvironmentID.Valid {
		response.ProjectEnvironmentID = uuid.UUID(deployment.ProjectEnvironmentID.Bytes).String()
	}

	if deployment.ImageID.Valid {
		response.ImageID = uuid.UUID(deployment.ImageID.Bytes).String()
	}

	if deployment.ActivatedAt.Valid {
		activatedAt := deployment.ActivatedAt.Time.Format("2006-01-02T15:04:05Z")
		response.ActivatedAt = &activatedAt
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleUpdateDeploymentStatus updates a deployment's status
func (s *Service) handleUpdateDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("userID").(pgtype.UUID)

	// Get deployment ID from path
	deploymentIDStr := r.PathValue("id")
	deploymentID, err := uuid.Parse(deploymentIDStr)
	if err != nil {
		http.Error(w, "Invalid deployment ID", http.StatusBadRequest)
		return
	}

	pgDeploymentID := pgtype.UUID{Bytes: deploymentID, Valid: true}

	// Get deployment
	deployment, err := s.db.Queries().DeploymentFindById(ctx, pgDeploymentID)
	if err != nil {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	// Verify user has access
	membership, err := s.db.Queries().OrganisationMemberFindByUserAndOrg(ctx, &database.OrganisationMemberFindByUserAndOrgParams{
		UserID:         userID,
		OrganisationID: deployment.OrganisationID,
	})
	if err != nil || membership == nil {
		http.Error(w, "Not authorized to update this deployment", http.StatusForbidden)
		return
	}

	// Parse request
	var req UpdateDeploymentStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate status
	validStatuses := []string{"pending", "building", "deploying", "active", "inactive", "failed"}
	isValid := false
	for _, status := range validStatuses {
		if req.Status == status {
			isValid = true
			break
		}
	}
	if !isValid {
		http.Error(w, "Invalid status", http.StatusBadRequest)
		return
	}

	// Set activation time if activating
	var activatedAt pgtype.Timestamptz
	if req.Status == "active" && !deployment.ActivatedAt.Valid {
		activatedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}

	// Update deployment
	params := database.DeploymentUpdateStatusParams{
		ID:          pgDeploymentID,
		Status:      req.Status,
		ActivatedAt: activatedAt,
	}

	updatedDeployment, err := s.db.Queries().DeploymentUpdateStatus(ctx, &params)
	if err != nil {
		s.logger.Error("Failed to update deployment", "error", err)
		http.Error(w, "Failed to update deployment", http.StatusInternalServerError)
		return
	}

	// Return updated deployment
	response := Deployment{
		ID:            uuid.UUID(updatedDeployment.ID.Bytes).String(),
		ProjectID:     uuid.UUID(updatedDeployment.ProjectID.Bytes).String(),
		Status:        updatedDeployment.Status,
		CommitHash:    updatedDeployment.CommitHash,
		DeploymentURL: updatedDeployment.DeploymentUrl.String,
		NanoID:        updatedDeployment.Nanoid.String,
		MinInstances:  int(updatedDeployment.MinInstances),
		CreatedAt:     updatedDeployment.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     updatedDeployment.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}

	if updatedDeployment.ActivatedAt.Valid {
		activatedAt := updatedDeployment.ActivatedAt.Time.Format("2006-01-02T15:04:05Z")
		response.ActivatedAt = &activatedAt
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// generateNanoID generates a random nano ID
func generateNanoID(length int) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, length)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(b)
}
