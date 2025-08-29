package edgeproxy

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// Service represents the edge proxy service
type Service struct {
	logger     *slog.Logger
	config     *Config
	httpClient *http.Client

	// Simple backend list for round-robin
	backends   []string
	backendsMu sync.RWMutex
	currentIdx atomic.Uint32
}

// Config holds the configuration for the edge proxy service
type Config struct {
	Port        string
	OperatorURL string
}

// NewService creates a new edge proxy service
func NewService(config *Config, logger *slog.Logger) (*Service, error) {
	s := &Service{
		logger:     logger,
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		backends:   make([]string, 0),
	}

	return s, nil
}

// Start starts the edge proxy service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting edge proxy service", "port", s.config.Port)

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

	// Proxy all other requests
	mux.HandleFunc("/", s.handleProxy)
}

// handleHealth handles health check requests
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status": "healthy",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleProxy handles proxy requests - simplified version
func (s *Service) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Simple round-robin to backends (if any configured)
	s.backendsMu.RLock()
	if len(s.backends) == 0 {
		s.backendsMu.RUnlock()
		http.Error(w, "No backends available", http.StatusServiceUnavailable)
		return
	}

	// Simple round-robin selection
	idx := s.currentIdx.Add(1) - 1
	backend := s.backends[idx%uint32(len(s.backends))]
	s.backendsMu.RUnlock()

	// Create a simple reverse proxy
	targetURL, err := url.Parse("http://" + backend)
	if err != nil {
		http.Error(w, "Invalid backend", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ServeHTTP(w, r)
}

// Close closes the edge proxy service
func (s *Service) Close() error {
	return nil
}

// AddBackend adds a backend to the load balancer
func (s *Service) AddBackend(backend string) {
	s.backendsMu.Lock()
	defer s.backendsMu.Unlock()
	s.backends = append(s.backends, backend)
}

// RemoveBackend removes a backend from the load balancer
func (s *Service) RemoveBackend(backend string) {
	s.backendsMu.Lock()
	defer s.backendsMu.Unlock()
	for i, b := range s.backends {
		if b == backend {
			s.backends = append(s.backends[:i], s.backends[i+1:]...)
			break
		}
	}
}
