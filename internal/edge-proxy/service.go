package edgeproxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/ssl"
	internaltls "github.com/zeitwork/zeitwork/internal/shared/tls"
	"golang.org/x/time/rate"
)

// Service represents the edge proxy service
type Service struct {
	logger     *slog.Logger
	config     *Config
	httpClient *http.Client

	// Backend worker nodes (no longer proxying to load balancer)
	backends   []*WorkerBackend
	backendsMu sync.RWMutex
	currentIdx atomic.Uint32 // For round-robin

	// Rate limiting
	rateLimiters  map[string]*rate.Limiter
	rateLimiterMu sync.RWMutex

	// SSL/TLS for external traffic
	tlsConfig  *tls.Config
	sslManager *ssl.Manager

	// mTLS for internal traffic to workers
	internalCA     *internaltls.InternalCA
	internalClient *http.Client

	// Domain-based routing
	routingCache map[string]*RouteTarget // domain -> backend
	routingMu    sync.RWMutex
}

// WorkerBackend represents a worker node backend
type WorkerBackend struct {
	ID          string
	Address     string // IP:Port for connection
	Healthy     bool
	Connections int32
	LastCheck   time.Time
}

// RouteTarget represents a routing target
type RouteTarget struct {
	BackendURL  *url.URL
	InstanceID  string
	LastUpdated time.Time
}

// Config holds the configuration for the edge proxy service
type Config struct {
	Port         string
	OperatorURL  string // For discovering worker nodes (no longer LoadBalancerURL)
	SSLCertPath  string
	SSLKeyPath   string
	RateLimitRPS int
	DatabaseURL  string // For routing lookups

	// mTLS configuration for internal communication
	EnableMTLS      bool
	InternalCAPath  string
	InternalKeyPath string
	InternalCertDir string
}

// NewService creates a new edge proxy service
func NewService(config *Config, logger *slog.Logger) (*Service, error) {
	s := &Service{
		logger:       logger,
		config:       config,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		backends:     make([]*WorkerBackend, 0),
		rateLimiters: make(map[string]*rate.Limiter),
		routingCache: make(map[string]*RouteTarget),
	}

	// Initialize SSL manager for automatic certificate management
	if os.Getenv("DATABASE_URL") != "" && os.Getenv("ENABLE_SSL_AUTOMATION") == "true" {
		sslConfig := &ssl.Config{
			DatabaseURL: os.Getenv("DATABASE_URL"),
			Email:       os.Getenv("ACME_EMAIL"),
			StagingMode: os.Getenv("ACME_STAGING") == "true",
			DNSProvider: os.Getenv("DNS_PROVIDER"),
		}

		manager, err := ssl.NewManager(sslConfig, logger)
		if err != nil {
			logger.Warn("Failed to initialize SSL manager", "error", err)
		} else {
			s.sslManager = manager
			// Start SSL manager
			go func() {
				if err := manager.Start(context.Background()); err != nil {
					logger.Error("Failed to start SSL manager", "error", err)
				}
			}()
			// Use dynamic TLS configuration
			s.tlsConfig = manager.GetTLSConfig()
			s.logger.Info("SSL automation enabled with Let's Encrypt")
		}
	} else if config.SSLCertPath != "" && config.SSLKeyPath != "" {
		// Fall back to static certificates if provided
		cert, err := tls.LoadX509KeyPair(config.SSLCertPath, config.SSLKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS certificates: %w", err)
		}

		s.tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			},
		}
	}

	// Initialize internal CA for mTLS with worker nodes
	if config.EnableMTLS {
		caConfig := &internaltls.InternalCAConfig{
			CertDir:        config.InternalCertDir,
			CAKeyPath:      config.InternalKeyPath,
			CACertPath:     config.InternalCAPath,
			RotationPeriod: 30 * 24 * time.Hour, // 30 days
			ValidityPeriod: 90 * 24 * time.Hour, // 90 days
			Organization:   "Zeitwork",
			Country:        "US",
		}

		// Set defaults if not provided
		if caConfig.CertDir == "" {
			caConfig.CertDir = "/var/lib/zeitwork/certs"
		}
		if caConfig.CAKeyPath == "" {
			caConfig.CAKeyPath = "/var/lib/zeitwork/ca/ca.key"
		}
		if caConfig.CACertPath == "" {
			caConfig.CACertPath = "/var/lib/zeitwork/ca/ca.crt"
		}

		internalCA, err := internaltls.NewInternalCA(caConfig, logger)
		if err != nil {
			logger.Warn("Failed to initialize internal CA, falling back to plain HTTP", "error", err)
		} else {
			s.internalCA = internalCA

			// Create mTLS client for internal communication
			clientTLSConfig, err := internalCA.GetClientTLSConfig("edge-proxy")
			if err != nil {
				logger.Warn("Failed to create mTLS client config", "error", err)
			} else {
				s.internalClient = internaltls.NewMTLSClient(clientTLSConfig)
				logger.Info("mTLS enabled for internal communication")
			}
		}
	}

	// If no internal client created, use regular HTTP client
	if s.internalClient == nil {
		s.internalClient = s.httpClient
	}

	return s, nil
}

