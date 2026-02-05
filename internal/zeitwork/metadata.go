package zeitwork

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const (
	tokenTTL             = 5 * time.Minute
	tokenCleanupInterval = 1 * time.Minute
)

var (
	ErrVMNotFound   = errors.New("vm not found")
	ErrTokenInvalid = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
	ErrIPMismatch   = errors.New("source IP does not match VM IP")
)

// vmEntry stores the token and env vars for a VM, keyed by VM ID.
type vmEntry struct {
	token     string
	envVars   []string
	vmIP      netip.Addr
	expiresAt time.Time
}

// MetadataServer serves VM environment variables via HTTP.
// It binds to 0.0.0.0:8111 and validates that requests come from known VM IPs.
// Tokens are one-time use and expire after 5 minutes.
//
// Endpoint: GET /v1/vms/{vm_id}/config?token={token}
type MetadataServer struct {
	vms sync.Map // map[vmID string]vmEntry

	server *http.Server
}

// NewMetadataServer creates a new metadata server instance.
func NewMetadataServer() *MetadataServer {
	return &MetadataServer{}
}

// RegisterVM stores environment variables for a VM with a one-time token.
// The token expires after 5 minutes if not consumed.
func (m *MetadataServer) RegisterVM(vmID string, token string, envVars []string, vmIP netip.Addr) {
	m.vms.Store(vmID, vmEntry{
		token:     token,
		envVars:   envVars,
		vmIP:      vmIP,
		expiresAt: time.Now().Add(tokenTTL),
	})
	slog.Debug("registered VM for metadata", "vmID", vmID, "token", token[:8]+"...", "vmIP", vmIP.String(), "envVarsCount", len(envVars))
}

// UnregisterVM removes a VM from the metadata server.
// Call this when a VM is deleted.
func (m *MetadataServer) UnregisterVM(vmID string) {
	m.vms.Delete(vmID)
	slog.Debug("unregistered VM from metadata server", "vmID", vmID)
}

// ConsumeConfig validates the token and source IP, returns env vars, and removes the entry (one-time use).
func (m *MetadataServer) ConsumeConfig(vmID string, token string, sourceIP string) ([]string, error) {
	value, ok := m.vms.LoadAndDelete(vmID)
	if !ok {
		return nil, ErrVMNotFound
	}

	entry := value.(vmEntry)

	// Check expiration first
	if time.Now().After(entry.expiresAt) {
		return nil, ErrTokenExpired
	}

	// Validate token
	if entry.token != token {
		// Re-store the entry since token was wrong (don't consume on wrong token)
		m.vms.Store(vmID, entry)
		return nil, ErrTokenInvalid
	}

	// Validate source IP matches the registered VM IP
	if entry.vmIP.String() != sourceIP {
		// Re-store the entry since IP was wrong
		m.vms.Store(vmID, entry)
		return nil, ErrIPMismatch
	}

	return entry.envVars, nil
}

// ServeHTTP handles HTTP requests to the metadata server.
// Endpoint: GET /v1/vms/{vm_id}/config?token={token}
func (m *MetadataServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /v1/vms/{vm_id}/config
	// Expected format: ["", "v1", "vms", "{vm_id}", "config"]
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 5 || parts[1] != "v1" || parts[2] != "vms" || parts[4] != "config" {
		http.NotFound(w, r)
		return
	}
	vmID := parts[3]
	if vmID == "" {
		http.Error(w, "missing vm_id", http.StatusBadRequest)
		return
	}

	// Extract source IP
	srcIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		slog.Warn("metadata request with invalid remote addr", "remoteAddr", r.RemoteAddr, "err", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Get token from query parameter
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token parameter", http.StatusBadRequest)
		return
	}

	// Consume config (validates token, IP, and returns env vars)
	envVars, err := m.ConsumeConfig(vmID, token, srcIP)
	if err != nil {
		slog.Warn("metadata request failed", "vmID", vmID, "ip", srcIP, "err", err)
		switch err {
		case ErrVMNotFound:
			http.Error(w, "vm not found", http.StatusNotFound)
		case ErrTokenInvalid, ErrIPMismatch:
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case ErrTokenExpired:
			http.Error(w, "token expired", http.StatusUnauthorized)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	slog.Info("served config to VM", "vmID", vmID, "ip", srcIP, "envVarsCount", len(envVars))

	// Return env vars as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{"env": envVars})
}

// Start begins the HTTP server on the given address.
// It also starts a background goroutine to cleanup expired entries.
func (m *MetadataServer) Start(ctx context.Context, addr string) error {
	m.server = &http.Server{
		Addr:    addr,
		Handler: m,
	}

	// Start cleanup goroutine
	go m.cleanupExpired(ctx)

	slog.Info("starting metadata server", "addr", addr)
	err := m.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully shuts down the metadata server.
func (m *MetadataServer) Stop(ctx context.Context) error {
	if m.server != nil {
		return m.server.Shutdown(ctx)
	}
	return nil
}

// cleanupExpired periodically removes expired VM entries from the store.
func (m *MetadataServer) cleanupExpired(ctx context.Context) {
	ticker := time.NewTicker(tokenCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			m.vms.Range(func(key, value interface{}) bool {
				entry := value.(vmEntry)
				if now.After(entry.expiresAt) {
					m.vms.Delete(key)
					slog.Debug("cleaned up expired VM entry", "vmID", key.(string))
				}
				return true
			})
		}
	}
}
