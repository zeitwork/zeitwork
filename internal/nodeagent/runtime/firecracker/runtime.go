package firecracker

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

//go:embed templates/vm-config.json.tmpl
var vmConfigTemplate string

//go:embed templates/zeitwork-app.initd
var appInitdTemplate string

// FirecrackerRuntime implements the Runtime interface using Firecracker
type FirecrackerRuntime struct {
	config  *config.FirecrackerRuntimeConfig
	logger  *slog.Logger
	queries *database.Queries

	// Track active VM instances
	instances map[string]*Client
	mutex     sync.RWMutex
}

// NewFirecrackerRuntime creates a new Firecracker runtime
func NewFirecrackerRuntime(cfg *config.FirecrackerRuntimeConfig, logger *slog.Logger, queries *database.Queries) (*FirecrackerRuntime, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil firecracker config")
	}

	r := &FirecrackerRuntime{
		config:    cfg,
		logger:    logger.With("runtime", "firecracker"),
		queries:   queries,
		instances: make(map[string]*Client),
	}

	_ = r.cleanupOrphanedTAPDevices()

	return r, nil
}

// CreateInstance creates a new Firecracker VM instance
func (f *FirecrackerRuntime) CreateInstance(ctx context.Context, spec *types.InstanceSpec) (*types.Instance, error) {
	if spec == nil {
		return nil, fmt.Errorf("spec is nil")
	}

	baseDir := instanceBaseDir()
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create base dir: %w", err)
	}

	instDir := filepath.Join(baseDir, spec.ID)
	logsDir := filepath.Join(instDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create instance dir: %w", err)
	}

	rootfsSrc := f.config.DefaultRootfsPath
	if _, err := os.Stat(rootfsSrc); err != nil {
		return nil, fmt.Errorf("base rootfs not found: %s: %w", rootfsSrc, err)
	}
	rootfsDst := filepath.Join(instDir, "rootfs.ext4")
	if err := copyFile(rootfsSrc, rootfsDst); err != nil {
		return nil, fmt.Errorf("copy rootfs: %w", err)
	}

	// Configure IPv6 for the guest by writing /etc/ipv6-addr into the rootfs
	ipv6 := GenerateIPv6AddressFromID(spec.ID)
	if err := writeIPv6AddrToRootfs(rootfsDst, ipv6); err != nil {
		f.logger.Warn("failed to inject IPv6 into rootfs", "err", err)
	}

	// Prepare rootfs with customer image
	var defaultPort int32
	if spec.NetworkConfig != nil {
		defaultPort = spec.NetworkConfig.DefaultPort
	}
	if err := f.prepareRootfsWithImage(rootfsDst, spec.ImageTag, spec.EnvironmentVariables, defaultPort); err != nil {
		return nil, fmt.Errorf("prepare rootfs with image: %w", err)
	}

	metricsFifo := filepath.Join(logsDir, "metrics.fifo")
	_ = os.Remove(metricsFifo)
	if err := mkfifo(metricsFifo, 0o644); err != nil {
		return nil, fmt.Errorf("create metrics fifo: %w", err)
	}
	metricsLog := filepath.Join(logsDir, "metrics.log")
	go tailFIFOToFile(metricsFifo, metricsLog)

	firecrackerLog := filepath.Join(logsDir, "firecracker.log")
	if _, err := os.Create(firecrackerLog); err != nil {
		return nil, fmt.Errorf("create firecracker log: %w", err)
	}

	tapName := tapNameFor(spec.ID)
	if !f.tapDeviceExists(tapName) {
		if err := f.createTAPDevice(tapName); err != nil {
			f.logger.Warn("failed to create TAP device", "tap", tapName, "err", err)
		}
	}

	cfgPath := filepath.Join(instDir, "vm-config.json")
	data := vmConfigData{
		KernelImagePath: f.config.DefaultKernelPath,
		BootArgs:        "console=ttyS0 reboot=k panic=1 pci=off",
		RootfsPath:      rootfsDst,
		TapDevice:       tapName,
		VCPUCount:       defaultResources(f.config, spec).VCPUs,
		MemSizeMiB:      defaultResources(f.config, spec).Memory,
		LogPath:         firecrackerLog,
		MetricsPath:     metricsFifo,
	}
	if err := renderVMConfigToFile(cfgPath, data); err != nil {
		return nil, fmt.Errorf("write vm config: %w", err)
	}

	apiSock := filepath.Join("/tmp", fmt.Sprintf("firecracker-%s.socket", shortID(spec.ID)))
	_ = os.Remove(apiSock)
	consoleLog := filepath.Join(logsDir, "console.log")

	client := &Client{
		InstanceID:    spec.ID,
		APISocketPath: apiSock,
		VMConfigPath:  cfgPath,
		LogsDir:       logsDir,
		ConsoleLog:    consoleLog,
		TapDevice:     tapName,
		RootfsPath:    rootfsDst,
	}

	f.mutex.Lock()
	f.instances[spec.ID] = client
	f.mutex.Unlock()

	now := time.Now()
	inst := &types.Instance{
		ID:        spec.ID,
		ImageID:   spec.ImageID,
		ImageTag:  spec.ImageTag,
		State:     types.InstanceStateCreating,
		Resources: defaultResources(f.config, spec),
		EnvVars:   spec.EnvironmentVariables,
		CreatedAt: now,
		RuntimeID: spec.ID,
	}
	inst.NetworkInfo = &types.NetworkInfo{IPAddress: ipv6}

	return inst, nil
}

