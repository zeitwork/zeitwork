package tls

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"
)

// MTLSTransport creates an HTTP transport with mutual TLS authentication
type MTLSTransport struct {
	*http.Transport
	tlsConfig *tls.Config
}

// NewMTLSTransport creates a new mTLS-enabled HTTP transport
func NewMTLSTransport(tlsConfig *tls.Config) *MTLSTransport {
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	return &MTLSTransport{
		Transport: transport,
		tlsConfig: tlsConfig,
	}
}

// RoundTrip implements the http.RoundTripper interface
func (t *MTLSTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Ensure we're using HTTPS for internal communication
	if req.URL.Scheme == "http" && !isLocalhost(req.URL.Host) {
		req.URL.Scheme = "https"
	}

	return t.Transport.RoundTrip(req)
}

// NewMTLSClient creates an HTTP client with mTLS
func NewMTLSClient(tlsConfig *tls.Config) *http.Client {
	return &http.Client{
		Transport: NewMTLSTransport(tlsConfig),
		Timeout:   30 * time.Second,
	}
}

// isLocalhost checks if the host is localhost
func isLocalhost(host string) bool {
	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		hostname = host
	}

	return hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
}

// UpgradeConnection upgrades an existing connection to use TLS
func UpgradeConnection(conn net.Conn, tlsConfig *tls.Config) (*tls.Conn, error) {
	tlsConn := tls.Server(conn, tlsConfig)

	// Perform TLS handshake
	if err := tlsConn.Handshake(); err != nil {
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}

	// Verify the connection
	state := tlsConn.ConnectionState()
	if !state.HandshakeComplete {
		return nil, fmt.Errorf("TLS handshake incomplete")
	}

	if len(state.PeerCertificates) == 0 {
		return nil, fmt.Errorf("no peer certificates provided")
	}

	return tlsConn, nil
}

// CreateTLSListener creates a TLS listener with the given configuration
func CreateTLSListener(address string, tlsConfig *tls.Config) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}

	return tls.NewListener(listener, tlsConfig), nil
}

// DialTLS establishes a TLS connection to the given address
func DialTLS(address string, tlsConfig *tls.Config) (*tls.Conn, error) {
	conn, err := tls.Dial("tcp", address, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial TLS: %w", err)
	}

	// Verify the connection
	state := conn.ConnectionState()
	if !state.HandshakeComplete {
		conn.Close()
		return nil, fmt.Errorf("TLS handshake incomplete")
	}

	return conn, nil
}

// VerifyPeerCertificate verifies that the peer certificate is valid
func VerifyPeerCertificate(state tls.ConnectionState, expectedCN string) error {
	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("no peer certificates")
	}

	cert := state.PeerCertificates[0]

	// Verify common name if specified
	if expectedCN != "" && cert.Subject.CommonName != expectedCN {
		return fmt.Errorf("certificate CN mismatch: expected %s, got %s",
			expectedCN, cert.Subject.CommonName)
	}

	// Check certificate validity
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return fmt.Errorf("certificate not valid at current time")
	}

	return nil
}
