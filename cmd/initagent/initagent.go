package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/samber/lo"
	"github.com/vishvananda/netlink"
)

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

// fetchConfig retrieves environment variables and log token from the metadata server.
func fetchConfig(metadataURL, token string) ([]string, string, error) {
	resp, err := http.Get(metadataURL + "?token=" + token)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("metadata server returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Env      []string `json:"env"`
		LogToken string   `json:"log_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("failed to decode config: %w", err)
	}
	return result.Env, result.LogToken, nil
}

type VMConfig struct {
	AppID         string `json:"app_id"`
	IPAddr        string `json:"ip_addr"`
	IPGw          string `json:"ip_gw"`
	MetadataURL   string `json:"metadata_url"`
	MetadataToken string `json:"metadata_token"`
	LogURL        string `json:"log_url"`
}

type Config struct {
	Root struct {
		Path string `json:"path"`
	} `json:"root"`
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

var cmdLineRegex = regexp.MustCompile("config=([^ ]+)")

// logForwarder batches log lines and sends them to the metadata server's log endpoint.
type logForwarder struct {
	logURL   string
	logToken string
	client   *http.Client

	mu      sync.Mutex
	buf     []logEntry
	closeCh chan struct{}
	wg      sync.WaitGroup
}

type logEntry struct {
	Message string `json:"message"`
	Level   string `json:"level"`
}

func newLogForwarder(logURL, logToken string) *logForwarder {
	lf := &logForwarder{
		logURL:   logURL,
		logToken: logToken,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		closeCh: make(chan struct{}),
	}
	lf.wg.Add(1)
	go lf.flushLoop()
	return lf
}

// append adds a log line to the buffer.
func (lf *logForwarder) append(message, level string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	lf.buf = append(lf.buf, logEntry{Message: message, Level: level})

	// Flush immediately if buffer is large enough
	if len(lf.buf) >= 50 {
		lf.flushLocked()
	}
}

// flushLoop periodically flushes the buffer.
func (lf *logForwarder) flushLoop() {
	defer lf.wg.Done()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lf.flush()
		case <-lf.closeCh:
			lf.flush()
			return
		}
	}
}

func (lf *logForwarder) flush() {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	lf.flushLocked()
}

func (lf *logForwarder) flushLocked() {
	if len(lf.buf) == 0 {
		return
	}

	entries := lf.buf
	lf.buf = nil

	batch := struct {
		Logs []logEntry `json:"logs"`
	}{Logs: entries}

	body, err := json.Marshal(batch)
	if err != nil {
		slog.Error("failed to marshal log batch", "err", err)
		return
	}

	url := lf.logURL + "?token=" + lf.logToken
	resp, err := lf.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Error("failed to send log batch", "err", err, "count", len(entries))
		return
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		slog.Error("log ingestion returned unexpected status", "status", resp.StatusCode, "count", len(entries))
	}
}

// close flushes remaining logs and stops the flush loop.
func (lf *logForwarder) close() {
	close(lf.closeCh)
	lf.wg.Wait()
}

// pipeReader reads lines from a reader and sends them to the log forwarder.
func pipeReader(r io.Reader, level string, lf *logForwarder, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(r)
	// Allow up to 1MB lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		// Print to our own stdout so it also goes to the CH console file for debugging
		fmt.Println(line)
		lf.append(line, level)
	}
}

func main() {
	// mount required things
	// mount the container fs
	checkErr(syscall.Mount("proc", "/proc", "proc", 0, ""))
	checkErr(syscall.Mount("devtmpfs", "/dev", "devtmpfs", 0, ""))
	checkErr(syscall.Mount("sysfs", "/sys", "sysfs", 0, ""))
	checkErr(syscall.Mount("/dev/vda", "/mnt", "ext4", 0, ""))
	checkErr(syscall.Mount("/mnt/rootfs", "/mnt/rootfs", "", syscall.MS_BIND|syscall.MS_REC, ""))

	cmdLine, err := os.ReadFile("/proc/cmdline")
	checkErr(err)

	vmConfigRaw := cmdLineRegex.FindStringSubmatch(string(cmdLine))
	if len(vmConfigRaw) != 2 {
		slog.Error("Unable to find config in cmdline", "cmdline", string(cmdLine))
		os.Exit(1)
	}

	var vmConfig VMConfig
	err = json.Unmarshal(lo.Must1(base64.StdEncoding.DecodeString(vmConfigRaw[1])), &vmConfig)
	checkErr(err)

	var config Config
	configRaw, err := os.ReadFile("/mnt/config.json")
	checkErr(err)
	err = json.Unmarshal(configRaw, &config)
	checkErr(err)

	// implement some microscopic % of the oci spec
	setupNetwork(vmConfig)

	// Fetch environment variables and log token from metadata server
	envVars, logToken, err := fetchConfig(vmConfig.MetadataURL, vmConfig.MetadataToken)
	checkErr(err)
	slog.Info("fetched config from metadata server", "envCount", len(envVars), "hasLogToken", logToken != "")

	// set hostname
	checkErr(syscall.Sethostname([]byte("zeit-" + vmConfig.AppID)))

	// Build the full environment
	env := append(config.Process.Env, envVars...)
	env = append(env, "ZEITWORK=1")

	// Set environment variables so exec.LookPath uses the container's PATH
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			os.Setenv(parts[0], parts[1])
		}
	}

	// Prepare rootfs
	checkErr(os.MkdirAll("/mnt/rootfs/sys/fs/cgroup", 0555))
	checkErr(os.MkdirAll("/mnt/rootfs/dev", 0555))
	checkErr(os.MkdirAll("/mnt/rootfs/proc", 0555))
	checkErr(syscall.Mount("/sys", "/mnt/rootfs/sys", "", syscall.MS_MOVE, ""))
	checkErr(syscall.Mount("/dev", "/mnt/rootfs/dev", "", syscall.MS_MOVE, ""))
	checkErr(syscall.Mount("/proc", "/mnt/rootfs/proc", "", syscall.MS_MOVE, ""))
	checkErr(syscall.Mount("/cgroup2", "/mnt/rootfs/sys/fs/cgroup", "cgroup2", 0, ""))

	checkErr(os.Chdir("/mnt/rootfs"))
	checkErr(syscall.Mount(".", "/", "", syscall.MS_MOVE, ""))
	checkErr(syscall.Chroot("."))
	checkErr(os.Chdir(config.Process.Cwd))

	checkErr(os.WriteFile("/etc/resolv.conf", []byte("nameserver 8.8.8.8"), 0644))

	binPath, err := exec.LookPath(config.Process.Args[0])
	checkErr(err)

	// Start log forwarder if we have a log URL and token
	var lf *logForwarder
	if vmConfig.LogURL != "" && logToken != "" {
		lf = newLogForwarder(vmConfig.LogURL, logToken)
	}

	slog.Info("starting application", "bin", binPath, "args", config.Process.Args)

	// Spawn the application as a child process (supervisor pattern)
	// The init agent stays as PID 1 and manages the app lifecycle:
	// - Captures stdout/stderr for log forwarding
	// - Forwards signals for graceful shutdown
	// - Reaps the child process
	cmd := exec.Command(binPath, config.Process.Args[1:]...)
	cmd.Env = env
	cmd.Dir = config.Process.Cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: config.Process.User.UID,
			Gid: config.Process.User.GID,
		},
	}

	// Pipe stdout and stderr for log capture
	var pipeWg sync.WaitGroup
	if lf != nil {
		stdout, err := cmd.StdoutPipe()
		checkErr(err)
		stderr, err := cmd.StderrPipe()
		checkErr(err)

		pipeWg.Add(2)
		go pipeReader(stdout, "info", lf, &pipeWg)
		go pipeReader(stderr, "error", lf, &pipeWg)
	} else {
		// No log forwarder — just inherit our stdout/stderr
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	checkErr(cmd.Start())
	slog.Info("application started", "pid", cmd.Process.Pid)

	// Forward SIGTERM and SIGINT to the child process
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Wait for child in a goroutine
	exitCh := make(chan error, 1)
	go func() {
		exitCh <- cmd.Wait()
	}()

	// Wait for either child exit or signal
	var exitErr error
	select {
	case exitErr = <-exitCh:
		// Child exited on its own
		slog.Info("application exited", "err", exitErr)
	case sig := <-sigCh:
		// Received signal — forward to child and wait with grace period
		slog.Info("received signal, forwarding to application", "signal", sig)
		if cmd.Process != nil {
			cmd.Process.Signal(sig)
		}

		// Wait up to 10 seconds for graceful shutdown
		select {
		case exitErr = <-exitCh:
			slog.Info("application exited after signal", "err", exitErr)
		case <-time.After(10 * time.Second):
			slog.Warn("grace period expired, killing application")
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			exitErr = <-exitCh
		}
	}

	// Wait for pipe readers to finish draining
	pipeWg.Wait()

	// Flush remaining logs
	if lf != nil {
		lf.close()
	}

	// Exit with the child's exit code
	if exitErr != nil {
		if exitError, ok := exitErr.(*exec.ExitError); ok {
			os.Exit(exitError.ExitCode())
		}
		os.Exit(1)
	}
	os.Exit(0)
}

func setupNetwork(config VMConfig) {
	lolink := lo.Must1(netlink.LinkByName("lo"))
	loaddr := lo.Must1(netlink.ParseAddr("127.0.0.1/8"))
	checkErr(netlink.AddrAdd(lolink, loaddr))
	checkErr(netlink.LinkSetUp(lolink))

	link := lo.Must1(netlink.LinkByName("eth0"))
	addr := lo.Must1(netlink.ParseAddr(config.IPAddr))
	gw := net.ParseIP(config.IPGw)
	def := lo.Must1(netlink.ParseIPNet("0.0.0.0/0"))

	checkErr(netlink.AddrAdd(link, addr))
	checkErr(netlink.LinkSetUp(link))
	checkErr(netlink.RouteAdd(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Gw:        gw,
		Dst:       def,
	}))
}
