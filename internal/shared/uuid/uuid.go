package uuid

import (
	gid "github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UUID is a wrapper around a UUID that can represent NULL values.
// It implements pgx's UUID scanner/valuer interfaces.
type UUID struct {
	Bytes [16]byte
	Valid bool
}

// New generates a new UUID v7
func New() UUID {
	id, err := gid.NewV7()
	if err != nil {
		panic("failed to generate uuid v7")
	}
	return UUID{
		Bytes: id,
		Valid: true,
	}
}

// FromGoogleUUID converts a google/uuid.UUID to our UUID type
func FromGoogleUUID(id gid.UUID) UUID {
	return UUID{
		Bytes: id,
		Valid: true,
	}
}

// Parse parses a UUID string
func Parse(s string) (UUID, error) {
	id, err := gid.Parse(s)
	if err != nil {
		return UUID{}, err
	}
	return UUID{
		Bytes: id,
		Valid: true,
	}, nil
}

// MustParse parses a UUID string, panicking on error
func MustParse(s string) UUID {
	u, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

// Nil returns an invalid/null UUID
func Nil() UUID {
	return UUID{Valid: false}
}

// String returns the string representation of the UUID
func (u UUID) String() string {
	if !u.Valid {
		return ""
	}
	return gid.UUID(u.Bytes).String()
}

// IsNil returns true if the UUID is null/invalid
func (u UUID) IsNil() bool {
	return !u.Valid
}

// ToGoogleUUID converts to a google/uuid.UUID
func (u UUID) ToGoogleUUID() gid.UUID {
	return u.Bytes
}

// ScanUUID implements pgtype.UUIDScanner interface for pgx
func (u *UUID) ScanUUID(v pgtype.UUID) error {
	u.Bytes = v.Bytes
	u.Valid = v.Valid
	return nil
}

// UUIDValue implements pgtype.UUIDValuer interface for pgx
func (u UUID) UUIDValue() (pgtype.UUID, error) {
	return pgtype.UUID{
		Bytes: u.Bytes,
		Valid: u.Valid,
	}, nil
}
