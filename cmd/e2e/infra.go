package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var (
	flagReuse bool
	flagSite  string
	flagPlan  string
)

var infraCmd = &cobra.Command{
	Use:   "infra",
	Short: "Manage E2E infrastructure",
}

var infraUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Provision bare metal servers, start local services, establish tunnels",
	RunE:  infraUp,
}

var infraDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Tear down servers, stop local services, kill tunnels",
	RunE:  infraDown,
}

var infraStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show cluster status",
	RunE:  infraStatus,
}

func init() {
	infraUpCmd.Flags().BoolVar(&flagReuse, "reuse", false, "Reuse existing servers if cluster.json exists")
	infraUpCmd.Flags().StringVar(&flagSite, "site", "FRA", "Latitude site slug")
	infraUpCmd.Flags().StringVar(&flagPlan, "plan", "c2-small-x86", "Latitude plan slug")
	infraCmd.AddCommand(infraUpCmd, infraDownCmd, infraStatusCmd)
}

func infraUp(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	start := time.Now()

	// Check if reusing existing cluster
	if flagReuse && ClusterExists() {
		slog.Info("reusing existing cluster")
		cluster, err := LoadCluster()
		if err != nil {
			return err
		}
		if err := ensureLocalServices(); err != nil {
			return err
		}
		if err := runMigrations(); err != nil {
			return err
		}
		if err := startTunnels(cluster); err != nil {
			return err
		}
		slog.Info("cluster ready (reused)", "duration", time.Since(start).Round(time.Second))
		return nil
	}

	projectID := os.Getenv("LATITUDE_PROJECT_ID")
	if projectID == "" {
		return fmt.Errorf("LATITUDE_PROJECT_ID environment variable is required")
	}
	client := newLatitudeClient()

	// Step 1: Generate SSH keypair
	slog.Info("generating SSH keypair")
	if err := generateSSHKeypair(); err != nil {
		return fmt.Errorf("failed to generate SSH keypair: %w", err)
	}

	// Step 2: Upload SSH key to Latitude
	sshKeyID, err := createSSHKey(ctx, client, projectID)
	if err != nil {
		return fmt.Errorf("failed to upload SSH key: %w", err)
	}
	slog.Info("SSH key uploaded", "id", sshKeyID)

	// Step 3: Create VLAN
	vlanID, vlanVID, err := createVLAN(ctx, client, projectID, flagSite)
	if err != nil {
		return fmt.Errorf("failed to create VLAN: %w", err)
	}
	slog.Info("VLAN created", "id", vlanID, "vid", vlanVID)

	cluster := &ClusterState{
		ProjectID:     projectID,
		VLANID:        vlanID,
		VLANVID:       vlanVID,
		SSHKeyID:      sshKeyID,
		SSHKeyPath:    sshKeyFile,
		SSHPubKeyPath: sshPubKeyFile,
		Site:          flagSite,
		Plan:          flagPlan,
		CreatedAt:     time.Now(),
	}

	// Step 4: Create 2 servers in parallel
	slog.Info("creating servers", "count", 2, "site", flagSite, "plan", flagPlan)
	var wg sync.WaitGroup
	var mu sync.Mutex
	serverErrors := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hostname := fmt.Sprintf("e2e-%d", idx+1)
			serverID, err := createServer(ctx, client, projectID, flagSite, flagPlan, hostname, sshKeyID)
			if err != nil {
				serverErrors[idx] = fmt.Errorf("failed to create %s: %w", hostname, err)
				return
			}
			mu.Lock()
			cluster.Servers = append(cluster.Servers, ServerState{
				ID:         serverID,
				Hostname:   hostname,
				InternalIP: fmt.Sprintf("10.200.0.%d", idx+1),
			})
			mu.Unlock()
			slog.Info("server created", "hostname", hostname, "id", serverID)
		}(i)
	}
	wg.Wait()
	for _, err := range serverErrors {
		if err != nil {
			return err
		}
	}

	// Save early so we can tear down if later steps fail
	if err := cluster.Save(); err != nil {
		return err
	}

	// Step 5: Wait for servers to be ready
	slog.Info("waiting for servers to be ready (this can take 10-15 minutes)")
	for i := range cluster.Servers {
		ip, err := waitForServer(ctx, client, cluster.Servers[i].ID, 15*time.Minute)
		if err != nil {
			return fmt.Errorf("server %s failed to become ready: %w", cluster.Servers[i].Hostname, err)
		}
		cluster.Servers[i].PublicIP = ip
		slog.Info("server ready", "hostname", cluster.Servers[i].Hostname, "ip", ip)
	}
	if err := cluster.Save(); err != nil {
		return err
	}

	// Step 6: Assign servers to VLAN
	for i := range cluster.Servers {
		assignID, err := assignServerToVLAN(ctx, client, cluster.Servers[i].ID, vlanID)
		if err != nil {
			return fmt.Errorf("failed to assign %s to VLAN: %w", cluster.Servers[i].Hostname, err)
		}
		cluster.Servers[i].AssignmentID = assignID
		slog.Info("server assigned to VLAN", "hostname", cluster.Servers[i].Hostname)
	}
	if err := cluster.Save(); err != nil {
		return err
	}

	// Step 7: Configure VLAN interfaces via SSH
	slog.Info("configuring VLAN interfaces")
	for _, s := range cluster.Servers {
		// Wait for SSH to be available
		for attempt := 0; attempt < 30; attempt++ {
			_, err := sshRun(cluster.SSHKeyPath, s.PublicIP, "echo ok")
			if err == nil {
				break
			}
			time.Sleep(10 * time.Second)
		}

		// Detect the primary NIC
		nic, err := sshRun(cluster.SSHKeyPath, s.PublicIP,
			`ip -o link show | awk -F': ' '/state UP/ && !/lo|vlan|docker|br-/{print $2; exit}'`)
		if err != nil {
			return fmt.Errorf("failed to detect NIC on %s: %w", s.Hostname, err)
		}

		// Configure 802.1Q VLAN sub-interface
		vlanCmds := fmt.Sprintf(`
ip link add link %s name vlan%d type vlan id %d 2>/dev/null || true
ip addr flush dev vlan%d 2>/dev/null || true
ip addr add %s/24 dev vlan%d
ip link set vlan%d up
`, nic, vlanVID, vlanVID, vlanVID, s.InternalIP, vlanVID, vlanVID)

		if _, err := sshRun(cluster.SSHKeyPath, s.PublicIP, vlanCmds); err != nil {
			return fmt.Errorf("failed to configure VLAN on %s: %w", s.Hostname, err)
		}
		slog.Info("VLAN interface configured", "hostname", s.Hostname, "ip", s.InternalIP)
	}

	// Step 8: Run Ansible (node role on all servers)
	slog.Info("running Ansible provisioning")
	if err := runAnsible(cluster); err != nil {
		return err
	}

	// Step 9: Start local Docker Compose services
	if err := ensureLocalServices(); err != nil {
		return err
	}

	// Step 10: Run DB migrations
	if err := runMigrations(); err != nil {
		return err
	}

	// Step 11: Start reverse SSH tunnels
	slog.Info("establishing reverse SSH tunnels")
	if err := startTunnels(cluster); err != nil {
		return err
	}

	// Step 12: Wait for zeitwork services to connect
	slog.Info("waiting for zeitwork services to connect via tunnels")
	time.Sleep(10 * time.Second)

	slog.Info("cluster ready",
		"servers", len(cluster.Servers),
		"duration", time.Since(start).Round(time.Second),
	)

	for _, s := range cluster.Servers {
		fmt.Printf("  %s: %s (internal: %s)\n", s.Hostname, s.PublicIP, s.InternalIP)
	}
	fmt.Printf("  DB: postgresql://zeitwork:zeitwork@127.0.0.1:15432/zeitwork\n")
	fmt.Printf("  Tunnels: %s\n", tunnelSummary())

	return nil
}

