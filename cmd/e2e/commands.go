package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <server-number>",
	Short: "SSH into a server (1-based index)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cluster, err := LoadCluster()
		if err != nil {
			return err
		}
		idx, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid server number: %s", args[0])
		}
		server, err := cluster.Server(idx)
		if err != nil {
			return err
		}
		return sshRunInteractive(cluster.SSHKeyPath, server.PublicIP)
	},
}

var logsCmd = &cobra.Command{
	Use:   "logs <server-number>",
	Short: "Tail zeitwork logs on a server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cluster, err := LoadCluster()
		if err != nil {
			return err
		}
		idx, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid server number: %s", args[0])
		}
		server, err := cluster.Server(idx)
		if err != nil {
			return err
		}
		return sshRunInteractive(cluster.SSHKeyPath, server.PublicIP, "journalctl", "-u", "zeitwork", "-f")
	},
}

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Open psql to the local E2E database",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := exec.Command("psql", "postgresql://zeitwork:zeitwork@127.0.0.1:15432/zeitwork?sslmode=disable")
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}
