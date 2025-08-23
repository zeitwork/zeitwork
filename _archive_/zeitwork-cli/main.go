package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	cli "github.com/zeitwork/zeitwork/internal/zeitwork-cli"
)

func main() {
	// Create the initial model
	m := cli.NewModel()

	// Create the Bubble Tea program
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Run the program
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running Zeitwork CLI: %v\n", err)
		os.Exit(1)
	}
}
