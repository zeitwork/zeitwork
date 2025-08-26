package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SetupStep represents the current step in the setup process
type SetupStep int

const (
	StepDatabaseURL SetupStep = iota
	StepCheckDatabase
	StepResetDatabase
	StepMigrateDatabase
	StepRegionSetup
	StepSSHKey
	StepOperatorIPs
	StepNodeIPs
	StepVerifyNodes
	StepConfirmSetup
	StepDeploying
	StepComplete
	StepError
)

// Model represents the main state of the CLI application
type Model struct {
	// Current step in the setup process
	currentStep SetupStep

	// UI components
	textInput    textinput.Model
	errorMessage string
	infoMessage  string

	// Configuration state
	config *SetupConfig

	// Deployment state
	deploymentState   *DeploymentState
	deploymentManager *DeploymentManager

	// UI state
	width      int
	height     int
	quitting   bool
	processing bool

	// Current input state
	currentPrompt   string
	currentInputs   []string
	minInputs       int
	collectingMulti bool

	// Node verification state
	nodeVerificationResults map[string]bool
	verifyingNodes          bool
}

// SetupConfig holds all the configuration for the setup
type SetupConfig struct {
	DatabaseURL    string
	Region         RegionConfig
	SSHKeyPath     string
	GenerateSSHKey bool
	OperatorIPs    []string
	NodeIPs        []string
}

// RegionConfig holds region configuration
type RegionConfig struct {
	Name    string
	Code    string
	Country string // ISO 3166-1 alpha-2 code
}

// DeploymentState tracks the deployment progress
type DeploymentState struct {
	CurrentPhase string
	Progress     int
	Total        int
	Logs         []string
}

// NewModel creates a new CLI model
func NewModel() Model {
	ti := textinput.New()
	ti.Placeholder = "Enter value..."
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50

	// Ensure .deploy directory exists
	deployDir := filepath.Join(".", ".deploy")
	os.MkdirAll(deployDir, 0700)

	// Initialize config
	config := &SetupConfig{}
	startStep := StepDatabaseURL

	// Try to load .env file
	envPath := ".env"
	if envVars, err := loadEnvFile(envPath); err == nil {
		// Load DATABASE_URL
		if dbURL, exists := envVars["DATABASE_URL"]; exists && dbURL != "" {
			config.DatabaseURL = dbURL
			startStep = StepCheckDatabase
		}

		// Load OPERATORS (comma-separated list)
		if operators, exists := envVars["OPERATORS"]; exists {
			config.OperatorIPs = parseIPList(operators)
		}

		// Load WORKERS (comma-separated list)
		if workers, exists := envVars["WORKERS"]; exists {
			config.NodeIPs = parseIPList(workers)
		}

		// Load optional REGION_NAME, REGION_CODE, REGION_COUNTRY
		if regionName, exists := envVars["REGION_NAME"]; exists {
			config.Region.Name = regionName
		}
		if regionCode, exists := envVars["REGION_CODE"]; exists {
			config.Region.Code = regionCode
		}
		if regionCountry, exists := envVars["REGION_COUNTRY"]; exists {
			config.Region.Country = regionCountry
		}

		// Load SSH_KEY_PATH if provided
		if sshKeyPath, exists := envVars["SSH_KEY_PATH"]; exists {
			config.SSHKeyPath = expandPath(sshKeyPath)
		} else {
			// Default to .deploy/id_rsa if it exists
			defaultKeyPath := filepath.Join(".deploy", "id_rsa")
			if fileExists(defaultKeyPath) {
				config.SSHKeyPath = defaultKeyPath
			}
		}
	}

	return Model{
		currentStep:   startStep,
		textInput:     ti,
		config:        config,
		currentInputs: []string{},
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	// If we loaded database URL from .env, immediately check the database
	if m.currentStep == StepCheckDatabase && m.config.DatabaseURL != "" {
		return checkDatabase(m.config.DatabaseURL)
	}
	return textinput.Blink
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = min(50, msg.Width-4)

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEsc:
			if m.collectingMulti && len(m.currentInputs) >= m.minInputs {
				// Finish collecting multiple inputs
				m.collectingMulti = false
				return m.processMultiInputComplete()
			}

		case tea.KeyEnter:
			if m.processing {
				return m, nil
			}
			return m.handleEnter()
		}

	case databaseCheckResult:
		return m.handleDatabaseCheckResult(msg)

	case databaseResetResult:
		return m.handleDatabaseResetResult(msg)

	case databaseMigrateResult:
		return m.handleDatabaseMigrateResult(msg)

	case deploymentProgress:
		return m.handleDeploymentProgress(msg)

	case *DeploymentManager:
		m.deploymentManager = msg
		return m, listenForDeploymentUpdates(msg)

	case nodeVerificationResult:
		if m.nodeVerificationResults == nil {
			m.nodeVerificationResults = make(map[string]bool)
		}
		m.nodeVerificationResults[msg.ip] = msg.success
		if msg.err != nil {
			m.infoMessage = fmt.Sprintf("Failed to connect to %s: %v", msg.ip, msg.err)
		}
		return m, nil

	case nodeVerificationComplete:
		m.verifyingNodes = false
		m.processing = false
		if msg.allSuccess {
			m.currentStep = StepConfirmSetup
			m.textInput.SetValue("")
			m.textInput.Placeholder = "y/n"
			m.infoMessage = "All nodes verified successfully!"
		} else {
			m.errorMessage = fmt.Sprintf("Failed to connect to nodes: %s", strings.Join(msg.failed, ", "))
			m.infoMessage = "Enter 'r' to retry, or 'c' to continue anyway"
			m.textInput.SetValue("")
			m.textInput.Placeholder = "r/c"
		}
		return m, nil

	case errorMsg:
		m.errorMessage = string(msg)
		m.currentStep = StepError
		m.processing = false
		return m, nil
	}

	// Update text input
	if !m.processing && m.currentStep != StepError && m.currentStep != StepComplete {
		m.textInput, cmd = m.textInput.Update(msg)
	}

	return m, cmd
}

