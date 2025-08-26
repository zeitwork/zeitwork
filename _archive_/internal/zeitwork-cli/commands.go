package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/ssh"
)

// checkDatabase checks if the database is already set up
func checkDatabase(dbURL string) tea.Cmd {
	return func() tea.Msg {
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			return databaseCheckResult{err: err}
		}
		defer db.Close()

		// Check connection
		if err := db.Ping(); err != nil {
			return databaseCheckResult{err: err}
		}

		// Check if tables exist
		var tableCount int
		query := `
			SELECT COUNT(*) 
			FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name IN ('regions', 'nodes', 'instances', 'deployments', 'projects')
		`
		err = db.QueryRow(query).Scan(&tableCount)
		if err != nil {
			return databaseCheckResult{err: err}
		}

		// If tables exist, database is already set up
		if tableCount > 0 {
			return databaseCheckResult{exists: true}
		}

		return databaseCheckResult{exists: false}
	}
}

// resetDatabase drops and recreates all Zeitwork tables
func resetDatabase(dbURL string) tea.Cmd {
	return func() tea.Msg {
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			return databaseResetResult{success: false, err: err}
		}
		defer db.Close()

		// Check connection
		if err := db.Ping(); err != nil {
			return databaseResetResult{success: false, err: err}
		}

		// Drop all Zeitwork tables in the correct order (respecting foreign key constraints)
		tablesToDrop := []string{
			"deployments",
			"instances",
			"nodes",
			"projects",
			"regions",
			"sessions",
			"users",
			"organisations",
			"waitlist",
			"images",
			"domains",
			"github_connections",
		}

		for _, table := range tablesToDrop {
			_, err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table))
			if err != nil {
				// Continue even if some tables don't exist
				continue
			}
		}

		return databaseResetResult{success: true, err: nil}
	}
}

// runDatabaseMigrations runs the database migrations
func runDatabaseMigrations(dbURL string) tea.Cmd {
	return func() tea.Msg {
		// Find project root
		projectRoot, err := findProjectRoot()
		if err != nil {
			return databaseMigrateResult{success: false, err: fmt.Errorf("could not find project root: %v", err)}
		}

		cmd := exec.Command("npm", "run", "db:migrate")
		cmd.Dir = filepath.Join(projectRoot, "packages", "database")
		cmd.Env = append(os.Environ(), fmt.Sprintf("DATABASE_URL=%s", dbURL))

		output, err := cmd.CombinedOutput()
		if err != nil {
			return databaseMigrateResult{success: false, err: fmt.Errorf("migration failed: %v\nOutput: %s", err, output)}
		}

		return databaseMigrateResult{success: true}
	}
}

