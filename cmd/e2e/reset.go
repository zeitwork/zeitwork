package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
)

// resetCluster wipes DB, kills VMs, clears MinIO, removes server-id, and restarts.
// DB and MinIO are local (Docker Compose), so reset is fast.
// Remote servers just need VM cleanup and zeitwork restart.
func resetCluster(cluster *ClusterState) error {
	// Step 1: Stop zeitwork on all servers
	for _, s := range cluster.Servers {
		slog.Info("stopping zeitwork", "hostname", s.Hostname)
		sshRun(cluster.SSHKeyPath, s.PublicIP, "systemctl stop zeitwork 2>/dev/null || true")   //nolint:errcheck
		sshRun(cluster.SSHKeyPath, s.PublicIP, "pkill -9 cloud-hypervisor 2>/dev/null || true") //nolint:errcheck
		sshRun(cluster.SSHKeyPath, s.PublicIP, "rm -rf /data/work/*")                           //nolint:errcheck
		sshRun(cluster.SSHKeyPath, s.PublicIP, "rm -f /data/server-id")                         //nolint:errcheck
	}

	// Step 2: Reset local DB (drop + recreate via Docker Compose)
	slog.Info("resetting local database")
	resetDB := exec.Command("docker", "compose", "-f", composeFile,
		"exec", "-T", "db", "psql", "-U", "zeitwork", "-c",
		"DROP SCHEMA public CASCADE; CREATE SCHEMA public;", "zeitwork")
	resetDB.Stdout = os.Stdout
	resetDB.Stderr = os.Stderr
	if err := resetDB.Run(); err != nil {
		return fmt.Errorf("failed to reset database: %w", err)
	}

	// Step 3: Re-run migrations
	if err := runMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations after reset: %w", err)
	}

	// Step 4: Clear MinIO bucket
	slog.Info("clearing MinIO bucket")
	clearMinIO := exec.Command("docker", "compose", "-f", composeFile, "run", "--rm",
		"--entrypoint", "sh", "minio-init", "-c",
		"mc alias set local http://s3:9000 ${MINIO_ACCESS_KEY} ${MINIO_SECRET_KEY} && "+
			"mc rm --recursive --force local/zeitwork-images/ 2>/dev/null; "+
			"mc mb --ignore-existing local/zeitwork-images")
	clearMinIO.Stdout = os.Stdout
	clearMinIO.Stderr = os.Stderr
	if err := clearMinIO.Run(); err != nil {
		slog.Warn("failed to clear MinIO bucket (may be empty)", "err", err)
	}

	// Step 5: Restart zeitwork on all servers
	for _, s := range cluster.Servers {
		slog.Info("starting zeitwork", "hostname", s.Hostname)
		sshRun(cluster.SSHKeyPath, s.PublicIP, "systemctl start zeitwork") //nolint:errcheck
	}

	slog.Info("cluster reset complete")
	return nil
}
