package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runAnsible generates a dynamic inventory, writes extra vars, and runs the Ansible playbook.
// All servers get the same node role — services are accessed via reverse SSH tunnels.
func runAnsible(cluster *ClusterState) error {
	// Detect the primary NIC name on each server (needed for nftables template)
	nics := make(map[string]string)
	for _, s := range cluster.Servers {
		nic, err := sshRun(cluster.SSHKeyPath, s.PublicIP, `ip -o link show | awk -F': ' '/state UP/ && !/lo|vlan|docker|br-/{print $2; exit}'`)
		if err != nil {
			return fmt.Errorf("failed to detect NIC on %s: %w", s.Hostname, err)
		}
		nics[s.Hostname] = strings.TrimSpace(nic)
		slog.Info("detected primary NIC", "hostname", s.Hostname, "nic", nics[s.Hostname])
	}

	// Generate dynamic inventory
	inventory, err := generateInventory(cluster, nics)
	if err != nil {
		return err
	}
	inventoryPath := filepath.Join(e2eDir, "inventory.ini")
	if err := os.WriteFile(inventoryPath, []byte(inventory), 0o644); err != nil {
		return fmt.Errorf("failed to write inventory: %w", err)
	}

	// Generate extra vars file with secrets from environment
	extraVarsPath := filepath.Join(e2eDir, "e2e-vars.yml")
	if err := generateExtraVars(extraVarsPath); err != nil {
		return err
	}

	// Run ansible-playbook
	absKeyPath, _ := filepath.Abs(cluster.SSHKeyPath)
	absInventory, _ := filepath.Abs(inventoryPath)
	absExtraVars, _ := filepath.Abs(extraVarsPath)

	cmd := exec.Command("ansible-playbook",
		"-i", absInventory,
		"e2e-site.yml",
		"--private-key", absKeyPath,
		"--extra-vars", "@"+absExtraVars,
	)
	cmd.Dir = "ansible"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "ANSIBLE_HOST_KEY_CHECKING=False")

	slog.Info("running ansible-playbook", "playbook", "e2e-site.yml")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ansible-playbook failed: %w", err)
	}
	return nil
}

// generateExtraVars writes the Ansible extra vars YAML file with secrets from environment variables.
func generateExtraVars(path string) error {
	vars := map[string]string{
		"db_password":              "E2E_DB_PASSWORD",
		"minio_access_key":         "E2E_MINIO_ACCESS_KEY",
		"minio_secret_key":         "E2E_MINIO_SECRET_KEY",
		"docker_registry_url":      "E2E_DOCKER_REGISTRY_URL",
		"docker_registry_username": "E2E_DOCKER_REGISTRY_USERNAME",
		"docker_registry_pat":      "E2E_DOCKER_REGISTRY_PAT",
		"github_app_id":            "E2E_GITHUB_APP_ID",
		"github_app_private_key":   "E2E_GITHUB_APP_PRIVATE_KEY",
		"encryption_key":           "E2E_ENCRYPTION_KEY",
	}

	defaults := map[string]string{
		"E2E_DB_PASSWORD":      "zeitwork",
		"E2E_MINIO_ACCESS_KEY": "minioadmin",
		"E2E_MINIO_SECRET_KEY": "minioadmin",
	}

	var b strings.Builder
	b.WriteString("---\n")

	for ansibleVar, envVar := range vars {
		val := os.Getenv(envVar)
		if val == "" {
			val = defaults[envVar]
		}
		if val == "" {
			slog.Warn("extra var not set, will be empty", "env", envVar, "ansible_var", ansibleVar)
		}
		fmt.Fprintf(&b, "%s: %q\n", ansibleVar, val)
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("failed to write extra vars: %w", err)
	}

	slog.Info("generated extra vars file", "path", path)
	return nil
}

// generateInventory creates an INI-format Ansible inventory from the cluster state.
func generateInventory(cluster *ClusterState, nics map[string]string) (string, error) {
	var b strings.Builder

	b.WriteString("[nodes]\n")
	for _, s := range cluster.Servers {
		nic := nics[s.Hostname]
		if nic == "" {
			return "", fmt.Errorf("no NIC detected for %s", s.Hostname)
		}

		fmt.Fprintf(&b, "%s ansible_host=%s ansible_user=root primary_nic=%s internal_ip=%s\n",
			s.Hostname,
			s.PublicIP,
			nic,
			s.InternalIP,
		)
	}

	return b.String(), nil
}
