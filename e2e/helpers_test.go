package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Cluster provides access to the E2E test cluster.
// DB connects to the local PG (via Docker Compose).
// SSH connects to bare metal servers (via Latitude).
type Cluster struct {
	Server1IP         string // Public IP of server 1
	Server2IP         string // Public IP of server 2
	Server1InternalIP string // VLAN IP of server 1
	Server2InternalIP string // VLAN IP of server 2
	SSHKeyPath        string
	DB                *pgxpool.Pool
}

// SetupCluster initializes the test cluster from environment variables.
// Skips the test if the cluster is not available.
func SetupCluster(t *testing.T) *Cluster {
	t.Helper()

	server1IP := os.Getenv("E2E_SERVER1_IP")
	server2IP := os.Getenv("E2E_SERVER2_IP")
	sshKey := os.Getenv("E2E_SSH_KEY")

	if server1IP == "" || server2IP == "" || sshKey == "" {
		t.Skip("E2E cluster not configured (set E2E_SERVER1_IP, E2E_SERVER2_IP, E2E_SSH_KEY)")
	}

	c := &Cluster{
		Server1IP:         server1IP,
		Server2IP:         server2IP,
		Server1InternalIP: os.Getenv("E2E_SERVER1_INTERNAL_IP"),
		Server2InternalIP: os.Getenv("E2E_SERVER2_INTERNAL_IP"),
		SSHKeyPath:        sshKey,
	}

	// Connect to the local PG (Docker Compose on developer's machine)
	dbURL := os.Getenv("E2E_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://zeitwork:zeitwork@127.0.0.1:15432/zeitwork?sslmode=disable"
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("failed to connect to E2E database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	c.DB = pool

	return c
}

// SSH executes a command on the specified server and returns stdout.
func (c *Cluster) SSH(t *testing.T, serverIP string, command string) string {
	t.Helper()
	cmd := exec.Command("ssh",
		"-i", c.SSHKeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"root@"+serverIP,
		command,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("SSH command failed on %s: %v\ncommand: %s\nstderr: %s", serverIP, err, command, stderr.String())
	}
	return strings.TrimSpace(stdout.String())
}

// QueryRow executes a single-row SQL query and returns the result as a string.
func (c *Cluster) QueryRow(t *testing.T, query string, args ...any) string {
	t.Helper()
	var result string
	err := c.DB.QueryRow(context.Background(), query, args...).Scan(&result)
	if err != nil {
		t.Fatalf("query failed: %v\nquery: %s", err, query)
	}
	return result
}

// QueryRowInt executes a single-row SQL query and returns the result as an int.
func (c *Cluster) QueryRowInt(t *testing.T, query string, args ...any) int {
	t.Helper()
	var result int
	err := c.DB.QueryRow(context.Background(), query, args...).Scan(&result)
	if err != nil {
		t.Fatalf("query failed: %v\nquery: %s", err, query)
	}
	return result
}

// WaitFor polls a condition function until it returns true or the timeout is reached.
func WaitFor(t *testing.T, description string, timeout time.Duration, interval time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s (timeout=%s)", description, timeout)
		}
		time.Sleep(interval)
	}
}

// WaitForQuery polls a SQL query until it returns the expected value.
func (c *Cluster) WaitForQuery(t *testing.T, description string, timeout time.Duration, expectedValue string, query string, args ...any) {
	t.Helper()
	WaitFor(t, description, timeout, 2*time.Second, func() bool {
		var result string
		err := c.DB.QueryRow(context.Background(), query, args...).Scan(&result)
		if err != nil {
			return false
		}
		return result == expectedValue
	})
}

// ServerCount returns the number of active servers in the database.
func (c *Cluster) ServerCount(t *testing.T) int {
	t.Helper()
	return c.QueryRowInt(t, "SELECT count(*) FROM servers WHERE status = 'active' AND deleted_at IS NULL")
}

