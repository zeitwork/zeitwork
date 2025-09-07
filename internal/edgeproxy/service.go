package edgeproxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/nats"
	pb "github.com/zeitwork/zeitwork/proto"
)

// VMEndpoint represents a VM endpoint with its IP address (IPv4 or IPv6)
// IPv6 format: fd00:00:<region_id>:<node_id>:<vm_id>/64
// IPv4 format: 172.20.x.x (Docker internal network)
type VMEndpoint struct {
	URL       *url.URL
	IPAddress string // Full IP address (IPv4 or IPv6)
	RegionID  string // Extracted from address (IPv6 only)
	NodeID    string // Extracted from address (IPv6 only)
	VMID      string // Extracted from address (IPv6 only)
	Port      int    // Service port
}

// Route represents a routing configuration for a domain
// Maps domain names to VM endpoints
type Route struct {
	Domain    string       // e.g., app.example.com
	Endpoints []VMEndpoint // Available VM endpoints
}

// Service represents the native Go edge proxy service
type Service struct {
	logger          *slog.Logger
	config          *Config
	db              *pgxpool.Pool
	queries         *database.Queries
	natsClient      *nats.Client
	routesMu        sync.RWMutex
	routes          map[string]*Route // Key: domain name
	portHttp        int
	portHttps       int
	templateManager *TemplateManager

	// TLS certificate cache
	certMu sync.RWMutex
	certs  map[string]*tls.Certificate
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

	// Initialize template manager
	templateManager, err := NewTemplateManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create template manager: %w", err)
	}

	s := &Service{
		logger:          logger,
		config:          config,
		db:              db,
		queries:         queries,
		natsClient:      natsClient,
		routes:          make(map[string]*Route),
		portHttp:        config.PortHttp,
		portHttps:       config.PortHttps,
		templateManager: templateManager,
		certs:           make(map[string]*tls.Certificate),
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

	// HTTPS server with dynamic certificate selection
	tlsConfig := &tls.Config{
		GetCertificate: s.getCertificateForClientHello,
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2", "http/1.1"},
	}
	httpsServer := &http.Server{
		Addr:      fmt.Sprintf(":%d", s.portHttps),
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	// Start HTTP server
	go func() {
		s.logger.Info("Starting HTTP proxy server", "port", s.portHttp)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server failed", "error", err)
		}
	}()

	// Start HTTPS server with dynamic certs
	go func() {
		s.logger.Info("Starting HTTPS proxy server", "port", s.portHttps)
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.portHttps))
		if err != nil {
			s.logger.Error("HTTPS listen failed", "error", err)
			return
		}
		tlsListener := tls.NewListener(ln, tlsConfig)
		if err := httpsServer.Serve(tlsListener); err != nil && err != http.ErrServerClosed {
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
		s.templateManager.ServeErrorPage(w, r, http.StatusNotFound, domain, "Domain not found in routing table")
		return
	}

	if len(route.Endpoints) == 0 {
		s.logger.Warn("No endpoints available for domain", "domain", domain)
		s.templateManager.ServeErrorPage(w, r, http.StatusBadGateway, domain, "No healthy VMs available")
		return
	}

	// Select a VM endpoint using random load balancing
	endpoint := s.selectVMEndpoint(route.Endpoints)
	if endpoint == nil {
		s.logger.Error("Failed to select VM endpoint", "domain", domain)
		s.templateManager.ServeErrorPage(w, r, http.StatusBadGateway, domain, "No healthy VM available")
		return
	}

	// Log the routing decision
	s.logger.Debug("Routing request",
		"domain", domain,
		"ip_address", endpoint.IPAddress,
		"region", endpoint.RegionID,
		"node", endpoint.NodeID,
		"vm", endpoint.VMID,
		"port", endpoint.Port)

	// Proxy the request to the selected VM with error handling
	proxy := httputil.NewSingleHostReverseProxy(endpoint.URL)

	// Custom error handler for proxy failures
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		s.logger.Error("Proxy request failed",
			"domain", domain,
			"ip_address", endpoint.IPAddress,
			"error", err)

		// Determine appropriate error code based on the error
		statusCode := http.StatusBadGateway
		description := "Backend service is temporarily unavailable"

		if strings.Contains(err.Error(), "connection refused") {
			description = "Backend service is not responding"
		} else if strings.Contains(err.Error(), "timeout") {
			statusCode = http.StatusGatewayTimeout
			description = "Backend service request timed out"
		} else if strings.Contains(err.Error(), "no such host") {
			description = "Backend service address cannot be resolved"
		}

		s.templateManager.ServeErrorPage(w, r, statusCode, domain, description)
	}

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

