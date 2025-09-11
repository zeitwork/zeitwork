//go:build firecracker_integration

package firecracker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// TestSnapshotCleanupRealWorld tests snapshot cleanup with real firecracker-containerd
// This reproduces the exact edge cases we see in production logs
func TestSnapshotCleanupRealWorld(t *testing.T) {
	// Check if we can access firecracker-containerd (skip root check since we'll use sudo)
	if os.Getenv("USER") != "root" && os.Getenv("SUDO_USER") == "" {
		t.Skip("This test needs to be run with sudo to access firecracker-containerd")
	}
	if os.Getenv("RUN_FC_INTEGRATION") == "" {
		t.Skip("set RUN_FC_INTEGRATION=1 to run this integration test")
	}

	// Use real firecracker-containerd configuration
	cfg := &config.FirecrackerRuntimeConfig{
		ContainerdSocket:    getEnvOrDefault("FIRECRACKER_CONTAINERD_SOCKET", "/run/firecracker-containerd/containerd.sock"),
		ContainerdNamespace: getEnvOrDefault("FIRECRACKER_CONTAINERD_NAMESPACE", "zeitwork"),
		RuntimeConfigPath:   getEnvOrDefault("FIRECRACKER_RUNTIME_CONFIG", "/etc/containerd/firecracker-runtime.json"),
		CNIConfDir:          getEnvOrDefault("CNI_CONF_DIR", "/etc/cni/net.d"),
		CNIBinDir:           getEnvOrDefault("CNI_BIN_DIR", "/opt/cni/bin"),
		CNIStateDir:         getEnvOrDefault("CNI_STATE_DIR", "/var/lib/cni/networks"),
		NetworkNamespace:    getEnvOrDefault("NETWORK_NAMESPACE", "zeitwork"),
		NetworkName:         getEnvOrDefault("FIRECRACKER_NETWORK_NAME", "fcnet6"),
		ImageRegistry:       os.Getenv("IMAGE_REGISTRY"),
		StartTimeout:        90 * time.Second,
		StopTimeout:         30 * time.Second,
		PullTimeout:         3 * time.Minute,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// For this test, we don't need the database - focus on snapshot cleanup
	rt, err := NewFirecrackerRuntime(cfg, logger, nil)
	if err != nil {
		t.Skipf("firecracker runtime unavailable: %v", err)
	}

	// Use a simple test image that we know exists
	image := getEnvOrDefault("FC_TEST_IMAGE", "local/hello3000:latest")

	// Ensure image exists in firecracker-containerd namespace
	if err := ensureTestImagePresent(t, cfg, image); err != nil {
		t.Skipf("skipping: unable to ensure image present: %v", err)
	}

	tests := []struct {
		name                    string
		setupFunc               func(t *testing.T, instanceName string) // Function to create the problematic state
		expectedBehavior        string
		shouldEventuallySucceed bool
	}{
		{
			name: "orphaned_snapshot_from_previous_run",
			setupFunc: func(t *testing.T, instanceName string) {
				// Create a container that will leave a snapshot behind
				ctx := context.Background()

				// First, try to run a container normally
				args := []string{"run", "-d", "--snapshotter", "devmapper", "--runtime", "aws.firecracker", image, instanceName}
				if _, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, args); err != nil {
					t.Logf("Initial container creation failed (expected): %v", err)
				}

				// Kill any running task to simulate abrupt termination
				killArgs := []string{"tasks", "kill", instanceName}
				if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, killArgs); err != nil {
					t.Logf("Task kill failed (may not exist): %v: %s", err, out)
				}

				// Delete the container but leave the snapshot
				containerArgs := []string{"containers", "delete", instanceName}
				if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, containerArgs); err != nil {
					t.Logf("Container delete failed (may not exist): %v: %s", err, out)
				}

				// Verify snapshot still exists
				listArgs := []string{"snapshots", "list"}
				if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, listArgs); err == nil {
					if strings.Contains(out, instanceName) {
						t.Logf("Successfully created orphaned snapshot for %s", instanceName)
					} else {
						t.Logf("Snapshot may not exist yet, output: %s", out)
					}
				}
			},
			expectedBehavior:        "Should clean up orphaned snapshot and succeed",
			shouldEventuallySucceed: true,
		},
		{
			name: "running_container_with_snapshot_conflict",
			setupFunc: func(t *testing.T, instanceName string) {
				// Create a running container that will conflict
				ctx := context.Background()

				args := []string{"run", "-d", "--snapshotter", "devmapper", "--runtime", "aws.firecracker", image, instanceName}
				if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, args); err != nil {
					t.Logf("Container creation for conflict test failed: %v: %s", err, out)
				} else {
					t.Logf("Created running container %s for conflict test", instanceName)

					// Wait a bit to ensure it's fully running
					time.Sleep(2 * time.Second)

					// Verify it's running
					listArgs := []string{"tasks", "list"}
					if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, listArgs); err == nil {
						if strings.Contains(out, instanceName) && strings.Contains(strings.ToUpper(out), "RUNNING") {
							t.Logf("Container %s is running and will conflict", instanceName)
						}
					}
				}
			},
			expectedBehavior:        "Should stop existing container, clean up, and succeed",
			shouldEventuallySucceed: true,
		},
		{
			name: "corrupted_snapshot_state",
			setupFunc: func(t *testing.T, instanceName string) {
				// This is harder to simulate, but we can try to create an inconsistent state
				ctx := context.Background()

				// Create and immediately kill to potentially leave inconsistent state
				args := []string{"run", "-d", "--snapshotter", "devmapper", "--runtime", "aws.firecracker", image, instanceName}
				if _, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, args); err == nil {
					// Immediately kill it
					killArgs := []string{"tasks", "kill", instanceName}
					runFCNS(ctx, cfg, cfg.ContainerdNamespace, killArgs)

					// Try to delete container quickly to potentially create race condition
					containerArgs := []string{"containers", "delete", instanceName}
					runFCNS(ctx, cfg, cfg.ContainerdNamespace, containerArgs)

					t.Logf("Created potentially corrupted state for %s", instanceName)
				}
			},
			expectedBehavior:        "Should handle corrupted state and eventually succeed or fail cleanly",
			shouldEventuallySucceed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instanceName := fmt.Sprintf("cleanup-test-%s-%d", tt.name, time.Now().Unix())

			// Clean up any existing state first
			cleanupTestInstance(t, cfg, instanceName)

			// Set up the problematic state
			tt.setupFunc(t, instanceName)

			// Create instance spec
			spec := &types.InstanceSpec{
				ID:            instanceName,
				ImageTag:      image,
				Resources:     &types.ResourceSpec{VCPUs: 1, Memory: 128},
				NetworkConfig: &types.NetworkConfig{DefaultPort: 3000},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			// Create instance - this should trigger our cleanup logic
			inst, err := rt.CreateInstance(ctx, spec)
			if err != nil {
				t.Fatalf("CreateInstance failed: %v", err)
			}
			defer func() {
				cleanupTestInstance(t, cfg, instanceName)
				rt.DeleteInstance(context.Background(), inst)
			}()

			// Try to start the instance - this is where our cleanup logic is tested
			startErr := rt.StartInstance(ctx, inst)

			if tt.shouldEventuallySucceed {
				if startErr != nil {
					t.Errorf("Expected StartInstance to eventually succeed for %s, but got error: %v", tt.name, startErr)

					// Let's debug what went wrong
					debugContainerdState(t, cfg, instanceName)
				} else {
					t.Logf("✅ StartInstance succeeded for %s as expected", tt.name)

					// Verify the instance is actually running
					if inst.NetworkInfo != nil && inst.NetworkInfo.IPAddress != "" {
						t.Logf("Instance has IP address: %s", inst.NetworkInfo.IPAddress)
					}
				}
			} else {
				if startErr == nil {
					t.Errorf("Expected StartInstance to fail for %s, but it succeeded", tt.name)
				} else {
					t.Logf("StartInstance failed as expected: %v", startErr)
				}
			}
		})
	}
}

