package events

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	sharedNats "github.com/zeitwork/zeitwork/internal/shared/nats"
)

// EventHandler defines the interface for handling events
type EventHandler interface {
	HandleEvent(ctx context.Context, data []byte) error
}

// Subscriber handles NATS event subscriptions and processing
type Subscriber struct {
	logger     *slog.Logger
	nodeID     uuid.UUID
	natsClient *sharedNats.Client

	// Event handlers
	instanceCreatedHandler *InstanceCreatedHandler
	instanceUpdatedHandler *InstanceUpdatedHandler
	nodeUpdatedHandler     *NodeUpdatedHandler

	// Subscription management
	subscriptions []*nats.Subscription
	subsMu        sync.RWMutex
}

// NewSubscriber creates a new NATS event subscriber
func NewSubscriber(logger *slog.Logger, nodeID uuid.UUID, natsClient *sharedNats.Client,
	instanceCreatedHandler *InstanceCreatedHandler,
	instanceUpdatedHandler *InstanceUpdatedHandler,
	nodeUpdatedHandler *NodeUpdatedHandler) *Subscriber {
	return &Subscriber{
		logger:                 logger,
		nodeID:                 nodeID,
		natsClient:             natsClient,
		instanceCreatedHandler: instanceCreatedHandler,
		instanceUpdatedHandler: instanceUpdatedHandler,
		nodeUpdatedHandler:     nodeUpdatedHandler,
	}
}

// Subscribe starts subscribing to relevant NATS topics
func (s *Subscriber) Subscribe(ctx context.Context) error {
	s.logger.Info("Starting NATS event subscriptions", "node_id", s.nodeID)

	// Subscribe to instance events
	if err := s.subscribeToTopic(ctx, "instance.created", s.handleInstanceCreated); err != nil {
		return fmt.Errorf("failed to subscribe to instance.created: %w", err)
	}

	if err := s.subscribeToTopic(ctx, "instance.updated", s.handleInstanceUpdated); err != nil {
		return fmt.Errorf("failed to subscribe to instance.updated: %w", err)
	}

	// Subscribe to node events
	if err := s.subscribeToTopic(ctx, "node.updated", s.handleNodeUpdated); err != nil {
		return fmt.Errorf("failed to subscribe to node.updated: %w", err)
	}

	s.logger.Info("NATS event subscriptions started")
	return nil
}

// Unsubscribe stops all NATS subscriptions
func (s *Subscriber) Unsubscribe() {
	s.logger.Info("Stopping NATS event subscriptions")

	s.subsMu.Lock()
	defer s.subsMu.Unlock()

	for _, sub := range s.subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			s.logger.Warn("Failed to unsubscribe", "error", err)
		}
	}

	s.subscriptions = nil
	s.logger.Info("NATS event subscriptions stopped")
}

// subscribeToTopic subscribes to a specific NATS topic
func (s *Subscriber) subscribeToTopic(ctx context.Context, subject string, handler nats.MsgHandler) error {
	sub, err := s.natsClient.WithContext(ctx).Subscribe(subject, handler)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	s.subsMu.Lock()
	s.subscriptions = append(s.subscriptions, sub)
	s.subsMu.Unlock()

	s.logger.Debug("Subscribed to NATS topic", "subject", subject)
	return nil
}

// Event handlers

// handleInstanceCreated handles instance.created events
func (s *Subscriber) handleInstanceCreated(msg *nats.Msg) {
	s.logger.Debug("Received instance.created event")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.instanceCreatedHandler.HandleEvent(ctx, msg.Data); err != nil {
		s.logger.Error("Failed to handle instance.created event", "error", err)
	}
}

// handleInstanceUpdated handles instance.updated events
func (s *Subscriber) handleInstanceUpdated(msg *nats.Msg) {
	s.logger.Debug("Received instance.updated event")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.instanceUpdatedHandler.HandleEvent(ctx, msg.Data); err != nil {
		s.logger.Error("Failed to handle instance.updated event", "error", err)
	}
}

// handleNodeUpdated handles node.updated events
func (s *Subscriber) handleNodeUpdated(msg *nats.Msg) {
	s.logger.Debug("Received node.updated event")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.nodeUpdatedHandler.HandleEvent(ctx, msg.Data); err != nil {
		s.logger.Error("Failed to handle node.updated event", "error", err)
	}
}
