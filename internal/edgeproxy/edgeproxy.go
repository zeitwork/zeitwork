package edgeproxy

import (
	"context"
	"crypto/tls"
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
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type Config struct {
	HTTPAddr    string    // HTTP listen address (e.g., ":80")
	HTTPSAddr   string    // HTTPS listen address (e.g., ":443")
	ACMEEmail   string    // Email for Let's Encrypt account
	ACMEStaging bool      // Use Let's Encrypt staging environment
	ServerID    uuid.UUID // This server's ID for determining local vs remote VMs

	// Database connection (shared with zeitwerk service, may go through PgBouncer)
	DB *database.DB

	// RouteChangeNotify receives signals from the WAL listener when routes may have changed.
	// The edge proxy debounces these and reloads routes.
	// If nil, only the 60s fallback poll is used.
	RouteChangeNotify <-chan struct{}
}

// Route represents routing information for a domain
type Route struct {
	Port             int32     // VM's port
	IP               string    // VM's IP address
	ServerID         uuid.UUID // Server that hosts this VM
	ServerInternalIP string    // Internal IP of the server hosting this VM
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

	// Enable on-demand TLS for automatic certificate acquisition
	certmagicConfig.OnDemand = &certmagic.OnDemandConfig{
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

	return s, nil
}

func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Initial route load
	if err := s.loadRoutes(ctx); err != nil {
		s.logger.Warn("initial route load failed", "error", err)
	}

	// Start route refresh: channel-driven from WAL listener + 60s fallback poll
	go s.routeRefreshLoop(ctx)

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

	// Don't close the DB -- it's shared with the zeitwerk service and owned by main()

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

		serverInternalIP := ""
		if row.ServerInternalIp.Valid {
			serverInternalIP = row.ServerInternalIp.String
		}

		newRoutes[row.DomainName] = Route{
			IP:               row.VmIp.Addr().String(),
			Port:             row.VmPort.Int32,
			ServerID:         row.ServerID,
			ServerInternalIP: serverInternalIP,
		}
	}

	s.mu.Lock()
	s.routes = newRoutes
	s.mu.Unlock()

	return nil
}

// routeRefreshLoop reloads routes on WAL change notifications, debounced,
// with a 60s fallback poll in case the WAL listener misses something.
func (s *Service) routeRefreshLoop(ctx context.Context) {
	fallbackTicker := time.NewTicker(60 * time.Second)
	defer fallbackTicker.Stop()

	// Debounce timer: after receiving a WAL notification, wait 100ms for more
	// events to arrive before actually reloading. This coalesces rapid-fire changes.
	var debounceTimer *time.Timer
	var debounceCh <-chan time.Time

	notify := s.cfg.RouteChangeNotify

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case <-notify:
			// WAL change notification received -- start/reset debounce timer
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(100 * time.Millisecond)
				debounceCh = debounceTimer.C
			} else {
				debounceTimer.Reset(100 * time.Millisecond)
			}

		case <-debounceCh:
			// Debounce period elapsed -- reload routes
			if err := s.loadRoutes(ctx); err != nil {
				s.logger.Error("failed to refresh routes (WAL-triggered)", "error", err)
			} else {
				s.logger.Debug("routes refreshed (WAL-triggered)")
			}
			debounceTimer = nil
			debounceCh = nil

		case <-fallbackTicker.C:
			// Fallback poll
			if err := s.loadRoutes(ctx); err != nil {
				s.logger.Error("failed to refresh routes (fallback poll)", "error", err)
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
// ACME TLS-ALPN-01 challenges are handled automatically by certmagic
// on the HTTPS port via special TLS handshake, so the HTTP port
// is only used for redirecting regular traffic to HTTPS.
func (s *Service) serveHTTP(w http.ResponseWriter, r *http.Request) {
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

	// Cross-server forwarding: if the VM is on another server, forward to that server's edge proxy
	if route.ServerID != s.cfg.ServerID && route.ServerInternalIP != "" {
		// Prevent forwarding loops
		if r.Header.Get("X-Zeitwork-Forwarded") != "" {
			s.logger.Error("forwarding loop detected", "host", host, "server_id", route.ServerID)
			http.Error(w, "Forwarding Loop Detected", http.StatusLoopDetected)
			return
		}

		// Forward to the other server's edge proxy
		targetURL := fmt.Sprintf("https://%s:443", route.ServerInternalIP)
		target, err := url.Parse(targetURL)
		if err != nil {
			s.logger.Error("invalid forward target URL", "url", targetURL, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // Internal traffic between our own servers
		}
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.Host = r.Host // Preserve the original Host header so the target server can route
			req.Header.Set("X-Zeitwork-Forwarded", "true")
			req.Header.Set("X-Forwarded-For", r.RemoteAddr)
			req.Header.Set("X-Forwarded-Proto", "https")
			req.Header.Set("X-Real-IP", r.RemoteAddr)
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			s.logger.Error("cross-server forward failed", "target", targetURL, "error", err)
			http.Error(w, "Bad Gateway - Remote Server Unavailable", http.StatusBadGateway)
		}
		proxy.ServeHTTP(w, r)
		return
	}

	// Local VM -- proxy directly
	if !s.checkVMHealth(route.IP, route.Port) {
		s.logger.Warn("VM health check failed",
			"host", host,
			"vm", fmt.Sprintf("%s:%d", route.IP, route.Port),
		)
		http.Error(w, "Service Unavailable - VM not responding", http.StatusServiceUnavailable)
		return
	}

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

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}
