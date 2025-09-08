package types

import (
	"context"

	"github.com/zeitwork/zeitwork/internal/database"
)

// BuildRuntime defines the interface for different build execution environments
type BuildRuntime interface {
	// Build executes a build in the runtime environment
	Build(ctx context.Context, build *database.ImageBuild) *BuildResult

	// Name returns the name of the runtime implementation
	Name() string

	// Cleanup performs any necessary cleanup operations
	Cleanup() error
}
