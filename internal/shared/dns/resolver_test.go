package dns

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestNetResolver_DirectARecord(t *testing.T) {
	resolver := &netResolver{
		maxDepth: 5,
		lookupCNAME: func(ctx context.Context, host string) (string, error) {
			return "", errors.New("no cname")
		},
		lookupIP: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			if host == "app.example.com" {
				return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
			}
			return nil, errors.New("no records")
		},
		lookupTXT: func(ctx context.Context, name string) ([]string, error) {
			return nil, errors.New("no txt")
		},
	}

	result, err := resolver.Resolve(context.Background(), "app.example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(result.HostChain) != 1 || result.HostChain[0] != "app.example.com" {
		t.Fatalf("unexpected host chain: %#v", result.HostChain)
	}

	if len(result.IPv4) != 1 || result.IPv4[0] != "203.0.113.10" {
		t.Fatalf("unexpected ipv4 list: %#v", result.IPv4)
	}
}

func TestNetResolver_CNAMEChain(t *testing.T) {
	resolver := &netResolver{
		maxDepth: 5,
		lookupCNAME: func(ctx context.Context, host string) (string, error) {
			switch host {
			case "app.customer.com":
				return "edge.zeitwork.com", nil
			case "edge.zeitwork.com":
				return "edge.zeitwork.com", nil
			default:
				return "", errors.New("no cname")
			}
		},
		lookupIP: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			if host == "edge.zeitwork.com" {
				return []net.IPAddr{{IP: net.ParseIP("198.51.100.20")}}, nil
			}
			return nil, errors.New("no records")
		},
		lookupTXT: func(ctx context.Context, name string) ([]string, error) {
			return nil, errors.New("no txt")
		},
	}

	result, err := resolver.Resolve(context.Background(), "app.customer.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(result.HostChain) != 2 {
		t.Fatalf("expected two hosts in chain, got %#v", result.HostChain)
	}

	if result.HostChain[1] != "edge.zeitwork.com" {
		t.Fatalf("expected canonical host to be edge.zeitwork.com, got %#v", result.HostChain)
	}

	if len(result.IPv4) != 1 || result.IPv4[0] != "198.51.100.20" {
		t.Fatalf("unexpected ipv4 list: %#v", result.IPv4)
	}
}

func TestNetResolver_LoopDetection(t *testing.T) {
	resolver := &netResolver{
		maxDepth: 5,
		lookupCNAME: func(ctx context.Context, host string) (string, error) {
			if host == "a.example.com" {
				return "b.example.com", nil
			}
			if host == "b.example.com" {
				return "a.example.com", nil
			}
			return "", errors.New("no cname")
		},
		lookupIP: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return nil, errors.New("no records")
		},
		lookupTXT: func(ctx context.Context, name string) ([]string, error) {
			return nil, errors.New("no txt")
		},
	}

	_, err := resolver.Resolve(context.Background(), "a.example.com")
	if err == nil || !strings.Contains(err.Error(), "loop") {
		t.Fatalf("expected loop detection error, got %v", err)
	}
}

func TestNetResolver_LookupTXT(t *testing.T) {
	resolver := &netResolver{
		maxDepth: 5,
		lookupCNAME: func(ctx context.Context, host string) (string, error) {
			return "", errors.New("no cname")
		},
		lookupIP: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return nil, errors.New("no records")
		},
		lookupTXT: func(ctx context.Context, name string) ([]string, error) {
			if name == "_zeitwork.example.com" {
				return []string{"abc123token"}, nil
			}
			return nil, errors.New("no txt records")
		},
	}

	records, err := resolver.LookupTXT(context.Background(), "_zeitwork.example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(records) != 1 || records[0] != "abc123token" {
		t.Fatalf("unexpected TXT records: %#v", records)
	}
}

func TestNetResolver_LookupTXT_NotFound(t *testing.T) {
	resolver := &netResolver{
		maxDepth: 5,
		lookupCNAME: func(ctx context.Context, host string) (string, error) {
			return "", errors.New("no cname")
		},
		lookupIP: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return nil, errors.New("no records")
		},
		lookupTXT: func(ctx context.Context, name string) ([]string, error) {
			return nil, errors.New("no txt records")
		},
	}

	_, err := resolver.LookupTXT(context.Background(), "_zeitwork.example.com")
	if err == nil {
		t.Fatalf("expected error for missing TXT record, got nil")
	}
}

func TestNetResolver_LookupTXT_EmptyName(t *testing.T) {
	resolver := &netResolver{
		maxDepth: 5,
		lookupCNAME: func(ctx context.Context, host string) (string, error) {
			return "", errors.New("no cname")
		},
		lookupIP: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return nil, errors.New("no records")
		},
		lookupTXT: func(ctx context.Context, name string) ([]string, error) {
			return []string{"should-not-reach"}, nil
		},
	}

	_, err := resolver.LookupTXT(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error for empty name, got nil")
	}
}

func TestNetResolver_LookupTXT_MultipleRecords(t *testing.T) {
	resolver := &netResolver{
		maxDepth: 5,
		lookupCNAME: func(ctx context.Context, host string) (string, error) {
			return "", errors.New("no cname")
		},
		lookupIP: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return nil, errors.New("no records")
		},
		lookupTXT: func(ctx context.Context, name string) ([]string, error) {
			if name == "_zeitwork.example.com" {
				return []string{"v=spf1 include:example.com", "abc123token"}, nil
			}
			return nil, errors.New("no txt records")
		},
	}

	records, err := resolver.LookupTXT(context.Background(), "_zeitwork.example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 TXT records, got %d: %#v", len(records), records)
	}

	found := false
	for _, r := range records {
		if r == "abc123token" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to find 'abc123token' in TXT records: %#v", records)
	}
}
