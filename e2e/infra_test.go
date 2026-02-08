package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestServerRegistration(t *testing.T) {
	c := SetupCluster(t)

	// Both servers should register themselves in the servers table
	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	count := c.ServerCount(t)
	if count != 2 {
		t.Fatalf("expected 2 active servers, got %d", count)
	}
	t.Logf("both servers registered successfully")
}

func TestIPRangeAllocation(t *testing.T) {
	c := SetupCluster(t)

	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	ids := c.ServerIDs(t)
	if len(ids) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(ids))
	}

	range1 := c.ServerIPRange(t, ids[0])
	range2 := c.ServerIPRange(t, ids[1])

	t.Logf("server 1 IP range: %s", range1)
	t.Logf("server 2 IP range: %s", range2)

	// Both should be /20 ranges
	if !strings.HasSuffix(range1, "/20") {
		t.Errorf("server 1 IP range should be /20, got: %s", range1)
	}
	if !strings.HasSuffix(range2, "/20") {
		t.Errorf("server 2 IP range should be /20, got: %s", range2)
	}

	// Ranges should be different
	if range1 == range2 {
		t.Errorf("servers should have different IP ranges, both got: %s", range1)
	}
}

func TestServerHeartbeat(t *testing.T) {
	c := SetupCluster(t)

	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	ids := c.ServerIDs(t)

	// Wait a bit for heartbeats to propagate
	time.Sleep(15 * time.Second)

	for _, id := range ids {
		age := c.QueryRow(t, "SELECT extract(epoch from (now() - last_heartbeat_at))::int::text FROM servers WHERE id = $1", id)
		t.Logf("server %s heartbeat age: %s seconds", id, age)

		// Heartbeat should be within the last 15 seconds
		if age > "15" {
			t.Errorf("server %s heartbeat is stale: %s seconds old", id, age)
		}
	}
}

func TestHostRoutes(t *testing.T) {
	c := SetupCluster(t)

	WaitFor(t, "both servers registered", 30*time.Second, 2*time.Second, func() bool {
		return c.ServerCount(t) >= 2
	})

	// Give host route sync time to run
	time.Sleep(35 * time.Second)

	// Server 1 should have a route to server 2's /20 range via server 2's internal IP
	routes := c.SSH(t, c.Server1IP, "ip route show")
	t.Logf("server 1 routes:\n%s", routes)

	if !strings.Contains(routes, c.Server2InternalIP) {
		t.Errorf("server 1 should have a route via server 2 internal IP %s", c.Server2InternalIP)
	}

	// Server 2 should have a route to server 1's /20 range via server 1's internal IP
	routes = c.SSH(t, c.Server2IP, "ip route show")
	t.Logf("server 2 routes:\n%s", routes)

	if !strings.Contains(routes, c.Server1InternalIP) {
		t.Errorf("server 2 should have a route via server 1 internal IP %s", c.Server1InternalIP)
	}
}
