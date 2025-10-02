package uuid

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// New generates a new UUID v7 and returns it as a pgtype.UUID
func New() pgtype.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		// Fallback to v4 if v7 fails (shouldn't happen)
		id = uuid.New()
	}
	var pgUUID pgtype.UUID
	pgUUID.Bytes = id
	pgUUID.Valid = true
	return pgUUID
}

// Parse parses a UUID string and returns it as a pgtype.UUID
func Parse(s string) (pgtype.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	var pgUUID pgtype.UUID
	pgUUID.Bytes = id
	pgUUID.Valid = true
	return pgUUID, nil
}

// ToString converts a pgtype.UUID to a string
func ToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}

// MustParse parses a UUID string and returns it as a pgtype.UUID, panicking on error
func MustParse(s string) pgtype.UUID {
	pgUUID, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return pgUUID
}
