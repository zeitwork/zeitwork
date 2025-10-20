package edgeproxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

// Config holds the edgeproxy configuration
type Config struct {
	HTTPAddr       string        // HTTP listen address (e.g., ":8080")
	DatabaseURL    string        // Database connection string
	RegionID       string        // UUID of the region this edgeproxy is running in
	UpdateInterval time.Duration // How often to refresh routes from database
}

// Route represents routing information for a domain
type Route struct {
	PublicIP string // VM's public IPv6 address
	Port     int32  // VM's port
	RegionID string // VM's region UUID
	RegionIP string // Region's public IP (load balancer) for cross-region routing
}

// Service is the edgeproxy service
type Service struct {
	cfg      Config
	db       *database.DB
	logger   *slog.Logger
	server   *http.Server
	routes   map[string]Route // domain -> route info
	mu       sync.RWMutex
	cancel   context.CancelFunc
	regionID pgtype.UUID
}

// NewService creates a new edgeproxy service
func NewService(cfg Config, logger *slog.Logger) (*Service, error) {
	// Parse region UUID
	regionUUID, err := uuid.Parse(cfg.RegionID)
	if err != nil {
		return nil, fmt.Errorf("invalid region id: %w", err)
	}

	// Set defaults
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if cfg.UpdateInterval == 0 {
		cfg.UpdateInterval = 10 * time.Second
	}

	// Initialize database connection
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	s := &Service{
		cfg:      cfg,
		db:       db,
		logger:   logger,
		routes:   make(map[string]Route),
		regionID: regionUUID,
	}

	s.server = &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      s,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return s, nil
}

// Start starts the edgeproxy service
func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Initial route load
	if err := s.loadRoutes(ctx); err != nil {
		s.logger.Warn("initial route load failed", "error", err)
	}

	// Start route refresh background goroutine
	go s.refreshRoutesLoop(ctx)

	// Start HTTP server in background
	s.logger.Info("starting edgeproxy HTTP server", "addr", s.cfg.HTTPAddr, "region_id", s.cfg.RegionID)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the edgeproxy service
func (s *Service) Stop(ctx context.Context) error {
	s.logger.Info("stopping edgeproxy")

	if s.cancel != nil {
		s.cancel()
	}

	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown HTTP server: %w", err)
		}
	}

	if s.db != nil {
		s.db.Close()
	}

	return nil
}

// loadRoutes fetches active routes from database and updates the routing table
func (s *Service) loadRoutes(ctx context.Context) error {
	rows, err := s.db.Queries().GetActiveRoutes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get active routes: %w", err)
	}

	newRoutes := make(map[string]Route)
	for _, row := range rows {
		// Skip routes where VM doesn't have a public IP yet
		if !row.VmPublicIp.Valid || row.VmPublicIp.String == "" {
			s.logger.Warn("skipping route with invalid public IP",
				"domain", row.DomainName,
			)
			continue
		}

		newRoutes[row.DomainName] = Route{
			PublicIP: row.VmPublicIp.String,
			Port:     row.VmPort,
			RegionID: uuid.ToString(row.VmRegionID),
			RegionIP: row.RegionLoadBalancerIp,
		}
	}

	s.mu.Lock()
	s.routes = newRoutes
	s.mu.Unlock()

	s.logger.Info("routes loaded", "count", len(newRoutes))
	for domain, route := range newRoutes {
		s.logger.Debug("route",
			"domain", domain,
			"vm", fmt.Sprintf("%s:%d", route.PublicIP, route.Port),
			"region", route.RegionID,
		)
	}

	return nil
}

// refreshRoutesLoop periodically refreshes routes from the database
func (s *Service) refreshRoutesLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.loadRoutes(ctx); err != nil {
				s.logger.Error("failed to refresh routes", "error", err)
			}
		}
	}
}

// checkVMHealth performs a health check on a VM
func (s *Service) checkVMHealth(ip string, port int32) bool {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	healthURL := fmt.Sprintf("http://%s:%d/", ip, port)
	resp, err := client.Get(healthURL)
	if err != nil {
		s.logger.Debug("health check failed", "url", healthURL, "error", err)
		return false
	}
	defer resp.Body.Close()

	// Consider 2xx and 3xx status codes as healthy
	healthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	s.logger.Debug("health check", "url", healthURL, "status", resp.StatusCode, "healthy", healthy)
	return healthy
}

// ServeHTTP implements http.Handler
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract host from Host header (strip port if present)
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		// No port in host, use as-is
		host = r.Host
	}

	// Look up route for this host
	s.mu.RLock()
	route, ok := s.routes[host]
	s.mu.RUnlock()

	if !ok {
		s.logger.Warn("no route found for host", "host", host, "remote_addr", r.RemoteAddr)
		http.Error(w, "Service Not Found", http.StatusNotFound)
		return
	}

	// Check if this is same-region or cross-region routing
	currentRegionIDStr := uuid.ToString(s.regionID)
	isSameRegion := route.RegionID == currentRegionIDStr

	var targetURL string
	if isSameRegion {
		// Same region: route directly to VM
		// First, health check the VM
		if !s.checkVMHealth(route.PublicIP, route.Port) {
			s.logger.Warn("VM health check failed",
				"host", host,
				"vm", fmt.Sprintf("%s:%d", route.PublicIP, route.Port),
			)
			http.Error(w, "Service Unavailable - VM not responding", http.StatusServiceUnavailable)
			return
		}

		targetURL = fmt.Sprintf("http://[%s]:%d", route.PublicIP, route.Port)
	} else {
		// Cross-region: route to other region's load balancer
		targetURL = fmt.Sprintf("http://%s:80", route.RegionIP)
	}

	// Parse target URL
	target, err := url.Parse(targetURL)
	if err != nil {
		s.logger.Error("invalid target URL", "url", targetURL, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize director to preserve original Host header
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = r.Host
		req.Header.Set("X-Forwarded-For", r.RemoteAddr)
		req.Header.Set("X-Forwarded-Proto", "http")
		req.Header.Set("X-Real-IP", r.RemoteAddr)
	}

	// Custom error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		s.logger.Error("proxy error",
			"host", host,
			"target", targetURL,
			"error", err,
		)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	s.logger.Debug("proxying request",
		"host", host,
		"path", r.URL.Path,
		"target", targetURL,
		"same_region", isSameRegion,
		"remote_addr", r.RemoteAddr,
	)

	proxy.ServeHTTP(w, r)
}
