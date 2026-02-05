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
	}

	_, err := resolver.Resolve(context.Background(), "a.example.com")
	if err == nil || !strings.Contains(err.Error(), "loop") {
		t.Fatalf("expected loop detection error, got %v", err)
	}
}
