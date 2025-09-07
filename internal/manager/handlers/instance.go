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

// InstanceUpdatedHandler handles instance update events
type InstanceUpdatedHandler struct {
	orchestrator *orchestration.DeploymentOrchestrator
	logger       *slog.Logger
}

func NewInstanceUpdatedHandler(orchestrator *orchestration.DeploymentOrchestrator, logger *slog.Logger) *InstanceUpdatedHandler {
	return &InstanceUpdatedHandler{orchestrator: orchestrator, logger: logger}
}

func (h *InstanceUpdatedHandler) EventType() events.EventType {
	return events.InstanceUpdated
}

func (h *InstanceUpdatedHandler) HandleEvent(ctx context.Context, data []byte) error {
	var event pb.InstanceUpdated
	if err := proto.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal InstanceUpdated: %w", err)
	}

	h.logger.Info("Instance updated", "instance_id", event.Id)
	return h.orchestrator.HandleInstanceUpdatedByID(ctx, uuid.MustParseUUID(event.Id))
}
