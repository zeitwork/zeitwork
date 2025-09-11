//go:build firecracker_integration

package firecracker

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
	nt "github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// Integration test: requires firecracker-containerd running, CNI configured, and image available or registry reachable.
// Enable by running: `go test ./... -tags firecracker_integration` with appropriate env vars.
func TestFirecrackerRuntime_CreateConnectCleanup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root privileges (for /dev/kvm, CNI state access)")
	}
	if os.Getenv("RUN_FC_INTEGRATION") == "" {
		t.Skip("set RUN_FC_INTEGRATION=1 to run this integration test")
	}

	cfg := &config.FirecrackerRuntimeConfig{
		ContainerdSocket:    getenv("FIRECRACKER_CONTAINERD_SOCKET", "/run/firecracker-containerd/containerd.sock"),
		ContainerdNamespace: getenv("FIRECRACKER_CONTAINERD_NAMESPACE", "zeitwork"),
		RuntimeConfigPath:   getenv("FIRECRACKER_RUNTIME_CONFIG", "/etc/containerd/firecracker-runtime.json"),
		CNIConfDir:          getenv("CNI_CONF_DIR", "/etc/cni/net.d"),
		CNIBinDir:           getenv("CNI_BIN_DIR", "/opt/cni/bin"),
		CNIStateDir:         getenv("CNI_STATE_DIR", "/var/lib/cni/networks"),
		NetworkNamespace:    getenv("NETWORK_NAMESPACE", "zeitwork"),
		NetworkName:         getenv("FIRECRACKER_NETWORK_NAME", "fcnet6"),
		ImageRegistry:       os.Getenv("IMAGE_REGISTRY"),
		StartTimeout:        90 * time.Second,
		StopTimeout:         30 * time.Second,
		PullTimeout:         3 * time.Minute,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	rt, err := NewFirecrackerRuntime(cfg, logger)
	if err != nil {
		t.Skipf("firecracker runtime unavailable: %v", err)
	}

	// Use simple HTTP image that listens on IPv6 :3000 (matches experiments)
	image := getenv("FC_TEST_IMAGE", "local/hello3000:latest")

	// Ensure image exists in firecracker-containerd namespace (build+import if necessary)
	if err := ensureImagePresent(t, cfg, image); err != nil {
		t.Skipf("skipping: unable to ensure image present: %v", err)
	}

	spec := &nt.InstanceSpec{
		ID:            fmt.Sprintf("itest-%d", time.Now().UnixNano()),
		ImageTag:      image,
		Resources:     &nt.ResourceSpec{VCPUs: 1, Memory: 128},
		NetworkConfig: &nt.NetworkConfig{DefaultPort: 3000},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	inst, err := rt.CreateInstance(ctx, spec)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer func() {
		_ = rt.DeleteInstance(context.Background(), inst)
	}()

	if err := rt.StartInstance(ctx, inst); err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	if inst.NetworkInfo == nil || inst.NetworkInfo.IPAddress == "" {
		t.Fatalf("expected IPv6 address on instance.NetworkInfo")
	}

	// Probe HTTP on IPv6:3000 until success
	if err := waitHTTPv6(fmt.Sprintf("http://[%s]:%d", inst.NetworkInfo.IPAddress, 3000), 2*time.Minute); err != nil {
		t.Fatalf("HTTP probe failed: %v", err)
	}

	// Stop and delete
	if err := rt.StopInstance(ctx, inst); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if err := rt.DeleteInstance(ctx, inst); err != nil {
		t.Fatalf("DeleteInstance: %v", err)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func waitHTTPv6(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}

// ensureImagePresent checks if the image exists in the containerd namespace, otherwise builds a tiny BusyBox HTTP image
// that listens on IPv6 and imports it. Requires Docker and firecracker-ctr.
func ensureImagePresent(t *testing.T, cfg *config.FirecrackerRuntimeConfig, image string) error {
	t.Helper()
	// Check via firecracker-ctr
	listCmd := exec.Command("/usr/local/bin/firecracker-ctr", "--address", cfg.ContainerdSocket, "-n", cfg.ContainerdNamespace, "images", "list", "-q")
	listCmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := listCmd.CombinedOutput()
	if err == nil {
		for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.TrimSpace(ln) == image {
				return nil
			}
		}
	}

	// Build minimal image with Docker and import
	tmpDir, err := os.MkdirTemp("", "fcimg-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	dockerfile := []byte("FROM busybox:latest\nRUN mkdir -p /www && echo 'hello world' > /www/index.html\nEXPOSE 3000\nCMD [\"busybox\", \"httpd\", \"-f\", \"-p\", \"[::]:3000\", \"-h\", \"/www\"]\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), dockerfile, 0644); err != nil {
		return err
	}
	// docker build
	if err := execCmd("docker", "build", "-t", image, tmpDir); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	// docker save
	tarPath := filepath.Join(tmpDir, "image.tar")
	if err := execCmd("docker", "save", image, "-o", tarPath); err != nil {
		return fmt.Errorf("docker save failed: %w", err)
	}
	// firecracker-ctr images import
	if err := execCmd("/usr/local/bin/firecracker-ctr", "--address", cfg.ContainerdSocket, "-n", cfg.ContainerdNamespace, "images", "import", tarPath); err != nil {
		return fmt.Errorf("images import failed: %w", err)
	}
	return nil
}

func execCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