// debugContainerdState helps debug what state containerd is in when tests fail
func debugContainerdState(t *testing.T, cfg *config.FirecrackerRuntimeConfig, instanceName string) {
	ctx := context.Background()

	t.Logf("=== Debugging containerd state for %s ===", instanceName)

	// List tasks
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, []string{"tasks", "list"}); err == nil {
		t.Logf("Tasks: %s", out)
	} else {
		t.Logf("Failed to list tasks: %v", err)
	}

	// List containers
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, []string{"containers", "list"}); err == nil {
		t.Logf("Containers: %s", out)
	} else {
		t.Logf("Failed to list containers: %v", err)
	}

	// List snapshots
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, []string{"snapshots", "list"}); err == nil {
		t.Logf("Snapshots: %s", out)
	} else {
		t.Logf("Failed to list snapshots: %v", err)
	}

	// List images
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, []string{"images", "list"}); err == nil {
		t.Logf("Images: %s", out)
	} else {
		t.Logf("Failed to list images: %v", err)
	}
}

// cleanupTestInstance ensures clean state before and after tests
func cleanupTestInstance(t *testing.T, cfg *config.FirecrackerRuntimeConfig, instanceName string) {
	ctx := context.Background()

	// Kill task
	killArgs := []string{"tasks", "kill", instanceName}
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, killArgs); err != nil {
		t.Logf("Task kill during cleanup: %v: %s", err, out)
	}

	// Delete container
	containerArgs := []string{"containers", "delete", instanceName}
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, containerArgs); err != nil {
		t.Logf("Container delete during cleanup: %v: %s", err, out)
	}

	// Remove snapshot
	snapshotArgs := []string{"snapshots", "remove", instanceName}
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, snapshotArgs); err != nil {
		t.Logf("Snapshot remove during cleanup: %v: %s", err, out)
	}

	// Also try with devmapper snapshotter explicitly
	devmapperArgs := []string{"snapshots", "--snapshotter", "devmapper", "remove", instanceName}
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, devmapperArgs); err != nil {
		t.Logf("Devmapper snapshot remove during cleanup: %v: %s", err, out)
	}
}

// Helper functions for the integration test
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ensureTestImagePresent is simplified - we'll assume the image exists or can be pulled
func ensureTestImagePresent(t *testing.T, cfg *config.FirecrackerRuntimeConfig, image string) error {
	ctx := context.Background()

	// Try to pull the image
	pullArgs := []string{"images", "pull", "--snapshotter", "devmapper", image}
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, pullArgs); err != nil {
		t.Logf("Image pull failed (may already exist): %v: %s", err, out)
	}

	// Check if image exists
	listArgs := []string{"images", "list", "-q"}
	if out, err := runFCNS(ctx, cfg, cfg.ContainerdNamespace, listArgs); err == nil {
		if strings.Contains(out, image) {
			return nil
		}
	}

	return fmt.Errorf("image %s not available", image)
}