// View renders the UI
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var s strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		MarginBottom(2)

	s.WriteString(headerStyle.Render("🚀 Zeitwork Platform Setup"))
	s.WriteString("\n\n")

	// Progress indicator
	s.WriteString(m.renderProgress())
	s.WriteString("\n\n")

	// Main content based on current step
	switch m.currentStep {
	case StepDatabaseURL:
		s.WriteString(m.renderDatabaseURLStep())
	case StepCheckDatabase:
		s.WriteString(m.renderCheckingDatabase())
	case StepResetDatabase:
		s.WriteString(m.renderResetDatabase())
	case StepMigrateDatabase:
		s.WriteString(m.renderMigratingDatabase())
	case StepRegionSetup:
		s.WriteString(m.renderRegionSetup())
	case StepSSHKey:
		s.WriteString(m.renderSSHKeyStep())
	case StepOperatorIPs:
		s.WriteString(m.renderOperatorIPsStep())
	case StepNodeIPs:
		s.WriteString(m.renderNodeIPsStep())
	case StepVerifyNodes:
		s.WriteString(m.renderVerifyNodes())
	case StepConfirmSetup:
		s.WriteString(m.renderConfirmation())
	case StepDeploying:
		s.WriteString(m.renderDeployment())
	case StepComplete:
		s.WriteString(m.renderComplete())
	case StepError:
		s.WriteString(m.renderError())
	}

	// Footer with help
	if m.currentStep != StepError && m.currentStep != StepComplete {
		footerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(2)

		help := "Press Ctrl+C to quit"
		if m.collectingMulti {
			help += " • Press Enter to add more • Press Esc when done"
		}
		s.WriteString("\n\n")
		s.WriteString(footerStyle.Render(help))
	}

	return s.String()
}

// Helper functions

func (m Model) renderProgress() string {
	steps := []string{
		"Database",
		"Region",
		"SSH Key",
		"Operators",
		"Nodes",
		"Deploy",
	}

	currentIdx := int(m.currentStep)
	if currentIdx > len(steps) {
		currentIdx = len(steps)
	}

	progressStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	activeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)
	completeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))

	var parts []string
	for i, step := range steps {
		if i < currentIdx {
			parts = append(parts, completeStyle.Render("✓ "+step))
		} else if i == currentIdx {
			parts = append(parts, activeStyle.Render("→ "+step))
		} else {
			parts = append(parts, progressStyle.Render("○ "+step))
		}
	}

	return strings.Join(parts, " ")
}

func (m Model) renderDatabaseURLStep() string {
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		MarginBottom(1)

	var s strings.Builder
	s.WriteString(promptStyle.Render("Step 1: Database Configuration"))
	s.WriteString("\n\n")
	s.WriteString(infoStyle.Render("Enter your PlanetScale PostgreSQL connection URL:"))
	s.WriteString("\n")
	s.WriteString(infoStyle.Render("Format: postgresql://user:pass@host.connect.psdb.cloud/db?sslmode=require"))
	s.WriteString("\n\n")

	m.textInput.Placeholder = "postgresql://..."
	s.WriteString(m.textInput.View())

	return s.String()
}

