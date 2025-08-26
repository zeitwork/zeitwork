package api

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
)

type Organization struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type CreateOrganizationRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// handleListOrganizations lists organizations for the authenticated user
func (s *Service) handleListOrganizations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("userID").(pgtype.UUID)

	// Get user's organization memberships
	memberships, err := s.db.Queries().OrganisationMemberFindByUser(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user organizations", "error", err, "userID", userID)
		http.Error(w, "Failed to get organizations", http.StatusInternalServerError)
		return
	}

	// Get organization details
	organizations := make([]Organization, 0, len(memberships))
	for _, membership := range memberships {
		org, err := s.db.Queries().OrganisationFindById(ctx, membership.OrganisationID)
		if err != nil {
			s.logger.Error("Failed to get organization", "error", err, "orgID", membership.OrganisationID)
			continue
		}

		organizations = append(organizations, Organization{
			ID:        uuid.UUID(org.ID.Bytes).String(),
			Name:      org.Name,
			Slug:      org.Slug,
			CreatedAt: org.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: org.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(organizations)
}

// handleCreateOrganization creates a new organization
func (s *Service) handleCreateOrganization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("userID").(pgtype.UUID)

	var req CreateOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" || req.Slug == "" {
		http.Error(w, "name and slug are required", http.StatusBadRequest)
		return
	}

	// Validate slug format (lowercase alphanumeric with hyphens)
	slugRegex := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	if !slugRegex.MatchString(req.Slug) {
		http.Error(w, "Invalid slug format. Use lowercase letters, numbers, and hyphens only.", http.StatusBadRequest)
		return
	}

	// Check if slug is already taken
	existing, _ := s.db.Queries().OrganisationFindBySlug(ctx, req.Slug)
	if existing != nil {
		http.Error(w, "Slug is already taken", http.StatusConflict)
		return
	}

	// Begin transaction
	err := s.db.WithTx(ctx, func(q *database.Queries) error {
		// Create organization
		orgParams := database.OrganisationCreateParams{
			Name: req.Name,
			Slug: req.Slug,
		}

		org, err := q.OrganisationCreate(ctx, &orgParams)
		if err != nil {
			return err
		}

		// Add user as member
		memberParams := database.OrganisationMemberCreateParams{
			UserID:         userID,
			OrganisationID: org.ID,
		}

		_, err = q.OrganisationMemberCreate(ctx, &memberParams)
		return err
	})

	if err != nil {
		s.logger.Error("Failed to create organization", "error", err)
		http.Error(w, "Failed to create organization", http.StatusInternalServerError)
		return
	}

	// Get the created organization
	org, err := s.db.Queries().OrganisationFindBySlug(ctx, req.Slug)
	if err != nil {
		s.logger.Error("Failed to get created organization", "error", err)
		http.Error(w, "Failed to get organization", http.StatusInternalServerError)
		return
	}

	// Return created organization
	response := Organization{
		ID:        uuid.UUID(org.ID.Bytes).String(),
		Name:      org.Name,
		Slug:      org.Slug,
		CreatedAt: org.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: org.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// handleGetOrganization gets a specific organization
func (s *Service) handleGetOrganization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("userID").(pgtype.UUID)

	// Get organization ID from path
	orgIDStr := r.PathValue("id")

	// Check if it's a UUID or slug
	var org *database.Organisation
	var err error

	if orgID, err := uuid.Parse(orgIDStr); err == nil {
		// It's a UUID
		pgOrgID := pgtype.UUID{Bytes: orgID, Valid: true}
		org, err = s.db.Queries().OrganisationFindById(ctx, pgOrgID)
	} else {
		// Try as slug
		org, err = s.db.Queries().OrganisationFindBySlug(ctx, orgIDStr)
	}

	if err != nil || org == nil {
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	// Verify user is member of organization
	membership, err := s.db.Queries().OrganisationMemberFindByUserAndOrg(ctx, &database.OrganisationMemberFindByUserAndOrgParams{
		UserID:         userID,
		OrganisationID: org.ID,
	})
	if err != nil || membership == nil {
		http.Error(w, "Not authorized to access this organization", http.StatusForbidden)
		return
	}

	// Return organization
	response := Organization{
		ID:        uuid.UUID(org.ID.Bytes).String(),
		Name:      org.Name,
		Slug:      org.Slug,
		CreatedAt: org.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: org.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
