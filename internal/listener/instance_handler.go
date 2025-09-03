package listener

import (
	"context"

	"github.com/jackc/pglogrepl"

	pb "github.com/zeitwork/zeitwork/proto"
)

// handleInstanceChange handles changes to the instances table
func (s *Service) handleInstanceChange(ctx context.Context, tuple *pglogrepl.TupleData, operation string, relation *pglogrepl.RelationMessageV2) error {
	return s.GenericHandler(ctx, tuple, operation, relation, "instance", s.InstanceCreated, s.InstanceUpdated)
}

// InstanceCreated handles instance creation business logic
func (s *Service) InstanceCreated(ctx context.Context, instanceID string) error {
	// Publish instance.created event
	msg := &pb.InstanceCreated{Id: instanceID}
	return s.PublishEvent("instance.created", msg, instanceID)
}

// InstanceUpdated handles instance update business logic
func (s *Service) InstanceUpdated(ctx context.Context, instanceID string) error {
	// Publish instance.updated event
	msg := &pb.InstanceUpdated{Id: instanceID}
	return s.PublishEvent("instance.updated", msg, instanceID)
}
