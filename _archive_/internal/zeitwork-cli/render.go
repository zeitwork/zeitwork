package cli

// Package cli render functions for the TUI

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderRegionSetup renders the region configuration step
func (m Model) renderRegionSetup() string {
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		MarginBottom(1)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42"))

	var s strings.Builder
	s.WriteString(promptStyle.Render("Step 2: Region Configuration"))
	s.WriteString("\n\n")

	// Show what we've collected so far
	if m.config.Region.Name != "" {
		s.WriteString(valueStyle.Render(fmt.Sprintf("✓ Region Name: %s", m.config.Region.Name)))
		s.WriteString("\n")
	}
	if m.config.Region.Code != "" {
		s.WriteString(valueStyle.Render(fmt.Sprintf("✓ Region Code: %s", m.config.Region.Code)))
		s.WriteString("\n")
	}
	if m.config.Region.Country != "" {
		s.WriteString(valueStyle.Render(fmt.Sprintf("✓ Country: %s", m.config.Region.Country)))
		s.WriteString("\n")
	}

	// Show current prompt
	if m.currentPrompt != "" {
		s.WriteString("\n")
		s.WriteString(infoStyle.Render(m.currentPrompt))
		s.WriteString("\n\n")
	} else if m.config.Region.Name == "" {
		s.WriteString(infoStyle.Render("Enter the name for your first region:"))
		s.WriteString("\n\n")
	}

	s.WriteString(m.textInput.View())

	if m.errorMessage != "" {
		s.WriteString("\n\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.errorMessage))
	}

	return s.String()
}

// renderSSHKeyStep renders the SSH key configuration step
func (m Model) renderSSHKeyStep() string {
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		MarginBottom(1)

	var s strings.Builder
	s.WriteString(promptStyle.Render("Step 3: SSH Key Configuration"))
	s.WriteString("\n\n")

	if m.currentPrompt != "" {
		s.WriteString(infoStyle.Render(m.currentPrompt))
	} else {
		s.WriteString(infoStyle.Render("Do you want to generate a new SSH key pair? (y/n)"))
		s.WriteString("\n")
		s.WriteString(infoStyle.Render("The key will be stored in .deploy/id_rsa"))
	}
	s.WriteString("\n\n")

	s.WriteString(m.textInput.View())

	if m.infoMessage != "" {
		s.WriteString("\n\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render(m.infoMessage))
	}

	if m.errorMessage != "" {
		s.WriteString("\n\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.errorMessage))
	}

	return s.String()
}

// renderOperatorIPsStep renders the operator IPs collection step
func (m Model) renderOperatorIPsStep() string {
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		MarginBottom(1)

	listStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		MarginLeft(2)

	var s strings.Builder
	s.WriteString(promptStyle.Render("Step 4: Operator Node IPs"))
	s.WriteString("\n\n")
	s.WriteString(infoStyle.Render("Enter IP addresses for operator nodes (minimum 3):"))
	s.WriteString("\n")
	s.WriteString(infoStyle.Render("These nodes will run the control plane services."))
	s.WriteString("\n\n")

	// Show collected IPs
	if len(m.currentInputs) > 0 {
		s.WriteString("Operator IPs added:\n")
		for i, ip := range m.currentInputs {
			s.WriteString(listStyle.Render(fmt.Sprintf("%d. %s", i+1, ip)))
			s.WriteString("\n")
		}
		s.WriteString("\n")
	}

	s.WriteString(m.textInput.View())

	if m.infoMessage != "" {
		s.WriteString("\n\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(m.infoMessage))
	}

	if m.errorMessage != "" {
		s.WriteString("\n\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.errorMessage))
	}

	return s.String()
}

// renderNodeIPsStep renders the worker node IPs collection step
func (m Model) renderNodeIPsStep() string {
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		MarginBottom(1)

	listStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		MarginLeft(2)

	var s strings.Builder
	s.WriteString(promptStyle.Render("Step 5: Worker Node IPs"))
	s.WriteString("\n\n")
	s.WriteString(infoStyle.Render("Enter IP addresses for worker nodes (minimum 3):"))
	s.WriteString("\n")
	s.WriteString(infoStyle.Render("These nodes will run the Firecracker VMs."))
	s.WriteString("\n\n")

	// Show collected IPs
	if len(m.currentInputs) > 0 {
		s.WriteString("Worker IPs added:\n")
		for i, ip := range m.currentInputs {
			s.WriteString(listStyle.Render(fmt.Sprintf("%d. %s", i+1, ip)))
			s.WriteString("\n")
		}
		s.WriteString("\n")
	}

	s.WriteString(m.textInput.View())

	if m.infoMessage != "" {
		s.WriteString("\n\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(m.infoMessage))
	}

	if m.errorMessage != "" {
		s.WriteString("\n\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.errorMessage))
	}

	return s.String()
}

// renderConfirmation renders the confirmation step
func (m Model) renderConfirmation() string {
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	sectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		MarginTop(1)

	itemStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		MarginLeft(2)

	sourceStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Italic(true)

	var s strings.Builder
	s.WriteString(promptStyle.Render("Step 6: Confirm Setup"))
	s.WriteString("\n\n")
	s.WriteString(sourceStyle.Render("(Configuration loaded from .env file)"))
	s.WriteString("\n\n")
	s.WriteString("Please review your configuration:\n\n")

	// Database
	s.WriteString(sectionStyle.Render("Database:"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render(fmt.Sprintf("URL: %s", maskDatabaseURL(m.config.DatabaseURL))))
	s.WriteString("\n\n")

	// Region
	s.WriteString(sectionStyle.Render("Region:"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render(fmt.Sprintf("Name: %s", m.config.Region.Name)))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render(fmt.Sprintf("Code: %s", m.config.Region.Code)))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render(fmt.Sprintf("Country: %s", m.config.Region.Country)))
	s.WriteString("\n\n")

	// SSH Key
	s.WriteString(sectionStyle.Render("SSH Key:"))
	s.WriteString("\n")
	if m.config.GenerateSSHKey {
		s.WriteString(itemStyle.Render("Generated new key pair"))
	} else {
		s.WriteString(itemStyle.Render(fmt.Sprintf("Using existing key: %s", m.config.SSHKeyPath)))
	}
	s.WriteString("\n\n")

	// Operators
	s.WriteString(sectionStyle.Render(fmt.Sprintf("Operator Nodes (%d):", len(m.config.OperatorIPs))))
	s.WriteString("\n")
	for _, ip := range m.config.OperatorIPs {
		s.WriteString(itemStyle.Render(fmt.Sprintf("• %s", ip)))
		s.WriteString("\n")
	}
	s.WriteString("\n")

	// Workers
	s.WriteString(sectionStyle.Render(fmt.Sprintf("Worker Nodes (%d):", len(m.config.NodeIPs))))
	s.WriteString("\n")
	for _, ip := range m.config.NodeIPs {
		s.WriteString(itemStyle.Render(fmt.Sprintf("• %s", ip)))
		s.WriteString("\n")
	}
	s.WriteString("\n")

	// Add warning about fresh deployment
	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true)

	s.WriteString(warningStyle.Render("⚠️  FRESH DEPLOYMENT"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render("This will completely reset all nodes:"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render("  • Remove all existing services"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render("  • Delete all configuration"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render("  • Clear all data"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render("  • Install everything from scratch"))
	s.WriteString("\n\n")

	s.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("220")).
		Bold(true).
		Render("Ready for fresh deployment? (y/n): "))

	s.WriteString(m.textInput.View())

	return s.String()
}

// renderDeployment renders the deployment progress
func (m Model) renderDeployment() string {
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	phaseStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)

	progressBarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42"))

	percentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("220")).
		Bold(true)

	var s strings.Builder
	s.WriteString(promptStyle.Render("🚀 Deploying Zeitwork Platform"))
	s.WriteString("\n\n")

	if m.deploymentState != nil {
		// Calculate progress
		progress := float64(m.deploymentState.Progress) / float64(m.deploymentState.Total)
		percentage := int(progress * 100)
		barWidth := 50
		filled := int(progress * float64(barWidth))

		// Current phase
		s.WriteString(phaseStyle.Render(m.deploymentState.CurrentPhase))
		s.WriteString("\n\n")

		// Enhanced progress bar
		progressBar := strings.Builder{}
		progressBar.WriteString("[")
		for i := 0; i < barWidth; i++ {
			if i < filled-1 && filled > 0 {
				progressBar.WriteString(progressBarStyle.Render("█"))
			} else if i == filled-1 && filled > 0 {
				// Animated leading edge
				if progress < 1.0 {
					progressBar.WriteString(progressBarStyle.Render("▓"))
				} else {
					progressBar.WriteString(progressBarStyle.Render("█"))
				}
			} else {
				progressBar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render("░"))
			}
		}
		progressBar.WriteString("] ")

		s.WriteString(progressBar.String())
		s.WriteString(percentStyle.Render(fmt.Sprintf("%d%%", percentage)))

		// Step counter
		stepInfo := lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Render(fmt.Sprintf(" (%d/%d steps)", m.deploymentState.Progress, m.deploymentState.Total))
		s.WriteString(stepInfo)
		s.WriteString("\n\n")

		// Recent logs with better formatting
		if len(m.deploymentState.Logs) > 0 {
			logHeader := lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Italic(true).
				Render("Recent activity:")
			s.WriteString(logHeader)
			s.WriteString("\n")

			for i, log := range m.deploymentState.Logs {
				// Fade older logs
				color := "244"
				if i >= len(m.deploymentState.Logs)-3 {
					color = "250" // Brighter for recent logs
				}
				logLine := lipgloss.NewStyle().
					Foreground(lipgloss.Color(color)).
					MarginLeft(2).
					Render(fmt.Sprintf("• %s", log))
				s.WriteString(logLine)
				s.WriteString("\n")
			}
		}

		// Add spinner effect for active deployment
		if progress < 1.0 {
			spinnerStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true)
			spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			spinnerIdx := (m.deploymentState.Progress * 3) % len(spinners)
			s.WriteString("\n")
			s.WriteString(spinnerStyle.Render(spinners[spinnerIdx] + " Working..."))
		}
	} else {
		// Initial state
		s.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Italic(true).
			Render("Initializing deployment..."))
	}

	return s.String()
}

// renderComplete renders the completion screen
func (m Model) renderComplete() string {
	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Bold(true).
		MarginBottom(2)

	sectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		MarginTop(1)

	itemStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		MarginLeft(2)

	var s strings.Builder
	s.WriteString(successStyle.Render("✅ Platform Setup Complete!"))
	s.WriteString("\n\n")

	s.WriteString(sectionStyle.Render("Access Points:"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render(fmt.Sprintf("• API: http://%s:8080", m.config.OperatorIPs[0])))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render(fmt.Sprintf("• Health: http://%s:8080/health", m.config.OperatorIPs[0])))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render(fmt.Sprintf("• Nodes: http://%s:8080/api/v1/nodes", m.config.OperatorIPs[0])))
	s.WriteString("\n\n")

	s.WriteString(sectionStyle.Render("Configuration saved to:"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render(".deploy/config.json"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render(".deploy/inventory.json"))
	s.WriteString("\n\n")

	s.WriteString(sectionStyle.Render("Next Steps:"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render("1. Configure DNS records for your domain"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render("2. Install SSL certificates"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render("3. Create your first project"))
	s.WriteString("\n")
	s.WriteString(itemStyle.Render("4. Deploy your application"))
	s.WriteString("\n\n")

	s.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("Press Ctrl+C to exit"))

	return s.String()
}

// Helper function to mask database URL
func maskDatabaseURL(url string) string {
	// Simple masking - in production, use proper URL parsing
	if idx := strings.Index(url, "@"); idx > 0 {
		start := strings.Index(url, "://")
		if start > 0 {
			start += 3
			masked := url[:start] + "****:****" + url[idx:]
			return masked
		}
	}
	return url
}
