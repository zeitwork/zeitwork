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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/nats"
	pguuid "github.com/zeitwork/zeitwork/internal/shared/uuid"
	pb "github.com/zeitwork/zeitwork/proto"
)

// Endpoint represents a backend endpoint with its IP address
type Endpoint struct {
	URL       *url.URL
	IPAddress string // Full IP address (IPv4 or IPv6)
	Port      int    // Service port
}

// Route represents a routing configuration for a domain
// Maps domain names to VM endpoints
type Route struct {
	Domain    string     // e.g., app.example.com
	Endpoints []Endpoint // Available endpoints
}

// Service represents the native Go edge proxy service
type Service struct {
	logger          *slog.Logger
	config          *Config
	db              *pgxpool.Pool
	queries         *database.Queries
	natsClient      *nats.Client
	routes          atomic.Value // stores map[string]*Route
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
		natsClient, err = nats.NewClient(config.NATSConfig, "edgeproxy")
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
		portHttp:        config.PortHttp,
		portHttps:       config.PortHttps,
		templateManager: templateManager,
		certs:           make(map[string]*tls.Certificate),
	}

	// Initialize routes map for atomic access
	s.routes.Store(make(map[string]*Route))

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
	// Separate muxes for HTTP (redirect) and HTTPS (proxy)
	httpMux := http.NewServeMux()
	httpsMux := http.NewServeMux()

	// Shared health handler (available over both HTTP and HTTPS)
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		s.setStandardHeaders(w)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}

	// HTTPS handlers: full proxy + health
	httpsMux.HandleFunc("/", s.handleProxy)
	httpsMux.HandleFunc("/health", healthHandler)

	// HTTP handlers: health + redirect all other paths to HTTPS
	httpMux.HandleFunc("/health", healthHandler)
	httpMux.HandleFunc("/", s.redirectToHTTPS)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.portHttp),
		Handler: httpMux,
	}

	// HTTPS server with dynamic certificate selection
	tlsConfig := &tls.Config{
		GetCertificate: s.getCertificateForClientHello,
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2", "http/1.1"},
	}
	httpsServer := &http.Server{
		Addr:      fmt.Sprintf(":%d", s.portHttps),
		Handler:   httpsMux,
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

	// Look up route for this domain (lock-free read)
	current := s.routes.Load().(map[string]*Route)
	route, exists := current[domain]

	if !exists {
		s.setStandardHeaders(w)
		s.logger.Debug("Domain not found in routing table", "domain", domain)
		s.templateManager.ServeErrorPage(w, r, http.StatusNotFound, domain, "Domain not found in routing table")
		return
	}

	if len(route.Endpoints) == 0 {
		s.setStandardHeaders(w)
		s.logger.Warn("No endpoints available for domain", "domain", domain)
		s.templateManager.ServeErrorPage(w, r, http.StatusBadGateway, domain, "No healthy endpoints available")
		return
	}

	// Select an endpoint using random load balancing
	endpoint := s.selectEndpoint(route.Endpoints)
	if endpoint == nil {
		s.setStandardHeaders(w)
		s.logger.Error("Failed to select endpoint", "domain", domain)
		s.templateManager.ServeErrorPage(w, r, http.StatusBadGateway, domain, "No healthy endpoint available")
		return
	}

	// Log the routing decision
	s.logger.Debug("Routing request",
		"domain", domain,
		"ip_address", endpoint.IPAddress,
		"port", endpoint.Port)

	// Proxy the request to the selected VM with error handling
	proxy := httputil.NewSingleHostReverseProxy(endpoint.URL)

	// Ensure Server header on successful proxy responses
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("Server", "Zeitwork")
		resp.Header.Set("X-Zeitwork-Endpoint", fmt.Sprintf("%s:%d", endpoint.IPAddress, endpoint.Port))
		return nil
	}

	// Custom error handler for proxy failures
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		s.setStandardHeaders(w)
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

// selectEndpoint randomly selects an endpoint from the available endpoints
func (s *Service) selectEndpoint(endpoints []Endpoint) *Endpoint {
	if len(endpoints) == 0 {
		return nil
	}

	// Random selection from available endpoints
	// All endpoints are assumed healthy as they're filtered during config loading
	selected := endpoints[rand.Intn(len(endpoints))]
	return &selected
}