func infraDown(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Stop tunnels first
	slog.Info("stopping tunnels")
	if err := stopTunnels(); err != nil {
		slog.Warn("failed to stop tunnels", "err", err)
	}

	// Stop local Docker Compose
	slog.Info("stopping local services")
	stopLocalServices()

	if !ClusterExists() {
		slog.Info("no cluster state found, nothing to tear down on Latitude")
		return nil
	}

	cluster, err := LoadCluster()
	if err != nil {
		return err
	}

	client := newLatitudeClient()

	// Delete servers in parallel
	var wg sync.WaitGroup
	for _, s := range cluster.Servers {
		wg.Add(1)
		go func(server ServerState) {
			defer wg.Done()
			if server.ID == "" {
				return
			}
			slog.Info("deleting server", "hostname", server.Hostname, "id", server.ID)
			if err := deleteServer(ctx, client, server.ID); err != nil {
				slog.Warn("failed to delete server", "hostname", server.Hostname, "err", err)
			}
		}(s)
	}
	wg.Wait()

	// Delete VLAN
	if cluster.VLANID != "" {
		slog.Info("deleting VLAN", "id", cluster.VLANID)
		if err := deleteVLAN(ctx, client, cluster.VLANID); err != nil {
			slog.Warn("failed to delete VLAN", "err", err)
		}
	}

	// Delete SSH key
	if cluster.SSHKeyID != "" {
		slog.Info("deleting SSH key", "id", cluster.SSHKeyID)
		if err := deleteSSHKey(ctx, client, cluster.SSHKeyID); err != nil {
			slog.Warn("failed to delete SSH key", "err", err)
		}
	}

	if err := RemoveClusterState(); err != nil {
		slog.Warn("failed to remove cluster state", "err", err)
	}

	slog.Info("teardown complete")
	return nil
}

