package proxy

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

	"github.com/zeitwork/zeitwork/internal/database"
)

// Config holds the proxy configuration
type Config struct {
	ListenAddr     string        // Address to listen on (e.g., ":8080")
	UpdateInterval time.Duration // How often to refresh routes from database
}

// Proxy is a reverse proxy that routes requests based on Host header
type Proxy struct {
	cfg    Config
	db     *database.DB
	logger *slog.Logger
	server *http.Server
	routes map[string]string // domain -> backend URL (http://ip:port)
	mu     sync.RWMutex
	cancel context.CancelFunc
}

// NewProxy creates a new reverse proxy
func NewProxy(cfg Config, db *database.DB, logger *slog.Logger) *Proxy {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.UpdateInterval == 0 {
		cfg.UpdateInterval = 10 * time.Second
	}

	p := &Proxy{
		cfg:    cfg,
		db:     db,
		logger: logger,
		routes: make(map[string]string),
	}

	p.server = &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: p,
	}

	return p
}

// Start starts the proxy server and route updater
func (p *Proxy) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	// Initial route update
	if err := p.updateRoutes(ctx); err != nil {
		p.logger.Warn("initial route update failed", "error", err)
	}

	// Start route updater goroutine
	go p.routeUpdater(ctx)

	// Start HTTP server
	p.logger.Info("starting reverse proxy", "addr", p.cfg.ListenAddr)

	go func() {
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			p.logger.Error("proxy server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the proxy
func (p *Proxy) Stop(ctx context.Context) error {
	p.logger.Info("stopping reverse proxy")

	if p.cancel != nil {
		p.cancel()
	}

	if p.server != nil {
		return p.server.Shutdown(ctx)
	}

	return nil
}

// routeUpdater periodically updates routes from the database
func (p *Proxy) routeUpdater(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.updateRoutes(ctx); err != nil {
				p.logger.Error("failed to update routes", "error", err)
			}
		}
	}
}

// updateRoutes fetches active routes from database and updates the routing table
func (p *Proxy) updateRoutes(ctx context.Context) error {
	routes, err := p.db.Queries().GetActiveRoutes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get active routes: %w", err)
	}

	newRoutes := make(map[string]string)
	for _, route := range routes {
		backendURL := fmt.Sprintf("http://%s:%d", route.IpAddress, route.DefaultPort)
		newRoutes[route.DomainName] = backendURL
	}

	p.mu.Lock()
	p.routes = newRoutes
	p.mu.Unlock()

	p.logger.Info("routes updated", "count", len(newRoutes))
	for domain, backend := range newRoutes {
		p.logger.Debug("route", "domain", domain, "backend", backend)
	}

	return nil
}

// ServeHTTP implements http.Handler
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract host from Host header (strip port if present)
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		// No port in host, use as-is
		host = r.Host
	}

	// Look up backend for this host
	p.mu.RLock()
	backendURL, ok := p.routes[host]
	p.mu.RUnlock()

	if !ok {
		p.logger.Warn("no backend found for host", "host", host, "remote_addr", r.RemoteAddr)
		http.Error(w, "Service Not Found", http.StatusNotFound)
		return
	}

	// Parse backend URL
	target, err := url.Parse(backendURL)
	if err != nil {
		p.logger.Error("invalid backend URL", "url", backendURL, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize director to preserve original Host header if needed
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
		p.logger.Error("proxy error",
			"host", host,
			"backend", backendURL,
			"error", err,
		)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	p.logger.Debug("proxying request",
		"host", host,
		"path", r.URL.Path,
		"backend", backendURL,
		"remote_addr", r.RemoteAddr,
	)

	proxy.ServeHTTP(w, r)
}