// Start starts the edge proxy service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting edge proxy service",
		"port", s.config.Port,
		"operator", s.config.OperatorURL,
		"rate_limit", s.config.RateLimitRPS,
		"ssl", s.tlsConfig != nil,
	)

	// Start backend discovery
	go s.discoverBackendsPeriodically(ctx)

	// Start health checking for backends
	go s.checkBackendHealthPeriodically(ctx)

	// Start cleanup goroutine for rate limiters
	go s.cleanupRateLimiters(ctx)

	// Create HTTP server
	mux := http.NewServeMux()
	s.setupRoutes(mux)

	server := &http.Server{
		Addr:      ":" + s.config.Port,
		Handler:   mux,
		TLSConfig: s.tlsConfig,
	}

	// Start server in goroutine
	go func() {
		var err error
		if s.tlsConfig != nil {
			// HTTPS server
			err = server.ListenAndServeTLS("", "")
		} else {
			// HTTP server
			err = server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			s.logger.Error("Failed to start server", "error", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown server
	s.logger.Info("Shutting down edge proxy service")
	return server.Shutdown(context.Background())
}

// setupRoutes sets up the HTTP routes for the edge proxy
func (s *Service) setupRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("GET /health", s.handleHealth)

	// Metrics endpoint
	mux.HandleFunc("GET /metrics", s.handleMetrics)

	// Proxy all other requests with middleware
	mux.HandleFunc("/", s.withMiddleware(s.handleProxy))
}

// withMiddleware wraps a handler with common middleware
func (s *Service) withMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Rate limiting
		if !s.checkRateLimit(r) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// CORS headers (configure as needed)
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if r.Method == "OPTIONS" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		// Call the next handler
		next(w, r)
	}
}

// handleHealth handles health check requests
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check if we have healthy backends
	s.backendsMu.RLock()
	healthyCount := 0
	totalCount := len(s.backends)
	for _, backend := range s.backends {
		if backend.Healthy {
			healthyCount++
		}
	}
	s.backendsMu.RUnlock()

	status := "healthy"
	if healthyCount == 0 && totalCount > 0 {
		status = "unhealthy"
	} else if healthyCount < totalCount {
		status = "degraded"
	}

	response := map[string]interface{}{
		"status":           status,
		"healthy_backends": healthyCount,
		"total_backends":   totalCount,
		"rate_limit_rps":   s.config.RateLimitRPS,
		"ssl_enabled":      s.tlsConfig != nil,
	}

	w.Header().Set("Content-Type", "application/json")
	if status != "healthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(response)
}

// handleMetrics handles metrics requests
func (s *Service) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.rateLimiterMu.RLock()
	activeClients := len(s.rateLimiters)
	s.rateLimiterMu.RUnlock()

	metrics := map[string]interface{}{
		"active_clients": activeClients,
		"rate_limit_rps": s.config.RateLimitRPS,
		"ssl_enabled":    s.tlsConfig != nil,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// handleProxy proxies requests to the load balancer
func (s *Service) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Add X-Forwarded headers
	if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		r.Header.Set("X-Real-IP", clientIP)
		r.Header.Set("X-Forwarded-For", clientIP)
	}

	if s.tlsConfig != nil {
		r.Header.Set("X-Forwarded-Proto", "https")
	} else {
		r.Header.Set("X-Forwarded-Proto", "http")
	}

	r.Header.Set("X-Forwarded-Host", r.Host)

	// Route based on domain to specific instance
	if backend := s.routeByDomain(r.Host); backend != nil {
		// Create a custom proxy for this specific backend
		proxy := httputil.NewSingleHostReverseProxy(backend)
		proxy.ErrorHandler = s.errorHandler
		proxy.ServeHTTP(w, r)
		return
	}

	// Otherwise, select a worker backend using round-robin
	backend := s.selectBackend()
	if backend == nil {
		s.logger.Error("No healthy backends available")
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// Create proxy to worker node
	targetURL, _ := url.Parse(fmt.Sprintf("http://%s", backend.Address))
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ErrorHandler = s.errorHandler

	// Use mTLS client if configured
	if s.internalClient != nil {
		proxy.Transport = s.internalClient.Transport
	}

	proxy.ServeHTTP(w, r)
}

