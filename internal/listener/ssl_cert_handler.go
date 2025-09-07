package listener

import (
	"context"

	"github.com/jackc/pglogrepl"

	pb "github.com/zeitwork/zeitwork/proto"
)

// handleSslCertChange handles changes to the ssl_certs table
func (s *Service) handleSslCertChange(ctx context.Context, tuple *pglogrepl.TupleData, operation string, relation *pglogrepl.RelationMessageV2) error {
	return s.GenericHandler(ctx, tuple, operation, relation, "ssl_cert", s.SslCertCreated, s.SslCertUpdated)
}

// SslCertCreated publishes ssl_cert.created
func (s *Service) SslCertCreated(ctx context.Context, certID string) error {
	msg := &pb.SslCertCreated{Id: certID}
	return s.PublishEvent("ssl_cert.created", msg, certID)
}

// SslCertUpdated publishes ssl_cert.updated
func (s *Service) SslCertUpdated(ctx context.Context, certID string) error {
	msg := &pb.SslCertUpdated{Id: certID}
	return s.PublishEvent("ssl_cert.updated", msg, certID)
}
