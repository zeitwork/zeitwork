package firecracker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Client holds runtime process details for a VM instance
type Client struct {
	InstanceID    string
	APISocketPath string
	VMConfigPath  string
	LogsDir       string
	ConsoleLog    string
	TapDevice     string
	RootfsPath    string
	Process       *os.Process
	Pid           int
}

func (c *Client) isRunning() bool {
	if c == nil {
		return false
	}
	if c.Pid > 0 {
		if err := syscall.Kill(c.Pid, syscall.Signal(0)); err == nil {
			return true
		}
	}
	return false
}

// Start launches a Firecracker process for this client
func (c *Client) Start(ctx context.Context) error {
	bin := firecrackerBinary()
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("firecracker binary not found at %s: %w", bin, err)
	}

	console, err := os.OpenFile(c.ConsoleLog, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open console log: %w", err)
	}

	cmd := exec.CommandContext(ctx, bin,
		"--api-sock", c.APISocketPath,
		"--config-file", c.VMConfigPath,
	)
	cmd.Stdout = console
	cmd.Stderr = console

	if err := cmd.Start(); err != nil {
		_ = console.Close()
		return fmt.Errorf("start firecracker: %w", err)
	}

	c.Process = cmd.Process
	c.Pid = cmd.Process.Pid

	// Wait for API socket to appear (best-effort)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c.APISocketPath != "" {
			if _, err := os.Stat(c.APISocketPath); err == nil {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

// Stop attempts graceful shutdown, then kills the process
func (c *Client) Stop(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if c.APISocketPath != "" {
		_ = c.put("/actions", map[string]string{"action_type": "SendCtrlAltDel"})
		time.Sleep(2 * time.Second)
	}
	if c.Process != nil {
		_ = c.Process.Kill()
		_, _ = c.Process.Wait()
		return nil
	}
	if c.Pid > 0 {
		_ = syscall.Kill(c.Pid, syscall.Signal(9))
	}
	return nil
}

// put performs a Firecracker API PUT against the unix socket
func (c *Client) put(path string, body any) error {
	if c.APISocketPath == "" {
		return fmt.Errorf("empty socket")
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", c.APISocketPath)
		},
	}
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(http.MethodPut, "http://unix"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("firecracker api %s failed: %s: %s", path, resp.Status, strings.TrimSpace(string(data)))
	}
	return nil
}

func firecrackerBinary() string {
	if v := os.Getenv("FIRECRACKER_BIN"); v != "" {
		return v
	}
	cands := []string{"/usr/local/bin/firecracker", "/usr/bin/firecracker", "firecracker"}
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "firecracker"
}

func firecrackerVersion() string {
	out, err := exec.Command(firecrackerBinary(), "--version").CombinedOutput()
	if err != nil {
		return "unknown"
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "unknown"
	}
	return s
}