// errorHandler handles errors from the reverse proxy
func (s *Service) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.Error("Proxy error", "error", err, "path", r.URL.Path)
	http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
}

// checkRateLimit checks if a request should be rate limited
func (s *Service) checkRateLimit(r *http.Request) bool {
	// Get client identifier (IP address)
	clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		clientIP = r.RemoteAddr
	}

	// Get or create rate limiter for this client
	s.rateLimiterMu.RLock()
	limiter, exists := s.rateLimiters[clientIP]
	s.rateLimiterMu.RUnlock()

	if !exists {
		// Create new rate limiter
		limiter = rate.NewLimiter(rate.Limit(s.config.RateLimitRPS), s.config.RateLimitRPS)

		s.rateLimiterMu.Lock()
		s.rateLimiters[clientIP] = limiter
		s.rateLimiterMu.Unlock()
	}

	// Check rate limit
	return limiter.Allow()
}

// cleanupRateLimiters periodically removes old rate limiters
func (s *Service) cleanupRateLimiters(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.rateLimiterMu.Lock()
			// In production, you'd want to track last access time
			// and remove limiters that haven't been used recently
			// For now, we'll just clear if we have too many
			if len(s.rateLimiters) > 10000 {
				s.rateLimiters = make(map[string]*rate.Limiter)
				s.logger.Info("Cleared rate limiter cache")
			}
			s.rateLimiterMu.Unlock()
		}
	}
}

// selectBackend selects a healthy worker backend using round-robin
func (s *Service) selectBackend() *WorkerBackend {
	s.backendsMu.RLock()
	defer s.backendsMu.RUnlock()

	// Get healthy backends
	var healthyBackends []*WorkerBackend
	for _, backend := range s.backends {
		if backend.Healthy {
			healthyBackends = append(healthyBackends, backend)
		}
	}

	if len(healthyBackends) == 0 {
		return nil
	}

	// Round-robin selection
	idx := s.currentIdx.Add(1) - 1
	return healthyBackends[idx%uint32(len(healthyBackends))]
}

// discoverBackendsPeriodically discovers worker node backends
func (s *Service) discoverBackendsPeriodically(ctx context.Context) {
	// Initial discovery
	s.discoverBackends(ctx)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.discoverBackends(ctx)
		}
	}
}

