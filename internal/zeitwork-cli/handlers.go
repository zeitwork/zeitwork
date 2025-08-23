package cli

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// handleEnter processes the Enter key based on the current step
func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.textInput.Value())

	switch m.currentStep {
	case StepDatabaseURL:
		if input == "" {
			m.errorMessage = "Database URL cannot be empty"
			return m, nil
		}
		if !strings.HasPrefix(input, "postgresql://") {
			m.errorMessage = "Invalid database URL format"
			return m, nil
		}
		m.config.DatabaseURL = input
		m.processing = true
		m.currentStep = StepCheckDatabase
		return m, checkDatabase(input)

	case StepResetDatabase:
		return m.handleDatabaseResetInput(input)

	case StepRegionSetup:
		return m.handleRegionInput(input)

	case StepSSHKey:
		return m.handleSSHKeyInput(input)

	case StepOperatorIPs:
		return m.handleIPInput(input, true)

	case StepNodeIPs:
		return m.handleIPInput(input, false)

	case StepVerifyNodes:
		// Handle retry option if verification failed
		if strings.ToLower(input) == "r" || strings.ToLower(input) == "retry" {
			m.processing = true
			m.verifyingNodes = true
			m.nodeVerificationResults = make(map[string]bool)
			m.textInput.SetValue("")
			m.infoMessage = "Retrying node connectivity verification..."
			m.errorMessage = ""
			return m, verifyNodeConnectivity(m.config)
		} else if strings.ToLower(input) == "c" || strings.ToLower(input) == "continue" {
			// Allow continuing despite failures (user may want to fix later)
			m.currentStep = StepConfirmSetup
			m.textInput.SetValue("")
			m.textInput.Placeholder = "y/n"
			m.infoMessage = ""
			m.errorMessage = ""
		}

	case StepConfirmSetup:
		if strings.ToLower(input) == "y" || strings.ToLower(input) == "yes" {
			m.currentStep = StepDeploying
			m.processing = true
			return m, startDeployment(m.config)
		} else if strings.ToLower(input) == "n" || strings.ToLower(input) == "no" {
			m.currentStep = StepDatabaseURL
			m.textInput.SetValue("")
			m.config = &SetupConfig{}
			m.currentInputs = []string{}
		}
	}

	return m, nil
}

// handleRegionInput handles region configuration input
func (m Model) handleRegionInput(input string) (tea.Model, tea.Cmd) {
	// Track which field we're collecting
	if m.config.Region.Name == "" {
		if input == "" {
			m.errorMessage = "Region name cannot be empty"
			return m, nil
		}
		m.config.Region.Name = input
		m.textInput.SetValue("")
		m.textInput.Placeholder = "e.g., us-east-1"
		m.currentPrompt = "Enter region code:"
		return m, nil
	}

	if m.config.Region.Code == "" {
		if input == "" {
			m.errorMessage = "Region code cannot be empty"
			return m, nil
		}
		// Validate region code format
		if !regexp.MustCompile(`^[a-z]{2}-[a-z]+-\d+$`).MatchString(input) {
			m.errorMessage = "Invalid region code format (expected: xx-xxxx-n)"
			return m, nil
		}
		m.config.Region.Code = input
		m.textInput.SetValue("")
		m.textInput.Placeholder = "e.g., US, DE, SG"
		m.currentPrompt = "Enter country code (ISO 3166-1 alpha-2):"
		return m, nil
	}

	if m.config.Region.Country == "" {
		if input == "" {
			m.errorMessage = "Country code cannot be empty"
			return m, nil
		}
		// Validate ISO country code
		if !isValidCountryCode(strings.ToUpper(input)) {
			m.errorMessage = "Invalid ISO country code"
			return m, nil
		}
		m.config.Region.Country = strings.ToUpper(input)
		m.currentStep = StepSSHKey
		m.textInput.SetValue("")
		m.textInput.Placeholder = "y/n"
		m.currentPrompt = ""
		return m, nil
	}

	return m, nil
}

