package dns

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
)

const (
	// DefaultMaxDepth is the default number of DNS resolution hops before bailing out.
	DefaultMaxDepth = 10
)

// Resolution captures the hostnames and IPs discovered while resolving a domain.
type Resolution struct {
	HostChain []string
	IPv4      []string
	IPv6      []string
}

// Resolver resolves hostnames, following CNAMEs up to a fixed depth.
type Resolver interface {
	Resolve(ctx context.Context, host string) (*Resolution, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

type netResolver struct {
	maxDepth    int
	lookupIP    func(ctx context.Context, host string) ([]net.IPAddr, error)
	lookupCNAME func(ctx context.Context, host string) (string, error)
	lookupTXT   func(ctx context.Context, name string) ([]string, error)
}

// NewResolver returns a Resolver backed by Go's default net.Resolver.
func NewResolver() Resolver {
	resolver := net.DefaultResolver
	return &netResolver{
		maxDepth: DefaultMaxDepth,
		lookupIP: resolver.LookupIPAddr,
		lookupCNAME: func(ctx context.Context, host string) (string, error) {
			return resolver.LookupCNAME(ctx, host)
		},
		lookupTXT: resolver.LookupTXT,
	}
}

func (r *netResolver) Resolve(ctx context.Context, host string) (*Resolution, error) {
	current := NormalizeHostname(host)
	if current == "" {
		return nil, fmt.Errorf("cannot resolve empty host")
	}

	visited := make(map[string]struct{})
	result := &Resolution{}

	for depth := 0; depth < r.maxDepth; depth++ {
		if _, seen := visited[current]; seen {
			return nil, fmt.Errorf("detected CNAME loop at %s", current)
		}
		visited[current] = struct{}{}
		result.HostChain = append(result.HostChain, current)

		cname, cnameErr := r.lookupCNAME(ctx, current)
		if cnameErr == nil {
			canonical := NormalizeHostname(cname)
			if canonical == "" {
				return nil, fmt.Errorf("empty canonical name returned for %s", current)
			}
			if canonical != current {
				current = canonical
				continue
			}
		}

		ipAddrs, ipErr := r.lookupIP(ctx, current)
		if ipErr != nil {
			return nil, fmt.Errorf("failed to resolve IPs for %s: %w", current, ipErr)
		}

		for _, addr := range ipAddrs {
			if addr.IP == nil {
				continue
			}
			if v4 := addr.IP.To4(); v4 != nil {
				result.IPv4 = appendUnique(result.IPv4, v4.String())
			} else {
				result.IPv6 = appendUnique(result.IPv6, addr.IP.String())
			}
		}

		if len(result.IPv4) == 0 && len(result.IPv6) == 0 {
			return nil, fmt.Errorf("no A/AAAA records found for %s", current)
		}

		return result, nil
	}

	return nil, fmt.Errorf("max DNS depth (%d) exceeded while resolving %s", r.maxDepth, host)
}

func (r *netResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	name = NormalizeHostname(name)
	if name == "" {
		return nil, fmt.Errorf("cannot lookup TXT for empty name")
	}
	return r.lookupTXT(ctx, name)
}

func appendUnique(list []string, value string) []string {
	if slices.Contains(list, value) {
		return list
	}
	return append(list, value)
}

// NormalizeHostname trims whitespace, removes trailing dots and lowercases hostnames.
func NormalizeHostname(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}
