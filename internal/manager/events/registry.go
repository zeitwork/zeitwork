package events

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/samber/lo"
)

// Registry manages event handlers with type safety
type Registry struct {
	handlers map[EventType]Handler
	logger   *slog.Logger
}

// NewRegistry creates a new event registry
func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		handlers: make(map[EventType]Handler),
		logger:   logger,
	}
}

// Register adds a handler for a specific event type
func (r *Registry) Register(handler Handler) error {
	eventType := handler.EventType()

	if _, exists := r.handlers[eventType]; exists {
		return fmt.Errorf("handler already registered for event type: %s", eventType)
	}

	r.handlers[eventType] = handler
	r.logger.Info("Registered event handler",
		"event_type", eventType,
		"handler", fmt.Sprintf("%T", handler))

	return nil
}

// MustRegister registers a handler and panics on error
func (r *Registry) MustRegister(handler Handler) {
	lo.Must0(r.Register(handler))
}

// GetHandler retrieves a handler for the given event type
func (r *Registry) GetHandler(eventType EventType) (Handler, bool) {
	handler, exists := r.handlers[eventType]
	return handler, exists
}

// GetAllEventTypes returns all registered event types
func (r *Registry) GetAllEventTypes() []EventType {
	return lo.Keys(r.handlers)
}

// HandleEvent routes an event to the appropriate handler
func (r *Registry) HandleEvent(ctx context.Context, eventType EventType, data []byte) error {
	handler, exists := r.GetHandler(eventType)
	if !exists {
		return fmt.Errorf("no handler registered for event type: %s", eventType)
	}

	r.logger.Debug("Routing event to handler",
		"event_type", eventType,
		"handler", fmt.Sprintf("%T", handler))

	return handler.HandleEvent(ctx, data)
}

// GetHandlerCount returns the number of registered handlers
func (r *Registry) GetHandlerCount() int {
	return len(r.handlers)
}
