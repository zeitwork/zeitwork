package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "e2e",
	Short: "Zeitwork E2E test harness",
	Long: `Manages bare metal servers on Latitude.sh for E2E testing.

Services (PG, PgBouncer, MinIO) run locally via Docker Compose.
Reverse SSH tunnels make them accessible on the servers at localhost.
Tests run locally with 'go test' and verify behavior on the remote servers.`,
}

func init() {
	rootCmd.AddCommand(infraCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(sshCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(dbCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
