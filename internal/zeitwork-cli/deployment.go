package cli

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/crypto/ssh"
)

// DeploymentManager manages the deployment process
type DeploymentManager struct {
	config      *SetupConfig
	updates     chan tea.Msg
	sshClients  map[string]*ssh.Client
	mu          sync.RWMutex
	complete    bool
	projectRoot string
}

// NewDeploymentManager creates a new deployment manager
func NewDeploymentManager(config *SetupConfig) *DeploymentManager {
	// Find project root (where go.mod is)
	projectRoot, _ := findProjectRoot()

	return &DeploymentManager{
		config:      config,
		updates:     make(chan tea.Msg, 100),
		sshClients:  make(map[string]*ssh.Client),
		projectRoot: projectRoot,
	}
}

// updateProgress sends a progress update
func (dm *DeploymentManager) updateProgress(phase string, progress, total int) {
	dm.updates <- deploymentProgress{
		phase:    phase,
		progress: progress,
		total:    total,
		log:      phase,
	}
}

// sendError sends an error message
func (dm *DeploymentManager) sendError(message string) {
	dm.updates <- errorMsg(message)
}

// isComplete checks if deployment is complete
func (dm *DeploymentManager) isComplete() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.complete
}

// setupDatabase initializes the database
func (dm *DeploymentManager) setupDatabase() error {
	// Migrations should have already been run during the setup phase
	// Just insert the initial region data
	db, err := sql.Open("postgres", dm.config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	// Insert initial region
	_, err = db.Exec(`
		INSERT INTO regions (id, name, code, country) 
		VALUES (gen_random_uuid(), $1, $2, $3)
		ON CONFLICT (code) DO NOTHING`,
		dm.config.Region.Name,
		dm.config.Region.Code,
		dm.config.Region.Country,
	)

	return err
}

// buildBinaries builds the Zeitwork binaries
func (dm *DeploymentManager) buildBinaries() error {
	// Change to project root
	if err := os.Chdir(dm.projectRoot); err != nil {
		return err
	}

	// Run make build
	cmd := exec.Command("make", "build")
	cmd.Dir = dm.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed: %s", output)
	}

	// Package binaries
	tarPath := filepath.Join(".deploy", "zeitwork-binaries.tar.gz")
	cmd = exec.Command("tar", "-czf", tarPath, "build/")
	cmd.Dir = dm.projectRoot
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to package binaries: %v", err)
	}

	return nil
}

// cleanupNode runs the cleanup script on a node to remove existing services
func (dm *DeploymentManager) cleanupNode(ip string) error {
	client, err := dm.getSSHClient(ip)
	if err != nil {
		return err
	}

	// Read cleanup script
	scriptPath := filepath.Join(dm.projectRoot, "internal", "zeitwork-cli", "scripts", "cleanup_node.sh")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		// Cleanup script doesn't exist, skip cleanup
		return nil
	}

	// Execute cleanup script
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// Run cleanup (ignore errors as node might not have been deployed before)
	session.Run(string(script))

	return nil
}

// deployOperator deploys services to an operator node
func (dm *DeploymentManager) deployOperator(ip string) error {
	client, err := dm.getSSHClient(ip)
	if err != nil {
		return err
	}

	// Always run cleanup first to ensure a fresh deployment
	// This removes ALL existing services, configuration, and data
	dm.updateProgress(fmt.Sprintf("Cleaning operator node %s", ip), 0, 0)
	dm.cleanupNode(ip)

	// Copy binaries
	if err := dm.copyBinaries(client, ip); err != nil {
		return err
	}

	// Run operator setup script
	scriptPath := filepath.Join(dm.projectRoot, "internal", "zeitwork-cli", "scripts", "setup_operator.sh")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("failed to read operator setup script: %v", err)
	}

	// Execute setup script with DATABASE_URL as argument
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// Pass DATABASE_URL as argument to the script
	command := fmt.Sprintf("bash -s %q", dm.config.DatabaseURL)
	session.Stdin = bytes.NewReader(script)

	if err := session.Run(command); err != nil {
		return fmt.Errorf("setup failed: %v\nstderr: %s", err, stderr.String())
	}

	return nil
}