func (m Model) renderCheckingDatabase() string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("220")).
		Render("⏳ Checking database connection and setup status...")
}

func (m Model) renderResetDatabase() string {
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		MarginBottom(1)

	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39"))

	var s strings.Builder
	s.WriteString(promptStyle.Render("⚠️  Database Already Contains Tables"))
	s.WriteString("\n\n")
	s.WriteString(warningStyle.Render("The database already contains Zeitwork tables."))
	s.WriteString("\n")
	s.WriteString(warningStyle.Render("Do you want to reset the database? This will DELETE ALL EXISTING DATA!"))
	s.WriteString("\n\n")
	s.WriteString(infoStyle.Render("• Type 'yes' to reset and start fresh"))
	s.WriteString("\n")
	s.WriteString(infoStyle.Render("• Type 'no' to continue with the existing database"))
	s.WriteString("\n\n")

	m.textInput.Placeholder = "yes/no"
	s.WriteString(m.textInput.View())

	return s.String()
}

func (m Model) renderMigratingDatabase() string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("220")).
		Render("⏳ Running database migrations...")
}

func (m Model) renderVerifyNodes() string {
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("220")).
		Bold(true).
		MarginBottom(1)

	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244"))

	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42"))

	failStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	var s strings.Builder
	s.WriteString(headerStyle.Render("🔍 Verifying Node Connectivity"))
	s.WriteString("\n\n")

	if m.verifyingNodes {
		s.WriteString(infoStyle.Render("Checking SSH connectivity to all nodes..."))
		s.WriteString("\n\n")

		// Show progress for each node
		allNodes := append(m.config.OperatorIPs, m.config.NodeIPs...)
		for _, ip := range allNodes {
			if result, exists := m.nodeVerificationResults[ip]; exists {
				if result {
					s.WriteString(successStyle.Render(fmt.Sprintf("✓ %s - Connected successfully", ip)))
				} else {
					s.WriteString(failStyle.Render(fmt.Sprintf("✗ %s - Connection failed", ip)))
				}
			} else {
				s.WriteString(infoStyle.Render(fmt.Sprintf("⏳ %s - Checking...", ip)))
			}
			s.WriteString("\n")
		}
	} else if !m.processing {
		// Show final results when not processing
		allNodes := append(m.config.OperatorIPs, m.config.NodeIPs...)
		if len(m.nodeVerificationResults) > 0 {
			s.WriteString("Verification Results:\n\n")
			for _, ip := range allNodes {
				if result, exists := m.nodeVerificationResults[ip]; exists {
					if result {
						s.WriteString(successStyle.Render(fmt.Sprintf("✓ %s - Connected successfully", ip)))
					} else {
						s.WriteString(failStyle.Render(fmt.Sprintf("✗ %s - Connection failed", ip)))
					}
					s.WriteString("\n")
				}
			}
		}
	}

	if m.errorMessage != "" {
		s.WriteString("\n")
		s.WriteString(failStyle.Render(m.errorMessage))
		s.WriteString("\n")
	}

	if m.infoMessage != "" {
		s.WriteString("\n")
		s.WriteString(infoStyle.Render(m.infoMessage))
		s.WriteString("\n")
	}

	// Show input field when not processing
	if !m.processing && !m.verifyingNodes {
		s.WriteString("\n")
		s.WriteString(m.textInput.View())
	}

	return s.String()
}

func (m Model) renderError() string {
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true).
		MarginBottom(1)

	msgStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Width(60)

	var s strings.Builder
	s.WriteString(errorStyle.Render("❌ Setup Failed"))
	s.WriteString("\n\n")
	s.WriteString(msgStyle.Render(m.errorMessage))
	s.WriteString("\n\n")
	s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Press Ctrl+C to exit"))

	return s.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Message types
type databaseCheckResult struct {
	exists bool
	err    error
}

type databaseResetResult struct {
	success bool
	err     error
}

type deploymentProgress struct {
	phase    string
	progress int
	total    int
	log      string
}

type databaseMigrateResult struct {
	success bool
	err     error
}

type nodeVerificationResult struct {
	ip      string
	success bool
	err     error
}

type nodeVerificationComplete struct {
	allSuccess bool
	failed     []string
}

type errorMsg string
