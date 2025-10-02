package edgeproxy

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type Config struct {
	EdgeProxyID          string `env:"EDGEPROXY_ID"`
	EdgeProxyRegionID    string `env:"EDGEPROXY_REGION_ID"`
	EdgeProxyDatabaseURL string `env:"EDGEPROXY_DATABASE_URL"`
	EdgeProxyPort        string `env:"EDGEPROXY_PORT" envDefault:"80"`
}

type Route struct {
	InstanceID  string
	IPAddress   string
	DefaultPort int32
	RegionID    string
	IsLocal     bool // true if instance is in same region as edge proxy
}

type Service struct {
	cfg      Config
	db       *database.DB
	logger   *slog.Logger
	routesMu sync.RWMutex
	routes   map[string][]Route // domain -> list of instances
	regionID string
}

func NewService(cfg Config, logger *slog.Logger) (*Service, error) {
	// Initialize database
	db, err := database.NewDB(cfg.EdgeProxyDatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	svc := &Service{
		cfg:      cfg,
		db:       db,
		logger:   logger,
		routes:   make(map[string][]Route),
		regionID: cfg.EdgeProxyRegionID,
	}

	// Load initial routes
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.updateRoutes(ctx); err != nil {
		logger.Warn("failed to load initial routes", "error", err)
	}

	return svc, nil
}

func (s *Service) Start() error {
	s.logger.Info("edgeproxy starting",
		"edgeproxy_id", s.cfg.EdgeProxyID,
		"region_id", s.cfg.EdgeProxyRegionID,
		"port", s.cfg.EdgeProxyPort,
	)

	// Start route updater in background
	go s.routeUpdater()

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + s.cfg.EdgeProxyPort,
		Handler:      s,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.logger.Info("edgeproxy listening",
		"port", s.cfg.EdgeProxyPort,
	)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

func (s *Service) Close() {
	s.logger.Info("shutting down edgeproxy")

	if s.db != nil {
		s.db.Close()
	}
}

func (s *Service) routeUpdater() {
	for {
		// Sleep 10s +/- 2s random offset
		offset := time.Duration(rand.Intn(5)-2) * time.Second
		sleepDuration := 10*time.Second + offset
		time.Sleep(sleepDuration)

		s.logger.Info("updating routes")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.updateRoutes(ctx); err != nil {
			s.logger.Error("failed to update routes", "error", err)
		}
		cancel()
	}
}

func (s *Service) updateRoutes(ctx context.Context) error {
	// Fetch active routes from database
	routeRows, err := s.db.Queries().GetActiveRoutes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get active routes: %w", err)
	}

	// Build new routes map with region-aware sorting
	newRoutes := make(map[string][]Route)
	localCount := 0
	remoteCount := 0

	for _, row := range routeRows {
		regionIDStr := uuid.ToString(row.RegionID)
		isLocal := regionIDStr == s.regionID
		if isLocal {
			localCount++
		} else {
			remoteCount++
		}

		route := Route{
			InstanceID:  row.IpAddress, // Store IP for logging
			IPAddress:   row.IpAddress,
			DefaultPort: row.DefaultPort,
			RegionID:    regionIDStr,
			IsLocal:     isLocal,
		}

		newRoutes[row.DomainName] = append(newRoutes[row.DomainName], route)
	}

	// Sort routes: local instances first, then remote
	for domain := range newRoutes {
		routes := newRoutes[domain]
		localRoutes := []Route{}
		remoteRoutes := []Route{}

		for _, route := range routes {
			if route.IsLocal {
				localRoutes = append(localRoutes, route)
			} else {
				remoteRoutes = append(remoteRoutes, route)
			}
		}

		// Combine: local first, then remote
		newRoutes[domain] = append(localRoutes, remoteRoutes...)
	}

	// Atomically update routes
	s.routesMu.Lock()
	s.routes = newRoutes
	s.routesMu.Unlock()

	s.logger.Info("routes updated",
		"domain_count", len(newRoutes),
		"total_routes", len(routeRows),
		"local_instances", localCount,
		"remote_instances", remoteCount,
	)

	return nil
}

// ServeHTTP implements http.Handler
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		s.logger.Warn("request without host header", "remote", r.RemoteAddr)
		http.Error(w, "Bad Request: missing Host header", http.StatusBadRequest)
		return
	}

	s.logger.Info("incoming request",
		"host", host,
		"method", r.Method,
		"path", r.URL.Path,
		"remote", r.RemoteAddr,
	)

	// Get routes for this domain
	s.routesMu.RLock()
	routes, exists := s.routes[host]
	s.routesMu.RUnlock()

	if !exists || len(routes) == 0 {
		s.logger.Warn("no route found for domain",
			"host", host,
		)
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// Simple routing: pick first available instance (local preferred due to sorting)
	route := routes[0]

	s.logger.Info("routing request",
		"host", host,
		"target_ip", route.IPAddress,
		"target_port", route.DefaultPort,
		"target_region", route.RegionID,
		"is_local", route.IsLocal,
	)

	// Proxy the request
	s.proxyRequest(w, r, route)
}

func (s *Service) proxyRequest(w http.ResponseWriter, r *http.Request, route Route) {
	// Build target URL
	targetURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", route.IPAddress, route.DefaultPort),
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Customize error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		s.logger.Error("proxy error",
			"target", targetURL.String(),
			"error", err,
		)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	// Update the request
	r.URL.Host = targetURL.Host
	r.URL.Scheme = targetURL.Scheme
	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Forwarded-Proto", "http")

	// Serve the request
	proxy.ServeHTTP(w, r)
}
