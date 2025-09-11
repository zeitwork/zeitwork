//go:build firecracker_integration

package firecracker

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
)

// TestDebugSnapshotConflict reproduces the exact snapshot conflict we see in production
func TestDebugSnapshotConflict(t *testing.T) {
	if os.Getenv("USER") != "root" && os.Getenv("SUDO_USER") == "" {
		t.Skip("This test needs to be run with sudo to access firecracker-containerd")
	}
	if os.Getenv("RUN_FC_INTEGRATION") == "" {
		t.Skip("set RUN_FC_INTEGRATION=1 to run this integration test")
	}

	cfg := &config.FirecrackerRuntimeConfig{
		ContainerdSocket:    "/run/firecracker-containerd/containerd.sock",
		ContainerdNamespace: "zeitwork",
		CNIStateDir:         "/var/lib/cni/networks",
		NetworkName:         "fcnet6",
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx := context.Background()

	// Use the exact same instance name pattern from production logs
	instanceName := "fc-019933e6-710c-726d-9a78-97b58ddadde3"
	image := "localhost:5001/nuxt-demo:29e4391"

	t.Logf("=== Testing with exact production scenario ===")
	t.Logf("Instance: %s", instanceName)
	t.Logf("Image: %s", image)

	// Step 1: Clean up any existing state
	t.Logf("\n=== Step 1: Initial cleanup ===")
	cleanupEverything(t, cfg, instanceName)

	// Step 2: Try to create the container (this should trigger the snapshot conflict)
	t.Logf("\n=== Step 2: First container creation attempt ===")
	args := []string{"run", "-d", "--snapshotter", "devmapper", "--runtime", "aws.firecracker", "--cap-add", "CAP_NET_ADMIN", "--net-host", image, instanceName}

	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, args); err != nil {
		t.Logf("First creation failed (expected): %v", err)
		t.Logf("Output: %s", out)

		// Check if it's the snapshot conflict
		if strings.Contains(out, "already exists") && strings.Contains(out, "snapshot") {
			t.Logf("✅ Reproduced the snapshot conflict!")

			// Step 3: Debug what state containerd is actually in
			t.Logf("\n=== Step 3: Debug containerd state ===")
			debugDetailedContainerdState(t, cfg, instanceName)

			// Step 4: Test our cleanup function
			t.Logf("\n=== Step 4: Testing our cleanup function ===")
			runtime := &Runtime{cfg: cfg, logger: logger, queries: nil}

			cleanupErr := runtime.cleanupContainerAndSnapshot(ctx, instanceName)
			if cleanupErr != nil {
				t.Logf("Cleanup failed: %v", cleanupErr)
			} else {
				t.Logf("✅ Cleanup succeeded")
			}

			// Step 5: Debug state after cleanup
			t.Logf("\n=== Step 5: Debug state after cleanup ===")
			debugDetailedContainerdState(t, cfg, instanceName)

			// Step 6: Try creation again
			t.Logf("\n=== Step 6: Retry container creation ===")
			if out2, err2 := runFCNS(ctx, cfg, cfg.ContainerdNamespace, args); err2 != nil {
				t.Logf("Retry failed: %v", err2)
				t.Logf("Retry output: %s", out2)

				if strings.Contains(out2, "already exists") && strings.Contains(out2, "snapshot") {
					t.Logf("❌ Still getting snapshot conflict after cleanup")

					// Step 7: Let's try the nuclear option - restart firecracker-containerd
					t.Logf("\n=== Step 7: Nuclear option analysis ===")
					analyzeSnapshotState(t, cfg, instanceName)
				} else {
					t.Logf("✅ Different error after cleanup - progress!")
				}
			} else {
				t.Logf("✅ Retry succeeded!")
			}
		} else {
			t.Logf("❌ Did not reproduce snapshot conflict, got different error")
		}
	} else {
		t.Logf("❌ First creation succeeded - no conflict to debug")
	}

	// Final cleanup
	t.Logf("\n=== Final cleanup ===")
	cleanupEverything(t, cfg, instanceName)
}

func cleanupEverything(t *testing.T, cfg *config.FirecrackerRuntimeConfig, instanceName string) {
	ctx := context.Background()

	// Kill task
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, []string{"tasks", "kill", instanceName}); err != nil {
		t.Logf("Task kill: %v: %s", err, out)
	}

	// Delete container
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, []string{"containers", "delete", instanceName}); err != nil {
		t.Logf("Container delete: %v: %s", err, out)
	}

	// Remove snapshot
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, []string{"snapshots", "remove", instanceName}); err != nil {
		t.Logf("Snapshot remove: %v: %s", err, out)
	}

	// Remove snapshot with devmapper
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, []string{"snapshots", "--snapshotter", "devmapper", "remove", instanceName}); err != nil {
		t.Logf("Devmapper snapshot remove: %v: %s", err, out)
	}
}

func debugDetailedContainerdState(t *testing.T, cfg *config.FirecrackerRuntimeConfig, instanceName string) {
	ctx := context.Background()

	// List everything related to our instance
	commands := []struct {
		name string
		args []string
	}{
		{"tasks", []string{"tasks", "list"}},
		{"containers", []string{"containers", "list"}},
		{"snapshots", []string{"snapshots", "list"}},
		{"devmapper-snapshots", []string{"snapshots", "--snapshotter", "devmapper", "list"}},
		{"images", []string{"images", "list"}},
	}

	for _, cmd := range commands {
		if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, cmd.args); err != nil {
			t.Logf("%s list failed: %v", cmd.name, err)
		} else {
			t.Logf("%s list:", cmd.name)
			lines := strings.Split(strings.TrimSpace(out), "\n")
			for _, line := range lines {
				if strings.Contains(line, instanceName) || strings.Contains(line, "REF") || strings.Contains(line, "KEY") {
					t.Logf("  %s", line)
				}
			}
		}
	}
}

func analyzeSnapshotState(t *testing.T, cfg *config.FirecrackerRuntimeConfig, instanceName string) {
	ctx := context.Background()

	t.Logf("=== Analyzing why snapshot cleanup isn't working ===")

	// Check if there are any active mount points
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, []string{"snapshots", "info", instanceName}); err != nil {
		t.Logf("Snapshot info: %v: %s", err, out)
	} else {
		t.Logf("Snapshot info: %s", out)
	}

	// Try to use the snapshotter directly to understand the issue
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, []string{"snapshots", "--snapshotter", "devmapper", "info", instanceName}); err != nil {
		t.Logf("Devmapper snapshot info: %v: %s", err, out)
	} else {
		t.Logf("Devmapper snapshot info: %s", out)
	}

	// Check if there are any references preventing cleanup
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, []string{"snapshots", "usage", instanceName}); err != nil {
		t.Logf("Snapshot usage: %v: %s", err, out)
	} else {
		t.Logf("Snapshot usage: %s", out)
	}
}