// StartInstance starts a Firecracker VM instance
func (f *FirecrackerRuntime) StartInstance(ctx context.Context, instance *types.Instance) error {
	client, err := f.getClient(instance.ID)
	if err != nil {
		return err
	}

	if err := client.Start(ctx); err != nil {
		return err
	}
	f.logger.Info("firecracker started", "instance", instance.ID, "pid", client.Pid, "api", client.APISocketPath)

	// allow boot
	time.Sleep(3 * time.Second)
	return nil
}

// StopInstance stops a Firecracker VM instance
func (f *FirecrackerRuntime) StopInstance(ctx context.Context, instance *types.Instance) error {
	client, err := f.getClient(instance.ID)
	if err != nil {
		return err
	}

	return client.Stop(ctx)
}

// DeleteInstance removes a Firecracker VM instance
func (f *FirecrackerRuntime) DeleteInstance(ctx context.Context, instance *types.Instance) error {
	_ = f.StopInstance(ctx, instance)

	client, _ := f.getClient(instance.ID)
	if client != nil && client.TapDevice != "" {
		_ = f.deleteTAPDevice(client.TapDevice)
	}

	instDir := filepath.Join(instanceBaseDir(), instance.ID)
	_ = os.RemoveAll(instDir)
	if client != nil && client.APISocketPath != "" {
		_ = os.Remove(client.APISocketPath)
	}

	f.mutex.Lock()
	delete(f.instances, instance.ID)
	f.mutex.Unlock()

	return nil
}

// GetInstanceState returns the current state of an instance
func (f *FirecrackerRuntime) GetInstanceState(ctx context.Context, instance *types.Instance) (types.InstanceState, error) {
	client, err := f.getClient(instance.ID)
	if err != nil {
		return types.InstanceStateFailed, err
	}
	if client == nil {
		return types.InstanceStateStopped, nil
	}
	if client.isRunning() {
		return types.InstanceStateRunning, nil
	}
	return types.InstanceStateStopped, nil
}

// ListInstances lists all Firecracker VM instances
func (f *FirecrackerRuntime) ListInstances(ctx context.Context) ([]*types.Instance, error) {
	f.mutex.RLock()
	defer f.mutex.RUnlock()
	out := make([]*types.Instance, 0, len(f.instances))
	for id := range f.instances {
		out = append(out, &types.Instance{
			ID:        id,
			State:     types.InstanceStateRunning,
			CreatedAt: time.Now(),
			RuntimeID: id,
		})
	}
	return out, nil
}

// IsInstanceRunning checks if an instance is currently running
func (f *FirecrackerRuntime) IsInstanceRunning(ctx context.Context, instance *types.Instance) (bool, error) {
	st, err := f.GetInstanceState(ctx, instance)
	if err != nil {
		return false, err
	}
	return st == types.InstanceStateRunning, nil
}

// GetStats returns resource usage statistics for an instance
func (f *FirecrackerRuntime) GetStats(ctx context.Context, instance *types.Instance) (*types.InstanceStats, error) {
	return &types.InstanceStats{
		InstanceID: instance.ID,
		Timestamp:  time.Now(),
	}, nil
}

// ExecuteCommand executes a command inside a VM instance
func (f *FirecrackerRuntime) ExecuteCommand(ctx context.Context, instance *types.Instance, cmd []string) (string, error) {
	return "", fmt.Errorf("not supported in firecracker runtime")
}

