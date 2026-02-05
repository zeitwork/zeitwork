package zeitwork

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	maxLogBatchSize      = 100         // Maximum number of log lines per batch
	maxLogMessageSize    = 1024 * 1024 // 1MB max per request body
)

var (
	ErrVMNotFound   = errors.New("vm not found")
	ErrTokenInvalid = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
	ErrIPMismatch   = errors.New("source IP does not match VM IP")
)

// LogEntry represents a single log line sent from the init agent.
type LogEntry struct {
	Message string `json:"message"`
	Level   string `json:"level"`
}

// LogBatch is the request body for the log ingestion endpoint.
type LogBatch struct {
	Logs []LogEntry `json:"logs"`
}

// LogCallback is called when log entries are received from a VM.
// deploymentID and organisationID identify which deployment the logs belong to.
type LogCallback func(ctx context.Context, deploymentID, organisationID string, logs []LogEntry)

// vmEntry stores the token and env vars for a VM, keyed by VM ID.
type vmEntry struct {
	token     string
	envVars   []string
	vmIP      netip.Addr
	expiresAt time.Time

	// Log ingestion fields (persisted after config is consumed)
	logToken       string
	deploymentID   string
	organisationID string
	configConsumed bool
}

// MetadataServer serves VM environment variables via HTTP.
// It binds to 0.0.0.0:8111 and validates that requests come from known VM IPs.
// Tokens are one-time use and expire after 5 minutes.
//
// Endpoints:
//
//	GET  /v1/vms/{vm_id}/config?token={token}  — fetch env vars (one-time)
//	POST /v1/vms/{vm_id}/logs?token={log_token} — ingest runtime logs
type MetadataServer struct {
	vms sync.Map // map[vmID string]vmEntry

	server *http.Server

	// OnLog is called when log entries are received from a VM.
	OnLog LogCallback
}

// NewMetadataServer creates a new metadata server instance.
func NewMetadataServer() *MetadataServer {
	return &MetadataServer{}
}

// RegisterVM stores environment variables for a VM with a one-time token.
// The token expires after 5 minutes if not consumed.
// deploymentID and organisationID are used for log ingestion routing.
// logToken is a separate long-lived token for the log ingestion endpoint.
func (m *MetadataServer) RegisterVM(vmID string, token string, logToken string, envVars []string, vmIP netip.Addr, deploymentID, organisationID string) {
	m.vms.Store(vmID, vmEntry{
		token:          token,
		envVars:        envVars,
		vmIP:           vmIP,
		expiresAt:      time.Now().Add(tokenTTL),
		logToken:       logToken,
		deploymentID:   deploymentID,
		organisationID: organisationID,
	})
	slog.Debug("registered VM for metadata", "vmID", vmID, "token", token[:8]+"...", "vmIP", vmIP.String(), "envVarsCount", len(envVars), "deploymentID", deploymentID)
}

// UnregisterVM removes a VM from the metadata server.
// Call this when a VM is deleted.
func (m *MetadataServer) UnregisterVM(vmID string) {
	m.vms.Delete(vmID)
	slog.Debug("unregistered VM from metadata server", "vmID", vmID)
}

// ConsumeConfig validates the token and source IP, returns env vars and log token.
// The entry is kept alive (marked as consumed) so that log ingestion can continue.
func (m *MetadataServer) ConsumeConfig(vmID string, token string, sourceIP string) ([]string, string, error) {
	value, ok := m.vms.Load(vmID)
	if !ok {
		return nil, "", ErrVMNotFound
	}

	entry := value.(vmEntry)

	// Don't allow re-consuming config
	if entry.configConsumed {
		return nil, "", ErrTokenInvalid
	}

	// Check expiration first
	if time.Now().After(entry.expiresAt) {
		m.vms.Delete(vmID)
		return nil, "", ErrTokenExpired
	}

	// Validate token
	if entry.token != token {
		return nil, "", ErrTokenInvalid
	}

	// Validate source IP matches the registered VM IP
	if entry.vmIP.String() != sourceIP {
		return nil, "", ErrIPMismatch
	}

	// Mark as consumed but keep entry alive for log ingestion
	entry.configConsumed = true
	m.vms.Store(vmID, entry)

	return entry.envVars, entry.logToken, nil
}

// validateLogRequest validates the log token and source IP for a log ingestion request.
func (m *MetadataServer) validateLogRequest(vmID string, logToken string, sourceIP string) (vmEntry, error) {
	value, ok := m.vms.Load(vmID)
	if !ok {
		return vmEntry{}, ErrVMNotFound
	}

	entry := value.(vmEntry)

	// Validate log token
	if entry.logToken != logToken {
		return vmEntry{}, ErrTokenInvalid
	}

	// Validate source IP matches the registered VM IP
	if entry.vmIP.String() != sourceIP {
		return vmEntry{}, ErrIPMismatch
	}

	return entry, nil
}

// ServeHTTP handles HTTP requests to the metadata server.
// Endpoints:
//
//	GET  /v1/vms/{vm_id}/config?token={token}
//	POST /v1/vms/{vm_id}/logs?token={log_token}
func (m *MetadataServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse path: /v1/vms/{vm_id}/{action}
	// Expected format: ["", "v1", "vms", "{vm_id}", "{action}"]
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 5 || parts[1] != "v1" || parts[2] != "vms" {
		http.NotFound(w, r)
		return
	}
	vmID := parts[3]
	action := parts[4]
	if vmID == "" {
		http.Error(w, "missing vm_id", http.StatusBadRequest)
		return
	}

	switch action {
	case "config":
		m.handleConfig(w, r, vmID)
	case "logs":
		m.handleLogs(w, r, vmID)
	default:
		http.NotFound(w, r)
	}
}

func (m *MetadataServer) handleConfig(w http.ResponseWriter, r *http.Request, vmID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	// Consume config (validates token, IP, and returns env vars + log token)
	envVars, logToken, err := m.ConsumeConfig(vmID, token, srcIP)
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

	// Return env vars and log token as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"env":       envVars,
		"log_token": logToken,
	})
}

func (m *MetadataServer) handleLogs(w http.ResponseWriter, r *http.Request, vmID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract source IP
	srcIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Get log token from query parameter
	logToken := r.URL.Query().Get("token")
	if logToken == "" {
		http.Error(w, "missing token parameter", http.StatusBadRequest)
		return
	}

	// Validate
	entry, err := m.validateLogRequest(vmID, logToken, srcIP)
	if err != nil {
		slog.Warn("log request failed", "vmID", vmID, "ip", srcIP, "err", err)
		switch err {
		case ErrVMNotFound:
			http.Error(w, "vm not found", http.StatusNotFound)
		case ErrTokenInvalid, ErrIPMismatch:
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	// Read and parse log batch
	body, err := io.ReadAll(io.LimitReader(r.Body, int64(maxLogMessageSize)))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var batch LogBatch
	if err := json.Unmarshal(body, &batch); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if len(batch.Logs) > maxLogBatchSize {
		http.Error(w, "batch too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Deliver logs via callback
	if m.OnLog != nil && len(batch.Logs) > 0 {
		m.OnLog(r.Context(), entry.deploymentID, entry.organisationID, batch.Logs)
	}

	w.WriteHeader(http.StatusNoContent)
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
// Only removes entries that haven't had their config consumed (active VMs stay).
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
				// Only clean up entries whose config was never consumed and token has expired
				if !entry.configConsumed && now.After(entry.expiresAt) {
					m.vms.Delete(key)
					slog.Debug("cleaned up expired VM entry", "vmID", key.(string))
				}
				return true
			})
		}
	}
}