func infraStatus(cmd *cobra.Command, args []string) error {
	// Local services
	fmt.Println("=== Local Services ===")
	composeStatus := exec.Command("docker", "compose", "-f", composeFile, "ps", "--format", "table")
	composeStatus.Stdout = os.Stdout
	composeStatus.Stderr = os.Stderr
	if err := composeStatus.Run(); err != nil {
		fmt.Println("  Docker Compose: not running")
	}

	// Tunnels
	fmt.Printf("\n=== Tunnels ===\n")
	fmt.Printf("  Status: %s\n", tunnelSummary())

	// Cluster
	fmt.Printf("\n=== Servers ===\n")
	if !ClusterExists() {
		fmt.Println("  No cluster provisioned")
		return nil
	}

	cluster, err := LoadCluster()
	if err != nil {
		return err
	}

	for _, s := range cluster.Servers {
		status := "unknown"
		if s.PublicIP != "" {
			out, err := sshRun(cluster.SSHKeyPath, s.PublicIP, "systemctl is-active zeitwork 2>/dev/null || echo inactive")
			if err == nil {
				status = out
			}
		}
		fmt.Printf("  %s: %s (internal: %s) zeitwork=%s\n", s.Hostname, s.PublicIP, s.InternalIP, status)
	}

	if cluster.SSHKeyPath != "" && areTunnelsRunning() {
		fmt.Printf("\n=== Tunnel Health ===\n")
		if err := checkTunnels(cluster); err != nil {
			fmt.Printf("  %v\n", err)
		}
	}

	return nil
}

// ensureLocalServices starts the E2E Docker Compose stack if not already running.
func ensureLocalServices() error {
	slog.Info("starting local Docker Compose services")
	cmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d", "--wait")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}
	return nil
}

// stopLocalServices stops the E2E Docker Compose stack.
func stopLocalServices() {
	cmd := exec.Command("docker", "compose", "-f", composeFile, "down", "-v")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}

// runMigrations runs drizzle-kit migrate against the local PG.
func runMigrations() error {
	slog.Info("running database migrations")
	cmd := exec.Command("bun", "--cwd", "./packages/database", "drizzle-kit", "migrate")
	cmd.Env = append(os.Environ(),
		"DATABASE_URL=postgresql://zeitwork:zeitwork@127.0.0.1:15432/zeitwork?sslmode=disable",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("drizzle-kit migrate failed: %w", err)
	}
	return nil
}
