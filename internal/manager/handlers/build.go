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

// BuildCreatedHandler handles build creation events
type BuildCreatedHandler struct {
	orchestrator *orchestration.DeploymentOrchestrator
	logger       *slog.Logger
}

func NewBuildCreatedHandler(orchestrator *orchestration.DeploymentOrchestrator, logger *slog.Logger) *BuildCreatedHandler {
	return &BuildCreatedHandler{orchestrator: orchestrator, logger: logger}
}

func (h *BuildCreatedHandler) EventType() events.EventType {
	return events.ImageBuildCreated
}

func (h *BuildCreatedHandler) HandleEvent(ctx context.Context, data []byte) error {
	var event pb.ImageBuildCreated
	if err := proto.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal ImageBuildCreated: %w", err)
	}

	h.logger.Info("Build created", "build_id", event.Id)
	return h.orchestrator.HandleBuildCreatedByID(ctx, uuid.MustParseUUID(event.Id))
}

// BuildCompletedHandler handles build completion events
type BuildCompletedHandler struct {
	orchestrator *orchestration.DeploymentOrchestrator
	logger       *slog.Logger
}

func NewBuildCompletedHandler(orchestrator *orchestration.DeploymentOrchestrator, logger *slog.Logger) *BuildCompletedHandler {
	return &BuildCompletedHandler{orchestrator: orchestrator, logger: logger}
}

func (h *BuildCompletedHandler) EventType() events.EventType {
	return events.ImageBuildUpdated
}

func (h *BuildCompletedHandler) HandleEvent(ctx context.Context, data []byte) error {
	var event pb.ImageBuildUpdated
	if err := proto.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal ImageBuildUpdated: %w", err)
	}

	h.logger.Info("Build updated", "build_id", event.Id)

	// The reconciler will handle determining if the build is actually completed
	// by querying the database and checking the status
	return h.orchestrator.HandleBuildCompletedByID(ctx, uuid.MustParseUUID(event.Id))
}
