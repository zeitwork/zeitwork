package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"

	"github.com/zeitwork/zeitwork/internal/manager/events"
	"github.com/zeitwork/zeitwork/internal/manager/orchestration"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
	pb "github.com/zeitwork/zeitwork/proto"
)

// DeploymentCreatedHandler handles deployment creation events
type DeploymentCreatedHandler struct {
	orchestrator *orchestration.DeploymentOrchestrator
	logger       *slog.Logger
}

func NewDeploymentCreatedHandler(orchestrator *orchestration.DeploymentOrchestrator, logger *slog.Logger) *DeploymentCreatedHandler {
	return &DeploymentCreatedHandler{orchestrator: orchestrator, logger: logger}
}

func (h *DeploymentCreatedHandler) EventType() events.EventType {
	return events.DeploymentCreated
}

func (h *DeploymentCreatedHandler) HandleEvent(ctx context.Context, data []byte) error {
	var event pb.DeploymentCreated
	if err := proto.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal DeploymentCreated: %w", err)
	}

	h.logger.Info("Deployment created", "deployment_id", event.Id)
	return h.orchestrator.HandleDeploymentCreatedByID(ctx, uuid.MustParseUUID(event.Id))
}

// DeploymentUpdatedHandler handles deployment update events
type DeploymentUpdatedHandler struct {
	orchestrator *orchestration.DeploymentOrchestrator
	logger       *slog.Logger
}

func NewDeploymentUpdatedHandler(orchestrator *orchestration.DeploymentOrchestrator, logger *slog.Logger) *DeploymentUpdatedHandler {
	return &DeploymentUpdatedHandler{orchestrator: orchestrator, logger: logger}
}

func (h *DeploymentUpdatedHandler) EventType() events.EventType {
	return events.DeploymentUpdated
}

func (h *DeploymentUpdatedHandler) HandleEvent(ctx context.Context, data []byte) error {
	var event pb.DeploymentUpdated
	if err := proto.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal DeploymentUpdated: %w", err)
	}

	h.logger.Info("Deployment updated", "deployment_id", event.Id)
	return h.orchestrator.HandleDeploymentUpdatedByID(ctx, uuid.MustParseUUID(event.Id))
}
