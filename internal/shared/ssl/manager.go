package ssl

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// Manager manages SSL certificates for the platform
type Manager struct {
	logger *slog.Logger
	db     *database.DB
	mu     sync.RWMutex

	// ACME configuration
	acmeClient *acme.Client
	acmeCache  autocert.Cache

	// Domains we manage certificates for
	domains []string

	// Certificate cache
	certCache map[string]*tls.Certificate

	// Configuration
	config *Config
}

// Config holds the SSL manager configuration
type Config struct {
	DatabaseURL   string
	ACMEDirectory string // Production: https://acme-v02.api.letsencrypt.org/directory
	Email         string
	StagingMode   bool   // Use Let's Encrypt staging for testing
	DNSProvider   string // For DNS-01 challenges (required for wildcards)
}

// NewManager creates a new SSL certificate manager
func NewManager(config *Config, logger *slog.Logger) (*Manager, error) {
	db, err := database.NewDB(config.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Set ACME directory based on staging mode
	acmeDir := config.ACMEDirectory
	if acmeDir == "" {
		if config.StagingMode {
			acmeDir = "https://acme-staging-v02.api.letsencrypt.org/directory"
		} else {
			acmeDir = acme.LetsEncryptURL
		}
	}

	return &Manager{
		logger:    logger,
		db:        db,
		config:    config,
		certCache: make(map[string]*tls.Certificate),
		domains: []string{
			"*.zeitwork.com",
			"*.zeitwork.app",
			"*.zeitwork-dns.com",
			"zeitwork.com",
			"zeitwork.app",
			"zeitwork-dns.com",
		},
	}, nil
}

// Start starts the SSL manager
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Starting SSL manager", "domains", m.domains)

	// Initialize ACME client
	if err := m.initACMEClient(); err != nil {
		return fmt.Errorf("failed to initialize ACME client: %w", err)
	}

	// Load existing certificates from database
	if err := m.loadCertificates(ctx); err != nil {
		m.logger.Error("Failed to load certificates", "error", err)
	}

	// Check and renew certificates
	go m.renewalLoop(ctx)

	// Initial certificate generation for missing domains
	for _, domain := range m.domains {
		if _, exists := m.certCache[domain]; !exists {
			go m.obtainCertificate(ctx, domain)
		}
	}

	return nil
}

// initACMEClient initializes the ACME client
func (m *Manager) initACMEClient() error {
	// Create account key
	accountKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate account key: %w", err)
	}

	// Create ACME client
	m.acmeClient = &acme.Client{
		Key:          accountKey,
		DirectoryURL: m.config.ACMEDirectory,
	}

	// Register account
	ctx := context.Background()
	account := &acme.Account{
		Contact: []string{"mailto:" + m.config.Email},
	}

	_, err = m.acmeClient.Register(ctx, account, acme.AcceptTOS)
	if err != nil && err != acme.ErrAccountAlreadyExists {
		return fmt.Errorf("failed to register ACME account: %w", err)
	}

	return nil
}

// obtainCertificate obtains a new certificate for a domain
func (m *Manager) obtainCertificate(ctx context.Context, domain string) error {
	m.logger.Info("Obtaining certificate", "domain", domain)

	// For wildcard certificates, we need DNS-01 challenge
	isWildcard := domain[0] == '*'

	if isWildcard {
		return m.obtainWildcardCertificate(ctx, domain)
	}

	return m.obtainStandardCertificate(ctx, domain)
}

// obtainStandardCertificate obtains a standard certificate using HTTP-01 challenge
func (m *Manager) obtainStandardCertificate(ctx context.Context, domain string) error {
	// Generate certificate key
	certKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate certificate key: %w", err)
	}

	// Create certificate request
	template := &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: domain},
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, certKey)
	if err != nil {
		return fmt.Errorf("failed to create CSR: %w", err)
	}

	// Create order
	order, err := m.acmeClient.AuthorizeOrder(ctx, acme.DomainIDs(domain))
	if err != nil {
		return fmt.Errorf("failed to authorize order: %w", err)
	}

	// Complete challenges
	for _, authzURL := range order.AuthzURLs {
		authz, err := m.acmeClient.GetAuthorization(ctx, authzURL)
		if err != nil {
			return fmt.Errorf("failed to get authorization: %w", err)
		}

		// Find HTTP-01 challenge
		var httpChallenge *acme.Challenge
		for _, c := range authz.Challenges {
			if c.Type == "http-01" {
				httpChallenge = c
				break
			}
		}

		if httpChallenge == nil {
			return fmt.Errorf("no HTTP-01 challenge found")
		}

		// Serve challenge response (simplified - in production, this needs proper HTTP server)
		token := httpChallenge.Token
		keyAuth, err := m.acmeClient.HTTP01ChallengeResponse(token)
		if err != nil {
			return fmt.Errorf("failed to get challenge response: %w", err)
		}

		// TODO: Serve keyAuth at /.well-known/acme-challenge/{token}
		m.logger.Info("Challenge token", "token", token, "keyAuth", keyAuth)

		// Accept challenge
		if _, err := m.acmeClient.Accept(ctx, httpChallenge); err != nil {
			return fmt.Errorf("failed to accept challenge: %w", err)
		}

		// Wait for validation
		if _, err := m.acmeClient.WaitAuthorization(ctx, authz.URI); err != nil {
			return fmt.Errorf("authorization failed: %w", err)
		}
	}

	// Finalize order
	certDER, _, err := m.acmeClient.CreateOrderCert(ctx, order.FinalizeURL, csrDER, true)
	if err != nil {
		return fmt.Errorf("failed to finalize order: %w", err)
	}

	// Store certificate
	return m.storeCertificate(ctx, domain, certDER, certKey)
}

