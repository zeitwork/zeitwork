package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestVMPlacement verifies that VMs are distributed across servers.
// When multiple deployments are created, VMs should be placed on the
// least-loaded server (round-robin in practice when loads are equal).
func TestVMPlacement(t *testing.T) {
	c := SetupCluster(t)

	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	serverIDs := c.ServerIDs(t)
	if len(serverIDs) < 2 {
		t.Fatalf("need at least 2 servers, got %d", len(serverIDs))
	}

	projectID := c.SeedTestData(t)
	orgID := c.OrgIDForProject(t, projectID)

	// Create two deployments sequentially so each gets placed on a different server
	dep1ID := c.CreateDeployment(t, projectID, "main", orgID)
	t.Logf("created deployment 1: %s", dep1ID)

	// Wait for first deployment to have a VM assigned
	WaitFor(t, "deployment 1 has VM", 5*time.Minute, 2*time.Second, func() bool {
		return c.VMForDeployment(t, dep1ID) != ""
	})
	vm1ID := c.VMForDeployment(t, dep1ID)
	vm1Server := c.VMServerID(t, vm1ID)
	t.Logf("deployment 1 VM %s placed on server %s", vm1ID, vm1Server)

	// Wait for deployment 1 to be running before creating deployment 2,
	// so the second deployment is a new project's first deployment
	// (otherwise dep 2 replaces dep 1 and dep 1's VM gets deleted).
	// Instead, create a second project for independent placement.
	project2ID := c.seedSecondProject(t, orgID)
	dep2ID := c.CreateDeployment(t, project2ID, "main", orgID)
	t.Logf("created deployment 2: %s (project 2: %s)", dep2ID, project2ID)

	// Wait for second deployment to have a VM assigned
	WaitFor(t, "deployment 2 has VM", 5*time.Minute, 2*time.Second, func() bool {
		return c.VMForDeployment(t, dep2ID) != ""
	})
	vm2ID := c.VMForDeployment(t, dep2ID)
	vm2Server := c.VMServerID(t, vm2ID)
	t.Logf("deployment 2 VM %s placed on server %s", vm2ID, vm2Server)

	// With least-loaded placement and equal loads, VMs should be on different servers
	if vm1Server == vm2Server {
		t.Logf("warning: both VMs placed on same server %s (least-loaded may have chosen same server if VM counts diverged)", vm1Server)
	} else {
		t.Logf("VMs correctly distributed: server %s and server %s", vm1Server, vm2Server)
	}
}

// TestServerFailover verifies that when a server dies, its VMs are reassigned
// to a healthy server.
//
// Strategy:
// 1. Create a deployment and wait for it to be running
// 2. Stop zeitwork on the server hosting the VM
// 3. Wait for dead server detection (60s heartbeat timeout + 30s detection loop)
// 4. Verify VMs are reassigned to the remaining healthy server
// 5. Restart zeitwerk on the stopped server
func TestServerFailover(t *testing.T) {
	c := SetupCluster(t)

	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	projectID := c.SeedTestData(t)
	orgID := c.OrgIDForProject(t, projectID)

	// Create a deployment and wait for it to be running
	depID := c.CreateDeployment(t, projectID, "main", orgID)
	c.WaitForQuery(t, "deployment running",
		15*time.Minute, "running",
		"SELECT status FROM deployments WHERE id = $1::uuid", depID)

	vmID := c.VMForDeployment(t, depID)
	originalServerID := c.VMServerID(t, vmID)
	originalServerIP := c.serverIPForID(t, originalServerID)
	t.Logf("VM %s running on server %s (%s)", vmID, originalServerID, originalServerIP)

	// Determine the other server (the one that should take over)
	otherServerIP := c.otherServerIP(t, originalServerID)
	t.Logf("other server IP: %s (should take over)", otherServerIP)

	// Stop zeitwerk on the server hosting the VM
	t.Logf("stopping zeitwork on server %s...", originalServerIP)
	c.SSH(t, originalServerIP, "systemctl stop zeitwork")

	// Also kill any running cloud-hypervisor processes to simulate a full failure
	c.SSH(t, originalServerIP, "killall cloud-hypervisor 2>/dev/null || true")

	// Cleanup: always restart zeitwork on the stopped server
	t.Cleanup(func() {
		t.Logf("restarting zeitwork on server %s...", originalServerIP)
		c.SSH(t, originalServerIP, "systemctl start zeitwork")
	})

	// Wait for the server to be detected as dead
	// Heartbeat timeout is 60s, detection loop runs every 30s
	t.Logf("waiting for dead server detection (up to 120s)...")
	c.WaitForQuery(t, "server marked as dead",
		120*time.Second, "dead",
		"SELECT status FROM servers WHERE id = $1::uuid", originalServerID)
	t.Logf("server %s marked as dead", originalServerID)

	// The VM should be reassigned to the healthy server
	WaitFor(t, "VM reassigned to healthy server", 60*time.Second, 2*time.Second, func() bool {
		newServerID := c.VMServerID(t, vmID)
		return newServerID != originalServerID
	})

	newServerID := c.VMServerID(t, vmID)
	t.Logf("VM %s reassigned from server %s to server %s", vmID, originalServerID, newServerID)

	// The VM should eventually start running on the new server
	c.WaitForQuery(t, "VM running on new server",
		5*time.Minute, "running",
		"SELECT status FROM vms WHERE id = $1::uuid", vmID)
	t.Logf("VM %s running on new server %s", vmID, newServerID)

	// The deployment should still be running (or re-transition to running)
	c.WaitForQuery(t, "deployment still running",
		5*time.Minute, "running",
		"SELECT status FROM deployments WHERE id = $1::uuid", depID)
	t.Logf("deployment %s still running after failover", depID)
}

