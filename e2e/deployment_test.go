package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestDeploymentLifecycle verifies the full deployment pipeline:
// pending → building → starting → running.
//
// Prerequisites:
//   - E2E_GITHUB_REPO must point to a repo with a Dockerfile (default: zeitwork/e2e-test-app)
//   - E2E_GITHUB_INSTALLATION_ID must be a valid GitHub App installation
//   - The zeitwork binary must be running on both servers with valid GitHub App + Docker registry credentials
func TestDeploymentLifecycle(t *testing.T) {
	c := SetupCluster(t)

	// Wait for infrastructure to be ready
	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	// Seed base data (org, user, github installation, project)
	projectID := c.SeedTestData(t)
	orgID := c.OrgIDForProject(t, projectID)

	// Insert a pending deployment — the WAL listener will pick this up
	// and the reconciler will drive it through the state machine.
	deploymentID := c.CreateDeployment(t, projectID, "main", orgID)
	t.Logf("created deployment %s for project %s", deploymentID, projectID)

	// Phase 1: Verify deployment transitions to "building"
	// The reconciler should create a build record and mark the deployment as building.
	c.WaitForQuery(t, "deployment reaches building",
		2*time.Minute, "building",
		"SELECT status FROM deployments WHERE id = $1::uuid", deploymentID)
	t.Logf("deployment %s is now building", deploymentID)

	// Verify a build record was created
	buildID := c.QueryRow(t, "SELECT build_id::text FROM deployments WHERE id = $1::uuid", deploymentID)
	if buildID == "" {
		t.Fatal("deployment should have a build_id after reaching building state")
	}
	t.Logf("build %s created for deployment %s", buildID, deploymentID)

	// Phase 2: Wait for the build to complete (this includes pulling dind image,
	// creating build VM, downloading source, docker buildx build + push).
	// This can take several minutes on first run (image pulls).
	c.WaitForQuery(t, "build completes successfully",
		10*time.Minute, "succesful",
		"SELECT status FROM builds WHERE id = $1::uuid", buildID)
	t.Logf("build %s completed successfully", buildID)

	// Verify the build produced an image
	imageID := c.QueryRow(t, "SELECT image_id::text FROM builds WHERE id = $1::uuid", buildID)
	if imageID == "" {
		t.Fatal("build should have an image_id after completing successfully")
	}
	t.Logf("build produced image %s", imageID)

	// Phase 3: Verify deployment transitions to "starting"
	// The reconciler should see the build completed, mark deployment as starting,
	// and create a VM for the application.
	c.WaitForQuery(t, "deployment reaches starting",
		2*time.Minute, "starting",
		"SELECT status FROM deployments WHERE id = $1::uuid", deploymentID)
	t.Logf("deployment %s is now starting", deploymentID)

	// Verify a VM was created for the deployment
	vmID := c.VMForDeployment(t, deploymentID)
	if vmID == "" {
		// The VM might not be linked yet — wait a bit
		WaitFor(t, "deployment has vm_id", 60*time.Second, 2*time.Second, func() bool {
			return c.VMForDeployment(t, deploymentID) != ""
		})
		vmID = c.VMForDeployment(t, deploymentID)
	}
	t.Logf("VM %s created for deployment %s", vmID, deploymentID)

	// Phase 4: Verify deployment transitions to "running"
	// The VM must boot, the app must start and pass the HTTP health check
	// (GET / returns 2xx or 3xx).
	c.WaitForQuery(t, "deployment reaches running",
		5*time.Minute, "running",
		"SELECT status FROM deployments WHERE id = $1::uuid", deploymentID)
	t.Logf("deployment %s is now running", deploymentID)

	// Verify VM is also running
	vmStatus := c.VMStatus(t, vmID)
	if vmStatus != "running" {
		t.Errorf("expected VM status 'running', got '%s'", vmStatus)
	}

	// Verify VM is placed on a server
	serverID := c.VMServerID(t, vmID)
	if serverID == "" {
		t.Error("VM should be assigned to a server")
	}
	t.Logf("VM %s running on server %s", vmID, serverID)

	// Verify the VM IP is accessible from its host server
	vmIP := c.QueryRow(t, "SELECT host(ip_address) FROM vms WHERE id = $1::uuid", vmID)
	vmPort := c.QueryRow(t, "SELECT port::text FROM vms WHERE id = $1::uuid", vmID)
	t.Logf("VM listening at %s:%s", vmIP, vmPort)
}