// deployWorker deploys node agent to a worker node
func (dm *DeploymentManager) deployWorker(ip string) error {
	client, err := dm.getSSHClient(ip)
	if err != nil {
		return err
	}

	// Always run cleanup first to ensure a fresh deployment
	// This removes ALL existing services, configuration, and data
	dm.updateProgress(fmt.Sprintf("Cleaning operator node %s", ip), 0, 0)
	dm.cleanupNode(ip)

	// Copy binaries
	if err := dm.copyBinaries(client, ip); err != nil {
		return err
	}

	// Build operator URL (use first operator for now)
	// TODO: Support multiple operators with load balancing
	if len(dm.config.OperatorIPs) == 0 {
		return fmt.Errorf("no operator IPs configured")
	}
	operatorURL := fmt.Sprintf("http://%s:8080", dm.config.OperatorIPs[0])

	// Run worker setup script
	scriptPath := filepath.Join(dm.projectRoot, "internal", "zeitwork-cli", "scripts", "setup_worker.sh")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("failed to read worker setup script: %v", err)
	}

	// Execute setup script with OPERATOR_URL as argument
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// Pass OPERATOR_URL as argument to the script
	command := fmt.Sprintf("bash -s %q", operatorURL)
	session.Stdin = bytes.NewReader(script)

	if err := session.Run(command); err != nil {
		return fmt.Errorf("setup failed: %v\nstderr: %s", err, stderr.String())
	}

	return nil
}

// verifyDeployment checks that all services are running
func (dm *DeploymentManager) verifyDeployment() error {
	// Check each operator
	for _, ip := range dm.config.OperatorIPs {
		client, err := dm.getSSHClient(ip)
		if err != nil {
			return err
		}

		session, err := client.NewSession()
		if err != nil {
			return err
		}
		defer session.Close()

		// Check service health
		checkCmd := "curl -s http://localhost:8080/health | grep -q healthy"
		if err := session.Run(checkCmd); err != nil {
			return fmt.Errorf("operator at %s is not healthy", ip)
		}
	}

	// Check each worker
	for _, ip := range dm.config.NodeIPs {
		client, err := dm.getSSHClient(ip)
		if err != nil {
			return err
		}

		session, err := client.NewSession()
		if err != nil {
			return err
		}
		defer session.Close()

		// Check service health
		checkCmd := "curl -s http://localhost:8081/health | grep -q healthy"
		if err := session.Run(checkCmd); err != nil {
			return fmt.Errorf("worker at %s is not healthy", ip)
		}
	}

	// Mark deployment as complete
	dm.mu.Lock()
	dm.complete = true
	dm.mu.Unlock()

	return nil
}

// getSSHClient gets or creates an SSH client for a given IP
func (dm *DeploymentManager) getSSHClient(ip string) (*ssh.Client, error) {
	dm.mu.RLock()
	if client, exists := dm.sshClients[ip]; exists {
		dm.mu.RUnlock()
		return client, nil
	}
	dm.mu.RUnlock()

	// Create new SSH client
	keyPath := expandPath(dm.config.SSHKeyPath)
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read SSH key: %v", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("unable to parse SSH key: %v", err)
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", ip), config)
	if err != nil {
		return nil, err
	}

	// Cache the client
	dm.mu.Lock()
	dm.sshClients[ip] = client
	dm.mu.Unlock()

	return client, nil
}

// copyBinaries copies the binary package to a remote node
func (dm *DeploymentManager) copyBinaries(client *ssh.Client, ip string) error {
	// Use SCP to copy the tar file
	tarPath := filepath.Join(dm.projectRoot, ".deploy", "zeitwork-binaries.tar.gz")

	// Read the tar file
	tarData, err := os.ReadFile(tarPath)
	if err != nil {
		return fmt.Errorf("failed to read binaries: %v", err)
	}

	// Create a session for copying
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// Create the file on remote
	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		fmt.Fprintf(w, "C0644 %d %s\n", len(tarData), "zeitwork-binaries.tar.gz")
		w.Write(tarData)
		fmt.Fprint(w, "\x00")
	}()

	if err := session.Run("scp -t /tmp/"); err != nil {
		return fmt.Errorf("failed to copy binaries: %v", err)
	}

	return nil
}
