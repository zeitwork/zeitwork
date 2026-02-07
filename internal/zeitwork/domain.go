package zeitwork

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/base58"
	dnsresolver "github.com/zeitwork/zeitwork/internal/shared/dns"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

func (s *Service) reconcileDomain(ctx context.Context, objectID uuid.UUID) error {
	domain, err := s.db.DomainFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	domainName := domain.Name

	// Step 1: Resolve DNS and check IP
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

	// Step 2: If TXT verification is required, check for the verification token
	if domain.TxtVerificationRequired {
		txtName := "_zeitwork." + domainName

		txtCtx, txtCancel := context.WithTimeout(ctx, domainResolveTimeout)
		txtRecords, err := s.dnsResolver.LookupTXT(txtCtx, txtName)
		txtCancel()
		if err != nil {
			// TXT lookup failed, retry later
			return nil
		}

		expectedToken := base58.Encode(domain.ID.Bytes[:])
		if !slices.Contains(txtRecords, expectedToken) {
			// TXT verification token not found
			return nil
		}
	}

	// Skip if already verified (avoid unnecessary DB update)
	if domain.VerifiedAt.Valid {
		return nil
	}

	// Step 3: If TXT verification passed and there are conflicting domains, soft-delete them
	if domain.TxtVerificationRequired {
		conflicts, err := s.db.DomainFindActiveByName(ctx, queries.DomainFindActiveByNameParams{
			Name: domainName,
			ID:   domain.ID,
		})
		if err != nil {
			return err
		}
		for _, conflict := range conflicts {
			slog.Info("soft-deleting conflicting domain after TXT verification",
				"verified_domain_id", domain.ID,
				"conflicting_domain_id", conflict.ID,
				"domain_name", domainName,
			)
			if err := s.db.DomainSoftDelete(ctx, conflict.ID); err != nil {
				return err
			}
		}
	}

	// Step 4: Mark verified
	s.db.DomainMarkVerified(ctx, domain.ID)

	return nil
}

const domainResolveTimeout = 10 * time.Second

func matchesAllowedIP(resolution *dnsresolver.Resolution, allowedIP string) bool {
	return slices.Contains(resolution.IPv4, allowedIP)
}