// VMCount returns the number of non-deleted VMs on a specific server.
func (c *Cluster) VMCount(t *testing.T, serverID string) int {
	t.Helper()
	return c.QueryRowInt(t, "SELECT count(*) FROM vms WHERE server_id = $1 AND deleted_at IS NULL", serverID)
}

// ServerIDs returns the IDs of all active servers.
func (c *Cluster) ServerIDs(t *testing.T) []string {
	t.Helper()
	rows, err := c.DB.Query(context.Background(), "SELECT id::text FROM servers WHERE status = 'active' AND deleted_at IS NULL ORDER BY created_at")
	if err != nil {
		t.Fatalf("failed to query server IDs: %v", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("failed to scan server ID: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

// ServerIPRange returns the IP range for a server.
func (c *Cluster) ServerIPRange(t *testing.T, serverID string) string {
	t.Helper()
	return c.QueryRow(t, "SELECT ip_range::text FROM servers WHERE id = $1", serverID)
}

// SeedTestData seeds the minimum data needed for deployment tests:
// user, organisation, github installation, project.
// Returns the project ID.
func (c *Cluster) SeedTestData(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	var orgID string
	err := c.DB.QueryRow(ctx, `
		INSERT INTO organisations (name, slug)
		VALUES ('E2E Test Org', 'e2e-test-org')
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`).Scan(&orgID)
	if err != nil {
		t.Fatalf("failed to seed organisation: %v", err)
	}

	var userID string
	err = c.DB.QueryRow(ctx, `
		INSERT INTO users (name, email, github_id, avatar_url, email_verified_at)
		VALUES ('E2E User', 'e2e@zeitwork.dev', 12345, '', now())
		ON CONFLICT (github_id) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	_, err = c.DB.Exec(ctx, `
		INSERT INTO organisation_members (user_id, organisation_id, role)
		VALUES ($1::uuid, $2::uuid, 'owner')
		ON CONFLICT DO NOTHING
	`, userID, orgID)
	if err != nil {
		t.Fatalf("failed to seed org membership: %v", err)
	}

	installationID := os.Getenv("E2E_GITHUB_INSTALLATION_ID")
	if installationID == "" {
		installationID = "99999"
	}
	_, err = c.DB.Exec(ctx, `
		INSERT INTO github_installations (installation_id, organisation_id)
		VALUES ($1, $2::uuid)
		ON CONFLICT (installation_id) DO NOTHING
	`, installationID, orgID)
	if err != nil {
		t.Fatalf("failed to seed github installation: %v", err)
	}

	githubRepo := os.Getenv("E2E_GITHUB_REPO")
	if githubRepo == "" {
		githubRepo = "zeitwork/e2e-test-app"
	}
	var projectID string
	err = c.DB.QueryRow(ctx, `
		INSERT INTO projects (name, slug, github_repository, github_installation_id, organisation_id, root_directory)
		VALUES ('E2E Test App', 'e2e-test-app', $1, $2, $3::uuid, '/')
		ON CONFLICT (slug, organisation_id) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`, githubRepo, installationID, orgID).Scan(&projectID)
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	t.Logf("seeded test data: org=%s user=%s project=%s", orgID, userID, projectID)
	return projectID
}

// prettyJSON formats a value as indented JSON for logging.
func prettyJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// ServerIP returns the public IP of server 1 or 2 (1-based index).
func (c *Cluster) ServerIP(index int) string {
	switch index {
	case 1:
		return c.Server1IP
	case 2:
		return c.Server2IP
	default:
		return ""
	}
}

// ServerInternalIP returns the VLAN IP of server 1 or 2 (1-based index).
func (c *Cluster) ServerInternalIP(index int) string {
	switch index {
	case 1:
		return c.Server1InternalIP
	case 2:
		return c.Server2InternalIP
	default:
		return fmt.Sprintf("unknown-server-%d", index)
	}
}
