package types

import (
	"time"
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
