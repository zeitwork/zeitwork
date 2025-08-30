package edgeproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	natsgo "github.com/nats-io/nats.go"

	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/nats"
)

// VMEndpoint represents a VM endpoint with its IPv6 address
// IPv6 format: fd00:00:<region_id>:<node_id>:<vm_id>/64
type VMEndpoint struct {
	URL      *url.URL
	IPv6     string // Full IPv6 address
	RegionID string // Extracted from IPv6
	NodeID   string // Extracted from IPv6
	VMID     string // Extracted from IPv6
	Port     int    // Service port
}

// Route represents a routing configuration for a domain
// Maps domain names to VM endpoints
type Route struct {
	Domain    string       // e.g., app.example.com
	Endpoints []VMEndpoint // Available VM endpoints
}

// Service represents the native Go edge proxy service
type Service struct {
	logger     *slog.Logger
	config     *Config
	db         *pgxpool.Pool
	queries    *database.Queries
	natsClient *nats.Client
	routesMu   sync.RWMutex
	routes     map[string]*Route // Key: domain name
	portHttp   int
	portHttps  int
}

// Config holds the configuration for the edge proxy service
type Config struct {
	PortHttp           int
	PortHttps          int
	DatabaseURL        string
	ConfigPollInterval time.Duration
	NATSConfig         *config.NATSConfig
}

// NewService creates a new native Go edge proxy service
func NewService(config *Config, logger *slog.Logger) (*Service, error) {
	// Set defaults
	if config.DatabaseURL == "" {
		config.DatabaseURL = "postgres://localhost/zeitwork"
	}

	if config.ConfigPollInterval == 0 {
		config.ConfigPollInterval = 6 * time.Hour
	}
	if config.PortHttp == 0 {
		config.PortHttp = 8080
	}
	if config.PortHttps == 0 {
		config.PortHttps = 8443
	}

	// Initialize database connection pool
	dbConfig, err := pgxpool.ParseConfig(config.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	db, err := pgxpool.NewWithConfig(context.Background(), dbConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	queries := database.New(db)

	// Create NATS client if configured
	var natsClient *nats.Client
	if config.NATSConfig != nil {
		var err error
		natsClient, err = nats.NewClient(config.NATSConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create NATS client: %w", err)
		}
		logger.Info("Connected to NATS for routing updates")
	}

	s := &Service{
		logger:     logger,
		config:     config,
		db:         db,
		queries:    queries,
		natsClient: natsClient,
		routes:     make(map[string]*Route),
		portHttp:   config.PortHttp,
		portHttps:  config.PortHttps,
	}

	return s, nil
}

// Start starts the edge proxy service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting edge proxy service")

	// Initial configuration load
	if err := s.loadAndApplyConfig(); err != nil {
		return fmt.Errorf("failed to load initial configuration: %w", err)
	}

	// Start configuration polling
	go s.configPoller(ctx)

	// Start NATS subscriber for real-time routing updates
	if s.natsClient != nil {
		go s.routingUpdateSubscriber(ctx)
	}

	// Start the HTTP server
	go s.startHTTPServer(ctx)

	// Wait for context cancellation
	<-ctx.Done()

	s.logger.Info("Shutting down edge proxy service")
	return nil
}

// configPoller polls the database for configuration changes
func (s *Service) configPoller(ctx context.Context) {
	ticker := time.NewTicker(s.config.ConfigPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.loadAndApplyConfig(); err != nil {
				s.logger.Error("Failed to reload configuration", "error", err)
			}
		}
	}
}

// startHTTPServer starts the main HTTP proxy server
func (s *Service) startHTTPServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleProxy)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.portHttp),
		Handler: mux,
	}

	httpsServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.portHttps),
		Handler: mux,
	}

	// Start HTTP server
	go func() {
		s.logger.Info("Starting HTTP proxy server", "port", s.portHttp)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server failed", "error", err)
		}
	}()

	// Start HTTPS server
	go func() {
		s.logger.Info("Starting HTTPS proxy server", "port", s.portHttps)
		// TODO: Add TLS certificate configuration
		if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTPS server failed", "error", err)
		}
	}()

	// Wait for context cancellation and shutdown
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("HTTP server shutdown failed", "error", err)
		}
	}()

	go func() {
		if err := httpsServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("HTTPS server shutdown failed", "error", err)
		}
	}()
}

