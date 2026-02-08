package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// sshRun executes a command on a remote server and returns stdout.
func sshRun(keyPath, host, command string) (string, error) {
	cmd := exec.Command("ssh",
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"root@"+host,
		command,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ssh %s: %w\nstderr: %s", host, err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// sshRunInteractive starts an interactive SSH session.
func sshRunInteractive(keyPath, host string, remoteCmd ...string) error {
	args := []string{
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"root@" + host,
	}
	args = append(args, remoteCmd...)
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// scpUpload copies a local file to a remote server.
func scpUpload(keyPath, localPath, host, remotePath string) error {
	cmd := exec.Command("scp",
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		localPath,
		fmt.Sprintf("root@%s:%s", host, remotePath),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
