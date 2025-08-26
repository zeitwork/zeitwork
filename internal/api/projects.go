package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
)

type Project struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	OrganizationID string `json:"organization_id"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type CreateProjectRequest struct {
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	OrganizationID string `json:"organization_id"`
}

type UpdateProjectRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// handleListProjects lists all projects for the authenticated user
func (s *Service) handleListProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("userID").(pgtype.UUID)

	// Get user's organizations
	memberships, err := s.db.Queries().OrganisationMemberFindByUser(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user organizations", "error", err, "userID", userID)
		http.Error(w, "Failed to get projects", http.StatusInternalServerError)
		return
	}

	var allProjects []*database.Project
	for _, membership := range memberships {
		projects, err := s.db.Queries().ProjectFindByOrganisation(ctx, membership.OrganisationID)
		if err != nil {
			s.logger.Error("Failed to get projects", "error", err, "orgID", membership.OrganisationID)
			continue
		}
		allProjects = append(allProjects, projects...)
	}

	// Convert to response format
	response := make([]Project, len(allProjects))
	for i, p := range allProjects {
		response[i] = Project{
			ID:             uuid.UUID(p.ID.Bytes).String(),
			Name:           p.Name,
			Slug:           p.Slug,
			OrganizationID: uuid.UUID(p.OrganisationID.Bytes).String(),
			CreatedAt:      p.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:      p.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCreateProject creates a new project
func (s *Service) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("userID").(pgtype.UUID)

	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" || req.Slug == "" || req.OrganizationID == "" {
		http.Error(w, "name, slug, and organization_id are required", http.StatusBadRequest)
		return
	}

	// Parse organization ID
	orgID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		http.Error(w, "Invalid organization ID", http.StatusBadRequest)
		return
	}
	pgOrgID := pgtype.UUID{Bytes: orgID, Valid: true}

	// Verify user is member of organization
	membership, err := s.db.Queries().OrganisationMemberFindByUserAndOrg(ctx, &database.OrganisationMemberFindByUserAndOrgParams{
		UserID:         userID,
		OrganisationID: pgOrgID,
	})
	if err != nil || membership == nil {
		http.Error(w, "Not authorized to create projects in this organization", http.StatusForbidden)
		return
	}

	// Create project
	params := database.ProjectCreateParams{
		Name:           req.Name,
		Slug:           req.Slug,
		OrganisationID: pgOrgID,
	}

	project, err := s.db.Queries().ProjectCreate(ctx, &params)
	if err != nil {
		s.logger.Error("Failed to create project", "error", err)
		http.Error(w, "Failed to create project", http.StatusInternalServerError)
		return
	}

	// Return created project
	response := Project{
		ID:             uuid.UUID(project.ID.Bytes).String(),
		Name:           project.Name,
		Slug:           project.Slug,
		OrganizationID: uuid.UUID(project.OrganisationID.Bytes).String(),
		CreatedAt:      project.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      project.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// handleGetProject gets a specific project
func (s *Service) handleGetProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("userID").(pgtype.UUID)

	// Get project ID from path
	projectIDStr := r.PathValue("id")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	pgProjectID := pgtype.UUID{Bytes: projectID, Valid: true}

	// Get project
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
		http.Error(w, "Not authorized to access this project", http.StatusForbidden)
		return
	}

	// Return project
	response := Project{
		ID:             uuid.UUID(project.ID.Bytes).String(),
		Name:           project.Name,
		Slug:           project.Slug,
		OrganizationID: uuid.UUID(project.OrganisationID.Bytes).String(),
		CreatedAt:      project.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      project.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleUpdateProject updates a project
func (s *Service) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("userID").(pgtype.UUID)

	// Get project ID from path
	projectIDStr := r.PathValue("id")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	pgProjectID := pgtype.UUID{Bytes: projectID, Valid: true}

	// Get project
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
		http.Error(w, "Not authorized to update this project", http.StatusForbidden)
		return
	}

	// Parse request
	var req UpdateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update project
	params := database.ProjectUpdateParams{
		ID:   pgProjectID,
		Name: req.Name,
		Slug: req.Slug,
	}

	updatedProject, err := s.db.Queries().ProjectUpdate(ctx, &params)
	if err != nil {
		s.logger.Error("Failed to update project", "error", err)
		http.Error(w, "Failed to update project", http.StatusInternalServerError)
		return
	}

	// Return updated project
	response := Project{
		ID:             uuid.UUID(updatedProject.ID.Bytes).String(),
		Name:           updatedProject.Name,
		Slug:           updatedProject.Slug,
		OrganizationID: uuid.UUID(updatedProject.OrganisationID.Bytes).String(),
		CreatedAt:      updatedProject.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      updatedProject.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDeleteProject deletes a project
func (s *Service) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("userID").(pgtype.UUID)

	// Get project ID from path
	projectIDStr := r.PathValue("id")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	pgProjectID := pgtype.UUID{Bytes: projectID, Valid: true}

	// Get project
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
		http.Error(w, "Not authorized to delete this project", http.StatusForbidden)
		return
	}

	// Delete project
	if err := s.db.Queries().ProjectDelete(ctx, pgProjectID); err != nil {
		s.logger.Error("Failed to delete project", "error", err)
		http.Error(w, "Failed to delete project", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