// handleSSHKeyInput handles SSH key generation decision
func (m Model) handleSSHKeyInput(input string) (tea.Model, tea.Cmd) {
	lower := strings.ToLower(input)
	if lower == "y" || lower == "yes" {
		m.config.GenerateSSHKey = true
		// Generate SSH key
		keyPath, err := generateSSHKeyPair()
		if err != nil {
			m.errorMessage = fmt.Sprintf("Failed to generate SSH key: %v", err)
			m.currentStep = StepError
			return m, nil
		}
		m.config.SSHKeyPath = keyPath
		m.infoMessage = fmt.Sprintf("SSH key generated: %s", keyPath)
	} else if lower == "n" || lower == "no" {
		m.config.GenerateSSHKey = false
		// Ask for existing key path
		m.textInput.SetValue("")
		m.textInput.Placeholder = "~/.ssh/id_rsa"
		m.currentPrompt = "Enter path to existing SSH private key:"
		return m, nil
	} else if m.currentPrompt == "Enter path to existing SSH private key:" {
		// User provided key path
		keyPath := expandPath(input)
		if !fileExists(keyPath) {
			m.errorMessage = fmt.Sprintf("SSH key not found: %s", keyPath)
			return m, nil
		}
		m.config.SSHKeyPath = keyPath
	} else {
		m.errorMessage = "Please enter 'y' or 'n'"
		return m, nil
	}

	// Move to operator IPs collection
	m.currentStep = StepOperatorIPs
	m.textInput.SetValue("")
	m.textInput.Placeholder = "e.g., 10.0.1.10"
	m.collectingMulti = true
	m.minInputs = 3
	m.currentInputs = []string{}
	return m, nil
}

// handleIPInput handles IP address collection for operators or nodes
func (m Model) handleIPInput(input string, isOperator bool) (tea.Model, tea.Cmd) {
	if input == "" && len(m.currentInputs) >= m.minInputs {
		// User pressed enter with no input, finish collection
		m.collectingMulti = false
		return m.processMultiInputComplete()
	}

	// Validate IP address
	if !isValidIP(input) {
		m.errorMessage = "Invalid IP address format"
		return m, nil
	}

	// Check for duplicates
	for _, ip := range m.currentInputs {
		if ip == input {
			m.errorMessage = "IP address already added"
			return m, nil
		}
	}

	// Add to current inputs
	m.currentInputs = append(m.currentInputs, input)
	m.textInput.SetValue("")

	// Check if we can finish
	if len(m.currentInputs) >= m.minInputs {
		m.infoMessage = fmt.Sprintf("Added %d IPs. Press Enter with empty input or Esc to finish, or add more IPs.", len(m.currentInputs))
	} else {
		remaining := m.minInputs - len(m.currentInputs)
		m.infoMessage = fmt.Sprintf("Added %d IPs. Need %d more (minimum %d required).", len(m.currentInputs), remaining, m.minInputs)
	}

	return m, nil
}

// processMultiInputComplete processes completion of multi-input collection
func (m Model) processMultiInputComplete() (tea.Model, tea.Cmd) {
	if m.currentStep == StepOperatorIPs {
		m.config.OperatorIPs = m.currentInputs
		m.currentStep = StepNodeIPs
		m.collectingMulti = true
		m.minInputs = 3
		m.currentInputs = []string{}
		m.textInput.SetValue("")
		m.textInput.Placeholder = "e.g., 10.0.1.20"
		m.infoMessage = ""
	} else if m.currentStep == StepNodeIPs {
		m.config.NodeIPs = m.currentInputs
		// Move to node verification step
		m.currentStep = StepVerifyNodes
		m.collectingMulti = false
		m.processing = true
		m.verifyingNodes = true
		m.nodeVerificationResults = make(map[string]bool)
		m.textInput.SetValue("")
		m.infoMessage = "Verifying connectivity to all nodes..."
		m.errorMessage = ""
		// Start the verification process
		return m, verifyNodeConnectivity(m.config)
	}
	return m, nil
}

// handleDatabaseCheckResult handles the result of database check
func (m Model) handleDatabaseCheckResult(msg databaseCheckResult) (tea.Model, tea.Cmd) {
	m.processing = false

	if msg.err != nil {
		m.errorMessage = fmt.Sprintf("Failed to connect to database: %v", msg.err)
		m.currentStep = StepError
		return m, nil
	}

	if msg.exists {
		// Database exists, ask if user wants to reset it
		m.currentStep = StepResetDatabase
		m.textInput.SetValue("")
		m.textInput.Placeholder = "yes/no"
		return m, nil
	}

	// Database doesn't have tables - run migrations first
	m.processing = true
	m.currentStep = StepMigrateDatabase
	return m, runDatabaseMigrations(m.config.DatabaseURL)
}

// handleDatabaseResetInput handles the user's response to database reset prompt
func (m Model) handleDatabaseResetInput(input string) (tea.Model, tea.Cmd) {
	lower := strings.ToLower(strings.TrimSpace(input))

	switch lower {
	case "yes", "y":
		// User wants to reset the database
		m.processing = true
		return m, resetDatabase(m.config.DatabaseURL)
	case "no", "n":
		// User doesn't want to reset, continue with existing database
		m.infoMessage = "Continuing with existing database..."

		// Check if we have enough config from .env to skip to confirmation
		if m.shouldSkipToConfirmation() {
			m.currentStep = StepConfirmSetup
			m.textInput.SetValue("")
			m.textInput.Placeholder = "y/n"
		} else {
			// Need more configuration, move to region setup
			m.currentStep = StepRegionSetup
			m.textInput.SetValue("")
			m.textInput.Placeholder = "e.g., US East"
			m.currentPrompt = "Enter region name:"
		}
		return m, nil
	default:
		m.errorMessage = "Please enter 'yes' or 'no'"
		m.textInput.SetValue("")
	}

	return m, nil
}

