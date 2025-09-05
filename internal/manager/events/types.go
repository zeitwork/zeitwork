package events

import (
	"context"
)

// EventType represents a typed event identifier
type EventType string

const (
	ImageBuildCreated EventType = "image_build.created"
	ImageBuildUpdated EventType = "image_build.updated"
	DeploymentCreated EventType = "deployment.created"
	DeploymentUpdated EventType = "deployment.updated"
)

// Handler defines the interface for event handlers
type Handler interface {
	HandleEvent(ctx context.Context, data []byte) error
	EventType() EventType
}

// Simple event handling - no over-engineering needed.
// Protobuf messages (pb.ImageBuildCreated, pb.DeploymentUpdated, etc.)
// are unmarshaled directly in handlers. Clean and simple.