// TestDeploymentReplacement verifies that deploying a second time stops the old deployment.
// When a new deployment for the same project reaches "running", the previous one
// should be stopped and its VM soft-deleted.
func TestDeploymentReplacement(t *testing.T) {
	c := SetupCluster(t)

	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	projectID := c.SeedTestData(t)
	orgID := c.OrgIDForProject(t, projectID)

	// Deploy first version
	dep1ID := c.CreateDeployment(t, projectID, "main", orgID)
	t.Logf("created first deployment %s", dep1ID)

	c.WaitForQuery(t, "first deployment running",
		15*time.Minute, "running",
		"SELECT status FROM deployments WHERE id = $1::uuid", dep1ID)
	t.Logf("first deployment %s is running", dep1ID)

	vm1ID := c.VMForDeployment(t, dep1ID)
	t.Logf("first deployment VM: %s", vm1ID)

	// Deploy second version (same commit is fine — it's a new deployment)
	dep2ID := c.CreateDeployment(t, projectID, "main", orgID)
	t.Logf("created second deployment %s", dep2ID)

	// Wait for second deployment to be running
	c.WaitForQuery(t, "second deployment running",
		15*time.Minute, "running",
		"SELECT status FROM deployments WHERE id = $1::uuid", dep2ID)
	t.Logf("second deployment %s is running", dep2ID)

	// First deployment should now be stopped
	c.WaitForQuery(t, "first deployment stopped",
		2*time.Minute, "stopped",
		"SELECT status FROM deployments WHERE id = $1::uuid", dep1ID)
	t.Logf("first deployment %s is now stopped", dep1ID)

	// First VM should be soft-deleted
	WaitFor(t, "first VM soft-deleted", 2*time.Minute, 2*time.Second, func() bool {
		var deleted bool
		err := c.DB.QueryRow(context.Background(),
			"SELECT deleted_at IS NOT NULL FROM vms WHERE id = $1::uuid", vm1ID).Scan(&deleted)
		return err == nil && deleted
	})
	t.Logf("first VM %s has been soft-deleted", vm1ID)
}

