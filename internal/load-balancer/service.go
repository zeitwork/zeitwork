package loadbalancer

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/zeitwork/zeitwork/internal/database"
	internaltls "github.com/zeitwork/zeitwork/internal/shared/tls"
)

// Service represents the L4 load balancer service
type Service struct {
	logger     *slog.Logger
	config     *Config
	httpClient *http.Client

	// Backend management
	backends   []*Backend
	backendsMu sync.RWMutex
	currentIdx atomic.Uint32 // For round-robin

	// TCP listener
	listener net.Listener

	// Connection tracking
	activeConnections sync.WaitGroup

	// mTLS for secure internal communication
	internalCA     *internaltls.InternalCA
	tlsConfig      *tls.Config
	internalClient *http.Client
}

// Config holds the configuration for the load balancer service
type Config struct {
	Port        string
	OperatorURL string
	Algorithm   string // round-robin, least-connections, ip-hash
	HealthPort  string // Port for health check HTTP endpoint

	// mTLS configuration for secure internal communication
	EnableMTLS      bool
	InternalCAPath  string
	InternalKeyPath string
	InternalCertDir string
}

// Backend represents a backend server
type Backend struct {
	ID          string
	Address     string // IP:Port for TCP connection
	Type        string // "edge-proxy" or "worker-node"
	Healthy     bool
	Connections int32
	LastCheck   time.Time
}

// NewService creates a new L4 load balancer service
func NewService(config *Config, logger *slog.Logger) (*Service, error) {
	// Set default health port if not specified
	if config.HealthPort == "" {
		config.HealthPort = "8083"
	}

	s := &Service{
		logger:     logger,
		config:     config,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		backends:   make([]*Backend, 0),
	}

	// Initialize internal CA for mTLS with edge proxy
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

			// Get server TLS configuration for accepting mTLS connections
			hostname, _ := os.Hostname()
			ips := []net.IP{}

			// Try to get local IPs
			addrs, err := net.InterfaceAddrs()
			if err == nil {
				for _, addr := range addrs {
					if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
						if ipnet.IP.To4() != nil || ipnet.IP.To16() != nil {
							ips = append(ips, ipnet.IP)
						}
					}
				}
			}

			tlsConfig, err := internalCA.GetServerTLSConfig(hostname, ips)
			if err != nil {
				logger.Warn("Failed to create mTLS server config", "error", err)
			} else {
				s.tlsConfig = tlsConfig

				// Create mTLS client for outgoing connections to worker nodes
				clientTLSConfig, err := internalCA.GetClientTLSConfig("load-balancer")
				if err != nil {
					logger.Warn("Failed to create mTLS client config", "error", err)
				} else {
					s.internalClient = internaltls.NewMTLSClient(clientTLSConfig)
					logger.Info("mTLS enabled for internal communication")
				}
			}
		}
	}

	// If no internal client created, use regular HTTP client
	if s.internalClient == nil {
		s.internalClient = s.httpClient
	}

	return s, nil
}

// Start starts the L4 load balancer service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting L4 load balancer service",
		"port", s.config.Port,
		"algorithm", s.config.Algorithm,
		"operator_url", s.config.OperatorURL,
		"health_port", s.config.HealthPort,
	)

	// Start service discovery
	go s.discoverBackendsPeriodically(ctx)

	// Start health checking
	go s.checkHealthPeriodically(ctx)

	// Start health check HTTP endpoint (for monitoring the LB itself)
	go s.startHealthEndpoint(ctx)

	// Create TCP listener (with TLS if configured)
	var listener net.Listener
	var err error
	if s.tlsConfig != nil {
		listener, err = internaltls.CreateTLSListener(":"+s.config.Port, s.tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to create TLS listener: %w", err)
		}
		s.logger.Info("Created mTLS listener for secure internal communication")
	} else {
		listener, err = net.Listen("tcp", ":"+s.config.Port)
		if err != nil {
			return fmt.Errorf("failed to create TCP listener: %w", err)
		}
	}
	s.listener = listener

	// Accept connections in a goroutine
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					s.logger.Error("Failed to accept connection", "error", err)
					continue
				}
			}

			// Handle each connection in a goroutine
			s.activeConnections.Add(1)
			go s.handleConnection(ctx, conn)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown
	s.logger.Info("Shutting down L4 load balancer service")

	// Close listener
	if s.listener != nil {
		s.listener.Close()
	}

	// Wait for active connections to finish (with timeout)
	done := make(chan struct{})
	go func() {
		s.activeConnections.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("All connections closed gracefully")
	case <-time.After(30 * time.Second):
		s.logger.Warn("Timeout waiting for connections to close")
	}

	return nil
}

