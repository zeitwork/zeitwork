package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/vishvananda/netlink"
)

const (
	hostCID   = 2    // Well-known VSOCK CID for the host
	agentPort = 1024 // VSOCK port for guest->host HTTP (config, logs)
	execPort  = 1025 // VSOCK port the guest listens on for exec
)

// customerPID is the PID of the customer app in the root PID namespace.
// Used by exec to join the customer's PID namespace via nsenter.
var customerPID int

// customerUID and customerGID are the UID/GID from the OCI config.
// Used by exec to run commands as the same user as the customer app.
var customerUID uint32
var customerGID uint32

func checkErr(err error) {
	if err != nil {
		slog.Error("fatal error", "err", err)
		panic(err)
	}
}

// OCI runtime bundle config (subset of fields we need).
type OCIConfig struct {
	Process struct {
		Args []string `json:"args"`
		Env  []string `json:"env"`
		Cwd  string   `json:"cwd"`
		User struct {
			UID uint32 `json:"uid"`
			GID uint32 `json:"gid"`
		} `json:"user"`
	} `json:"process"`
}

func main() {
	slog.Info("initagent starting")

	// ── Phase 1: Early mounts ──────────────────────────────────────────
	checkErr(syscall.Mount("proc", "/proc", "proc", 0, ""))
	checkErr(syscall.Mount("devtmpfs", "/dev", "devtmpfs", 0, ""))
	checkErr(syscall.Mount("sysfs", "/sys", "sysfs", 0, ""))
	checkErr(syscall.Mount("/dev/vda", "/mnt", "ext4", 0, ""))
	checkErr(syscall.Mount("/mnt/rootfs", "/mnt/rootfs", "", syscall.MS_BIND|syscall.MS_REC, ""))

	// ── Phase 2: Read OCI config from disk ─────────────────────────────
	var ociConfig OCIConfig
	configRaw, err := os.ReadFile("/mnt/config.json")
	checkErr(err)
	checkErr(json.Unmarshal(configRaw, &ociConfig))
	slog.Info("loaded OCI config", "args", ociConfig.Process.Args, "cwd", ociConfig.Process.Cwd, "uid", ociConfig.Process.User.UID, "gid", ociConfig.Process.User.GID)

	// ── Phase 3: Fetch config from host via VSOCK HTTP ─────────────────
	configResp := fetchConfig()
	slog.Info("received config from host",
		"env_count", len(configResp.Env),
		"ip_addr", configResp.IPAddr,
		"ip_gw", configResp.IPGW,
		"hostname", configResp.Hostname,
	)

	// ── Phase 4: Setup system ──────────────────────────────────────────
	setupNetwork(configResp.IPAddr, configResp.IPGW)
	checkErr(syscall.Sethostname([]byte(configResp.Hostname)))

	// ── Phase 5: Prepare rootfs ────────────────────────────────────────
	checkErr(os.MkdirAll("/mnt/rootfs/sys/fs/cgroup", 0555))
	checkErr(os.MkdirAll("/mnt/rootfs/dev", 0555))
	checkErr(os.MkdirAll("/mnt/rootfs/proc", 0555))
	checkErr(syscall.Mount("/sys", "/mnt/rootfs/sys", "", syscall.MS_MOVE, ""))
	checkErr(syscall.Mount("/dev", "/mnt/rootfs/dev", "", syscall.MS_MOVE, ""))
	checkErr(syscall.Mount("/proc", "/mnt/rootfs/proc", "", syscall.MS_MOVE, ""))
	checkErr(syscall.Mount("cgroup2", "/mnt/rootfs/sys/fs/cgroup", "cgroup2", 0, ""))

	// Bind-mount helper binaries into the container rootfs.
	checkErr(os.MkdirAll("/mnt/rootfs/.zeitwork", 0755))
	// busybox — needed for nsenter in exec sessions. Must be named "busybox"
	// so the multi-call dispatch works when invoked directly.
	checkErr(os.WriteFile("/mnt/rootfs/.zeitwork/busybox", nil, 0755))
	checkErr(syscall.Mount("/usr/bin/busybox", "/mnt/rootfs/.zeitwork/busybox", "", syscall.MS_BIND, ""))
	// initexec — mounts /proc, drops to target UID/GID, execs customer command.
	checkErr(os.WriteFile("/mnt/rootfs/.zeitwork/initexec", nil, 0755))
	checkErr(syscall.Mount("/usr/bin/initexec", "/mnt/rootfs/.zeitwork/initexec", "", syscall.MS_BIND, ""))

	checkErr(os.Chdir("/mnt/rootfs"))
	checkErr(syscall.Mount(".", "/", "", syscall.MS_MOVE, ""))
	checkErr(syscall.Chroot("."))
	checkErr(os.Chdir(ociConfig.Process.Cwd))

	checkErr(os.MkdirAll("/dev/pts", 0755))
	checkErr(syscall.Mount("devpts", "/dev/pts", "devpts", 0, "ptmxmode=0666,newinstance"))
	checkErr(os.WriteFile("/etc/resolv.conf", []byte("nameserver 8.8.8.8\n"), 0644))

	// ── Phase 6: Spawn customer app in PID namespace ───────────────────
	// Merge env: OCI image env + user env from host + ZEITWORK=1
	env := append(ociConfig.Process.Env, configResp.Env...)
	env = append(env, "ZEITWORK=1")

	customerUID = ociConfig.Process.User.UID
	customerGID = ociConfig.Process.User.GID

	// initexec handles mount /proc, privilege drop, and exec in pure Go syscalls.
	// Usage: initexec <uid> <gid> <cwd> -- <command> [args...]
	initexecArgs := []string{
		"initexec",
		strconv.FormatUint(uint64(customerUID), 10),
		strconv.FormatUint(uint64(customerGID), 10),
		ociConfig.Process.Cwd,
		"--",
	}
	cmd := &exec.Cmd{
		Path: "/.zeitwork/initexec",
		Args: append(initexecArgs, ociConfig.Process.Args...),
		Env:  env,
		SysProcAttr: &syscall.SysProcAttr{
			Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
		},
	}

	// Stream logs: tee both stdout and stderr to the VM console and the host log stream.
	logWriter := startLogStream()
	combined := io.MultiWriter(os.Stdout, logWriter)
	cmd.Stdout = combined
	cmd.Stderr = combined
	cmd.Stdin = nil

	slog.Info("starting customer app", "args", ociConfig.Process.Args, "cwd", ociConfig.Process.Cwd)
	checkErr(cmd.Start())
	customerPID = cmd.Process.Pid
	slog.Info("customer app started", "pid", customerPID)

	// ── Phase 7: Start guest server (exec) ─────────────────────────────
	go startGuestServer()

	// ── Phase 8: Wait for child exit ───────────────────────────────────
	waitErr := cmd.Wait()

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
		slog.Error("customer app exited with error", "err", waitErr, "exit_code", exitCode)
	} else {
		slog.Info("customer app exited cleanly")
	}

	logWriter.Close()

	slog.Info("initagent exiting", "app_exit_code", exitCode)

	syscall.Sync()
	checkErr(syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF))
}

// setupNetwork configures lo and eth0 with the given IP and gateway.
func setupNetwork(ipAddr, ipGw string) {
	// Loopback
	loLink, err := netlink.LinkByName("lo")
	checkErr(err)
	loAddr, err := netlink.ParseAddr("127.0.0.1/8")
	checkErr(err)
	checkErr(netlink.AddrAdd(loLink, loAddr))
	checkErr(netlink.LinkSetUp(loLink))

	// eth0
	link, err := netlink.LinkByName("eth0")
	checkErr(err)
	addr, err := netlink.ParseAddr(ipAddr)
	checkErr(err)
	gw := net.ParseIP(ipGw)
	def, err := netlink.ParseIPNet("0.0.0.0/0")
	checkErr(err)

	checkErr(netlink.AddrAdd(link, addr))
	checkErr(netlink.LinkSetUp(link))
	checkErr(netlink.RouteAdd(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Gw:        gw,
		Dst:       def,
	}))
}
