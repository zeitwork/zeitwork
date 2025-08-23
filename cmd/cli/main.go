package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/zeitwork/zeitwork/internal/cli"
)

func main() {
	// Create temporary directory for scripts
	tmpDir, err := os.MkdirTemp("", "zeitwork-cli-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temp directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	// Get all embedded scripts from the cli package
	scripts := cli.GetScripts()

	// Write all scripts to temp directory
	for name, content := range scripts {
		scriptPath := filepath.Join(tmpDir, name)
		if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing script %s: %v\n", name, err)
			os.Exit(1)
		}
	}

	// Execute main script with all arguments
	cmd := exec.Command("bash", filepath.Join(tmpDir, "main.sh"))
	cmd.Args = append(cmd.Args, os.Args[1:]...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("ZEITWORK_SCRIPTS_DIR=%s", tmpDir))

	// Connect stdin, stdout, stderr for interactive use
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command
	if err := cmd.Run(); err != nil {
		// Don't print error message as bash script already handles that
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}