// handleConnection handles a single TCP connection
func (s *Service) handleConnection(ctx context.Context, clientConn net.Conn) {
	defer s.activeConnections.Done()
	defer clientConn.Close()

	// Get client address for logging and IP hash
	clientAddr := clientConn.RemoteAddr().String()

	// Select backend based on algorithm
	backend := s.selectBackend(clientAddr)
	if backend == nil {
		s.logger.Warn("No healthy backends available", "client", clientAddr)
		return
	}

	// Update connection count
	atomic.AddInt32(&backend.Connections, 1)
	defer atomic.AddInt32(&backend.Connections, -1)

	// Connect to backend
	backendConn, err := net.DialTimeout("tcp", backend.Address, 5*time.Second)
	if err != nil {
		s.logger.Error("Failed to connect to backend",
			"backend", backend.ID,
			"address", backend.Address,
			"error", err)
		return
	}
	defer backendConn.Close()

	s.logger.Debug("Proxying connection",
		"client", clientAddr,
		"backend", backend.Address)

	// Create a context for this connection
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Bidirectional copy
	errChan := make(chan error, 2)

	// Client to backend
	go func() {
		_, err := io.Copy(backendConn, clientConn)
		errChan <- err
		cancel()
	}()

	// Backend to client
	go func() {
		_, err := io.Copy(clientConn, backendConn)
		errChan <- err
		cancel()
	}()

	// Wait for either direction to finish or context cancellation
	select {
	case <-connCtx.Done():
		// Connection finished or context cancelled
	case err := <-errChan:
		if err != nil && err != io.EOF {
			s.logger.Debug("Connection error", "error", err)
		}
	}
}

// selectBackend selects a backend based on the configured algorithm
func (s *Service) selectBackend(clientAddr string) *Backend {
	s.backendsMu.RLock()
	defer s.backendsMu.RUnlock()

	// Get healthy backends
	healthyBackends := make([]*Backend, 0, len(s.backends))
	for _, backend := range s.backends {
		if backend.Healthy {
			healthyBackends = append(healthyBackends, backend)
		}
	}

	if len(healthyBackends) == 0 {
		return nil
	}

	switch s.config.Algorithm {
	case "least-connections":
		// Select backend with least connections
		selected := healthyBackends[0]
		for _, backend := range healthyBackends[1:] {
			if backend.Connections < selected.Connections {
				selected = backend
			}
		}
		return selected

	case "ip-hash":
		// Hash the client IP to select backend
		hash := 0
		for _, c := range clientAddr {
			hash = hash*31 + int(c)
		}
		if hash < 0 {
			hash = -hash
		}
		return healthyBackends[hash%len(healthyBackends)]

	default: // round-robin
		// Get next index and increment
		idx := s.currentIdx.Add(1) - 1
		return healthyBackends[idx%uint32(len(healthyBackends))]
	}
}

// startHealthEndpoint starts an HTTP endpoint for health checks
func (s *Service) startHealthEndpoint(ctx context.Context) {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		s.backendsMu.RLock()
		healthyCount := 0
		for _, backend := range s.backends {
			if backend.Healthy {
				healthyCount++
			}
		}
		totalCount := len(s.backends)
		s.backendsMu.RUnlock()

		response := map[string]interface{}{
			"status":           "healthy",
			"type":             "L4",
			"algorithm":        s.config.Algorithm,
			"total_backends":   totalCount,
			"healthy_backends": healthyCount,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Backend status endpoint
	mux.HandleFunc("GET /backends", func(w http.ResponseWriter, r *http.Request) {
		s.backendsMu.RLock()
		backends := make([]map[string]interface{}, len(s.backends))
		for i, backend := range s.backends {
			backends[i] = map[string]interface{}{
				"id":          backend.ID,
				"address":     backend.Address,
				"healthy":     backend.Healthy,
				"connections": backend.Connections,
				"last_check":  backend.LastCheck,
			}
		}
		s.backendsMu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(backends)
	})

	server := &http.Server{
		Addr:    ":" + s.config.HealthPort,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Failed to start health endpoint", "error", err)
		}
	}()

	<-ctx.Done()
	server.Shutdown(context.Background())
}

// discoverBackendsPeriodically discovers backends from the operator
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

// discoverBackends discovers backends from the operator or database
func (s *Service) discoverBackends(ctx context.Context) {
	// Try operator API first
	if s.config.OperatorURL != "" {
		if err := s.discoverFromOperator(ctx); err != nil {
			s.logger.Error("Failed to discover from operator, trying database", "error", err)
			// Fall back to database
			s.discoverFromDatabase(ctx)
		}
		return
	}

	// Use database directly if no operator URL
	s.discoverFromDatabase(ctx)
}

