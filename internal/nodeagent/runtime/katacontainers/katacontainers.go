package katacontainers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zeitwork/zeitwork/internal/nodeagent/runtime"
)

// KataRuntime implements the Runtime interface using Kata Containers
type KataRuntime struct {
	logger *slog.Logger
}

// NewKataRuntime creates a new Kata runtime
func NewKataRuntime(logger *slog.Logger) (*KataRuntime, error) {
	return &KataRuntime{
		logger: logger,
	}, nil
}

// Start creates and starts a container
func (k *KataRuntime) Start(ctx context.Context, instanceID, imageName, ipAddress string, vcpus, memory, port int, envVars map[string]string) error {
	return fmt.Errorf("kata runtime not yet implemented")
}

// Stop stops and removes a container
func (k *KataRuntime) Stop(ctx context.Context, instanceID string) error {
	return fmt.Errorf("kata runtime not yet implemented")
}

// List returns all running containers managed by this runtime
func (k *KataRuntime) List(ctx context.Context) ([]runtime.Container, error) {
	return nil, fmt.Errorf("kata runtime not yet implemented")
}

// GetStatus returns the status of a specific container
func (k *KataRuntime) GetStatus(ctx context.Context, instanceID string) (*runtime.Container, error) {
	return nil, fmt.Errorf("kata runtime not yet implemented")
}

// Close cleans up the runtime
func (k *KataRuntime) Close() error {
	return nil
}
