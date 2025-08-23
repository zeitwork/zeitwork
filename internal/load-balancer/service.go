package loadbalancer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// Service represents the load balancer service
type Service struct {
	logger     *slog.Logger
	config     *Config
	httpClient *http.Client

	// Backend management
	backends   []*Backend
	backendsMu sync.RWMutex
	currentIdx atomic.Uint32 // For round-robin

	// Reverse proxy
	proxy *httputil.ReverseProxy
}

// Config holds the configuration for the load balancer service
type Config struct {
	Port        string
	OperatorURL string
	Algorithm   string // round-robin, least-connections, ip-hash
}

// Backend represents a backend server
type Backend struct {
	ID          string
	URL         *url.URL
	Healthy     bool
	Connections int32
	LastCheck   time.Time
}

// NewService creates a new load balancer service
func NewService(config *Config, logger *slog.Logger) (*Service, error) {
	s := &Service{
		logger:     logger,
		config:     config,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		backends:   make([]*Backend, 0),
	}

	// Create reverse proxy with custom director
	s.proxy = &httputil.ReverseProxy{
		Director:     s.director,
		ErrorHandler: s.errorHandler,
	}

	return s, nil
}

// Start starts the load balancer service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting load balancer service",
		"port", s.config.Port,
		"algorithm", s.config.Algorithm,
		"operator_url", s.config.OperatorURL,
	)

	// Start service discovery
	go s.discoverBackendsPeriodically(ctx)

	// Start health checking
	go s.checkHealthPeriodically(ctx)

	// Create HTTP server
	mux := http.NewServeMux()
	s.setupRoutes(mux)

	server := &http.Server{
		Addr:    ":" + s.config.Port,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Failed to start HTTP server", "error", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown server
	s.logger.Info("Shutting down load balancer service")
	return server.Shutdown(context.Background())
}

// setupRoutes sets up the HTTP routes for the load balancer
func (s *Service) setupRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("GET /health", s.handleHealth)

	// Backend management
	mux.HandleFunc("GET /backends", s.handleGetBackends)

	// Proxy all other requests
	mux.HandleFunc("/", s.handleProxy)
}

// handleHealth handles health check requests
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
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
		"algorithm":        s.config.Algorithm,
		"total_backends":   totalCount,
		"healthy_backends": healthyCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetBackends returns the list of backends
func (s *Service) handleGetBackends(w http.ResponseWriter, r *http.Request) {
	s.backendsMu.RLock()
	backends := make([]map[string]interface{}, len(s.backends))
	for i, backend := range s.backends {
		backends[i] = map[string]interface{}{
			"id":          backend.ID,
			"url":         backend.URL.String(),
			"healthy":     backend.Healthy,
			"connections": backend.Connections,
			"last_check":  backend.LastCheck,
		}
	}
	s.backendsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(backends)
}

// handleProxy proxies requests to backend servers
func (s *Service) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Select backend based on algorithm
	backend := s.selectBackend(r)
	if backend == nil {
		http.Error(w, "No healthy backends available", http.StatusServiceUnavailable)
		return
	}

	// Update connection count
	atomic.AddInt32(&backend.Connections, 1)
	defer atomic.AddInt32(&backend.Connections, -1)

	// Set the backend URL in the request context
	ctx := context.WithValue(r.Context(), "backend", backend)
	r = r.WithContext(ctx)

	// Proxy the request
	s.proxy.ServeHTTP(w, r)
}

// selectBackend selects a backend based on the configured algorithm
func (s *Service) selectBackend(r *http.Request) *Backend {
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
		clientIP := r.RemoteAddr
		hash := 0
		for _, c := range clientIP {
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

// director modifies the request to point to the selected backend
func (s *Service) director(req *http.Request) {
	backend, ok := req.Context().Value("backend").(*Backend)
	if !ok {
		s.logger.Error("No backend in request context")
		return
	}

	// Update the request to point to the backend
	req.URL.Scheme = backend.URL.Scheme
	req.URL.Host = backend.URL.Host
	req.Host = backend.URL.Host

	// Add X-Forwarded headers
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		req.Header.Set("X-Forwarded-For", clientIP)
	}
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("X-Forwarded-Host", req.Host)
}

// errorHandler handles errors from the reverse proxy
func (s *Service) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.Error("Proxy error", "error", err, "path", r.URL.Path)
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
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

// discoverBackends discovers backends from the operator
func (s *Service) discoverBackends(ctx context.Context) {
	// Query operator for running instances
	req, err := http.NewRequestWithContext(ctx, "GET",
		s.config.OperatorURL+"/api/v1/instances?state=running", nil)
	if err != nil {
		s.logger.Error("Failed to create discovery request", "error", err)
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error("Failed to discover backends", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.logger.Error("Discovery failed", "status", resp.StatusCode)
		return
	}

	// Parse instances
	var instances []struct {
		ID          string `json:"id"`
		IPAddress   string `json:"ip_address"`
		DefaultPort int    `json:"default_port"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&instances); err != nil {
		s.logger.Error("Failed to parse instances", "error", err)
		return
	}

	// Update backends
	newBackends := make([]*Backend, 0, len(instances))
	for _, inst := range instances {
		port := inst.DefaultPort
		if port == 0 {
			port = 8080
		}

		backendURL, err := url.Parse(fmt.Sprintf("http://%s:%d", inst.IPAddress, port))
		if err != nil {
			s.logger.Error("Failed to parse backend URL", "error", err, "instance", inst.ID)
			continue
		}

		newBackends = append(newBackends, &Backend{
			ID:      inst.ID,
			URL:     backendURL,
			Healthy: true, // Will be checked by health checker
		})
	}

	s.backendsMu.Lock()
	s.backends = newBackends
	s.backendsMu.Unlock()

	s.logger.Info("Discovered backends", "count", len(newBackends))
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

	for _, backend := range backends {
		go s.checkBackendHealth(ctx, backend)
	}
}

// checkBackendHealth checks the health of a single backend
func (s *Service) checkBackendHealth(ctx context.Context, backend *Backend) {
	healthURL := fmt.Sprintf("%s/health", backend.URL.String())

	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		backend.Healthy = false
		backend.LastCheck = time.Now()
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		backend.Healthy = false
		backend.LastCheck = time.Now()
		s.logger.Warn("Backend unhealthy", "id", backend.ID, "error", err)
		return
	}
	defer resp.Body.Close()

	backend.Healthy = resp.StatusCode == http.StatusOK
	backend.LastCheck = time.Now()

	if !backend.Healthy {
		s.logger.Warn("Backend unhealthy", "id", backend.ID, "status", resp.StatusCode)
	}
}

// Close closes the load balancer service
func (s *Service) Close() error {
	return nil
}