// discoverFromOperator queries the operator API for edge proxy backends
func (s *Service) discoverFromOperator(ctx context.Context) error {
	// Query operator for edge proxy instances
	// The L4 load balancer should discover Edge Proxies, not worker nodes
	req, err := http.NewRequestWithContext(ctx, "GET",
		s.config.OperatorURL+"/api/v1/edge-proxies", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to query operator: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("operator returned status %d", resp.StatusCode)
	}

	// Parse edge proxy endpoints
	var edgeProxies []struct {
		ID       string `json:"id"`
		Address  string `json:"address"`
		Port     int    `json:"port"`
		Hostname string `json:"hostname"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&edgeProxies); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Update backends with edge proxy instances
	newBackends := make([]*Backend, 0, len(edgeProxies))
	for _, proxy := range edgeProxies {
		port := proxy.Port
		if port == 0 {
			port = 8083 // Default edge proxy port
		}

		address := proxy.Address
		if address == "" && proxy.Hostname != "" {
			address = proxy.Hostname
		}

		newBackends = append(newBackends, &Backend{
			ID:      proxy.ID,
			Address: fmt.Sprintf("%s:%d", address, port),
			Type:    "edge-proxy",
			Healthy: true, // Will be checked by health checker
		})
	}

	s.backendsMu.Lock()
	s.backends = newBackends
	s.backendsMu.Unlock()

	s.logger.Info("Discovered backends from operator", "count", len(newBackends))
	return nil
}

// discoverFromDatabase queries the database directly for backends
func (s *Service) discoverFromDatabase(ctx context.Context) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		s.logger.Warn("No database URL configured, using default backend")
		// Use a default backend for testing
		s.backendsMu.Lock()
		if len(s.backends) == 0 {
			s.backends = []*Backend{
				{ID: "default", Address: "127.0.0.1:8081", Healthy: true},
			}
		}
		s.backendsMu.Unlock()
		return
	}

	db, err := database.NewDB(dbURL)
	if err != nil {
		s.logger.Error("Failed to connect to database", "error", err)
		return
	}
	defer db.Close()

	// Query for running instances
	instances, err := db.Queries().InstanceFindByState(ctx, "running")
	if err != nil {
		s.logger.Error("Failed to query instances", "error", err)
		return
	}

	// Convert to backends
	newBackends := make([]*Backend, 0, len(instances))
	for _, inst := range instances {
		if inst.IpAddress != "" {
			port := 8080 // Default application port

			// Try to get port from environment variables
			if inst.EnvironmentVariables != "" {
				var env map[string]interface{}
				if err := json.Unmarshal([]byte(inst.EnvironmentVariables), &env); err == nil {
					if p, ok := env["PORT"]; ok {
						switch v := p.(type) {
						case float64:
							port = int(v)
						case string:
							if parsed, err := strconv.Atoi(v); err == nil {
								port = parsed
							}
						}
					}
				}
			}

			newBackends = append(newBackends, &Backend{
				ID:      uuid.UUID(inst.ID.Bytes).String(),
				Address: fmt.Sprintf("%s:%d", inst.IpAddress, port),
				Healthy: false, // Will be checked by health checker
			})
		}
	}

	s.backendsMu.Lock()
	s.backends = newBackends
	s.backendsMu.Unlock()

	s.logger.Info("Discovered backends from database", "count", len(newBackends))
}

// checkHealthPeriodically checks backend health periodically
func (s *Service) checkHealthPeriodically(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkHealth(ctx)
		}
	}
}

// checkHealth checks the health of all backends
func (s *Service) checkHealth(ctx context.Context) {
	s.backendsMu.RLock()
	backends := make([]*Backend, len(s.backends))
	copy(backends, s.backends)
	s.backendsMu.RUnlock()

	var wg sync.WaitGroup
	for _, backend := range backends {
		wg.Add(1)
		go func(b *Backend) {
			defer wg.Done()
			s.checkBackendHealth(ctx, b)
		}(backend)
	}
	wg.Wait()
}

// checkBackendHealth checks the health of a single backend using TCP
func (s *Service) checkBackendHealth(ctx context.Context, backend *Backend) {
	// Try to establish a TCP connection
	dialer := net.Dialer{
		Timeout: 5 * time.Second,
	}

	conn, err := dialer.DialContext(ctx, "tcp", backend.Address)
	if err != nil {
		if backend.Healthy {
			s.logger.Warn("Backend became unhealthy", "id", backend.ID, "address", backend.Address, "error", err)
		}
		backend.Healthy = false
		backend.LastCheck = time.Now()
		return
	}
	conn.Close()

	if !backend.Healthy {
		s.logger.Info("Backend became healthy", "id", backend.ID, "address", backend.Address)
	}
	backend.Healthy = true
	backend.LastCheck = time.Now()
}

// Close closes the load balancer service
func (s *Service) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
