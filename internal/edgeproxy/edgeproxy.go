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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

// Config holds the edgeproxy configuration
type Config struct {
	HTTPAddr              string        // HTTP listen address (e.g., ":80")
	HTTPSAddr             string        // HTTPS listen address (e.g., ":443")
	DatabaseURL           string        // Database connection string
	RegionID              string        // UUID of the region this edgeproxy is running in
	UpdateInterval        time.Duration // How often to refresh routes from database
	ACMEEmail             string        // Email for Let's Encrypt account
	ACMEStaging           bool          // Use Let's Encrypt staging environment
	ACMECertCheckInterval time.Duration // How often to check for certificates needing renewal
}

// Route represents routing information for a domain
type Route struct {
	PublicIP string // VM's public IP address
	Port     int32  // VM's port
	RegionID string // VM's region UUID
	RegionIP string // Region's public IP (load balancer) for cross-region routing
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
	regionID    pgtype.UUID
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
	if cfg.HTTPSAddr == "" {
		cfg.HTTPSAddr = ":8443"
	}
	if cfg.UpdateInterval == 0 {
		cfg.UpdateInterval = 10 * time.Second
	}
	if cfg.ACMECertCheckInterval == 0 {
		cfg.ACMECertCheckInterval = 1 * time.Hour
	}
	if cfg.ACMEEmail == "" {
		return nil, fmt.Errorf("ACME email is required")
	}

	// Initialize database connection
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize certmagic with PostgreSQL storage
	storage := NewPostgreSQLStorage(db)
	certmagicConfig := certmagic.NewDefault()
	certmagicConfig.Storage = storage

	// Configure ACME issuer with HTTP-01 challenge
	issuer := certmagic.NewACMEIssuer(certmagicConfig, certmagic.ACMEIssuer{
		Email:                   cfg.ACMEEmail,
		Agreed:                  true,
		DisableHTTPChallenge:    false,     // Enable HTTP-01 challenge
		DisableTLSALPNChallenge: true,      // Disable TLS-ALPN-01, use HTTP-01 only
		ListenHost:              "0.0.0.0", // Listen on all interfaces for HTTP-01
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

	s := &Service{
		cfg:       cfg,
		db:        db,
		logger:    logger,
		certmagic: certmagicConfig,
		routes:    make(map[string]Route),
		regionID:  regionUUID,
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

	// Start certificate acquisition loop
	go s.certificateAcquisitionLoop(ctx)

	// Get all verified domains for initial certificate management
	domains, err := s.getVerifiedDomains(ctx)
	if err != nil {
		s.logger.Warn("failed to get verified domains", "error", err)
	} else if len(domains) > 0 {
		s.logger.Info("managing certificates for domains", "count", len(domains))
		// Start async certificate acquisition for all domains
		go func() {
			for _, domain := range domains {
				if err := s.certmagic.ManageAsync(context.Background(), []string{domain}); err != nil {
					s.logger.Error("failed to manage certificate", "domain", domain, "error", err)
				}
			}
		}()
	}

	// Configure HTTPS server with certmagic
	s.httpsServer = &http.Server{
		Addr:         s.cfg.HTTPSAddr,
		Handler:      http.HandlerFunc(s.serveHTTPS),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig:    s.certmagic.TLSConfig(),
	}

	// Start HTTP server in background (for ACME challenges and redirects)
	s.logger.Info("starting edgeproxy HTTP server", "addr", s.cfg.HTTPAddr, "region_id", s.cfg.RegionID)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", "error", err)
		}
	}()

	// Start HTTPS server in background
	s.logger.Info("starting edgeproxy HTTPS server", "addr", s.cfg.HTTPSAddr, "region_id", s.cfg.RegionID)
	go func() {
		if err := s.httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTPS server error", "error", err)
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

// getVerifiedDomains returns all verified domain names
func (s *Service) getVerifiedDomains(ctx context.Context) ([]string, error) {
	domains, err := s.db.Queries().GetDomainsNeedingCertificates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get domains needing certificates: %w", err)
	}

	var domainNames []string
	for _, domain := range domains {
		domainNames = append(domainNames, domain.Name)
	}

	return domainNames, nil
}

// certificateAcquisitionLoop periodically checks for domains needing certificates
func (s *Service) certificateAcquisitionLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.ACMECertCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.acquireCertificatesForDomains(ctx); err != nil {
				s.logger.Error("failed to acquire certificates", "error", err)
			}
		}
	}
}