// loadAndApplyConfig loads configuration from database and applies it to the proxy
func (s *Service) loadAndApplyConfig() error {
	// Get all active deployments with their routes
	rows, err := s.queries.DeploymentsGetActiveRoutes(context.Background())
	if err != nil {
		return fmt.Errorf("failed to load routes from database: %w", err)
	}

	// Build domain -> endpoints mapping
	routeMap := make(map[string]*Route)

	for _, row := range rows {
		// Extract domain
		domain := strings.ToLower(row.Domain)
		if domain == "" {
			continue
		}

		// Skip unhealthy endpoints
		if !row.Healthy {
			s.logger.Debug("Skipping unhealthy VM",
				"domain", domain,
				"ip_address", row.IpAddress)
			continue
		}

		// Create endpoint URL (use IP and instance default_port)
		var endpointURL *url.URL
		var err error
		endpointURL, err = url.Parse(fmt.Sprintf("http://%s:%d", row.IpAddress, row.DefaultPort))

		if err != nil {
			s.logger.Error("Failed to parse VM endpoint URL",
				"ip_address", row.IpAddress,
				"port", row.DefaultPort,
				"error", err)
			continue
		}

		// Create endpoint
		endpoint := Endpoint{
			URL:       endpointURL,
			IPAddress: row.IpAddress,
			Port:      int(row.DefaultPort),
		}

		// Register route under bare domain and common port-suffixed variants
		keys := []string{domain}
		if s.portHttp != 80 {
			keys = append(keys, fmt.Sprintf("%s:%d", domain, s.portHttp))
		}
		if s.portHttps != 443 {
			keys = append(keys, fmt.Sprintf("%s:%d", domain, s.portHttps))
		}
		for _, key := range keys {
			route, exists := routeMap[key]
			if !exists {
				route = &Route{Domain: key, Endpoints: make([]Endpoint, 0)}
				routeMap[key] = route
			}
			// Add endpoint to route (avoid duplicates)
			endpointExists := false
			for _, ep := range route.Endpoints {
				if ep.IPAddress == endpoint.IPAddress && ep.Port == endpoint.Port {
					endpointExists = true
					break
				}
			}
			if !endpointExists {
				route.Endpoints = append(route.Endpoints, endpoint)
			}
		}
	}

	// Atomically swap the routes map
	s.routes.Store(routeMap)

	// Log routing table summary
	totalDomains := len(routeMap)
	totalEndpoints := 0
	for domain, route := range routeMap {
		totalEndpoints += len(route.Endpoints)
		s.logger.Debug("Domain route configured",
			"domain", domain,
			"endpoint_count", len(route.Endpoints))
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

	// 5) Derive wildcard from requested domain and attempt load (e.g., app.example.com -> *.example.com)
	if wildcard := deriveBaseWildcard(domain); wildcard != "" {
		loaded, err = s.loadCertificateFromDB(wildcard)
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

// setStandardHeaders sets common response headers for all edgeproxy responses.
func (s *Service) setStandardHeaders(w http.ResponseWriter) {
	w.Header().Set("Server", "Zeitwork")
}

// deriveBaseWildcard builds a wildcard candidate from the requested domain, e.g.
// "app.example.com" -> "*.example.com". Returns empty string if not derivable.
func deriveBaseWildcard(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		base := strings.Join(parts[len(parts)-2:], ".")
		return "*." + base
	}
	return ""
}

// redirectToHTTPS redirects all HTTP requests to HTTPS preserving host and path.
func (s *Service) redirectToHTTPS(w http.ResponseWriter, r *http.Request) {
	// Allow local HTTP health checks without redirect
	if r.URL.Path == "/health" {
		s.setStandardHeaders(w)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
		return
	}

	// Strip incoming port and rebuild with configured HTTPS port
	hostOnly := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		hostOnly = h
	}

	httpsHost := hostOnly
	if s.portHttps != 443 {
		httpsHost = net.JoinHostPort(hostOnly, strconv.Itoa(s.portHttps))
	}

	target := &url.URL{
		Scheme: "https",
		Host:   httpsHost,
		Path:   r.URL.Path,
	}
	// Preserve query string if present
	target.RawQuery = r.URL.RawQuery

	s.setStandardHeaders(w)
	http.Redirect(w, r, target.String(), http.StatusPermanentRedirect)
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
	row, err := s.queries.SslCertsGetById(context.Background(), pguuid.MustParseUUID(id))
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
