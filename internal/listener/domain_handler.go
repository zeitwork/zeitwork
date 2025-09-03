package listener

import (
	"context"

	"github.com/jackc/pglogrepl"

	pb "github.com/zeitwork/zeitwork/proto"
)

// handleDomainChange handles changes to the domains table
func (s *Service) handleDomainChange(ctx context.Context, tuple *pglogrepl.TupleData, operation string, relation *pglogrepl.RelationMessageV2) error {
	return s.GenericHandler(ctx, tuple, operation, relation, "domain", s.DomainCreated, s.DomainUpdated)
}

// DomainCreated handles domain creation business logic
func (s *Service) DomainCreated(ctx context.Context, domainID string) error {
	// Publish domain.created event
	msg := &pb.DomainCreated{Id: domainID}
	return s.PublishEvent("domain.created", msg, domainID)
}

// DomainUpdated handles domain update business logic
func (s *Service) DomainUpdated(ctx context.Context, domainID string) error {
	// Publish domain.updated event
	msg := &pb.DomainUpdated{Id: domainID}
	return s.PublishEvent("domain.updated", msg, domainID)
}