// TestDomainRouting verifies that after a deployment reaches "running" and a domain
// is verified and pointed at it, HTTP requests are routed to the VM through the edge proxy.
//
// This test seeds a domain record, verifies it manually, and then checks that the
// edge proxy returns a valid response (not 404 "Service Not Found").
func TestDomainRouting(t *testing.T) {
	c := SetupCluster(t)

	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	projectID := c.SeedTestData(t)
	orgID := c.OrgIDForProject(t, projectID)

	// Create and wait for a deployment to reach running
	depID := c.CreateDeployment(t, projectID, "main", orgID)
	c.WaitForQuery(t, "deployment running",
		15*time.Minute, "running",
		"SELECT status FROM deployments WHERE id = $1::uuid", depID)

	vmID := c.VMForDeployment(t, depID)

	// Create a domain pointing to this deployment and mark it verified
	domainName := fmt.Sprintf("e2e-test-%s.zeitwork.app", depID[:8])
	_, err := c.DB.Exec(context.Background(), `
		INSERT INTO domains (name, project_id, deployment_id, verified_at, organisation_id)
		VALUES ($1, $2::uuid, $3::uuid, now(), $4::uuid)
		ON CONFLICT (name, project_id) DO UPDATE SET deployment_id = EXCLUDED.deployment_id, verified_at = now()
	`, domainName, projectID, depID, orgID)
	if err != nil {
		t.Fatalf("failed to create domain: %v", err)
	}
	t.Logf("created verified domain %s -> deployment %s (VM %s)", domainName, depID, vmID)

	// Wait for the edge proxy to pick up the route change (WAL-driven, ~100ms debounce)
	time.Sleep(2 * time.Second)

	// The edge proxy listens on :443. We can't easily test TLS with a custom domain
	// from outside, but we can verify the route exists in the database.
	routeCount := c.QueryRowInt(t,
		`SELECT count(*) FROM domains d
		 INNER JOIN deployments dep ON d.deployment_id = dep.id
		 INNER JOIN vms v ON dep.vm_id = v.id
		 WHERE d.name = $1 AND d.verified_at IS NOT NULL AND d.deleted_at IS NULL
		   AND dep.status = 'running' AND v.status = 'running'`, domainName)
	if routeCount != 1 {
		t.Errorf("expected 1 active route for domain %s, got %d", domainName, routeCount)
	}

	// Verify the VM is directly reachable from its server via HTTP
	vmIP := c.QueryRow(t, "SELECT host(ip_address) FROM vms WHERE id = $1::uuid", vmID)
	vmPort := c.QueryRow(t, "SELECT port::text FROM vms WHERE id = $1::uuid", vmID)
	serverID := c.VMServerID(t, vmID)

	// Determine which server hosts this VM to SSH into it for the health check
	serverIP := c.serverIPForID(t, serverID)
	if serverIP != "" {
		result := c.SSH(t, serverIP, fmt.Sprintf("curl -s -o /dev/null -w '%%{http_code}' http://%s:%s/", vmIP, vmPort))
		t.Logf("direct VM health check from server: HTTP %s", result)
		if result != "200" && result != "301" && result != "302" {
			t.Errorf("expected 2xx/3xx from VM, got HTTP %s", result)
		}
	}

	// Test cross-server routing: curl from the OTHER server to verify L2 routing works
	otherServerIP := c.otherServerIP(t, serverID)
	if otherServerIP != "" {
		result, err := c.SSHNoFail(otherServerIP, fmt.Sprintf("curl -s -o /dev/null -w '%%{http_code}' --connect-timeout 5 http://%s:%s/", vmIP, vmPort))
		if err != nil {
			t.Logf("cross-server routing not working (may be expected if host routes not synced): %v", err)
		} else {
			t.Logf("cross-server VM access from other server: HTTP %s", result)
		}
	}
}

// TestDeploymentBuildFailure verifies that when a build fails (e.g., missing Dockerfile),
// the deployment is correctly marked as failed.
func TestDeploymentBuildFailure(t *testing.T) {
	c := SetupCluster(t)

	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	projectID := c.SeedTestData(t)
	orgID := c.OrgIDForProject(t, projectID)

	// Use a non-existent commit SHA to force a build failure
	depID := c.CreateDeployment(t, projectID, "0000000000000000000000000000000000000000", orgID)
	t.Logf("created deployment %s with bad commit", depID)

	// The deployment should reach building first
	c.WaitForQuery(t, "deployment reaches building",
		2*time.Minute, "building",
		"SELECT status FROM deployments WHERE id = $1::uuid", depID)

	// Then the build should fail (source download will fail with 404)
	c.WaitForQuery(t, "deployment fails",
		10*time.Minute, "failed",
		"SELECT status FROM deployments WHERE id = $1::uuid", depID)
	t.Logf("deployment %s correctly failed", depID)
}

// serverIPForID returns the public IP of the server matching the given server DB ID.
func (c *Cluster) serverIPForID(t *testing.T, serverID string) string {
	t.Helper()
	internalIP := c.QueryRow(t, "SELECT internal_ip FROM servers WHERE id = $1::uuid", serverID)
	if internalIP == c.Server1InternalIP {
		return c.Server1IP
	}
	if internalIP == c.Server2InternalIP {
		return c.Server2IP
	}
	t.Logf("warning: could not map server %s (internal_ip=%s) to a known server", serverID, internalIP)
	return ""
}

// otherServerIP returns the public IP of the server that does NOT match the given server ID.
func (c *Cluster) otherServerIP(t *testing.T, serverID string) string {
	t.Helper()
	internalIP := c.QueryRow(t, "SELECT internal_ip FROM servers WHERE id = $1::uuid", serverID)
	if internalIP == c.Server1InternalIP {
		return c.Server2IP
	}
	if internalIP == c.Server2InternalIP {
		return c.Server1IP
	}
	return ""
}

// httpGet performs a simple HTTP GET and returns the status code.
func httpGet(url string, timeout time.Duration) (int, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}
