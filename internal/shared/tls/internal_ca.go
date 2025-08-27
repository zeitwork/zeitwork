package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// InternalCA manages internal TLS certificates for service-to-service communication
type InternalCA struct {
	logger *slog.Logger
	mu     sync.RWMutex

	// CA certificate and key
	caCert    *x509.Certificate
	caKey     *rsa.PrivateKey
	caCertPEM []byte
	caKeyPEM  []byte

	// Certificate storage
	certDir string

	// Certificate cache
	certCache map[string]*tls.Certificate

	// Certificate rotation
	rotationPeriod time.Duration
	validityPeriod time.Duration
}

// InternalCAConfig holds configuration for internal CA
type InternalCAConfig struct {
	CertDir        string
	CAKeyPath      string
	CACertPath     string
	RotationPeriod time.Duration
	ValidityPeriod time.Duration
	Organization   string
	Country        string
}

// NewInternalCA creates or loads an internal CA
func NewInternalCA(config *InternalCAConfig, logger *slog.Logger) (*InternalCA, error) {
	ca := &InternalCA{
		logger:         logger,
		certDir:        config.CertDir,
		certCache:      make(map[string]*tls.Certificate),
		rotationPeriod: config.RotationPeriod,
		validityPeriod: config.ValidityPeriod,
	}

	// Ensure certificate directory exists
	if err := os.MkdirAll(config.CertDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cert directory: %w", err)
	}

	// Try to load existing CA
	if config.CAKeyPath != "" && config.CACertPath != "" {
		if err := ca.loadCA(config.CAKeyPath, config.CACertPath); err == nil {
			logger.Info("Loaded existing CA certificate")
			return ca, nil
		}
	}

	// Generate new CA if not loaded
	logger.Info("Generating new CA certificate")
	if err := ca.generateCA(config.Organization, config.Country); err != nil {
		return nil, fmt.Errorf("failed to generate CA: %w", err)
	}

	// Save CA certificate and key
	if config.CAKeyPath != "" && config.CACertPath != "" {
		if err := ca.saveCA(config.CAKeyPath, config.CACertPath); err != nil {
			logger.Warn("Failed to save CA certificate", "error", err)
		}
	}

	return ca, nil
}

// generateCA generates a new CA certificate and key
func (ca *InternalCA) generateCA(organization, country string) error {
	// Generate CA private key
	caKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("failed to generate CA key: %w", err)
	}

	// Create CA certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{organization},
			Country:       []string{country},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    "Zeitwork Internal CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Create CA certificate
	caCertDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &caKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("failed to create CA certificate: %w", err)
	}

	// Parse the certificate
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Encode to PEM
	caCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCertDER,
	})

	caKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caKey),
	})

	ca.mu.Lock()
	ca.caCert = caCert
	ca.caKey = caKey
	ca.caCertPEM = caCertPEM
	ca.caKeyPEM = caKeyPEM
	ca.mu.Unlock()

	return nil
}

// loadCA loads existing CA certificate and key
func (ca *InternalCA) loadCA(keyPath, certPath string) error {
	// Load CA certificate
	certPEM, err := ioutil.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("failed to read CA certificate: %w", err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return fmt.Errorf("failed to decode CA certificate PEM")
	}

	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Load CA private key
	keyPEM, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read CA key: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return fmt.Errorf("failed to decode CA key PEM")
	}

	caKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		// Try PKCS8 format
		keyInterface, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse CA key: %w", err)
		}
		var ok bool
		caKey, ok = keyInterface.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("CA key is not RSA")
		}
	}

	ca.mu.Lock()
	ca.caCert = caCert
	ca.caKey = caKey
	ca.caCertPEM = certPEM
	ca.caKeyPEM = keyPEM
	ca.mu.Unlock()

	return nil
}

// saveCA saves CA certificate and key to files
func (ca *InternalCA) saveCA(keyPath, certPath string) error {
	ca.mu.RLock()
	defer ca.mu.RUnlock()

	// Save certificate
	if err := ioutil.WriteFile(certPath, ca.caCertPEM, 0644); err != nil {
		return fmt.Errorf("failed to save CA certificate: %w", err)
	}

	// Save private key (with restricted permissions)
	if err := ioutil.WriteFile(keyPath, ca.caKeyPEM, 0600); err != nil {
		return fmt.Errorf("failed to save CA key: %w", err)
	}

	return nil
}

// GenerateServerCertificate generates a server certificate signed by the CA
func (ca *InternalCA) GenerateServerCertificate(hostname string, ips []net.IP, additionalDNS []string) (*tls.Certificate, error) {
	ca.mu.RLock()
	caCert := ca.caCert
	caKey := ca.caKey
	ca.mu.RUnlock()

	if caCert == nil || caKey == nil {
		return nil, fmt.Errorf("CA not initialized")
	}

	// Check cache first
	cacheKey := fmt.Sprintf("server:%s", hostname)
	if cert := ca.getCachedCert(cacheKey); cert != nil {
		ca.logger.Debug("Using cached server certificate", "hostname", hostname)
		return cert, nil
	}

	// Generate new private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			Organization: []string{"Zeitwork"},
			Country:      []string{"US"},
			CommonName:   hostname,
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(ca.validityPeriod),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		IPAddresses:  ips,
		DNSNames:     append([]string{hostname}, additionalDNS...),
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Create tls.Certificate
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS certificate: %w", err)
	}

	// Cache the certificate
	ca.setCachedCert(cacheKey, &cert)

	// Save to disk for persistence
	ca.saveCertificate(hostname, certPEM, keyPEM)

	ca.logger.Info("Generated server certificate", "hostname", hostname)

	return &cert, nil
}