// obtainWildcardCertificate obtains a wildcard certificate using DNS-01 challenge
func (m *Manager) obtainWildcardCertificate(ctx context.Context, domain string) error {
	m.logger.Info("Obtaining wildcard certificate requires DNS-01 challenge", "domain", domain)

	// For production, this would integrate with Route53, Cloudflare, etc.
	// For now, generate a self-signed certificate for development
	return m.generateSelfSignedCertificate(ctx, domain)
}

// generateSelfSignedCertificate generates a self-signed certificate for development
func (m *Manager) generateSelfSignedCertificate(ctx context.Context, domain string) error {
	// Generate key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Zeitwork"},
			CommonName:   domain,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add DNS names
	if domain[0] == '*' {
		template.DNSNames = []string{domain, domain[2:]} // *.example.com and example.com
	} else {
		template.DNSNames = []string{domain}
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	// PEM encode
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	// Store in database
	_, err = m.db.Queries().TlsCertificateCreate(ctx, &database.TlsCertificateCreateParams{
		Domain:      domain,
		Certificate: string(certPEM),
		PrivateKey:  string(keyPEM),
		ExpiresAt:   pgtype.Timestamptz{Time: template.NotAfter, Valid: true},
		Issuer:      "self-signed",
		AutoRenew:   true,
	})

	if err != nil {
		return fmt.Errorf("failed to store certificate: %w", err)
	}

	// Update cache
	cert, _ := tls.X509KeyPair(certPEM, keyPEM)
	m.mu.Lock()
	m.certCache[domain] = &cert
	m.mu.Unlock()

	m.logger.Info("Generated self-signed certificate", "domain", domain)
	return nil
}

// storeCertificate stores a certificate in the database
func (m *Manager) storeCertificate(ctx context.Context, domain string, certDER [][]byte, key *rsa.PrivateKey) error {
	// Convert to PEM
	var certPEM []byte
	for _, der := range certDER {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	// Parse certificate to get expiry
	cert, err := x509.ParseCertificate(certDER[0])
	if err != nil {
		return err
	}

	// Store in database
	_, err = m.db.Queries().TlsCertificateCreate(ctx, &database.TlsCertificateCreateParams{
		Domain:      domain,
		Certificate: string(certPEM),
		PrivateKey:  string(keyPEM),
		ExpiresAt:   pgtype.Timestamptz{Time: cert.NotAfter, Valid: true},
		Issuer:      "letsencrypt",
		AutoRenew:   true,
	})

	if err != nil {
		return fmt.Errorf("failed to store certificate: %w", err)
	}

	// Update cache
	tlsCert, _ := tls.X509KeyPair(certPEM, keyPEM)
	m.mu.Lock()
	m.certCache[domain] = &tlsCert
	m.mu.Unlock()

	return nil
}

// loadCertificates loads certificates from the database
func (m *Manager) loadCertificates(ctx context.Context) error {
	certs, err := m.db.Queries().TlsCertificateList(ctx)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cert := range certs {
		tlsCert, err := tls.X509KeyPair([]byte(cert.Certificate), []byte(cert.PrivateKey))
		if err != nil {
			m.logger.Error("Failed to parse certificate", "domain", cert.Domain, "error", err)
			continue
		}
		m.certCache[cert.Domain] = &tlsCert
		m.logger.Info("Loaded certificate", "domain", cert.Domain, "expires", cert.ExpiresAt)
	}

	return nil
}

// renewalLoop checks for certificate renewals
func (m *Manager) renewalLoop(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkRenewals(ctx)
		}
	}
}

// checkRenewals checks if any certificates need renewal
func (m *Manager) checkRenewals(ctx context.Context) {
	certs, err := m.db.Queries().TlsCertificateList(ctx)
	if err != nil {
		m.logger.Error("Failed to list certificates", "error", err)
		return
	}

	for _, cert := range certs {
		// Renew if expiring within 30 days
		if cert.ExpiresAt.Time.Before(time.Now().Add(30 * 24 * time.Hour)) {
			m.logger.Info("Certificate expiring soon, renewing", "domain", cert.Domain, "expires", cert.ExpiresAt)
			go m.obtainCertificate(ctx, cert.Domain)
		}
	}
}

// GetCertificate returns a certificate for the given domain
func (m *Manager) GetCertificate(domain string) (*tls.Certificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try exact match
	if cert, ok := m.certCache[domain]; ok {
		return cert, nil
	}

	// Try wildcard match
	for pattern, cert := range m.certCache {
		if pattern[0] == '*' && matchesWildcard(domain, pattern) {
			return cert, nil
		}
	}

	return nil, fmt.Errorf("no certificate found for domain: %s", domain)
}

// matchesWildcard checks if a domain matches a wildcard pattern
func matchesWildcard(domain, pattern string) bool {
	// Simple wildcard matching for *.example.com
	if pattern[0] != '*' {
		return false
	}
	suffix := pattern[1:] // Remove *
	return len(domain) >= len(suffix) && domain[len(domain)-len(suffix):] == suffix
}

// GetTLSConfig returns a TLS configuration that uses the certificate manager
func (m *Manager) GetTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return m.GetCertificate(hello.ServerName)
		},
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
	}
}
