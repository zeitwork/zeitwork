package operator

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// listDeployments returns all deployments
func (s *Service) listDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	deployments, err := s.db.Queries().DeploymentFind(ctx)
	if err != nil {
		s.logger.Error("Failed to list deployments", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deployments)
}

// getDeployment returns a specific deployment
func (s *Service) getDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract deployment ID from path using Go 1.22's PathValue
	deploymentIDStr := r.PathValue("id")
	deploymentID, err := uuid.Parse(deploymentIDStr)
	if err != nil {
		http.Error(w, "Invalid deployment ID", http.StatusBadRequest)
		return
	}

	pgUUID := pgtype.UUID{Bytes: deploymentID, Valid: true}
	deployment, err := s.db.Queries().DeploymentFindById(ctx, pgUUID)
	if err != nil {
		s.logger.Error("Failed to get deployment", "error", err, "deploymentID", deploymentID)
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deployment)
}

// createDeployment creates a new deployment (stub)
func (s *Service) createDeployment(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement deployment creation
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// updateDeploymentStatus updates the status of a deployment (stub)
func (s *Service) updateDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement deployment status update
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
