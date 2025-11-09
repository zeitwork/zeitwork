package edgeproxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
)

// PostgreSQLStorage implements certmagic.Storage using PostgreSQL via sqlc
type PostgreSQLStorage struct {
	db *database.DB
}

// NewPostgreSQLStorage creates a new PostgreSQL storage adapter for certmagic
func NewPostgreSQLStorage(db *database.DB) *PostgreSQLStorage {
	return &PostgreSQLStorage{db: db}
}

// Store saves a value at the given key
func (s *PostgreSQLStorage) Store(ctx context.Context, key string, value []byte) error {
	// Encode binary data as base64 for storage in text field
	encodedValue := base64.StdEncoding.EncodeToString(value)

	err := s.db.Queries().StoreCertmagicData(ctx, &database.StoreCertmagicDataParams{
		Key:   key,
		Value: encodedValue,
		Modified: pgtype.Timestamptz{
			Time:  time.Now(),
			Valid: true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to store key %s: %w", key, err)
	}

	return nil
}

// Load retrieves the value at the given key
func (s *PostgreSQLStorage) Load(ctx context.Context, key string) ([]byte, error) {
	data, err := s.db.Queries().LoadCertmagicData(ctx, key)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("key not found: %s", key)
		}
		return nil, fmt.Errorf("failed to load key %s: %w", key, err)
	}

	// Decode base64 to get original binary data
	value, err := base64.StdEncoding.DecodeString(data.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to decode value for key %s: %w", key, err)
	}

	return value, nil
}

// Delete removes the value at the given key
func (s *PostgreSQLStorage) Delete(ctx context.Context, key string) error {
	rowsAffected, err := s.db.Queries().DeleteCertmagicData(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete key %s: %w", key, err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("key not found: %s", key)
	}

	return nil
}

// Exists returns true if the key exists
func (s *PostgreSQLStorage) Exists(ctx context.Context, key string) bool {
	exists, err := s.db.Queries().ExistsCertmagicData(ctx, key)
	if err != nil {
		return false
	}

	return exists
}

// List returns all keys that match the prefix
func (s *PostgreSQLStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	var keys []string
	var err error

	prefixParam := pgtype.Text{
		String: prefix,
		Valid:  true,
	}

	if recursive {
		keys, err = s.db.Queries().ListCertmagicDataRecursive(ctx, prefixParam)
	} else {
		keys, err = s.db.Queries().ListCertmagicDataNonRecursive(ctx, prefixParam)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list keys with prefix %s: %w", prefix, err)
	}

	return keys, nil
}

// Stat returns information about the key
func (s *PostgreSQLStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	data, err := s.db.Queries().StatCertmagicData(ctx, key)
	if err != nil {
		if err == pgx.ErrNoRows {
			return certmagic.KeyInfo{}, fmt.Errorf("key not found: %s", key)
		}
		return certmagic.KeyInfo{}, fmt.Errorf("failed to stat key %s: %w", key, err)
	}

	// Decode to get size
	value, err := base64.StdEncoding.DecodeString(data.Value)
	if err != nil {
		return certmagic.KeyInfo{}, fmt.Errorf("failed to decode value for key %s: %w", key, err)
	}

	return certmagic.KeyInfo{
		Key:        key,
		Modified:   data.Modified.Time,
		Size:       int64(len(value)),
		IsTerminal: !strings.HasSuffix(key, "/"),
	}, nil
}

// Lock acquires a lock for the given key
func (s *PostgreSQLStorage) Lock(ctx context.Context, key string) error {
	// Try to acquire lock with 5-minute expiration
	lockExpiration := pgtype.Timestamptz{
		Time:  time.Now().Add(5 * time.Minute),
		Valid: true,
	}

	rowsAffected, err := s.db.Queries().AcquireCertmagicLock(ctx, &database.AcquireCertmagicLockParams{
		Key:     key,
		Expires: lockExpiration,
	})
	if err != nil {
		return fmt.Errorf("failed to acquire lock for key %s: %w", key, err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("lock already held for key: %s", key)
	}

	return nil
}

// Unlock releases a lock for the given key
func (s *PostgreSQLStorage) Unlock(ctx context.Context, key string) error {
	rowsAffected, err := s.db.Queries().ReleaseCertmagicLock(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to release lock for key %s: %w", key, err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no lock found for key: %s", key)
	}

	return nil
}

// Ensure PostgreSQLStorage implements certmagic.Storage
var _ certmagic.Storage = (*PostgreSQLStorage)(nil)
