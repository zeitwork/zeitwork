package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
)

var (
	flagTest       string
	flagDeployOnly bool
	flagNoReset    bool
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Build, deploy, and run E2E tests",
	Long: `Cross-compiles the zeitwork binary, uploads to servers, optionally resets state,
and runs the E2E test suite locally.

  --deploy-only   Only compile + upload + restart. Skip reset and tests.
  --no-reset      Skip state reset. Run tests against existing state.
  --test          Run only tests matching this pattern (passed to -run).`,
	RunE: runRun,
}

func init() {
	runCmd.Flags().StringVar(&flagTest, "test", "", "Run only tests matching this pattern")
	runCmd.Flags().BoolVar(&flagDeployOnly, "deploy-only", false, "Only deploy binary, skip reset and tests")
	runCmd.Flags().BoolVar(&flagNoReset, "no-reset", false, "Skip state reset")
}

func runRun(cmd *cobra.Command, args []string) error {
	cluster, err := LoadCluster()
	if err != nil {
		return err
	}

	start := time.Now()

	// Step 1: Cross-compile
	slog.Info("cross-compiling zeitwork binary")
	buildPath := ".e2e/zeitwork"
	buildCmd := exec.Command("go", "build", "-o", buildPath, "./cmd/zeitwork/")
	buildCmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH=amd64",
	)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("cross-compilation failed: %w", err)
	}
	slog.Info("binary compiled", "duration", time.Since(start).Round(time.Millisecond))

	// Step 2: SCP binary to all servers + restart
	slog.Info("uploading binary to servers")
	for _, s := range cluster.Servers {
		sshRun(cluster.SSHKeyPath, s.PublicIP, "systemctl stop zeitwork 2>/dev/null || true") //nolint:errcheck

		if err := scpUpload(cluster.SSHKeyPath, buildPath, s.PublicIP, "/data/zeitwork"); err != nil {
			return fmt.Errorf("failed to upload binary to %s: %w", s.Hostname, err)
		}
		sshRun(cluster.SSHKeyPath, s.PublicIP, "chmod +x /data/zeitwork") //nolint:errcheck
		slog.Info("binary uploaded", "hostname", s.Hostname)
	}

	if flagDeployOnly {
		for _, s := range cluster.Servers {
			sshRun(cluster.SSHKeyPath, s.PublicIP, "systemctl start zeitwork") //nolint:errcheck
		}
		slog.Info("deploy complete (--deploy-only)", "duration", time.Since(start).Round(time.Millisecond))
		return nil
	}

	// Step 3: Reset state
	if !flagNoReset {
		slog.Info("resetting cluster state")
		if err := resetCluster(cluster); err != nil {
			return fmt.Errorf("state reset failed: %w", err)
		}
	} else {
		for _, s := range cluster.Servers {
			sshRun(cluster.SSHKeyPath, s.PublicIP, "systemctl start zeitwork") //nolint:errcheck
		}
	}

	// Step 4: Ensure tunnels are up
	if !areTunnelsRunning() {
		slog.Info("restarting tunnels")
		if err := startTunnels(cluster); err != nil {
			return fmt.Errorf("failed to start tunnels: %w", err)
		}
	}

	// Step 5: Wait for servers to register
	slog.Info("waiting for servers to register")
	time.Sleep(5 * time.Second)

	// Step 6: Run tests locally
	slog.Info("running E2E tests")
	testArgs := []string{"test", "./e2e/", "-v", "-timeout", "30m", "-count=1"}
	if flagTest != "" {
		testArgs = append(testArgs, "-run", flagTest)
	}
	testCmd := exec.Command("go", testArgs...)
	testCmd.Env = append(os.Environ(),
		fmt.Sprintf("E2E_SERVER1_IP=%s", cluster.Servers[0].PublicIP),
		fmt.Sprintf("E2E_SERVER2_IP=%s", cluster.Servers[1].PublicIP),
		fmt.Sprintf("E2E_SERVER1_INTERNAL_IP=%s", cluster.Servers[0].InternalIP),
		fmt.Sprintf("E2E_SERVER2_INTERNAL_IP=%s", cluster.Servers[1].InternalIP),
		fmt.Sprintf("E2E_SSH_KEY=%s", cluster.SSHKeyPath),
		"E2E_DATABASE_URL=postgresql://zeitwork:zeitwork@127.0.0.1:15432/zeitwork?sslmode=disable",
	)
	testCmd.Stdout = os.Stdout
	testCmd.Stderr = os.Stderr
	if err := testCmd.Run(); err != nil {
		return fmt.Errorf("tests failed: %w", err)
	}

	slog.Info("E2E tests passed", "duration", time.Since(start).Round(time.Millisecond))
	return nil
}