// acquireCertificatesForDomains proactively acquires certificates for verified domains
func (s *Service) acquireCertificatesForDomains(ctx context.Context) error {
	domains, err := s.db.Queries().GetDomainsNeedingCertificates(ctx)
	if err != nil {
		return fmt.Errorf("failed to get domains needing certificates: %w", err)
	}

	if len(domains) == 0 {
		return nil
	}

	s.logger.Info("acquiring certificates for domains", "count", len(domains))

	for _, domain := range domains {
		// Update status to pending
		err := s.db.Queries().UpdateDomainCertificateStatus(ctx, &database.UpdateDomainCertificateStatusParams{
			ID: domain.ID,
			SslCertificateStatus: database.NullSslCertificateStatuses{
				SslCertificateStatuses: database.SslCertificateStatusesPending,
				Valid:                  true,
			},
			SslCertificateIssuedAt:  pgtype.Timestamptz{Valid: false},
			SslCertificateExpiresAt: pgtype.Timestamptz{Valid: false},
			SslCertificateError:     pgtype.Text{Valid: false},
		})
		if err != nil {
			s.logger.Error("failed to update certificate status", "domain", domain.Name, "error", err)
			continue
		}

		// Acquire certificate asynchronously
		go func(domainName string, domainID pgtype.UUID) {
			s.logger.Info("acquiring certificate", "domain", domainName)

			err := s.certmagic.ManageSync(context.Background(), []string{domainName})
			if err != nil {
				s.logger.Error("failed to obtain certificate", "domain", domainName, "error", err)

				// Update with error status
				_ = s.db.Queries().UpdateDomainCertificateStatus(context.Background(), &database.UpdateDomainCertificateStatusParams{
					ID: domainID,
					SslCertificateStatus: database.NullSslCertificateStatuses{
						SslCertificateStatuses: database.SslCertificateStatusesFailed,
						Valid:                  true,
					},
					SslCertificateIssuedAt:  pgtype.Timestamptz{Valid: false},
					SslCertificateExpiresAt: pgtype.Timestamptz{Valid: false},
					SslCertificateError: pgtype.Text{
						String: err.Error(),
						Valid:  true,
					},
				})
				return
			}

			s.logger.Info("certificate obtained successfully", "domain", domainName)

			// Update with success status
			// Certificate typically valid for 90 days
			issuedAt := time.Now()
			expiresAt := issuedAt.Add(90 * 24 * time.Hour)

			_ = s.db.Queries().UpdateDomainCertificateStatus(context.Background(), &database.UpdateDomainCertificateStatusParams{
				ID: domainID,
				SslCertificateStatus: database.NullSslCertificateStatuses{
					SslCertificateStatuses: database.SslCertificateStatusesActive,
					Valid:                  true,
				},
				SslCertificateIssuedAt: pgtype.Timestamptz{
					Time:  issuedAt,
					Valid: true,
				},
				SslCertificateExpiresAt: pgtype.Timestamptz{
					Time:  expiresAt,
					Valid: true,
				},
				SslCertificateError: pgtype.Text{Valid: false},
			})
		}(domain.Name, domain.ID)
	}

	return nil
}

// serveHTTP handles HTTP requests (ACME challenges and redirects to HTTPS)
func (s *Service) serveHTTP(w http.ResponseWriter, r *http.Request) {
	// ACME HTTP-01 challenges are handled automatically by certmagic
	// when DisableHTTPChallenge is false. Certmagic spins up a temporary
	// HTTP server to handle challenges. Our HTTP server on port 8080 is only
	// for redirecting regular traffic to HTTPS.
	// The challenges are served by certmagic on port 80 directly via its internal solver.

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

	s.logger.Debug("region comparison",
		"host", host,
		"current_region", currentRegionIDStr,
		"vm_region", route.RegionID,
		"is_same_region", isSameRegion,
	)

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

		targetURL = fmt.Sprintf("http://%s:%d", route.PublicIP, route.Port)
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
		req.Header.Set("X-Forwarded-Proto", "https")
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
