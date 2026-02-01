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

	"github.com/caddyserver/certmagic"
	"github.com/zeitwork/zeitwork/internal/database"
)

type Config struct {
	HTTPAddr       string        // HTTP listen address (e.g., ":80")
	HTTPSAddr      string        // HTTPS listen address (e.g., ":443")
	DatabaseURL    string        // Database connection string
	UpdateInterval time.Duration // How often to refresh routes from database
	ACMEEmail      string        // Email for Let's Encrypt account
	ACMEStaging    bool          // Use Let's Encrypt staging environment
}

// Route represents routing information for a domain
type Route struct {
	Port int32  // VM's port
	IP   string // VM's IP address
}

// Service is the edgeproxy service
type Service struct {
	cfg         Config
	db          *database.DB
	logger      *slog.Logger
	httpServer  *http.Server
	httpsServer *http.Server
	certmagic   *certmagic.Config
	routes      map[string]Route // domain -> route info
	mu          sync.RWMutex
	cancel      context.CancelFunc
}

// NewService creates a new edgeproxy service
func NewService(cfg Config, logger *slog.Logger) (*Service, error) {
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if cfg.HTTPSAddr == "" {
		cfg.HTTPSAddr = ":8443"
	}
	if cfg.UpdateInterval == 0 {
		cfg.UpdateInterval = 10 * time.Second
	}
	if cfg.ACMEEmail == "" {
		return nil, fmt.Errorf("ACME email is required")
	}

	// Initialize database connection
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize certmagic with PostgreSQL storage
	storage := NewPostgreSQLStorage(db)
	certmagicConfig := certmagic.NewDefault()
	certmagicConfig.Storage = storage

	// Configure ACME issuer with TLS-ALPN-01 challenge
	issuer := certmagic.NewACMEIssuer(certmagicConfig, certmagic.ACMEIssuer{
		Email:                   cfg.ACMEEmail,
		Agreed:                  true,
		DisableHTTPChallenge:    true,  // Disable HTTP-01
		DisableTLSALPNChallenge: false, // Enable TLS-ALPN-01 (works on port 443)
	})

	// Use staging environment if configured
	if cfg.ACMEStaging {
		issuer.CA = certmagic.LetsEncryptStagingCA
		logger.Info("using Let's Encrypt staging environment")
	} else {
		issuer.CA = certmagic.LetsEncryptProductionCA
		logger.Info("using Let's Encrypt production environment")
	}

	certmagicConfig.Issuers = []certmagic.Issuer{issuer}

	// Enable on-demand TLS for automatic certificate acquisition
	// This is the correct approach for TLS-ALPN-01 challenges
	certmagicConfig.OnDemand = &certmagic.OnDemandConfig{
		// DecisionFunc checks if we should obtain a certificate for this domain
		DecisionFunc: func(ctx context.Context, name string) error {
			domainLogger := logger.With("domain", name)

			// Always allow edge.zeitwork.com - it's the edge proxy's own domain
			if name == "edge.zeitwork.com" {
				domainLogger.Info("allowing edge proxy root domain")
				return nil
			}

			// Check if domain exists and is verified using sqlc
			verifiedAt, err := db.DomainVerified(ctx, name)
			if err != nil {
				domainLogger.Warn("domain not found or not verified", "error", err)
				return fmt.Errorf("domain not authorized: %s", name)
			}

			if !verifiedAt.Valid {
				domainLogger.Warn("domain not verified")
				return fmt.Errorf("domain not verified: %s", name)
			}

			domainLogger.Info("domain authorized for certificate issuance")
			return nil
		},
	}

	s := &Service{
		cfg:       cfg,
		db:        db,
		logger:    logger,
		certmagic: certmagicConfig,
		routes:    make(map[string]Route),
	}

	// HTTP server (for ACME challenges and redirects)
	s.httpServer = &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      http.HandlerFunc(s.serveHTTP),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// HTTPS server will be configured in Start()

	return s, nil
}

func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Initial route load
	if err := s.loadRoutes(ctx); err != nil {
		s.logger.Warn("initial route load failed", "error", err)
	}

	// Start route refresh background goroutine
	go s.refreshRoutesLoop(ctx)

	// Note: Certificate acquisition is on-demand via TLS-ALPN-01
	s.httpsServer = &http.Server{
		Addr:         s.cfg.HTTPSAddr,
		Handler:      http.HandlerFunc(s.serveHTTPS),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig:    s.certmagic.TLSConfig(),
	}

	// Start HTTP server in background (for ACME challenges and redirects)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", "error", err)
		}
	}()

	// Start HTTPS server in background
	go func() {
		if err := s.httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		}
	}()

	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}

	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown HTTP server: %w", err)
		}
	}

	if s.httpsServer != nil {
		if err := s.httpsServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown HTTPS server: %w", err)
		}
	}

	if s.db != nil {
		s.db.Close()
	}

	return nil
}

func (s *Service) loadRoutes(ctx context.Context) error {
	rows, err := s.db.RouteFindActive(ctx)
	if err != nil {
		return fmt.Errorf("failed to get active routes: %w", err)
	}

	newRoutes := make(map[string]Route)
	for _, row := range rows {
		// Skip routes where VM doesn't have a public IP yet
		if !row.VmIp.IsValid() {
			continue
		}

		newRoutes[row.DomainName] = Route{
			IP:   row.VmIp.Addr().String(),
			Port: row.VmPort.Int32,
		}
	}

	s.mu.Lock()
	s.routes = newRoutes
	s.mu.Unlock()

	return nil
}

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

// serveHTTP handles HTTP requests (redirects to HTTPS)
// Note: With on-demand TLS via TLS-ALPN-01, certificates are obtained automatically
// during the first HTTPS request. We don't need proactive acquisition loops.
// Certmagic handles certificate renewal automatically via its maintenance routine.
func (s *Service) serveHTTP(w http.ResponseWriter, r *http.Request) {
	// ACME TLS-ALPN-01 challenges are handled automatically by certmagic
	// on the HTTPS port (8443) via special TLS handshake, so HTTP port
	// is only used for redirecting regular traffic to HTTPS.

	// Redirect all HTTP traffic to HTTPS
	target := "https://" + r.Host + r.URL.RequestURI()
	s.logger.Debug("redirecting to HTTPS", "from", r.URL.String(), "to", target)
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// serveHTTPS handles HTTPS requests (main proxy logic)
func (s *Service) serveHTTPS(w http.ResponseWriter, r *http.Request) {
	// Extract host from Host header (strip port if present)
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		// No port in host, use as-is
		host = r.Host
	}

	// Handle edge.zeitwork.com with a simple message
	if host == "edge.zeitwork.com" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, this is the Zeitwork edge proxy.\n"))
		return
	}

	// Look up route for this host
	s.mu.RLock()
	route, ok := s.routes[host]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "Service Not Found", http.StatusNotFound)
		return
	}

	var targetURL string

	if !s.checkVMHealth(route.IP, route.Port) {
		s.logger.Warn("VM health check failed",
			"host", host,
			"vm", fmt.Sprintf("%s:%d", route.IP, route.Port),
		)
		http.Error(w, "Service Unavailable - VM not responding", http.StatusServiceUnavailable)
		return
	}

	targetURL = fmt.Sprintf("http://%s:%d", route.IP, route.Port)

	target, err := url.Parse(targetURL)
	if err != nil {
		s.logger.Error("invalid target URL", "url", targetURL, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = r.Host
		req.Header.Set("X-Forwarded-For", r.RemoteAddr)
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Real-IP", r.RemoteAddr)
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}