// handleProxy handles incoming HTTP requests and routes them to appropriate VMs
// Routing pattern: app.example.com -> [fd00:00:0000:0001:0001/64:port, ...]
// IPv6 format: fd00:00:<region_id>:<node_id>:<vm_id>/64
func (s *Service) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Extract domain from request
	domain := strings.ToLower(r.Host)

	// Look up route for this domain
	s.routesMu.RLock()
	route, exists := s.routes[domain]
	s.routesMu.RUnlock()

	if !exists {
		s.logger.Debug("Domain not found in routing table", "domain", domain)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("404 - domain not found"))
		return
	}

	if len(route.Endpoints) == 0 {
		s.logger.Warn("No endpoints available for domain", "domain", domain)
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("502 - no healthy VMs available"))
		return
	}

	// Select a VM endpoint using random load balancing
	endpoint := s.selectVMEndpoint(route.Endpoints)
	if endpoint == nil {
		s.logger.Error("Failed to select VM endpoint", "domain", domain)
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("502 - no healthy VM available"))
		return
	}

	// Log the routing decision
	s.logger.Debug("Routing request",
		"domain", domain,
		"vm_ipv6", endpoint.IPv6,
		"region", endpoint.RegionID,
		"node", endpoint.NodeID,
		"vm", endpoint.VMID,
		"port", endpoint.Port)

	// Proxy the request to the selected VM
	proxy := httputil.NewSingleHostReverseProxy(endpoint.URL)
	proxy.ServeHTTP(w, r)
}

// selectVMEndpoint randomly selects a VM endpoint from the available endpoints
func (s *Service) selectVMEndpoint(endpoints []VMEndpoint) *VMEndpoint {
	if len(endpoints) == 0 {
		return nil
	}

	// Random selection from available endpoints
	// All endpoints are assumed healthy as they're filtered during config loading
	selected := endpoints[rand.Intn(len(endpoints))]
	return &selected
}

// parseIPv6VM extracts region, node, and VM IDs from an IPv6 address
// Expected format: fd00:00:<region_id>:<node_id>:<vm_id>/64
func parseIPv6VM(ipv6 string) (regionID, nodeID, vmID string) {
	// Remove the /64 suffix if present
	ipv6 = strings.TrimSuffix(ipv6, "/64")

	// Split the IPv6 address into segments
	segments := strings.Split(ipv6, ":")
	if len(segments) >= 5 {
		regionID = segments[2]
		nodeID = segments[3]
		vmID = segments[4]
	}
	return
}