// GetLogs retrieves logs from a VM instance
func (f *FirecrackerRuntime) GetLogs(ctx context.Context, instance *types.Instance, lines int) ([]string, error) {
	client, err := f.getClient(instance.ID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("instance not found")
	}
	data, err := os.ReadFile(client.ConsoleLog)
	if err != nil {
		return nil, err
	}
	all := strings.Split(string(data), "\n")
	if lines <= 0 || lines >= len(all) {
		return all, nil
	}
	start := len(all) - 1 - lines
	if start < 0 {
		start = 0
	}
	return all[start : len(all)-1], nil
}

// CleanupOrphanedInstances removes instances that are no longer desired
func (f *FirecrackerRuntime) CleanupOrphanedInstances(ctx context.Context, desiredInstances []*types.Instance) error {
	desired := make(map[string]struct{}, len(desiredInstances))
	for _, d := range desiredInstances {
		desired[d.ID] = struct{}{}
	}

	f.mutex.RLock()
	ids := make([]string, 0, len(f.instances))
	for id := range f.instances {
		if _, ok := desired[id]; !ok {
			ids = append(ids, id)
		}
	}
	f.mutex.RUnlock()

	for _, id := range ids {
		inst := &types.Instance{ID: id}
		if err := f.DeleteInstance(ctx, inst); err != nil {
			f.logger.Warn("failed to cleanup instance", "id", id, "err", err)
		}
	}
	return nil
}

// GetRuntimeInfo returns information about the Firecracker runtime
func (f *FirecrackerRuntime) GetRuntimeInfo() *types.RuntimeInfo {
	return &types.RuntimeInfo{
		Type:    "firecracker",
		Version: firecrackerVersion(),
		Status:  "ok",
	}
}

// Helper methods

// createTAPDevice creates a TAP network device (requires root)
func (f *FirecrackerRuntime) createTAPDevice(name string) error {
	if err := runCommand("ip", "tuntap", "add", "dev", name, "mode", "tap"); err != nil {
		return err
	}
	_ = runCommand("ip", "link", "set", name, "up")
	_ = runCommand("ip", "link", "set", name, "master", "br-zeitwork")
	return nil
}

// tapDeviceExists checks if a TAP device already exists
func (f *FirecrackerRuntime) tapDeviceExists(name string) bool {
	_, err := net.InterfaceByName(name)
	return err == nil
}

// runCommand executes a system command
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// deleteTAPDevice removes a TAP network device
func (f *FirecrackerRuntime) deleteTAPDevice(name string) error {
	return runCommand("ip", "link", "del", name)
}

// cleanupOrphanedTAPDevices removes orphaned TAP devices from previous runs
func (f *FirecrackerRuntime) cleanupOrphanedTAPDevices() error { return nil }

// getClient retrieves the firecracker client for an instance
func (f *FirecrackerRuntime) getClient(instanceID string) (*Client, error) {
	f.mutex.RLock()
	defer f.mutex.RUnlock()
	c, ok := f.instances[instanceID]
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}
	return c, nil
}

// Client and its methods are defined in client.go

// vmConfigData is the template model for Firecracker JSON config
type vmConfigData struct {
	KernelImagePath string
	BootArgs        string
	RootfsPath      string
	TapDevice       string
	VCPUCount       int32
	MemSizeMiB      int32
	LogPath         string
	MetricsPath     string
}

