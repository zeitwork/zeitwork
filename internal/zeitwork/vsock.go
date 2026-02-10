package zeitwork

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/rpc"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

const (
	// vsockPort is the VSOCK port the guest agent connects to.
	// Cloud Hypervisor maps guest AF_VSOCK CID=2:1024 to host UDS at {socket_path}_1024.
	vsockPort = 1024

	// execPort is the VSOCK port the guest listens on for exec connections.
	// The host connects via the base VSOCK UDS with "CONNECT 1025\n".
	execPort = 1025
)

// VSockManager manages per-VM HTTP servers over VSOCK UDS sockets.
// Each VM gets its own UDS listener and HTTP server.
// VM identity is derived from which listener accepted the connection.
type VSockManager struct {
	mu        sync.Mutex
	db        *database.DB
	vms       map[uuid.UUID]*vmState
	listeners map[uuid.UUID]net.Listener
}

// vmState holds the pre-loaded config for a single VM.
type vmState struct {
	vmID     uuid.UUID
	envVars  []string
	ipAddr   string // e.g. "10.0.0.1/31"
	ipGw     string // e.g. "10.0.0.0"
	hostname string // e.g. "zeit-{vm_id}"
}

// NewVSockManager creates a new VSOCK manager.
func NewVSockManager(db *database.DB) *VSockManager {
	return &VSockManager{
		db:        db,
		vms:       make(map[uuid.UUID]*vmState),
		listeners: make(map[uuid.UUID]net.Listener),
	}
}

// VSocketPath returns the base VSOCK socket path for a VM.
// This is the path passed to Cloud Hypervisor's --vsock flag.
// It is also used by the host to connect TO the guest (CONNECT handshake).
func VSocketPath(vmID uuid.UUID) string {
	return fmt.Sprintf("/tmp/vsock-%s.sock", vmID.String())
}

// VSocketGuestPath returns the UDS path for guest-initiated connections on a given port.
// Cloud Hypervisor creates this when the guest dials AF_VSOCK CID=2:<port>.
func VSocketGuestPath(vmID uuid.UUID) string {
	return fmt.Sprintf("%s_%d", VSocketPath(vmID), vsockPort)
}

// RegisterVM sets up the UDS listener and HTTP server for a VM.
// Must be called BEFORE starting the Cloud Hypervisor process.
func (m *VSockManager) RegisterVM(vmID uuid.UUID, envVars []string, ipAddr, ipGw, hostname string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := &vmState{
		vmID:     vmID,
		envVars:  envVars,
		ipAddr:   ipAddr,
		ipGw:     ipGw,
		hostname: hostname,
	}

	m.vms[vmID] = state

	// Create the UDS listener at {vsockPath}_{port}.
	// When the guest dials AF_VSOCK CID=2:1024, Cloud Hypervisor connects to this socket.
	udsPath := VSocketGuestPath(vmID)

	_ = os.Remove(udsPath) // remove stale socket file if it exists

	lis, err := net.Listen("unix", udsPath)
	if err != nil {
		return fmt.Errorf("failed to listen on VSOCK UDS %s: %w", udsPath, err)
	}

	m.listeners[vmID] = lis

	// Create HTTP server with handlers for this VM.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config", m.handleConfig(state))
	mux.HandleFunc("POST /logs", m.handleLogs(state))

	srv := &http.Server{Handler: mux}

	go func() {
		slog.Info("VSOCK HTTP server started", "vm_id", vmID, "uds_path", udsPath)
		if err := srv.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Debug("VSOCK HTTP server ended", "vm_id", vmID, "err", err)
		}
	}()

	return nil
}

// UnregisterVM stops the UDS listener and cleans up state for a VM.
func (m *VSockManager) UnregisterVM(vmID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if lis, ok := m.listeners[vmID]; ok {
		lis.Close()
		delete(m.listeners, vmID)
	}

	delete(m.vms, vmID)

	_ = os.Remove(VSocketPath(vmID))
	_ = os.Remove(VSocketGuestPath(vmID))

	slog.Info("VSOCK VM unregistered", "vm_id", vmID)
}

// Stop cleans up all listeners.
func (m *VSockManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for vmID, lis := range m.listeners {
		lis.Close()
		_ = os.Remove(VSocketPath(vmID))
		_ = os.Remove(VSocketGuestPath(vmID))
	}
}

// handleConfig returns the VM's env vars and network configuration.
func (m *VSockManager) handleConfig(state *vmState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("served config via VSOCK", "vm_id", state.vmID, "env_count", len(state.envVars))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rpc.ConfigResponse{
			Env:      state.envVars,
			IPAddr:   state.ipAddr,
			IPGW:     state.ipGw,
			Hostname: state.hostname,
		})
	}
}

// handleLogs receives a long-lived POST with raw log lines (one per line).
// The guest keeps the request body open and writes lines as they come.
func (m *VSockManager) handleLogs(state *vmState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("log stream started", "vm_id", state.vmID)

		scanner := bufio.NewScanner(r.Body)
		// Allow up to 1MB per line
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			if dbErr := m.db.VMLogCreate(r.Context(), queries.VMLogCreateParams{
				ID:      uuid.New(),
				VmID:    state.vmID,
				Message: scanner.Text(),
				Level:   pgtype.Text{String: "info", Valid: true},
			}); dbErr != nil {
				slog.Error("failed to write vm log", "vm_id", state.vmID, "err", dbErr)
			}
		}

		if err := scanner.Err(); err != nil {
			slog.Debug("log stream ended with error", "vm_id", state.vmID, "err", err)
		} else {
			slog.Debug("log stream ended", "vm_id", state.vmID)
		}

		w.WriteHeader(http.StatusOK)
	}
}
