package zeitwork

import (
	"context"
	"slices"
	"time"

	dnsresolver "github.com/zeitwork/zeitwork/internal/shared/dns"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

func (s *Service) reconcileDomain(ctx context.Context, objectID uuid.UUID) error {
	domain, err := s.db.DomainFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	domainName := domain.Name

	resolveCtx, cancel := context.WithTimeout(ctx, domainResolveTimeout)
	resolution, err := s.dnsResolver.Resolve(resolveCtx, domainName)
	cancel()
	if err != nil {
		// domain resolution failed
		return nil
	}

	matchedIP := matchesAllowedIP(resolution, s.cfg.IPAdress)

	if !matchedIP {
		// domain does not point to allowed targets
		return nil
	}

	s.db.DomainMarkVerified(ctx, domain.ID)

	return nil
}

const domainResolveTimeout = 10 * time.Second

func matchesAllowedIP(resolution *dnsresolver.Resolution, allowedIP string) bool {
	return slices.Contains(resolution.IPv4, allowedIP)
}
