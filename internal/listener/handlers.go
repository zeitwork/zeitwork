package listener

import (
	"context"
	"fmt"

	"github.com/jackc/pglogrepl"
)

// handleDMLOperation handles INSERT, UPDATE operations for all tables
// DELETE operations are ignored since we use soft-delete
func (s *Service) handleDMLOperation(ctx context.Context, relationID uint32, tuple *pglogrepl.TupleData, operation string) error {
	relation, ok := s.relations[relationID]
	if !ok {
		return fmt.Errorf("unknown relation ID %d", relationID)
	}

	// Only handle INSERT and UPDATE operations
	if operation != "INSERT" && operation != "UPDATE" {
		s.logger.Debug("Ignoring operation (soft-delete used)",
			"table", relation.RelationName,
			"operation", operation)
		return nil
	}

	s.logger.Debug("Handling DML operation",
		"table", relation.RelationName,
		"operation", operation,
		"relation_id", relationID)

	switch relation.RelationName {
	case "deployments":
		return s.handleDeploymentChange(ctx, tuple, operation, relation)
	case "domains":
		return s.handleDomainChange(ctx, tuple, operation, relation)
	case "instances":
		return s.handleInstanceChange(ctx, tuple, operation, relation)
	case "deployment_instances":
		return s.handleDeploymentInstanceChange(ctx, tuple, operation, relation)
	case "image_builds":
		return s.handleImageBuildChange(ctx, tuple, operation, relation)
	default:
		s.logger.Debug("Ignoring change for unhandled table", "table", relation.RelationName)
		return nil
	}
}
