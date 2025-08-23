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
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Service represents the edge proxy service
type Service struct {
	logger     *slog.Logger
	config     *Config
	httpClient *http.Client

	// Reverse proxy to load balancer
	proxy           *httputil.ReverseProxy
	loadBalancerURL *url.URL

	// Rate limiting
	rateLimiters  map[string]*rate.Limiter
	rateLimiterMu sync.RWMutex

	// SSL/TLS
	tlsConfig *tls.Config
}

// Config holds the configuration for the edge proxy service
type Config struct {
	Port            string
	LoadBalancerURL string
	SSLCertPath     string
	SSLKeyPath      string
	RateLimitRPS    int
}

// NewService creates a new edge proxy service
func NewService(config *Config, logger *slog.Logger) (*Service, error) {
	// Parse load balancer URL
	lbURL, err := url.Parse(config.LoadBalancerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid load balancer URL: %w", err)
	}

	s := &Service{
		logger:          logger,
		config:          config,
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		loadBalancerURL: lbURL,
		rateLimiters:    make(map[string]*rate.Limiter),
	}

	// Create reverse proxy
	s.proxy = httputil.NewSingleHostReverseProxy(lbURL)
	s.proxy.ErrorHandler = s.errorHandler

	// Configure TLS if certificates are provided
	if config.SSLCertPath != "" && config.SSLKeyPath != "" {
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

	return s, nil
}

// Start starts the edge proxy service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting edge proxy service",
		"port", s.config.Port,
		"load_balancer", s.config.LoadBalancerURL,
		"rate_limit", s.config.RateLimitRPS,
		"ssl", s.tlsConfig != nil,
	)

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
	// Check load balancer health
	lbHealthy := s.checkLoadBalancerHealth()

	status := "healthy"
	if !lbHealthy {
		status = "degraded"
	}

	response := map[string]interface{}{
		"status":                status,
		"load_balancer_healthy": lbHealthy,
		"rate_limit_rps":        s.config.RateLimitRPS,
		"ssl_enabled":           s.tlsConfig != nil,
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

	// Proxy the request
	s.proxy.ServeHTTP(w, r)
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

// checkLoadBalancerHealth checks if the load balancer is healthy
func (s *Service) checkLoadBalancerHealth() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	healthURL := fmt.Sprintf("%s/health", s.loadBalancerURL.String())
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return false
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Warn("Load balancer health check failed", "error", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// Close closes the edge proxy service
func (s *Service) Close() error {
	return nil
}
