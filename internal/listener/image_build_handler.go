package listener

import (
	"context"

	"github.com/jackc/pglogrepl"

	pb "github.com/zeitwork/zeitwork/proto"
)

// handleImageBuildChange handles changes to the image_builds table
func (s *Service) handleImageBuildChange(ctx context.Context, tuple *pglogrepl.TupleData, operation string, relation *pglogrepl.RelationMessageV2) error {
	return s.GenericHandler(ctx, tuple, operation, relation, "image_build", s.ImageBuildCreated, s.ImageBuildUpdated)
}

// ImageBuildCreated handles image build creation business logic
func (s *Service) ImageBuildCreated(ctx context.Context, imageBuildID string) error {
	// Publish image_build.created event
	msg := &pb.ImageBuildCreated{Id: imageBuildID}
	return s.PublishEvent("image_build.created", msg, imageBuildID)
}

// ImageBuildUpdated handles image build update business logic
func (s *Service) ImageBuildUpdated(ctx context.Context, imageBuildID string) error {
	// Publish image_build.updated event
	msg := &pb.ImageBuildUpdated{Id: imageBuildID}
	return s.PublishEvent("image_build.updated", msg, imageBuildID)
}
