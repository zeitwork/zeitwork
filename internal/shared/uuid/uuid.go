package uuid

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ParseUUID converts a string UUID to pgtype.UUID
func ParseUUID(id string) (pgtype.UUID, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		return pgUUID, fmt.Errorf("invalid UUID format: %w", err)
	}
	return pgUUID, nil
}

// MustParseUUID converts a string UUID to pgtype.UUID, panicking on error
func MustParseUUID(id string) pgtype.UUID {
	pgUUID, err := ParseUUID(id)
	if err != nil {
		panic(fmt.Sprintf("failed to parse UUID %s: %v", id, err))
	}
	return pgUUID
}

// GenerateUUID generates a new UUIDv7 string
func GenerateUUID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// GeneratePgUUID generates a new UUIDv7 as pgtype.UUID
func GeneratePgUUID() pgtype.UUID {
	return MustParseUUID(GenerateUUID())
}