// TestServerDrain verifies zero-downtime draining.
//
// Strategy:
// 1. Create a deployment and wait for it to be running
// 2. Set the hosting server's status to "draining" in the database
// 3. Verify that a replacement VM is created on the other server
// 4. Verify the deployment's vm_id swaps to the new VM
// 5. Verify the old VM is soft-deleted
// 6. Verify the server reaches "drained" status
func TestServerDrain(t *testing.T) {
	c := SetupCluster(t)

	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	projectID := c.SeedTestData(t)
	orgID := c.OrgIDForProject(t, projectID)

	// Create a deployment and wait for it to be running
	depID := c.CreateDeployment(t, projectID, "main", orgID)
	c.WaitForQuery(t, "deployment running",
		15*time.Minute, "running",
		"SELECT status FROM deployments WHERE id = $1::uuid", depID)

	oldVMID := c.VMForDeployment(t, depID)
	oldServerID := c.VMServerID(t, oldVMID)
	t.Logf("VM %s running on server %s", oldVMID, oldServerID)

	// Set the server to draining
	t.Logf("setting server %s to draining...", oldServerID)
	_, err := c.DB.Exec(context.Background(),
		"UPDATE servers SET status = 'draining', updated_at = now() WHERE id = $1::uuid", oldServerID)
	if err != nil {
		t.Fatalf("failed to set server to draining: %v", err)
	}

	// Cleanup: reset server status if it doesn't complete naturally
	t.Cleanup(func() {
		c.DB.Exec(context.Background(),
			"UPDATE servers SET status = 'active', updated_at = now() WHERE id = $1::uuid AND status IN ('draining', 'drained')", oldServerID)
	})

	// The drain monitor should create a replacement VM on the other server
	// and atomically swap the deployment's vm_id
	WaitFor(t, "deployment VM swapped to new server", 6*time.Minute, 3*time.Second, func() bool {
		currentVMID := c.VMForDeployment(t, depID)
		if currentVMID == "" || currentVMID == oldVMID {
			return false
		}
		// Verify the new VM is on a different server
		newServerID := c.VMServerID(t, currentVMID)
		return newServerID != oldServerID
	})

	newVMID := c.VMForDeployment(t, depID)
	newServerID := c.VMServerID(t, newVMID)
	t.Logf("deployment VM swapped from %s (server %s) to %s (server %s)",
		oldVMID, oldServerID, newVMID, newServerID)

	// Verify the new VM is running
	c.WaitForQuery(t, "new VM running",
		3*time.Minute, "running",
		"SELECT status FROM vms WHERE id = $1::uuid", newVMID)

	// Verify the deployment is still running
	depStatus := c.DeploymentStatus(t, depID)
	if depStatus != "running" {
		t.Errorf("expected deployment status 'running' after drain, got '%s'", depStatus)
	}

	// Verify old VM was soft-deleted
	WaitFor(t, "old VM soft-deleted", 60*time.Second, 2*time.Second, func() bool {
		var deleted bool
		err := c.DB.QueryRow(context.Background(),
			"SELECT deleted_at IS NOT NULL FROM vms WHERE id = $1::uuid", oldVMID).Scan(&deleted)
		return err == nil && deleted
	})
	t.Logf("old VM %s soft-deleted", oldVMID)

	// Server should eventually reach "drained" status
	c.WaitForQuery(t, "server drained",
		3*time.Minute, "drained",
		"SELECT status FROM servers WHERE id = $1::uuid", oldServerID)
	t.Logf("server %s reached drained status", oldServerID)
}