// loadAndApplyConfig loads configuration from database and applies it to the proxy
func (s *Service) loadAndApplyConfig() error {
	// Get all active deployments with their routes
	rows, err := s.queries.DeploymentsGetActiveRoutes(context.Background())
	if err != nil {
		return fmt.Errorf("failed to load routes from database: %w", err)
	}

	// Build domain -> VM endpoints mapping
	routeMap := make(map[string]*Route)

	for _, row := range rows {
		// Extract domain
		domain := strings.ToLower(row.Domain)
		if domain == "" {
			continue
		}

		// Skip unhealthy VMs
		if !row.Healthy {
			s.logger.Debug("Skipping unhealthy VM",
				"domain", domain,
				"ipv6", row.IpAddress)
			continue
		}

		// Create VM endpoint URL
		endpointURL, err := url.Parse(fmt.Sprintf("http://[%s]:%d", row.IpAddress, row.DefaultPort))
		if err != nil {
			s.logger.Error("Failed to parse VM endpoint URL",
				"ipv6", row.IpAddress,
				"port", row.DefaultPort,
				"error", err)
			continue
		}

		// Parse IPv6 to extract region, node, and VM IDs
		regionID, nodeID, vmID := parseIPv6VM(row.IpAddress)

		// Create VM endpoint
		vmEndpoint := VMEndpoint{
			URL:      endpointURL,
			IPv6:     row.IpAddress,
			RegionID: regionID,
			NodeID:   nodeID,
			VMID:     vmID,
			Port:     int(row.DefaultPort),
		}

		// Get or create route for this domain
		route, exists := routeMap[domain]
		if !exists {
			route = &Route{
				Domain:    domain,
				Endpoints: make([]VMEndpoint, 0),
			}
			routeMap[domain] = route
		}

		// Add VM endpoint to route (avoid duplicates)
		endpointExists := false
		for _, ep := range route.Endpoints {
			if ep.IPv6 == vmEndpoint.IPv6 && ep.Port == vmEndpoint.Port {
				endpointExists = true
				break
			}
		}
		if !endpointExists {
			route.Endpoints = append(route.Endpoints, vmEndpoint)
		}
	}

	// Update routes atomically
	s.routesMu.Lock()
	s.routes = routeMap
	s.routesMu.Unlock()

	// Log routing table summary
	totalDomains := len(routeMap)
	totalEndpoints := 0
	for domain, route := range routeMap {
		totalEndpoints += len(route.Endpoints)
		s.logger.Debug("Domain route configured",
			"domain", domain,
			"vm_count", len(route.Endpoints))
	}

	s.logger.Info("Routing table updated",
		"domains", totalDomains,
		"total_endpoints", totalEndpoints)

	return nil
}

// routingUpdateSubscriber subscribes to NATS routing update messages
func (s *Service) routingUpdateSubscriber(ctx context.Context) {
	s.logger.Info("Starting NATS routing update subscriber")

	// Subscribe to general routing updates
	sub, err := s.natsClient.WithContext(ctx).Subscribe("routing.updates", s.handleRoutingUpdate)
	if err != nil {
		s.logger.Error("Failed to subscribe to routing updates", "error", err)
		return
	}
	defer sub.Unsubscribe()

	s.logger.Info("Subscribed to NATS routing updates")

	// Wait for context cancellation
	<-ctx.Done()
	s.logger.Info("Routing update subscriber stopped")
}

// handleRoutingUpdate handles incoming routing update messages from NATS
func (s *Service) handleRoutingUpdate(msg *natsgo.Msg) {
	s.logger.Debug("Received routing update message", "subject", msg.Subject)

	// Parse the JSON message since we don't use protobuf for routing updates anymore
	var change struct {
		Table     string `json:"table"`
		Operation string `json:"operation"`
		ID        string `json:"id"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := json.Unmarshal(msg.Data, &change); err != nil {
		s.logger.Error("Failed to unmarshal routing update", "error", err)
		return
	}

	s.logger.Info("Processing routing update",
		"table", change.Table,
		"operation", change.Operation,
		"id", change.ID)

	// Trigger configuration reload for routing-relevant changes
	if s.isRoutingRelevantChange(change.Table) {
		if err := s.TriggerConfigReload(); err != nil {
			s.logger.Error("Failed to reload configuration after routing update", "error", err)
		} else {
			s.logger.Info("Configuration reloaded due to routing update",
				"table", change.Table,
				"operation", change.Operation)
		}
	}
}

// isRoutingRelevantChange determines if a database change affects routing
func (s *Service) isRoutingRelevantChange(table string) bool {
	switch table {
	case "deployment", "domain", "instance", "deployment_instance":
		return true
	default:
		return false
	}
}

// Close closes the edge proxy service
func (s *Service) Close() error {
	if s.natsClient != nil {
		if err := s.natsClient.Close(); err != nil {
			s.logger.Error("Failed to close NATS client", "error", err)
		}
	}
	if s.db != nil {
		s.db.Close()
	}
	return nil
}

// TriggerConfigReload triggers an immediate configuration reload
func (s *Service) TriggerConfigReload() error {
	return s.loadAndApplyConfig()
}