// startDeployment begins the deployment process
func startDeployment(config *SetupConfig) tea.Cmd {
	return func() tea.Msg {
		// Save configuration to .deploy directory
		if err := saveConfiguration(config); err != nil {
			return errorMsg(fmt.Sprintf("Failed to save configuration: %v", err))
		}

		// Create deployment manager
		dm := NewDeploymentManager(config)

		// Start deployment in a goroutine and send progress updates
		go func() {
			// Initial progress
			dm.updateProgress("Starting deployment", 0, 100)
			time.Sleep(500 * time.Millisecond) // Brief pause for visual feedback

			// Phase 1: Database setup
			dm.updateProgress("Setting up database", 5, 100)
			if err := dm.setupDatabase(); err != nil {
				dm.sendError(fmt.Sprintf("Database setup failed: %v", err))
				return
			}
			dm.updateProgress("Database setup complete", 10, 100)
			time.Sleep(200 * time.Millisecond)

			// Phase 2: Build binaries
			dm.updateProgress("Building binaries", 15, 100)
			if err := dm.buildBinaries(); err != nil {
				dm.sendError(fmt.Sprintf("Build failed: %v", err))
				return
			}
			dm.updateProgress("Binaries built", 25, 100)
			time.Sleep(200 * time.Millisecond)

			// Phase 3: Deploy operators
			dm.updateProgress("Deploying operator nodes", 30, 100)
			for i, ip := range config.OperatorIPs {
				progress := 30 + (i * 15 / len(config.OperatorIPs))
				dm.updateProgress(fmt.Sprintf("Deploying operator %d/%d (%s)", i+1, len(config.OperatorIPs), ip),
					progress, 100)
				if err := dm.deployOperator(ip); err != nil {
					dm.sendError(fmt.Sprintf("Failed to deploy operator to %s: %v", ip, err))
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
			dm.updateProgress("All operators deployed", 45, 100)
			time.Sleep(200 * time.Millisecond)

			// Phase 4: Deploy workers
			dm.updateProgress("Deploying worker nodes", 50, 100)
			for i, ip := range config.NodeIPs {
				progress := 50 + (i * 30 / len(config.NodeIPs))
				dm.updateProgress(fmt.Sprintf("Deploying worker %d/%d (%s)", i+1, len(config.NodeIPs), ip),
					progress, 100)
				if err := dm.deployWorker(ip); err != nil {
					dm.sendError(fmt.Sprintf("Failed to deploy worker to %s: %v", ip, err))
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
			dm.updateProgress("All workers deployed", 80, 100)
			time.Sleep(200 * time.Millisecond)

			// Phase 5: Verify deployment
			dm.updateProgress("Verifying deployment", 85, 100)
			time.Sleep(500 * time.Millisecond)
			if err := dm.verifyDeployment(); err != nil {
				dm.sendError(fmt.Sprintf("Verification failed: %v", err))
				return
			}
			dm.updateProgress("Deployment successful!", 100, 100)
			time.Sleep(200 * time.Millisecond)
			dm.updateProgress("complete", 100, 100)
		}()

		// Return the deployment manager first so the model can store it
		return dm
	}
}

// listenForDeploymentUpdates creates a command that listens for deployment progress
func listenForDeploymentUpdates(dm *DeploymentManager) tea.Cmd {
	return func() tea.Msg {
		select {
		case update := <-dm.updates:
			return update
		case <-time.After(100 * time.Millisecond):
			// Check if deployment is complete
			if dm.isComplete() {
				return deploymentProgress{
					phase:    "complete",
					progress: 100,
					total:    100,
				}
			}
			// Continue listening
			return listenForDeploymentUpdates(dm)()
		}
	}
}

// verifyNodeConnectivity verifies SSH connectivity to all configured nodes
func verifyNodeConnectivity(config *SetupConfig) tea.Cmd {
	return func() tea.Msg {
		// Prepare SSH client configuration
		keyPath := expandPath(config.SSHKeyPath)
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nodeVerificationComplete{
				allSuccess: false,
				failed:     append(config.OperatorIPs, config.NodeIPs...),
			}
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nodeVerificationComplete{
				allSuccess: false,
				failed:     append(config.OperatorIPs, config.NodeIPs...),
			}
		}

		sshConfig := &ssh.ClientConfig{
			User: "root",
			Auth: []ssh.AuthMethod{
				ssh.PublicKeys(signer),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         10 * time.Second,
		}

		// Combine all IPs to verify
		allIPs := append(config.OperatorIPs, config.NodeIPs...)

		// Use a channel to collect results
		resultChan := make(chan nodeVerificationResult, len(allIPs))
		var wg sync.WaitGroup

		// Verify each node in parallel
		for _, ip := range allIPs {
			wg.Add(1)
			go func(nodeIP string) {
				defer wg.Done()

				// Try to connect
				client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", nodeIP), sshConfig)
				if err != nil {
					resultChan <- nodeVerificationResult{
						ip:      nodeIP,
						success: false,
						err:     err,
					}
					return
				}
				defer client.Close()

				// Try to run a simple command to verify the connection works
				session, err := client.NewSession()
				if err != nil {
					resultChan <- nodeVerificationResult{
						ip:      nodeIP,
						success: false,
						err:     err,
					}
					return
				}
				defer session.Close()

				// Run a simple command
				if err := session.Run("echo 'Connection test successful'"); err != nil {
					resultChan <- nodeVerificationResult{
						ip:      nodeIP,
						success: false,
						err:     err,
					}
					return
				}

				resultChan <- nodeVerificationResult{
					ip:      nodeIP,
					success: true,
					err:     nil,
				}
			}(ip)
		}

		// Wait for all verifications to complete
		go func() {
			wg.Wait()
			close(resultChan)
		}()

		// Collect results
		var failed []string
		allSuccess := true
		for result := range resultChan {
			if !result.success {
				failed = append(failed, result.ip)
				allSuccess = false
			}
		}

		return nodeVerificationComplete{
			allSuccess: allSuccess,
			failed:     failed,
		}
	}
}

// verifyNodeWithProgress creates a command that verifies a single node and sends progress
func verifyNodeWithProgress(ip string, sshConfig *ssh.ClientConfig) tea.Cmd {
	return func() tea.Msg {
		// Try to connect
		client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", ip), sshConfig)
		if err != nil {
			return nodeVerificationResult{
				ip:      ip,
				success: false,
				err:     err,
			}
		}
		defer client.Close()

		// Try to run a simple command to verify the connection works
		session, err := client.NewSession()
		if err != nil {
			return nodeVerificationResult{
				ip:      ip,
				success: false,
				err:     err,
			}
		}
		defer session.Close()

		// Run a simple command
		if err := session.Run("echo 'Connection test successful'"); err != nil {
			return nodeVerificationResult{
				ip:      ip,
				success: false,
				err:     err,
			}
		}

		return nodeVerificationResult{
			ip:      ip,
			success: true,
			err:     nil,
		}
	}
}

// saveConfiguration saves the setup configuration to disk
func saveConfiguration(config *SetupConfig) error {
	// Ensure .deploy directory exists
	deployDir := filepath.Join(".", ".deploy")
	if err := os.MkdirAll(deployDir, 0700); err != nil {
		return err
	}

	// Save main configuration (without sensitive data)
	configPath := filepath.Join(deployDir, "config.json")
	configData := map[string]interface{}{
		"region":         config.Region,
		"operator_count": len(config.OperatorIPs),
		"node_count":     len(config.NodeIPs),
		"created_at":     time.Now().Format(time.RFC3339),
	}

	configJSON, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(configPath, configJSON, 0600); err != nil {
		return err
	}

	// Save inventory
	inventoryPath := filepath.Join(deployDir, "inventory.json")
	inventoryData := map[string]interface{}{
		"operators": config.OperatorIPs,
		"workers":   config.NodeIPs,
		"ssh_key":   config.SSHKeyPath,
	}

	inventoryJSON, err := json.MarshalIndent(inventoryData, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(inventoryPath, inventoryJSON, 0600); err != nil {
		return err
	}

	// Save database URL separately (encrypted in production)
	dbPath := filepath.Join(deployDir, "database.env")
	dbContent := fmt.Sprintf("DATABASE_URL=%s\n", config.DatabaseURL)
	if err := os.WriteFile(dbPath, []byte(dbContent), 0600); err != nil {
		return err
	}

	return nil
}
