package listener

import (
	"context"

	"github.com/jackc/pglogrepl"

	pb "github.com/zeitwork/zeitwork/proto"
)

// handleDeploymentChange handles changes to the deployments table
func (s *Service) handleDeploymentChange(ctx context.Context, tuple *pglogrepl.TupleData, operation string, relation *pglogrepl.RelationMessageV2) error {
	return s.GenericHandler(ctx, tuple, operation, relation, "deployment", s.DeploymentCreated, s.DeploymentUpdated)
}

// DeploymentCreated handles deployment creation business logic
func (s *Service) DeploymentCreated(ctx context.Context, deploymentID string) error {
	// Publish deployment.created event
	msg := &pb.DeploymentCreated{Id: deploymentID}
	return s.PublishEvent("deployment.created", msg, deploymentID)
}

// DeploymentUpdated handles deployment update business logic
func (s *Service) DeploymentUpdated(ctx context.Context, deploymentID string) error {
	// Publish deployment.updated event
	msg := &pb.DeploymentUpdated{Id: deploymentID}
	return s.PublishEvent("deployment.updated", msg, deploymentID)
}
