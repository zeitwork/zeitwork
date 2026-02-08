package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	e2eDir        = ".e2e"
	clusterFile   = ".e2e/cluster.json"
	sshKeyFile    = ".e2e/id_ed25519"
	sshPubKeyFile = ".e2e/id_ed25519.pub"
	tunnelFile    = ".e2e/tunnels.json"
	composeFile   = "e2e/docker-compose.yml"
)

// ServerState tracks the state of a single bare metal server.
type ServerState struct {
	ID           string `json:"id"`            // Latitude server ID (e.g., "sv_xxx")
	Hostname     string `json:"hostname"`      // e.g., "e2e-1"
	PublicIP     string `json:"public_ip"`     // Public IPv4
	InternalIP   string `json:"internal_ip"`   // VLAN IP (e.g., "10.200.0.1")
	AssignmentID string `json:"assignment_id"` // VLAN assignment ID (for teardown)
}

// ClusterState is persisted to .e2e/cluster.json between CLI invocations.
type ClusterState struct {
	ProjectID     string        `json:"project_id"`
	VLANID        string        `json:"vlan_id"`
	VLANVID       int64         `json:"vlan_vid"`
	SSHKeyID      string        `json:"ssh_key_id"`
	SSHKeyPath    string        `json:"ssh_key_path"`
	SSHPubKeyPath string        `json:"ssh_pub_key_path"`
	Servers       []ServerState `json:"servers"`
	Site          string        `json:"site"`
	Plan          string        `json:"plan"`
	CreatedAt     time.Time     `json:"created_at"`
}

// Server returns the server at 1-based index.
func (c *ClusterState) Server(index int) (*ServerState, error) {
	if index < 1 || index > len(c.Servers) {
		return nil, fmt.Errorf("server index %d out of range (have %d servers)", index, len(c.Servers))
	}
	return &c.Servers[index-1], nil
}

// LoadCluster reads the cluster state from disk.
func LoadCluster() (*ClusterState, error) {
	data, err := os.ReadFile(clusterFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no cluster found — run 'e2e infra up' first")
		}
		return nil, fmt.Errorf("failed to read cluster state: %w", err)
	}
	var state ClusterState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse cluster state: %w", err)
	}
	return &state, nil
}

// Save persists the cluster state to disk.
func (c *ClusterState) Save() error {
	if err := os.MkdirAll(filepath.Dir(clusterFile), 0o755); err != nil {
		return fmt.Errorf("failed to create %s directory: %w", e2eDir, err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cluster state: %w", err)
	}
	if err := os.WriteFile(clusterFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write cluster state: %w", err)
	}
	return nil
}

// ClusterExists returns true if a cluster state file exists on disk.
func ClusterExists() bool {
	_, err := os.Stat(clusterFile)
	return err == nil
}

// RemoveClusterState removes the .e2e/ directory entirely.
func RemoveClusterState() error {
	return os.RemoveAll(e2eDir)
}
