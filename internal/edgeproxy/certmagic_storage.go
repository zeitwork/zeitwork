package edgeproxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/database/queries"
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

	// Log ACME account files for debugging
	if strings.HasSuffix(key, ".json") && len(value) > 0 {
		slog.Info("storing certmagic JSON", "key", key, "size", len(value), "content", string(value))
	}

	err := s.db.Queries().StoreCertmagicData(ctx, queries.StoreCertmagicDataParams{
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
	data, err := s.db.LoadCertmagicData(ctx, key)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Return fs.ErrNotExist - the standard Go error for "not found"
			// This tells certmagic the key doesn't exist and it should create new
			return nil, fs.ErrNotExist
		}
		return nil, err
	}

	// Handle empty value as not found
	if data.Value == "" {
		return nil, fs.ErrNotExist
	}

	// Decode base64 to get original binary data
	value, err := base64.StdEncoding.DecodeString(data.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to decode value for key %s: %w", key, err)
	}

	// Log ACME account files for debugging
	if strings.HasSuffix(key, ".json") && len(value) > 0 {
		slog.Info("loaded certmagic JSON", "key", key, "size", len(value), "content", string(value))
	}

	return value, nil
}

// Delete removes the value at the given key
// Like os.RemoveAll, returns nil even if key doesn't exist
func (s *PostgreSQLStorage) Delete(ctx context.Context, key string) error {
	_, err := s.db.DeleteCertmagicData(ctx, key)
	if err != nil {
		return err
	}
	return nil
}

// Exists returns true if the key exists
func (s *PostgreSQLStorage) Exists(ctx context.Context, key string) bool {
	exists, err := s.db.ExistsCertmagicData(ctx, key)
	if err != nil {
		return false
	}

	return exists
}

// List returns all keys that match the prefix
func (s *PostgreSQLStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	prefixParam := pgtype.Text{
		String: prefix,
		Valid:  true,
	}

	if recursive {
		return s.db.ListCertmagicDataRecursive(ctx, prefixParam)
	}
	return s.db.ListCertmagicDataNonRecursive(ctx, prefixParam)
}

// Stat returns information about the key
func (s *PostgreSQLStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	data, err := s.db.StatCertmagicData(ctx, key)
	if err != nil {
		if err == pgx.ErrNoRows {
			return certmagic.KeyInfo{}, fs.ErrNotExist
		}
		return certmagic.KeyInfo{}, err
	}

	// Decode to get size
	value, err := base64.StdEncoding.DecodeString(data.Value)
	if err != nil {
		return certmagic.KeyInfo{}, err
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

	rowsAffected, err := s.db.AcquireCertmagicLock(ctx, queries.AcquireCertmagicLockParams{
		Key:     key,
		Expires: lockExpiration,
	})
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("lock already held for key: %s", key)
	}

	return nil
}

// Unlock releases a lock for the given key
// Like os.Remove, returns nil even if lock doesn't exist
func (s *PostgreSQLStorage) Unlock(ctx context.Context, key string) error {
	_, err := s.db.ReleaseCertmagicLock(ctx, key)
	if err != nil {
		return err
	}
	return nil
}

// Ensure PostgreSQLStorage implements certmagic.Storage
var _ certmagic.Storage = (*PostgreSQLStorage)(nil)
