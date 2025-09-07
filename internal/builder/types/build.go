package types

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// BuildResult represents the result of a build operation
type BuildResult struct {
	Success   bool
	ImageTag  string
	ImageHash string // SHA256 hash of the built image
	ImageSize int64  // Size of the image in bytes
	BuildLog  string
	Error     error
	Duration  time.Duration
}

// EnrichedBuild combines ImageBuild with related deployment and project information
type EnrichedBuild struct {
	// From ImageBuild
	ID             pgtype.UUID
	Status         string
	DeploymentID   pgtype.UUID
	StartedAt      pgtype.Timestamptz
	CompletedAt    pgtype.Timestamptz
	FailedAt       pgtype.Timestamptz
	OrganisationID pgtype.UUID
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz

	// From Deployment
	CommitHash string
	ProjectID  pgtype.UUID

	// From Project
	GithubRepository string
	DefaultBranch    string
}