// discoverBackends discovers worker node backends from operator or database
func (s *Service) discoverBackends(ctx context.Context) {
	if s.config.OperatorURL != "" {
		// Query operator for worker nodes
		req, err := http.NewRequestWithContext(ctx, "GET",
			s.config.OperatorURL+"/api/v1/nodes?type=worker&state=ready", nil)
		if err != nil {
			s.logger.Error("Failed to create request", "error", err)
			return
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			s.logger.Error("Failed to query operator", "error", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			s.logger.Error("Operator returned error", "status", resp.StatusCode)
			return
		}

		var nodes []struct {
			ID        string `json:"id"`
			Hostname  string `json:"hostname"`
			IPAddress string `json:"ip_address"`
			Port      int    `json:"port"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
			s.logger.Error("Failed to decode response", "error", err)
			return
		}

		// Update backends
		newBackends := make([]*WorkerBackend, 0, len(nodes))
		for _, node := range nodes {
			port := node.Port
			if port == 0 {
				port = 8081 // Default node agent port
			}

			address := node.IPAddress
			if address == "" {
				address = node.Hostname
			}

			newBackends = append(newBackends, &WorkerBackend{
				ID:      node.ID,
				Address: fmt.Sprintf("%s:%d", address, port),
				Healthy: true, // Will be checked by health checker
			})
		}

		s.backendsMu.Lock()
		s.backends = newBackends
		s.backendsMu.Unlock()

		s.logger.Info("Discovered worker backends", "count", len(newBackends))
	}
}

// checkBackendHealthPeriodically checks health of worker backends
func (s *Service) checkBackendHealthPeriodically(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkBackendHealth()
		}
	}
}

// checkBackendHealth checks health of all backends
func (s *Service) checkBackendHealth() {
	s.backendsMu.RLock()
	backends := make([]*WorkerBackend, len(s.backends))
	copy(backends, s.backends)
	s.backendsMu.RUnlock()

	for _, backend := range backends {
		// Check health via HTTP
		client := s.httpClient
		if s.internalClient != nil {
			client = s.internalClient
		}

		url := fmt.Sprintf("http://%s/health", backend.Address)
		resp, err := client.Get(url)
		if err != nil {
			backend.Healthy = false
			backend.LastCheck = time.Now()
			s.logger.Debug("Backend unhealthy", "id", backend.ID, "error", err)
			continue
		}
		resp.Body.Close()

		backend.Healthy = resp.StatusCode == http.StatusOK
		backend.LastCheck = time.Now()

		if !backend.Healthy {
			s.logger.Warn("Backend unhealthy", "id", backend.ID, "status", resp.StatusCode)
		}
	}
}

// Close closes the edge proxy service
func (s *Service) Close() error {
	return nil
}

// routeByDomain routes traffic based on the domain
func (s *Service) routeByDomain(host string) *url.URL {
	// Extract domain from host (remove port if present)
	domain := host
	if idx := strings.Index(host, ":"); idx != -1 {
		domain = host[:idx]
	}

	// Check cache first
	s.routingMu.RLock()
	target, exists := s.routingCache[domain]
	s.routingMu.RUnlock()

	if exists && time.Since(target.LastUpdated) < 5*time.Minute {
		return target.BackendURL
	}

	// Query database for routing information
	if backend := s.lookupDomainRoute(domain); backend != nil {
		// Update cache
		s.routingMu.Lock()
		s.routingCache[domain] = &RouteTarget{
			BackendURL:  backend,
			LastUpdated: time.Now(),
		}
		s.routingMu.Unlock()
		return backend
	}

	return nil
}

// lookupDomainRoute looks up the backend for a domain in the database
func (s *Service) lookupDomainRoute(domain string) *url.URL {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil
	}

	db, err := database.NewDB(dbURL)
	if err != nil {
		s.logger.Error("Failed to connect to database", "error", err)
		return nil
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Query routing cache table for the domain
	route, err := db.Queries().RoutingCacheFindByDomain(ctx, domain)
	if err != nil || !route.DeploymentID.Valid {
		// Try to find by deployment URL pattern
		// Format: project-nanoid-org.zeitwork.app
		if strings.HasSuffix(domain, ".zeitwork.app") {
			parts := strings.Split(domain, ".")
			if len(parts) >= 2 {
				subdomain := parts[0]
				// Extract nanoid from subdomain (format: project-nanoid-org)
				subParts := strings.Split(subdomain, "-")
				if len(subParts) >= 2 {
					nanoid := subParts[len(subParts)-2] // Second to last part

					// Find deployment by nanoid
					deployment, err := db.Queries().DeploymentFindByNanoID(ctx, pgtype.Text{String: nanoid, Valid: true})
					if err == nil && deployment.Status == "active" {
						// Find running instances for this deployment
						instances, err := db.Queries().InstanceFindByDeployment(ctx, deployment.ID)
						if err == nil && len(instances) > 0 {
							// Pick a random instance (simple load balancing)
							instance := instances[time.Now().Unix()%int64(len(instances))]
							if instance.IpAddress != "" {
								backendURL, err := url.Parse(fmt.Sprintf("http://%s:8080", instance.IpAddress))
								if err == nil {
									// Update routing cache for next time
									go s.updateRoutingCache(domain, instance.IpAddress)
									return backendURL
								}
							}
						}
					}
				}
			}
		}
		return nil
	}

	// Use cached route - parse instances JSON to get an IP
	if route.Instances != nil {
		var instances []string
		if err := json.Unmarshal(route.Instances, &instances); err == nil && len(instances) > 0 {
			// Pick a random instance
			instance := instances[time.Now().Unix()%int64(len(instances))]
			backendURL, err := url.Parse(fmt.Sprintf("http://%s:8080", instance))
			if err == nil {
				return backendURL
			}
		}
	}

	return nil
}

// updateRoutingCache updates the routing cache in the database
func (s *Service) updateRoutingCache(domain, targetIP string) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return
	}

	db, err := database.NewDB(dbURL)
	if err != nil {
		return
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Get deployment for this domain
	var deploymentID pgtype.UUID
	route, err := db.Queries().RoutingCacheFindByDomain(ctx, domain)
	if err == nil {
		deploymentID = route.DeploymentID
	}

	// Update instances list
	instances, _ := json.Marshal([]string{targetIP})

	// Update or insert routing cache entry
	_, err = db.Queries().RoutingCacheUpsert(ctx, &database.RoutingCacheUpsertParams{
		Domain:       domain,
		DeploymentID: deploymentID,
		Instances:    instances,
	})
	if err != nil {
		s.logger.Error("Failed to update routing cache", "error", err)
	}
}