// GenerateClientCertificate generates a client certificate for mutual TLS
func (ca *InternalCA) GenerateClientCertificate(clientID string) (*tls.Certificate, error) {
	ca.mu.RLock()
	caCert := ca.caCert
	caKey := ca.caKey
	ca.mu.RUnlock()

	if caCert == nil || caKey == nil {
		return nil, fmt.Errorf("CA not initialized")
	}

	// Check cache first
	cacheKey := fmt.Sprintf("client:%s", clientID)
	if cert := ca.getCachedCert(cacheKey); cert != nil {
		ca.logger.Debug("Using cached client certificate", "client_id", clientID)
		return cert, nil
	}

	// Generate new private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			Organization: []string{"Zeitwork"},
			Country:      []string{"US"},
			CommonName:   clientID,
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(ca.validityPeriod),
		SubjectKeyId: []byte{1, 2, 3, 4, 7},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Create tls.Certificate
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS certificate: %w", err)
	}

	// Cache the certificate
	ca.setCachedCert(cacheKey, &cert)

	// Save to disk for persistence
	ca.saveCertificate(clientID, certPEM, keyPEM)

	ca.logger.Info("Generated client certificate", "client_id", clientID)

	return &cert, nil
}

// GetCACertificate returns the CA certificate in PEM format
func (ca *InternalCA) GetCACertificate() []byte {
	ca.mu.RLock()
	defer ca.mu.RUnlock()

	return ca.caCertPEM
}

// GetCAPool returns a certificate pool with the CA certificate
func (ca *InternalCA) GetCAPool() *x509.CertPool {
	ca.mu.RLock()
	defer ca.mu.RUnlock()

	pool := x509.NewCertPool()
	pool.AddCert(ca.caCert)
	return pool
}

// GetServerTLSConfig returns TLS configuration for a server with mTLS
func (ca *InternalCA) GetServerTLSConfig(hostname string, ips []net.IP) (*tls.Config, error) {
	// Generate server certificate
	cert, err := ca.GenerateServerCertificate(hostname, ips, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate server certificate: %w", err)
	}

	// Get CA pool for client verification
	caPool := ca.GetCAPool()

	return &tls.Config{
		Certificates: []tls.Certificate{*cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}, nil
}

// GetClientTLSConfig returns TLS configuration for a client with mTLS
func (ca *InternalCA) GetClientTLSConfig(clientID string) (*tls.Config, error) {
	// Generate client certificate
	cert, err := ca.GenerateClientCertificate(clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate client certificate: %w", err)
	}

	// Get CA pool for server verification
	caPool := ca.GetCAPool()

	return &tls.Config{
		Certificates:       []tls.Certificate{*cert},
		RootCAs:            caPool,
		InsecureSkipVerify: false,
		MinVersion:         tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}, nil
}

// Certificate caching

func (ca *InternalCA) getCachedCert(key string) *tls.Certificate {
	ca.mu.RLock()
	defer ca.mu.RUnlock()

	cert, exists := ca.certCache[key]
	if !exists {
		return nil
	}

	// Check if certificate is still valid
	if len(cert.Certificate) > 0 {
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil || time.Now().After(x509Cert.NotAfter) {
			// Certificate expired or invalid
			delete(ca.certCache, key)
			return nil
		}
	}

	return cert
}

func (ca *InternalCA) setCachedCert(key string, cert *tls.Certificate) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	ca.certCache[key] = cert
}

// saveCertificate saves a certificate to disk
func (ca *InternalCA) saveCertificate(name string, certPEM, keyPEM []byte) error {
	certPath := filepath.Join(ca.certDir, fmt.Sprintf("%s.crt", name))
	keyPath := filepath.Join(ca.certDir, fmt.Sprintf("%s.key", name))

	if err := ioutil.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("failed to save certificate: %w", err)
	}

	if err := ioutil.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("failed to save key: %w", err)
	}

	return nil
}

// RotateCertificates rotates all certificates that are near expiration
func (ca *InternalCA) RotateCertificates() error {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	rotationThreshold := time.Now().Add(ca.rotationPeriod)
	expiredKeys := []string{}

	for key, cert := range ca.certCache {
		if len(cert.Certificate) > 0 {
			x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil || x509Cert.NotAfter.Before(rotationThreshold) {
				// Mark for rotation
				expiredKeys = append(expiredKeys, key)
			}
		}
	}

	// Remove expired certificates from cache
	for _, key := range expiredKeys {
		delete(ca.certCache, key)
		ca.logger.Info("Rotated certificate", "key", key)
	}

	return nil
}
