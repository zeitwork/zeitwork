package operator

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
)

// NodeCreateRequest represents a request to create a new node
type NodeCreateRequest struct {
	Hostname  string          `json:"hostname"`
	IPAddress string          `json:"ip_address"`
	RegionID  *string         `json:"region_id,omitempty"`
	Resources json.RawMessage `json:"resources"`
}

// NodeUpdateStateRequest represents a request to update node state
type NodeUpdateStateRequest struct {
	State string `json:"state"`
}

// listNodes returns all nodes
func (s *Service) listNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Query for specific state if provided
	state := r.URL.Query().Get("state")

	var nodes []*database.Node
	var err error

	if state != "" {
		nodes, err = s.db.Queries().NodeFindByState(ctx, state)
	} else {
		nodes, err = s.db.Queries().NodeFind(ctx)
	}

	if err != nil {
		s.logger.Error("Failed to list nodes", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

// getNode returns a specific node
func (s *Service) getNode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract node ID from path using Go 1.22's PathValue
	nodeIDStr := r.PathValue("id")
	nodeID, err := uuid.Parse(nodeIDStr)
	if err != nil {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	pgUUID := pgtype.UUID{Bytes: nodeID, Valid: true}
	node, err := s.db.Queries().NodeFindById(ctx, pgUUID)
	if err != nil {
		s.logger.Error("Failed to get node", "error", err, "nodeID", nodeID)
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(node)
}

// createNode creates a new node
func (s *Service) createNode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req NodeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Hostname == "" || req.IPAddress == "" {
		http.Error(w, "hostname and ip_address are required", http.StatusBadRequest)
		return
	}

	// Set default resources if not provided
	if req.Resources == nil {
		req.Resources = json.RawMessage(`{"vcpu": 2, "memory": 2048}`)
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

	// Create node in database
	params := database.NodeCreateParams{
		RegionID:  regionID,
		Hostname:  req.Hostname,
		IpAddress: req.IPAddress,
		State:     "booting",
		Resources: req.Resources,
	}

	node, err := s.db.Queries().NodeCreate(ctx, &params)
	if err != nil {
		s.logger.Error("Failed to create node", "error", err)
		http.Error(w, "Failed to create node", http.StatusInternalServerError)
		return
	}

	// TODO: Notify the node agent on the new node to register itself

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(node)
}

// updateNodeState updates the state of a node
func (s *Service) updateNodeState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract node ID from path using Go 1.22's PathValue
	nodeIDStr := r.PathValue("id")
	nodeID, err := uuid.Parse(nodeIDStr)
	if err != nil {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	var req NodeUpdateStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate state
	validStates := []string{"booting", "ready", "draining", "down", "terminated", "error", "unknown"}
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

	pgUUID := pgtype.UUID{Bytes: nodeID, Valid: true}
	params := database.NodeUpdateStateParams{
		ID:    pgUUID,
		State: req.State,
	}

	node, err := s.db.Queries().NodeUpdateState(ctx, &params)
	if err != nil {
		s.logger.Error("Failed to update node state", "error", err, "nodeID", nodeID)
		http.Error(w, "Failed to update node state", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(node)
}

// deleteNode deletes a node
func (s *Service) deleteNode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract node ID from path using Go 1.22's PathValue
	nodeIDStr := r.PathValue("id")
	nodeID, err := uuid.Parse(nodeIDStr)
	if err != nil {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	pgUUID := pgtype.UUID{Bytes: nodeID, Valid: true}

	// Check if node has any running instances
	instances, err := s.db.Queries().InstanceFindByNode(ctx, pgUUID)
	if err != nil {
		s.logger.Error("Failed to check node instances", "error", err, "nodeID", nodeID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if len(instances) > 0 {
		http.Error(w, "Cannot delete node with running instances", http.StatusConflict)
		return
	}

	// Delete the node
	if err := s.db.Queries().NodeDelete(ctx, pgUUID); err != nil {
		s.logger.Error("Failed to delete node", "error", err, "nodeID", nodeID)
		http.Error(w, "Failed to delete node", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