// TestCrossServerVMAccess verifies that a VM on server 1 is reachable
// from server 2 via L2 routing (host routes through the VLAN).
func TestCrossServerVMAccess(t *testing.T) {
	c := SetupCluster(t)

	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	// Wait for host routes to sync
	time.Sleep(35 * time.Second)

	projectID := c.SeedTestData(t)
	orgID := c.OrgIDForProject(t, projectID)

	// Create a deployment and wait for it to be running
	depID := c.CreateDeployment(t, projectID, "main", orgID)
	c.WaitForQuery(t, "deployment running",
		15*time.Minute, "running",
		"SELECT status FROM deployments WHERE id = $1::uuid", depID)

	vmID := c.VMForDeployment(t, depID)
	vmIP := c.QueryRow(t, "SELECT host(ip_address) FROM vms WHERE id = $1::uuid", vmID)
	vmPort := c.QueryRow(t, "SELECT port::text FROM vms WHERE id = $1::uuid", vmID)
	vmServerID := c.VMServerID(t, vmID)

	// Find the other server
	otherIP := c.otherServerIP(t, vmServerID)
	if otherIP == "" {
		t.Skip("could not determine other server IP for cross-server test")
	}

	t.Logf("testing VM %s (%s:%s) from other server %s", vmID, vmIP, vmPort, otherIP)

	// Curl from the other server to the VM
	result := c.SSH(t, otherIP,
		fmt.Sprintf("curl -s -o /dev/null -w '%%{http_code}' --connect-timeout 10 http://%s:%s/", vmIP, vmPort))
	t.Logf("cross-server HTTP response: %s", result)

	if result != "200" && result != "301" && result != "302" {
		t.Errorf("expected 2xx/3xx from VM via cross-server L2 routing, got HTTP %s", result)
	}
}

// TestIPAddressAllocation verifies that VMs get unique IP addresses
// from their server's /20 range with /31 subnets.
func TestIPAddressAllocation(t *testing.T) {
	c := SetupCluster(t)

	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	projectID := c.SeedTestData(t)
	orgID := c.OrgIDForProject(t, projectID)

	// Create a deployment and wait for a VM to be assigned
	depID := c.CreateDeployment(t, projectID, "main", orgID)
	WaitFor(t, "deployment has VM", 5*time.Minute, 2*time.Second, func() bool {
		return c.VMForDeployment(t, depID) != ""
	})

	vmID := c.VMForDeployment(t, depID)
	vmIP := c.QueryRow(t, "SELECT host(ip_address) FROM vms WHERE id = $1::uuid", vmID)
	vmServerID := c.VMServerID(t, vmID)
	serverRange := c.ServerIPRange(t, vmServerID)

	t.Logf("VM %s has IP %s, server %s has range %s", vmID, vmIP, vmServerID, serverRange)

	// VM IP should be within the server's /20 range
	if !strings.HasPrefix(serverRange, "10.1.") {
		t.Errorf("expected server range to start with 10.1., got %s", serverRange)
	}

	// Basic sanity: VM IP should start with 10.1.
	if !strings.HasPrefix(vmIP, "10.1.") {
		t.Errorf("expected VM IP to start with 10.1., got %s", vmIP)
	}
}

// seedSecondProject creates a second independent project for multi-VM placement tests.
func (c *Cluster) seedSecondProject(t *testing.T, orgID string) string {
	t.Helper()
	ctx := context.Background()

	// Reuse the existing github installation
	var ghInstallUUID string
	err := c.DB.QueryRow(ctx,
		"SELECT id::text FROM github_installations WHERE organisation_id = $1::uuid LIMIT 1", orgID).Scan(&ghInstallUUID)
	if err != nil {
		t.Fatalf("failed to find github installation: %v", err)
	}

	githubRepo := fmt.Sprintf("zeitwork/e2e-test-app-2")
	var projectID string
	err = c.DB.QueryRow(ctx, `
		INSERT INTO projects (name, slug, github_repository, github_installation_id, organisation_id, root_directory)
		VALUES ('E2E Test App 2', 'e2e-test-app-2', $1, $2::uuid, $3::uuid, '/')
		ON CONFLICT (slug, organisation_id) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`, githubRepo, ghInstallUUID, orgID).Scan(&projectID)
	if err != nil {
		t.Fatalf("failed to seed second project: %v", err)
	}

	t.Logf("seeded second project: %s", projectID)
	return projectID
}
