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
	"github.com/zeitwork/zeitwork/internal/shared/base58"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type Config struct {
	HTTPAddr    string // HTTP listen address (e.g., ":80")
	HTTPSAddr   string // HTTPS listen address (e.g., ":443")
	ACMEEmail   string // Email for Let's Encrypt account
	ACMEStaging bool   // Use Let's Encrypt staging environment

	// DB is the shared database connection from the main process.
	// The edge proxy does not create its own connection because the WAL listener
	// and zeitwork service coordinate route-change notifications through the same DB.
	DB *database.DB

	// RouteChangeNotify receives signals when routes may have changed
	// (from the WAL listener). The edge proxy debounces these and reloads.
	RouteChangeNotify <-chan struct{}
}

// Route represents routing information for a domain.
// With L2 routing between servers, the edge proxy proxies directly to the VM IP.
// The kernel routing table handles cross-server delivery via VLAN host routes.
type Route struct {
	Port               int32     // VM's port
	IP                 string    // VM's IP address
	ServerID           uuid.UUID // Server hosting the VM
	VmID               uuid.UUID // VM serving this route
	RedirectTo         string    // Optional redirect URL
	RedirectStatusCode int32     // Optional redirect status code
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
	if cfg.ACMEEmail == "" {
		return nil, fmt.Errorf("ACME email is required")
	}
	if cfg.DB == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	db := cfg.DB

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

	// Enable on-demand TLS for automatic certificate acquisition.
	// This is the correct approach for TLS-ALPN-01 challenges.
	certmagicConfig.OnDemand = &certmagic.OnDemandConfig{
		// DecisionFunc checks if we should obtain a certificate for this domain
		DecisionFunc: func(ctx context.Context, name string) error {
			domainLogger := logger.With("domain", name)

			// Always allow edge.zeitwork.com
			if name == "edge.zeitwork.com" {
				domainLogger.Info("allowing edge proxy root domain")
				return nil
			}

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

	s.httpServer = &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      http.HandlerFunc(s.serveHTTP),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return s, nil
}

func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Initial route load
	if err := s.loadRoutes(ctx); err != nil {
		s.logger.Warn("initial route load failed", "error", err)
	}

	// Start WAL-driven route refresh with fallback polling
	go s.refreshRoutesLoop(ctx)

	s.httpsServer = &http.Server{
		Addr:         s.cfg.HTTPSAddr,
		Handler:      http.HandlerFunc(s.serveHTTPS),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig:    s.certmagic.TLSConfig(),
	}

	// Start HTTP server in background
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", "error", err)
		}
	}()

	// Start HTTPS server in background
	go func() {
		if err := s.httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTPS server error", "error", err)
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

	return nil
}

func (s *Service) loadRoutes(ctx context.Context) error {
	rows, err := s.db.RouteFindActive(ctx)
	if err != nil {
		return fmt.Errorf("failed to get active routes: %w", err)
	}

	newRoutes := make(map[string]Route)
	for _, row := range rows {
		// Handle redirects first (doesn't need a VM IP)
		if row.RedirectTo.Valid {
			newRoutes[row.DomainName] = Route{
				RedirectTo:         row.RedirectTo.String,
				RedirectStatusCode: row.RedirectStatusCode.Int32,
			}
			continue
		}

		// Skip routes where VM doesn't have an IP yet
		if !row.VmIp.IsValid() {
			continue
		}

		// With L2 routing, we proxy directly to the VM IP regardless of which
		// server it's on. The kernel routing table (host routes per-server)
		// delivers packets across the VLAN transparently.
		newRoutes[row.DomainName] = Route{
			IP:       row.VmIp.Addr().String(),
			Port:     row.VmPort.Int32,
			ServerID: row.ServerID,
			VmID:     row.VmID,
		}
	}

	s.mu.Lock()
	s.routes = newRoutes
	s.mu.Unlock()

	return nil
}

// refreshRoutesLoop reloads routes when notified via WAL changes,
// with a 100ms debounce window and a 60-second fallback poll.
func (s *Service) refreshRoutesLoop(ctx context.Context) {
	const debounceWindow = 100 * time.Millisecond
	const fallbackInterval = 60 * time.Second

	fallbackTicker := time.NewTicker(fallbackInterval)
	defer fallbackTicker.Stop()

	var debounceTimer *time.Timer

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case <-s.cfg.RouteChangeNotify:
			// WAL notification received — debounce: reset the timer
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounceWindow, func() {
				if err := s.loadRoutes(ctx); err != nil {
					s.logger.Error("failed to refresh routes (WAL-triggered)", "error", err)
				}
			})

		case <-fallbackTicker.C:
			// Safety net: poll even if WAL notifications are missed
			if err := s.loadRoutes(ctx); err != nil {
				s.logger.Error("failed to refresh routes (fallback poll)", "error", err)
			}
		}
	}
}

// serveHTTP handles HTTP requests (redirects to HTTPS)
func (s *Service) serveHTTP(w http.ResponseWriter, r *http.Request) {
	target := "https://" + r.Host + r.URL.RequestURI()
	s.logger.Debug("redirecting to HTTPS", "from", r.URL.String(), "to", target)
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// serveHTTPS handles HTTPS requests (main proxy logic)
func (s *Service) serveHTTPS(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}

	// Handle edge.zeitwork.com
	if host == "edge.zeitwork.com" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, this is the Zeitwork edge proxy.\n"))
		return
	}

	// Look up route
	s.mu.RLock()
	route, ok := s.routes[host]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "Service Not Found", http.StatusNotFound)
		return
	}

	// Handle redirect routes
	if route.RedirectTo != "" {
		statusCode := int(route.RedirectStatusCode)
		if statusCode < 300 || statusCode > 399 {
			statusCode = http.StatusMovedPermanently // Default to 301
		}
		s.logger.Debug("redirecting domain", "host", host, "to", route.RedirectTo, "status", statusCode)
		http.Redirect(w, r, route.RedirectTo, statusCode)
		return
	}

	// Proxy directly to the VM. With L2 routing, the kernel routing table
	// handles delivery to VMs on other servers via VLAN host routes.
	targetURL := fmt.Sprintf("http://%s:%d", route.IP, route.Port)

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

	zeitworkID := base58.Encode(route.ServerID.Bytes[:]) + ":" + base58.Encode(route.VmID.Bytes[:])

	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("Server", "Zeitwork")
		resp.Header.Set("X-Zeitwork-Id", zeitworkID)
		return nil
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		s.logger.Warn("proxy error", "host", host, "target", targetURL, "error", err)
		w.Header().Set("Server", "Zeitwork")
		w.Header().Set("X-Zeitwork-Id", zeitworkID)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}