func renderVMConfigToFile(path string, data vmConfigData) error {
	t, err := template.New("vm-config").Parse(vmConfigTemplate)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if err := t.Execute(f, data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func instanceBaseDir() string {
	if v := os.Getenv("ZEITWORK_FC_BASE"); v != "" {
		return v
	}
	return "/var/lib/zeitwork/firecracker/instances"
}

func defaultResources(cfg *config.FirecrackerRuntimeConfig, spec *types.InstanceSpec) *types.ResourceSpec {
	if spec != nil && spec.Resources != nil {
		r := *spec.Resources
		if r.VCPUs == 0 {
			r.VCPUs = cfg.DefaultVCpus
		}
		if r.Memory == 0 {
			r.Memory = cfg.DefaultMemoryMB
		}
		return &r
	}
	return &types.ResourceSpec{
		VCPUs:  cfg.DefaultVCpus,
		Memory: cfg.DefaultMemoryMB,
	}
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func tapNameFor(id string) string {
	// Use a hash to ensure uniqueness within 15 char limit
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	hash := h.Sum32()
	// Create unique name: tap-zw-XXXXXXXX (15 chars max)
	return fmt.Sprintf("tap-zw-%08x", hash)
}

func mkfifo(path string, mode uint32) error {
	if err := exec.Command("mkfifo", path).Run(); err == nil {
		return nil
	}
	return fmt.Errorf("mkfifo failed for %s", path)
}

func tailFIFOToFile(fifoPath, outPath string) {
	for {
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		f, err := os.OpenFile(fifoPath, os.O_RDONLY, 0)
		if err != nil {
			_ = out.Close()
			time.Sleep(time.Second)
			continue
		}
		_, _ = io.Copy(out, f)
		_ = f.Close()
		_ = out.Close()
		time.Sleep(200 * time.Millisecond)
	}
}

// Firecracker API helper moved to client.go

func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	if _, err := io.Copy(d, s); err != nil {
		return err
	}
	return d.Sync()
}

// GenerateIPv6AddressFromID creates a stable IPv6 within fd00:42::/64 using a hash of the instance ID
func GenerateIPv6AddressFromID(instanceID string) string {
	const prefix = "fd00:42::"
	h := fnv.New32a()
	_, _ = h.Write([]byte(instanceID))
	val := h.Sum32()
	// Map into low 16 bits to get a compact suffix
	suffix := int(val & 0xFFFF)
	return fmt.Sprintf("%s%x", prefix, 0x10+suffix)
}

// writeIPv6AddrToRootfs mounts the ext4 rootfs image, writes /etc/ipv6-addr with the given address, then unmounts.
func writeIPv6AddrToRootfs(rootfsPath, ipv6Addr string) error {
	mnt := rootfsPath + ".mnt"
	if err := os.MkdirAll(mnt, 0o755); err != nil {
		return err
	}
	defer func() { _ = os.Remove(mnt) }()
	if err := exec.Command("mount", "-o", "loop", rootfsPath, mnt).Run(); err != nil {
		return err
	}
	defer func() { _ = exec.Command("umount", mnt).Run() }()
	etcDir := filepath.Join(mnt, "etc")
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(etcDir, "ipv6-addr"), []byte(ipv6Addr), 0o644)
}

// Helper: prepare rootfs with image and launcher
func (f *FirecrackerRuntime) prepareRootfsWithImage(rootfsPath string, imageTag string, env map[string]string, defaultPort int32) error {
	mnt := rootfsPath + ".mnt"
	if err := os.MkdirAll(mnt, 0o755); err != nil {
		return err
	}
	defer func() { _ = os.Remove(mnt) }()
	if err := exec.Command("mount", "-o", "loop", rootfsPath, mnt).Run(); err != nil {
		return fmt.Errorf("mount rootfs: %w", err)
	}
	defer func() { _ = exec.Command("umount", mnt).Run() }()

	// Ensure directories
	appDir := filepath.Join(mnt, "app")
	etcZw := filepath.Join(mnt, "etc", "zeitwork")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(etcZw, 0o755); err != nil {
		return err
	}

	// Export docker image filesystem into /app
	containerIDBytes, err := exec.Command("docker", "create", imageTag).CombinedOutput()
	if err != nil {
		f.logger.Error("docker create failed", "image", imageTag, "out", string(containerIDBytes), "err", err)
		return fmt.Errorf("docker create: %w", err)
	}
	containerID := strings.TrimSpace(string(containerIDBytes))
	defer func() { _ = exec.Command("docker", "rm", "-f", containerID).Run() }()

	// Use docker export with separate arguments to avoid shell escaping issues
	exportCmd := exec.Command("docker", "export", containerID)
	exportOut, err := exportCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("docker export stdout pipe: %w", err)
	}

	tarCmd := exec.Command("tar", "-C", appDir, "-x")
	tarCmd.Stdin = exportOut

	if err := exportCmd.Start(); err != nil {
		return fmt.Errorf("docker export start: %w", err)
	}

	if err := tarCmd.Run(); err != nil {
		f.logger.Error("tar extract failed", "err", err)
		return fmt.Errorf("tar extract: %w", err)
	}

	if err := exportCmd.Wait(); err != nil {
		f.logger.Error("docker export failed", "err", err)
		return fmt.Errorf("docker export: %w", err)
	}

	// // Write env as shell-friendly file (host and inside image)
	// if env == nil {
	// 	env = map[string]string{}
	// }
	// if defaultPort > 0 {
	// 	if _, ok := env["PORT"]; !ok {
	// 		env["PORT"] = fmt.Sprintf("%d", defaultPort)
	// 	}
	// }
	// if _, ok := env["HOST"]; !ok {
	// 	env["HOST"] = "0.0.0.0"
	// }
	// if _, ok := env["NITRO_HOST"]; !ok {
	// 	env["NITRO_HOST"] = "0.0.0.0"
	// }
	// if _, ok := env["NUXT_HOST"]; !ok {
	// 	env["NUXT_HOST"] = "0.0.0.0"
	// }
	// var bldr strings.Builder
	// for k, v := range env {
	// 	esc := strings.ReplaceAll(v, "'", "'\\''")
	// 	fmt.Fprintf(&bldr, "export %s='%s'\n", k, esc)
	// }
	// if err := os.WriteFile(filepath.Join(etcZw, "env.sh"), []byte(bldr.String()), 0o644); err != nil {
	// 	return err
	// }
	// // also write inside image
	// etcZwApp := filepath.Join(appDir, "etc", "zeitwork")
	// if err := os.MkdirAll(etcZwApp, 0o755); err != nil {
	// 	return err
	// }
	// if err := os.WriteFile(filepath.Join(etcZwApp, "env.sh"), []byte(bldr.String()), 0o644); err != nil {
	// 	return err
	// }

	// Inspect image for entrypoint/cmd/workingdir
	entry, cmd, workingDir, err := dockerInspectImageConfig(imageTag)
	if err != nil {
		f.logger.Warn("docker inspect failed; using shell fallback", "image", imageTag, "err", err)
	}

	// Write OpenRC init script from template
	launcherPath := filepath.Join(mnt, "etc", "init.d", "zeitwork-app")
	if err := os.WriteFile(launcherPath, []byte(appInitdTemplate), 0o755); err != nil {
		return err
	}

	// Build argv and working directory inside chroot (image root)
	args := composeArgs(entry, cmd)
	if len(args) == 0 {
		args = []string{"/bin/sh", "-lc", "exec ./server || exec /bin/sh -c 'node server.js || ./app || sleep 1d'"}
	}
	cmdline := shellJoin(args)
	wd := workingDir
	if wd == "" {
		wd = "/"
	}

	// Create app-start.sh inside the image to avoid quoting issues
	appStartDir := filepath.Join(appDir, "usr", "local", "bin")
	if err := os.MkdirAll(appStartDir, 0o755); err != nil {
		return err
	}
	appStart := fmt.Sprintf(`#!/bin/sh
set -e
export PATH="/usr/local/bin:/usr/bin:/bin:$PATH"
[ -f /etc/zeitwork/env.sh ] && . /etc/zeitwork/env.sh || true
cd %s
exec %s
`, wd, cmdline)
	if err := os.WriteFile(filepath.Join(appStartDir, "app-start.sh"), []byte(appStart), 0o755); err != nil {
		return err
	}

	// Write host-side runner that chroots into the image and runs app-start.sh
	runScript := `#!/bin/sh
set -e
exec chroot /app /bin/sh /usr/local/bin/app-start.sh
`
	runPath := filepath.Join(mnt, "usr", "local", "bin", "zeitwork-run")
	if err := os.MkdirAll(filepath.Dir(runPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(runPath, []byte(runScript), 0o755); err != nil {
		return err
	}

	// Enable service
	_ = exec.Command("chroot", mnt, "rc-update", "add", "zeitwork-app", "default").Run()
	return nil
}

func dockerInspectImageConfig(imageTag string) ([]string, []string, string, error) {
	cmd := exec.Command("docker", "image", "inspect", imageTag, "--format", "{{json .Config}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, nil, "", err
	}
	var cfg struct {
		Entrypoint []string `json:"Entrypoint"`
		Cmd        []string `json:"Cmd"`
		WorkingDir string   `json:"WorkingDir"`
	}
	if err := json.Unmarshal(out, &cfg); err != nil {
		return nil, nil, "", err
	}
	return cfg.Entrypoint, cfg.Cmd, cfg.WorkingDir, nil
}

func composeArgs(entry, cmd []string) []string {
	out := []string{}
	out = append(out, entry...)
	out = append(out, cmd...)
	return out
}

func shellEscape(s string) string {
	// Simple shell escaping for single quotes
	return strings.ReplaceAll(s, "'", "'\\''")
}

func shellJoin(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		if strings.ContainsAny(a, " \"'$") {
			parts = append(parts, fmt.Sprintf("'%s'", shellEscape(a)))
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}
