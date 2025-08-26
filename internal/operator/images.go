package operator

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
)

// listImages returns all images
func (s *Service) listImages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	images, err := s.db.Queries().ImageFind(ctx)
	if err != nil {
		s.logger.Error("Failed to list images", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(images)
}

// getImage returns a specific image
func (s *Service) getImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract image ID from path using Go 1.22's PathValue
	imageIDStr := r.PathValue("id")
	imageID, err := uuid.Parse(imageIDStr)
	if err != nil {
		http.Error(w, "Invalid image ID", http.StatusBadRequest)
		return
	}

	pgUUID := pgtype.UUID{Bytes: imageID, Valid: true}
	image, err := s.db.Queries().ImageFindById(ctx, pgUUID)
	if err != nil {
		s.logger.Error("Failed to get image", "error", err, "imageID", imageID)
		http.Error(w, "Image not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(image)
}

// createImage creates a new image (stub)
func (s *Service) createImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GitHubRepo string `json:"github_repo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.GitHubRepo == "" {
		http.Error(w, "github_repo is required", http.StatusBadRequest)
		return
	}
	name := strings.ReplaceAll(req.GitHubRepo, "/", "-")
	repository, err := json.Marshal(map[string]string{"type": "github", "repo": req.GitHubRepo})
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	params := database.ImageCreateParams{
		Name:       name,
		Status:     "pending",
		Repository: repository,
		ImageSize:  pgtype.Int4{Int32: 0, Valid: true},
		ImageHash:  "",
	}
	image, err := s.db.Queries().ImageCreate(r.Context(), &params)
	if err != nil {
		s.logger.Error("Failed to create image", "error", err)
		http.Error(w, "Failed to create image", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(image)
}

// updateImageStatus updates the status of an image (stub)
func (s *Service) updateImageStatus(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement image status update
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// deleteImage deletes an image (stub)
func (s *Service) deleteImage(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement image deletion
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
