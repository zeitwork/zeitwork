package operator

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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
	// TODO: Implement image creation
	http.Error(w, "Not implemented", http.StatusNotImplemented)
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
