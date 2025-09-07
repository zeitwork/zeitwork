package types

import (
	"context"
)

// Runtime defines the interface for certificate issuance backends
type Runtime interface {
	// EnsureCertificate ensures a certificate exists and is valid for name.
	// Implementations should create/renew as needed.
	EnsureCertificate(ctx context.Context, name string, isWildcard bool) error

	// Name returns the runtime name (e.g., "local", "acme")
	Name() string

	// Cleanup releases any underlying resources
	Cleanup() error
}