// detectIPVersion determines if an address is IPv4 or IPv6
func detectIPVersion(address string) int {
	// Remove any CIDR suffix
	address = strings.Split(address, "/")[0]

	if strings.Contains(address, ":") {
		return 6 // IPv6
	}
	return 4 // IPv4
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

// normalizeIPAddress converts IP address to the format expected by the runtime
func normalizeIPAddress(address string, ipVersion int) string {
	// Remove CIDR suffix
	address = strings.Split(address, "/")[0]

	if ipVersion == 6 {
		// Convert IPv6 address format to match what Docker actually assigns
		// Convert fd00:00:199:199:199 -> fd00::199:199:199
		if strings.HasPrefix(address, "fd00:00:") {
			parts := strings.Split(address, ":")
			if len(parts) >= 5 {
				return fmt.Sprintf("fd00::%s:%s:%s",
					strings.TrimLeft(parts[2], "0"),
					strings.TrimLeft(parts[3], "0"),
					strings.TrimLeft(parts[4], "0"))
			}
		}
		// TODO: Production IPv6 Implementation
		// - Add fd12:: support for production IPv6 (avoids conflicts with existing fd62:: on host)
		// - Implement proper IPv6 routing for production environments
		// - Consider using ULA ranges that don't conflict with existing network infrastructure
		// - Add IPv6 firewall rules and security considerations
		if strings.HasPrefix(address, "fd12:00:") {
			// TODO: Implement fd12:: address normalization for production
			// Similar to fd00:: but with different prefix
		}
	}

	// IPv4 addresses are used as-is
	return address
}

// loadAndApplyConfig loads configuration from database and applies it to the proxy
//
// Development vs Production IP Handling:
// - Development: Uses Docker IPv4 addresses (172.20.x.x) which are accessible from macOS host
// - Production: Uses IPv6 addresses (fd00:: or fd12::) with proper routing infrastructure
// - The edge proxy automatically detects IP version and handles both transparently
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
				"ip_address", row.IpAddress)
			continue
		}

		// Detect IP version and normalize address
		ipVersion := detectIPVersion(row.IpAddress)
		normalizedAddress := normalizeIPAddress(row.IpAddress, ipVersion)

		// Create VM endpoint URL based on IP version
		var endpointURL *url.URL
		var err error

		if ipVersion == 6 {
			// IPv6 addresses need brackets in URLs
			endpointURL, err = url.Parse(fmt.Sprintf("http://[%s]:%d", normalizedAddress, row.DefaultPort))
		} else {
			// IPv4 addresses don't need brackets
			endpointURL, err = url.Parse(fmt.Sprintf("http://%s:%d", normalizedAddress, row.DefaultPort))
		}

		if err != nil {
			s.logger.Error("Failed to parse VM endpoint URL",
				"ip_address", row.IpAddress,
				"normalized", normalizedAddress,
				"port", row.DefaultPort,
				"error", err)
			continue
		}

		// Extract region, node, and VM IDs (only for IPv6)
		var regionID, nodeID, vmID string
		if ipVersion == 6 {
			regionID, nodeID, vmID = parseIPv6VM(row.IpAddress)
		}

		// Create VM endpoint
		vmEndpoint := VMEndpoint{
			URL:       endpointURL,
			IPAddress: row.IpAddress,
			RegionID:  regionID,
			NodeID:    nodeID,
			VMID:      vmID,
			Port:      int(row.DefaultPort),
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
			if ep.IPAddress == vmEndpoint.IPAddress && ep.Port == vmEndpoint.Port {
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

// routingUpdateSubscriber subscribes to NATS routing-relevant events
func (s *Service) routingUpdateSubscriber(ctx context.Context) {
	s.logger.Info("Starting NATS routing update subscriber")

	// Subscribe to deployment events
	deploymentSub, err := s.natsClient.WithContext(ctx).Subscribe("deployment.>", s.handleRoutingUpdate)
	if err != nil {
		s.logger.Error("Failed to subscribe to deployment events", "error", err)
		return
	}
	defer deploymentSub.Unsubscribe()

	// Subscribe to instance events
	instanceSub, err := s.natsClient.WithContext(ctx).Subscribe("instance.>", s.handleRoutingUpdate)
	if err != nil {
		s.logger.Error("Failed to subscribe to instance events", "error", err)
		return
	}
	defer instanceSub.Unsubscribe()

	// Subscribe to domain events
	domainSub, err := s.natsClient.WithContext(ctx).Subscribe("domain.>", s.handleRoutingUpdate)
	if err != nil {
		s.logger.Error("Failed to subscribe to domain events", "error", err)
		return
	}
	defer domainSub.Unsubscribe()

	// Subscribe to deployment_instance events
	deploymentInstanceSub, err := s.natsClient.WithContext(ctx).Subscribe("deployment_instance.>", s.handleRoutingUpdate)
	if err != nil {
		s.logger.Error("Failed to subscribe to deployment_instance events", "error", err)
		return
	}
	defer deploymentInstanceSub.Unsubscribe()

	// Subscribe to ssl_cert events for prefetch
	sslCertSub, err := s.natsClient.WithContext(ctx).Subscribe("ssl_cert.>", s.handleRoutingUpdate)
	if err != nil {
		s.logger.Error("Failed to subscribe to ssl_cert events", "error", err)
		return
	}
	defer sslCertSub.Unsubscribe()

	s.logger.Info("Subscribed to NATS routing events (deployment, instance, domain, deployment_instance, ssl_cert)")

	// Wait for context cancellation
	<-ctx.Done()
	s.logger.Info("Routing update subscriber stopped")
}

// handleRoutingUpdate handles incoming routing update messages from NATS
func (s *Service) handleRoutingUpdate(msg *natsgo.Msg) {
	s.logger.Debug("Received routing update message", "subject", msg.Subject)

	// Extract table and operation from subject (e.g., "deployment.created" -> "deployment", "created")
	parts := strings.Split(msg.Subject, ".")
	if len(parts) != 2 {
		s.logger.Error("Invalid subject format", "subject", msg.Subject)
		return
	}
	table := parts[0]
	operation := parts[1]

	// Parse the protobuf message based on the subject
	var id string
	var err error

	switch msg.Subject {
	case "deployment.created":
		var event pb.DeploymentCreated
		if err = proto.Unmarshal(msg.Data, &event); err == nil {
			id = event.GetId()
		}
	case "deployment.updated":
		var event pb.DeploymentUpdated
		if err = proto.Unmarshal(msg.Data, &event); err == nil {
			id = event.GetId()
		}
	case "instance.created":
		var event pb.InstanceCreated
		if err = proto.Unmarshal(msg.Data, &event); err == nil {
			id = event.GetId()
		}
	case "instance.updated":
		var event pb.InstanceUpdated
		if err = proto.Unmarshal(msg.Data, &event); err == nil {
			id = event.GetId()
		}
	case "domain.created":
		var event pb.DomainCreated
		if err = proto.Unmarshal(msg.Data, &event); err == nil {
			id = event.GetId()
		}
	case "domain.updated":
		var event pb.DomainUpdated
		if err = proto.Unmarshal(msg.Data, &event); err == nil {
			id = event.GetId()
		}
	case "deployment_instance.created":
		var event pb.DeploymentInstanceCreated
		if err = proto.Unmarshal(msg.Data, &event); err == nil {
			id = event.GetId()
		}
	case "deployment_instance.updated":
		var event pb.DeploymentInstanceUpdated
		if err = proto.Unmarshal(msg.Data, &event); err == nil {
			id = event.GetId()
		}
	case "ssl_cert.created":
		var event pb.SslCertCreated
		if err = proto.Unmarshal(msg.Data, &event); err == nil {
			id = event.GetId()
			// Prefetch cert by id
			s.prefetchCertByID(id)
			return
		}
	case "ssl_cert.updated":
		var event pb.SslCertUpdated
		if err = proto.Unmarshal(msg.Data, &event); err == nil {
			id = event.GetId()
			s.prefetchCertByID(id)
			return
		}
	default:
		s.logger.Debug("Ignoring unhandled event", "subject", msg.Subject)
		return
	}

	if err != nil {
		s.logger.Error("Failed to unmarshal protobuf message", "subject", msg.Subject, "error", err)
		return
	}

	s.logger.Info("Processing routing update",
		"table", table,
		"operation", operation,
		"id", id)

	// Trigger configuration reload for routing-relevant changes
	if s.isRoutingRelevantChange(table) {
		if err := s.TriggerConfigReload(); err != nil {
			s.logger.Error("Failed to reload configuration after routing update", "error", err)
		} else {
			s.logger.Info("Configuration reloaded due to routing update",
				"table", table,
				"operation", operation,
				"id", id)
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

// getCertificateForClientHello is called during TLS handshake to provide a certificate for the SNI name
func (s *Service) getCertificateForClientHello(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := strings.ToLower(hello.ServerName)
	if domain == "" {
		return nil, fmt.Errorf("missing SNI domain")
	}

	// 1) Try exact match
	s.certMu.RLock()
	cert, ok := s.certs[domain]
	s.certMu.RUnlock()
	if ok {
		return cert, nil
	}

	// 2) Try wildcard *.base domain if SNI is subdomain of base
	baseWildcard := s.getBaseWildcard()
	if baseWildcard != "" && strings.HasSuffix(domain, strings.TrimPrefix(baseWildcard, "*")) {
		s.certMu.RLock()
		cert, ok = s.certs[baseWildcard]
		s.certMu.RUnlock()
		if ok {
			return cert, nil
		}
	}

	// 3) Load from DB and cache
	loaded, err := s.loadCertificateFromDB(domain)
	if err == nil && loaded != nil {
		return loaded, nil
	}

	// 4) Fallback to loading wildcard and cache
	if baseWildcard != "" {
		loaded, err = s.loadCertificateFromDB(baseWildcard)
		if err == nil && loaded != nil {
			return loaded, nil
		}
	}

	return nil, fmt.Errorf("no certificate for domain %s", domain)
}

func (s *Service) getBaseWildcard() string {
	// Best-effort: detect from any cached wildcard
	s.certMu.RLock()
	defer s.certMu.RUnlock()
	for name := range s.certs {
		if strings.HasPrefix(name, "*.") {
			return name
		}
	}
	return ""
}

func (s *Service) loadCertificateFromDB(name string) (*tls.Certificate, error) {
	key := fmt.Sprintf("certs/%s", name)
	row, err := s.queries.SslCertsGetByKey(context.Background(), key)
	if err != nil || row == nil {
		return nil, fmt.Errorf("cert not found")
	}
	// The value column stores combined cert+key PEM (as written by certmanager local runtime)
	block, rest := pem.Decode([]byte(row.Value))
	if block == nil || len(rest) == 0 {
		return nil, fmt.Errorf("invalid PEM bundle")
	}
	cert, err := tls.X509KeyPair([]byte(row.Value), []byte(row.Value))
	if err != nil {
		return nil, err
	}
	s.certMu.Lock()
	s.certs[name] = &cert
	s.certMu.Unlock()
	return &cert, nil
}

func (s *Service) prefetchCertByID(id string) {
	// Load cert by id (reads key and value), then cache by domain name extracted from key
	row, err := s.queries.SslCertsGetById(context.Background(), uuid.MustParse(id))
	if err != nil || row == nil {
		s.logger.Error("Failed to prefetch cert by id", "id", id, "error", err)
		return
	}
	name := row.Key
	// stored key is like certs/<domain>
	parts := strings.SplitN(name, "/", 2)
	if len(parts) == 2 {
		name = parts[1]
	}
	if _, err := s.loadCertificateFromDB(name); err != nil {
		s.logger.Error("Prefetch loadCertificateFromDB failed", "name", name, "error", err)
	} else {
		s.logger.Info("Prefetched certificate", "name", name)
	}
}
