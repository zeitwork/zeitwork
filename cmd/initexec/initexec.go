// initexec is a tiny helper binary that runs inside the VM's container rootfs.
// It mounts /proc in the new PID namespace, drops to the target UID/GID,
// and exec's the customer command. This replaces the previous busybox shell
// wrapper which couldn't reliably drop privileges with numeric UIDs.
//
// Usage: initexec <uid> <gid> <cwd> -- <command> [args...]
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func main() {
	// Parse: initexec <uid> <gid> <cwd> -- <command> [args...]
	args := os.Args[1:]

	if len(args) < 5 {
		fmt.Fprintf(os.Stderr, "usage: initexec <uid> <gid> <cwd> -- <command> [args...]\n")
		os.Exit(1)
	}

	uid, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid uid: %s\n", args[0])
		os.Exit(1)
	}

	gid, err := strconv.Atoi(args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid gid: %s\n", args[1])
		os.Exit(1)
	}

	cwd := args[2]

	if args[3] != "--" {
		fmt.Fprintf(os.Stderr, "expected '--' separator, got %q\n", args[3])
		os.Exit(1)
	}

	command := args[4:]
	if len(command) == 0 {
		fmt.Fprintf(os.Stderr, "no command specified\n")
		os.Exit(1)
	}

	// 1. Mount /proc for the new PID namespace (requires CAP_SYS_ADMIN, still root here)
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		fmt.Fprintf(os.Stderr, "mount /proc: %v\n", err)
		os.Exit(1)
	}

	// 2. Change to the working directory (while still root)
	if err := os.Chdir(cwd); err != nil {
		fmt.Fprintf(os.Stderr, "chdir %s: %v\n", cwd, err)
		os.Exit(1)
	}

	// 3. Drop privileges: clear groups, set gid, set uid (order matters)
	if uid != 0 || gid != 0 {
		if err := syscall.Setgroups([]int{}); err != nil {
			fmt.Fprintf(os.Stderr, "setgroups: %v\n", err)
			os.Exit(1)
		}
		if err := syscall.Setgid(gid); err != nil {
			fmt.Fprintf(os.Stderr, "setgid(%d): %v\n", gid, err)
			os.Exit(1)
		}
		if err := syscall.Setuid(uid); err != nil {
			fmt.Fprintf(os.Stderr, "setuid(%d): %v\n", uid, err)
			os.Exit(1)
		}
	}

	// 4. Resolve the command via PATH and exec
	binary, err := exec.LookPath(command[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", command[0], err)
		os.Exit(1)
	}

	// syscall.Exec replaces this process entirely
	if err := syscall.Exec(binary, command, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "exec %s: %v\n", binary, err)
		os.Exit(1)
	}
}
