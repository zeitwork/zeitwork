package listener

import (
	"context"

	"github.com/jackc/pglogrepl"

	pb "github.com/zeitwork/zeitwork/proto"
)

// handleDeploymentInstanceChange handles changes to the deployment_instances table
func (s *Service) handleDeploymentInstanceChange(ctx context.Context, tuple *pglogrepl.TupleData, operation string, relation *pglogrepl.RelationMessageV2) error {
	return s.GenericHandler(ctx, tuple, operation, relation, "deployment_instance", s.DeploymentInstanceCreated, s.DeploymentInstanceUpdated)
}

// DeploymentInstanceCreated handles deployment instance creation business logic
func (s *Service) DeploymentInstanceCreated(ctx context.Context, deploymentInstanceID string) error {
	// Publish deployment_instance.created event
	msg := &pb.DeploymentInstanceCreated{Id: deploymentInstanceID}
	return s.PublishEvent("deployment_instance.created", msg, deploymentInstanceID)
}

// DeploymentInstanceUpdated handles deployment instance update business logic
func (s *Service) DeploymentInstanceUpdated(ctx context.Context, deploymentInstanceID string) error {
	// Publish deployment_instance.updated event
	msg := &pb.DeploymentInstanceUpdated{Id: deploymentInstanceID}
	return s.PublishEvent("deployment_instance.updated", msg, deploymentInstanceID)
}
