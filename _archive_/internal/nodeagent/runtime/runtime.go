package runtime

import (
	"context"
	"io"
)

// Container represents a running container instance
type Container struct {
	ID                   string
	InstanceID           string
	ImageName            string
	State                string // running, stopped, etc
	IPAddress            string
	EnvironmentVariables map[string]string
}

// Runtime defines the interface for container runtime operations
type Runtime interface {
	// Start creates and starts a container
	Start(ctx context.Context, instanceID, imageName, ipAddress string, vcpus, memory, port int, envVars map[string]string) error

	// Stop stops and removes a container
	Stop(ctx context.Context, instanceID string) error

	// List returns all running containers managed by this runtime
	List(ctx context.Context) ([]Container, error)

	// GetStatus returns the status of a specific container
	GetStatus(ctx context.Context, instanceID string) (*Container, error)

	// StreamLogs streams logs from a container
	StreamLogs(ctx context.Context, instanceID string, follow bool) (io.ReadCloser, error)

	// Close cleans up the runtime
	Close() error
}
