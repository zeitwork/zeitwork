package events

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zeitwork/zeitwork/internal/nodeagent/state"
	pb "github.com/zeitwork/zeitwork/proto"
	"google.golang.org/protobuf/proto"
)

// InstanceCreatedHandler handles instance creation events
type InstanceCreatedHandler struct {
	stateManager *state.Manager
	logger       *slog.Logger
}

// NewInstanceCreatedHandler creates a new instance created handler
func NewInstanceCreatedHandler(stateManager *state.Manager, logger *slog.Logger) *InstanceCreatedHandler {
	return &InstanceCreatedHandler{
		stateManager: stateManager,
		logger:       logger,
	}
}

// HandleEvent processes instance.created events
func (h *InstanceCreatedHandler) HandleEvent(ctx context.Context, data []byte) error {
	var event pb.InstanceCreated
	if err := proto.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal InstanceCreated: %w", err)
	}

	h.logger.Info("Instance created", "instance_id", event.Id)

	// Update desired state for this instance
	if err := h.stateManager.UpdateDesiredState(ctx, event.Id); err != nil {
		return fmt.Errorf("failed to update desired state for instance %s: %w", event.Id, err)
	}

	// Trigger reconciliation
	if err := h.stateManager.Reconcile(ctx); err != nil {
		return fmt.Errorf("failed to reconcile state after instance creation: %w", err)
	}

	return nil
}

// InstanceUpdatedHandler handles instance update events
type InstanceUpdatedHandler struct {
	stateManager *state.Manager
	logger       *slog.Logger
}

// NewInstanceUpdatedHandler creates a new instance updated handler
func NewInstanceUpdatedHandler(stateManager *state.Manager, logger *slog.Logger) *InstanceUpdatedHandler {
	return &InstanceUpdatedHandler{
		stateManager: stateManager,
		logger:       logger,
	}
}

// HandleEvent processes instance.updated events
func (h *InstanceUpdatedHandler) HandleEvent(ctx context.Context, data []byte) error {
	var event pb.InstanceUpdated
	if err := proto.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal InstanceUpdated: %w", err)
	}

	h.logger.Info("Instance updated", "instance_id", event.Id)

	// Update desired state for this instance
	if err := h.stateManager.UpdateDesiredState(ctx, event.Id); err != nil {
		return fmt.Errorf("failed to update desired state for instance %s: %w", event.Id, err)
	}

	// Trigger reconciliation
	if err := h.stateManager.Reconcile(ctx); err != nil {
		return fmt.Errorf("failed to reconcile state after instance update: %w", err)
	}

	return nil
}

// NodeUpdatedHandler handles node update events
type NodeUpdatedHandler struct {
	stateManager *state.Manager
	logger       *slog.Logger
}

// NewNodeUpdatedHandler creates a new node updated handler
func NewNodeUpdatedHandler(stateManager *state.Manager, logger *slog.Logger) *NodeUpdatedHandler {
	return &NodeUpdatedHandler{
		stateManager: stateManager,
		logger:       logger,
	}
}

// HandleEvent processes node.updated events
func (h *NodeUpdatedHandler) HandleEvent(ctx context.Context, data []byte) error {
	var event pb.NodeUpdated
	if err := proto.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal NodeUpdated: %w", err)
	}

	h.logger.Info("Node updated", "node_id", event.Id)

	// For node events, refresh all desired state in case node configuration changed
	if err := h.stateManager.RefreshDesiredState(ctx); err != nil {
		return fmt.Errorf("failed to refresh desired state after node update: %w", err)
	}

	// Trigger reconciliation
	if err := h.stateManager.Reconcile(ctx); err != nil {
		return fmt.Errorf("failed to reconcile state after node update: %w", err)
	}

	return nil
}