// handleDatabaseResetResult handles the result of database reset
func (m Model) handleDatabaseResetResult(msg databaseResetResult) (tea.Model, tea.Cmd) {
	m.processing = false

	if msg.err != nil {
		m.errorMessage = fmt.Sprintf("Failed to reset database: %v", msg.err)
		m.currentStep = StepError
		return m, nil
	}

	if !msg.success {
		m.errorMessage = "Database reset failed"
		m.currentStep = StepError
		return m, nil
	}

	// Database reset successful, now run migrations
	m.processing = true
	m.currentStep = StepMigrateDatabase
	return m, runDatabaseMigrations(m.config.DatabaseURL)
}

// handleDatabaseMigrateResult handles the result of database migration
func (m Model) handleDatabaseMigrateResult(msg databaseMigrateResult) (tea.Model, tea.Cmd) {
	m.processing = false

	if msg.err != nil {
		m.errorMessage = fmt.Sprintf("Database migration failed: %v", msg.err)
		m.currentStep = StepError
		return m, nil
	}

	// Migration successful, check if we have enough config from .env
	if m.shouldSkipToConfirmation() {
		m.currentStep = StepConfirmSetup
		m.textInput.SetValue("")
		m.textInput.Placeholder = "y/n"
	} else {
		// Need more configuration, move to region setup
		m.currentStep = StepRegionSetup
		m.textInput.SetValue("")
		m.textInput.Placeholder = "e.g., US East"
		m.currentPrompt = "Enter region name:"
	}
	return m, nil
}

// shouldSkipToConfirmation checks if we have enough config from .env to skip to confirmation
func (m Model) shouldSkipToConfirmation() bool {
	// Check if we have all required configuration from .env
	return m.config.Region.Name != "" &&
		m.config.Region.Code != "" &&
		m.config.Region.Country != "" &&
		m.config.SSHKeyPath != "" &&
		len(m.config.OperatorIPs) >= 3 &&
		len(m.config.NodeIPs) >= 3
}

// handleDeploymentProgress handles deployment progress updates
func (m Model) handleDeploymentProgress(msg deploymentProgress) (tea.Model, tea.Cmd) {
	if m.deploymentState == nil {
		m.deploymentState = &DeploymentState{
			Logs: []string{},
		}
	}

	m.deploymentState.CurrentPhase = msg.phase
	m.deploymentState.Progress = msg.progress
	m.deploymentState.Total = msg.total

	if msg.log != "" {
		m.deploymentState.Logs = append(m.deploymentState.Logs, msg.log)
		// Keep only last 10 logs
		if len(m.deploymentState.Logs) > 10 {
			m.deploymentState.Logs = m.deploymentState.Logs[len(m.deploymentState.Logs)-10:]
		}
	}

	// Check if deployment is complete
	if msg.progress >= msg.total && msg.phase == "complete" {
		m.currentStep = StepComplete
		m.processing = false
		return m, nil
	}

	// Continue listening for more updates
	if m.deploymentManager != nil {
		return m, listenForDeploymentUpdates(m.deploymentManager)
	}
	return m, nil
}

// Validation helpers

func isValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

func isValidCountryCode(code string) bool {
	// Common ISO 3166-1 alpha-2 country codes
	validCodes := map[string]bool{
		"US": true, "CA": true, "GB": true, "DE": true, "FR": true,
		"IT": true, "ES": true, "NL": true, "BE": true, "CH": true,
		"AT": true, "DK": true, "SE": true, "NO": true, "FI": true,
		"PL": true, "CZ": true, "HU": true, "RO": true, "BG": true,
		"JP": true, "CN": true, "KR": true, "IN": true, "SG": true,
		"MY": true, "TH": true, "ID": true, "PH": true, "VN": true,
		"AU": true, "NZ": true, "BR": true, "MX": true, "AR": true,
		"IL": true, "AE": true, "SA": true, "ZA": true, "EG": true,
		"RU": true, "UA": true, "TR": true, "GR": true, "PT": true,
		"IE": true, "IS": true, "LU": true, "MT": true, "CY": true,
	}
	return validCodes[code]
}
