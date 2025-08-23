package operator

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
)

// InstanceCreateRequest represents a request to create a new instance
type InstanceCreateRequest struct {
	NodeID               string          `json:"node_id"`
	ImageID              string          `json:"image_id"`
	RegionID             *string         `json:"region_id,omitempty"`
	Resources            json.RawMessage `json:"resources"`
	DefaultPort          int32           `json:"default_port"`
	EnvironmentVariables string          `json:"environment_variables,omitempty"`
}

// InstanceUpdateStateRequest represents a request to update instance state
type InstanceUpdateStateRequest struct {
	State string `json:"state"`
}

// listInstances returns all instances
func (s *Service) listInstances(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Query for specific state if provided
	state := r.URL.Query().Get("state")
	nodeID := r.URL.Query().Get("node_id")

	var instances []*database.Instance
	var err error

	if state != "" {
		instances, err = s.db.Queries().InstanceFindByState(ctx, state)
	} else if nodeID != "" {
		uid, err := uuid.Parse(nodeID)
		if err != nil {
			http.Error(w, "Invalid node ID", http.StatusBadRequest)
			return
		}
		pgUUID := pgtype.UUID{Bytes: uid, Valid: true}
		instances, err = s.db.Queries().InstanceFindByNode(ctx, pgUUID)
	} else {
		instances, err = s.db.Queries().InstanceFind(ctx)
	}

	if err != nil {
		s.logger.Error("Failed to list instances", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(instances)
}

// getInstance returns a specific instance
func (s *Service) getInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract instance ID from path using Go 1.22's PathValue
	instanceIDStr := r.PathValue("id")
	instanceID, err := uuid.Parse(instanceIDStr)
	if err != nil {
		http.Error(w, "Invalid instance ID", http.StatusBadRequest)
		return
	}

	pgUUID := pgtype.UUID{Bytes: instanceID, Valid: true}
	instance, err := s.db.Queries().InstanceFindById(ctx, pgUUID)
	if err != nil {
		s.logger.Error("Failed to get instance", "error", err, "instanceID", instanceID)
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(instance)
}

// createInstance creates a new instance
func (s *Service) createInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req InstanceCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.NodeID == "" || req.ImageID == "" {
		http.Error(w, "node_id and image_id are required", http.StatusBadRequest)
		return
	}

	// Parse UUIDs
	nodeID, err := uuid.Parse(req.NodeID)
	if err != nil {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	imageID, err := uuid.Parse(req.ImageID)
	if err != nil {
		http.Error(w, "Invalid image ID", http.StatusBadRequest)
		return
	}

	// Set default resources if not provided
	if req.Resources == nil {
		req.Resources = json.RawMessage(`{"vcpu": 1, "memory": 1024}`)
	}

	// Set default environment variables if not provided
	if req.EnvironmentVariables == "" {
		req.EnvironmentVariables = "{}"
	}

	// Parse region ID if provided
	var regionID pgtype.UUID
	if req.RegionID != nil {
		uid, err := uuid.Parse(*req.RegionID)
		if err != nil {
			http.Error(w, "Invalid region ID", http.StatusBadRequest)
			return
		}
		regionID = pgtype.UUID{Bytes: uid, Valid: true}
	}

	// Create instance in database
	params := database.InstanceCreateParams{
		RegionID:             regionID,
		NodeID:               pgtype.UUID{Bytes: nodeID, Valid: true},
		ImageID:              pgtype.UUID{Bytes: imageID, Valid: true},
		State:                "pending",
		Resources:            req.Resources,
		DefaultPort:          req.DefaultPort,
		IpAddress:            "10.0.0.2", // TODO: Allocate IP address properly
		EnvironmentVariables: req.EnvironmentVariables,
	}

	instance, err := s.db.Queries().InstanceCreate(ctx, &params)
	if err != nil {
		s.logger.Error("Failed to create instance", "error", err)
		http.Error(w, "Failed to create instance", http.StatusInternalServerError)
		return
	}

	// TODO: Send request to node agent to actually create the instance

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(instance)
}

// updateInstanceState updates the state of an instance
func (s *Service) updateInstanceState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract instance ID from path using Go 1.22's PathValue
	instanceIDStr := r.PathValue("id")
	instanceID, err := uuid.Parse(instanceIDStr)
	if err != nil {
		http.Error(w, "Invalid instance ID", http.StatusBadRequest)
		return
	}

	var req InstanceUpdateStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate state
	validStates := []string{"pending", "starting", "running", "stopping", "stopped", "failed", "terminated"}
	isValid := false
	for _, state := range validStates {
		if req.State == state {
			isValid = true
			break
		}
	}
	if !isValid {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	pgUUID := pgtype.UUID{Bytes: instanceID, Valid: true}
	params := database.InstanceUpdateStateParams{
		ID:    pgUUID,
		State: req.State,
	}

	instance, err := s.db.Queries().InstanceUpdateState(ctx, &params)
	if err != nil {
		s.logger.Error("Failed to update instance state", "error", err, "instanceID", instanceID)
		http.Error(w, "Failed to update instance state", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(instance)
}

// deleteInstance deletes an instance
func (s *Service) deleteInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract instance ID from path using Go 1.22's PathValue
	instanceIDStr := r.PathValue("id")
	instanceID, err := uuid.Parse(instanceIDStr)
	if err != nil {
		http.Error(w, "Invalid instance ID", http.StatusBadRequest)
		return
	}

	pgUUID := pgtype.UUID{Bytes: instanceID, Valid: true}

	// TODO: Send request to node agent to stop and remove the instance

	// Delete the instance from database
	if err := s.db.Queries().InstanceDelete(ctx, pgUUID); err != nil {
		s.logger.Error("Failed to delete instance", "error", err, "instanceID", instanceID)
		http.Error(w, "Failed to delete instance", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
