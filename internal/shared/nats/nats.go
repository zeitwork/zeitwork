package nats

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/zeitwork/zeitwork/internal/shared/config"
)

// Client wraps the NATS connection with simple functionality
type Client struct {
	conn *nats.Conn
}

// NewClient creates a new NATS client with the provided configuration
func NewClient(cfg *config.NATSConfig) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("NATS configuration is required")
	}

	// Create connection options for dev-local setup
	opts := []nats.Option{
		nats.Name("zeitwork-client"),
		nats.MaxReconnects(cfg.MaxReconnects),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.Timeout(cfg.Timeout),
	}

	// Connect to NATS
	conn, err := nats.Connect(cfg.URLs[0], opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	slog.Info("Connected to NATS", "url", cfg.URLs[0])

	return &Client{
		conn: conn,
	}, nil
}

// NewSimpleClient creates a NATS client with default local configuration
func NewSimpleClient() (*Client, error) {
	cfg := &config.NATSConfig{
		URLs:          []string{"nats://localhost:4222"},
		MaxReconnects: -1, // Unlimited reconnects
		ReconnectWait: nats.DefaultReconnectWait,
		Timeout:       nats.DefaultTimeout,
	}

	return NewClient(cfg)
}

// Publish publishes a message to the given subject
func (c *Client) Publish(subject string, data []byte) error {
	return c.conn.Publish(subject, data)
}

// PublishString publishes a string message to the given subject
func (c *Client) PublishString(subject, data string) error {
	return c.conn.Publish(subject, []byte(data))
}

// Subscribe creates a subscription to the given subject
func (c *Client) Subscribe(subject string, handler func(*nats.Msg)) (*nats.Subscription, error) {
	return c.conn.Subscribe(subject, handler)
}

// QueueSubscribe creates a queue subscription to the given subject
// Queue subscriptions allow multiple subscribers to form a queue group where only one subscriber
// receives each message, enabling load balancing and ensuring work is not duplicated
func (c *Client) QueueSubscribe(subject, queueGroup string, handler func(*nats.Msg)) (*nats.Subscription, error) {
	return c.conn.QueueSubscribe(subject, queueGroup, handler)
}

// SubscribeSync creates a synchronous subscription to the given subject
func (c *Client) SubscribeSync(subject string) (*nats.Subscription, error) {
	return c.conn.SubscribeSync(subject)
}

// QueueSubscribeSync creates a synchronous queue subscription to the given subject
func (c *Client) QueueSubscribeSync(subject, queueGroup string) (*nats.Subscription, error) {
	return c.conn.QueueSubscribeSync(subject, queueGroup)
}

// Request sends a request and waits for a response
func (c *Client) Request(subject string, data []byte) (*nats.Msg, error) {
	return c.conn.Request(subject, data, nats.DefaultTimeout)
}

// Close closes the NATS connection
func (c *Client) Close() error {
	if c.conn != nil {
		c.conn.Close()
		slog.Info("NATS connection closed")
	}
	return nil
}

// IsConnected returns true if the client is connected to NATS
func (c *Client) IsConnected() bool {
	return c.conn != nil && c.conn.IsConnected()
}

// Flush flushes any pending messages
func (c *Client) Flush() error {
	return c.conn.Flush()
}

// Stats returns connection statistics
func (c *Client) Stats() nats.Statistics {
	return c.conn.Stats()
}

// WithContext returns a context-aware wrapper for graceful shutdowns
func (c *Client) WithContext(ctx context.Context) *ContextClient {
	return &ContextClient{
		client: c,
		ctx:    ctx,
	}
}

// ContextClient wraps the NATS client with context support
type ContextClient struct {
	client *Client
	ctx    context.Context
}

// Publish publishes a message with context cancellation support
func (cc *ContextClient) Publish(subject string, data []byte) error {
	select {
	case <-cc.ctx.Done():
		return cc.ctx.Err()
	default:
		return cc.client.Publish(subject, data)
	}
}

// Subscribe creates a subscription that respects context cancellation
func (cc *ContextClient) Subscribe(subject string, handler func(*nats.Msg)) (*nats.Subscription, error) {
	sub, err := cc.client.Subscribe(subject, func(msg *nats.Msg) {
		select {
		case <-cc.ctx.Done():
			return
		default:
			handler(msg)
		}
	})

	if err != nil {
		return nil, err
	}

	// Monitor context and unsubscribe when cancelled
	go func() {
		<-cc.ctx.Done()
		sub.Unsubscribe()
	}()

	return sub, nil
}

// QueueSubscribe creates a queue subscription that respects context cancellation
func (cc *ContextClient) QueueSubscribe(subject, queueGroup string, handler func(*nats.Msg)) (*nats.Subscription, error) {
	sub, err := cc.client.QueueSubscribe(subject, queueGroup, func(msg *nats.Msg) {
		select {
		case <-cc.ctx.Done():
			return
		default:
			handler(msg)
		}
	})

	if err != nil {
		return nil, err
	}

	// Monitor context and unsubscribe when cancelled
	go func() {
		<-cc.ctx.Done()
		sub.Unsubscribe()
	}()

	return sub, nil
}
