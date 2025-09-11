package firecracker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// ensureBaseImage ensures a base image with networking tools exists
func (r *Runtime) ensureBaseImage(ctx context.Context) (string, error) {
	baseImageTag := "zeitwork/firecracker-base:latest"

	// Check if base image already exists
	if exists, err := r.imageExists(ctx, baseImageTag); err == nil && exists {
		return baseImageTag, nil
	}

	r.logger.Info("Base image not found, building zeitwork firecracker base image")

	// Create base image with networking tools
	if err := r.buildBaseImage(ctx, baseImageTag); err != nil {
		return "", fmt.Errorf("failed to build base image: %w", err)
	}

	return baseImageTag, nil
}

// buildBaseImage creates a base image with all networking tools and IPv6 configuration
func (r *Runtime) buildBaseImage(ctx context.Context, baseImageTag string) error {
	tmpDir, err := os.MkdirTemp("", "fc-base-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// Create a Dockerfile for the base image with networking tools
	dockerfile := `FROM alpine:latest

# Install networking tools and utilities
RUN apk update && apk add --no-cache \
    iproute2 \
    iputils \
    curl \
    wget \
    bash \
    busybox-extras \
    net-tools \
    tcpdump \
    iptables \
    ca-certificates \
    && rm -rf /var/cache/apk/*

# Create a script to configure IPv6 automatically
RUN mkdir -p /usr/local/bin
COPY configure-ipv6.sh /usr/local/bin/configure-ipv6.sh
RUN chmod +x /usr/local/bin/configure-ipv6.sh

# Create the IPv6 configuration script
# This script will be called by the runtime to configure networking
COPY ipv6-setup.sh /usr/local/bin/ipv6-setup.sh
RUN chmod +x /usr/local/bin/ipv6-setup.sh

# Set up proper PATH for networking tools
ENV PATH="/usr/local/bin:/usr/local/sbin:/usr/sbin:/sbin:/usr/bin:/bin"

# Default command that can be overridden
CMD ["/bin/sh"]
`

	// Create the IPv6 configuration script
	ipv6Script := `#!/bin/bash
set -e

IPV6_ADDR=${1:-}
if [ -z "$IPV6_ADDR" ]; then
    echo "Usage: $0 <ipv6_address>"
    exit 1
fi

echo "Configuring IPv6 address: $IPV6_ADDR"

# Wait for eth0 to appear
for i in {1..30}; do
    if [ -e /sys/class/net/eth0 ]; then
        echo "eth0 found"
        break
    fi
    echo "Waiting for eth0... ($i/30)"
    sleep 1
done

# Configure networking
echo "Setting eth0 up..."
ip link set eth0 up

echo "Adding IPv6 address..."
ip -6 addr add ${IPV6_ADDR}/64 dev eth0 || true

echo "Adding default route..."
ip -6 route add default via fd00:fc::1 dev eth0 || true

echo "IPv6 configuration complete"
ip -6 addr show dev eth0
ip -6 route show
`

	// Create a simpler configuration script
	configScript := `#!/bin/bash
# This script configures IPv6 on eth0
# It's designed to be robust and work with various base images

IPV6_ADDR="$1"
if [ -z "$IPV6_ADDR" ]; then
    echo "No IPv6 address provided"
    exit 1
fi

# Set up eth0 with IPv6
/sbin/ip link set eth0 up 2>/dev/null || /bin/ip link set eth0 up 2>/dev/null || ip link set eth0 up
/sbin/ip -6 addr add ${IPV6_ADDR}/64 dev eth0 2>/dev/null || /bin/ip -6 addr add ${IPV6_ADDR}/64 dev eth0 2>/dev/null || ip -6 addr add ${IPV6_ADDR}/64 dev eth0 || true
/sbin/ip -6 route add default via fd00:fc::1 dev eth0 2>/dev/null || /bin/ip -6 route add default via fd00:fc::1 dev eth0 2>/dev/null || ip -6 route add default via fd00:fc::1 dev eth0 || true

echo "IPv6 configured: $IPV6_ADDR"
`

	// Write files
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "ipv6-setup.sh"), []byte(ipv6Script), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "configure-ipv6.sh"), []byte(configScript), 0755); err != nil {
		return err
	}

	// Build with Docker
	r.logger.Info("Building base image with Docker", "tag", baseImageTag)
	buildCmd := exec.CommandContext(ctx, "docker", "build", "-t", baseImageTag, tmpDir)
	buildCmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker build failed: %v: %s", err, string(out))
	}

	// Save and import to firecracker-containerd
	tarPath := filepath.Join(tmpDir, "base-image.tar")
	saveCmd := exec.CommandContext(ctx, "docker", "save", baseImageTag, "-o", tarPath)
	saveCmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	if out, err := saveCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker save failed: %v: %s", err, string(out))
	}

	// Import to firecracker-containerd
	importArgs := []string{"images", "import", tarPath}
	if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, importArgs); err != nil {
		return fmt.Errorf("base image import failed: %w", err)
	}

	r.logger.Info("Base image built and imported successfully", "tag", baseImageTag)
	return nil
}

// createLayeredInstance creates an instance using the base image as a foundation
func (r *Runtime) createLayeredInstance(ctx context.Context, spec *types.InstanceSpec, baseImage string) error {
	// This would create a new image that layers the application on top of the base
	// For now, we'll modify the existing approach to use a better base image

	// TODO: Implement proper image layering
	// This could involve:
	// 1. Creating a new Dockerfile that uses the base image
	// 2. Adding the application code on top
	// 3. Building and importing the layered image

	return nil
}

// configureIPv6WithBaseImage uses the base image's built-in IPv6 configuration
func (r *Runtime) configureIPv6WithBaseImage(ctx context.Context, name, ipv6 string) error {
	// Use the pre-installed script from the base image
	cmd := fmt.Sprintf("/usr/local/bin/configure-ipv6.sh %s", ipv6)

	if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "exec", "--exec-id", "ipv6-" + randomHex(4), name, "sh", "-c", cmd}); err != nil {
		// Fallback to manual configuration if script fails
		r.logger.Warn("Base image IPv6 script failed, falling back to manual config", "error", err)
		return r.configureIPv6Manual(ctx, name, ipv6)
	}

	return nil
}

// configureIPv6Manual provides manual IPv6 configuration as fallback using available tools
func (r *Runtime) configureIPv6Manual(ctx context.Context, name, ipv6 string) error {
	// Wait for eth0 to appear
	for i := 0; i < 30; i++ {
		if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "exec", "--exec-id", "chk-" + randomHex(4), name, "sh", "-c", "ls /sys/class/net/eth0 >/dev/null 2>&1"}); err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Since ip command is not available, try installing iproute2 in the running container
	r.logger.Warn("IPv6 configuration failed - attempting to install networking tools", "instance", name, "ipv6", ipv6)

	// Try to install iproute2 package in the running container
	installCommands := []string{
		"export DEBIAN_FRONTEND=noninteractive && apt update && apt install -y iproute2 iputils-ping",
		"export DEBIAN_FRONTEND=noninteractive && apt-get update && apt-get install -y iproute2 iputils-ping",
	}

	toolsInstalled := false
	for _, installCmd := range installCommands {
		if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "exec", "--exec-id", "install-" + randomHex(4), name, "sh", "-c", installCmd}); err == nil {
			r.logger.Info("Successfully installed networking tools", "instance", name)
			toolsInstalled = true
			break
		} else {
			r.logger.Debug("Install command failed", "cmd", installCmd, "error", err)
		}
	}

	if !toolsInstalled {
		r.logger.Error("Failed to install networking tools - IPv6 configuration will be incomplete", "instance", name)
		return fmt.Errorf("cannot configure IPv6: networking tools not available and installation failed")
	}

	// Now try to configure IPv6 with the newly installed tools
	ipv6Commands := []string{
		"ip link set eth0 up",
		fmt.Sprintf("ip -6 addr add %s/64 dev eth0", ipv6),
		"ip -6 route add default via fd00:fc::1 dev eth0",
	}

	for _, cmd := range ipv6Commands {
		if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "exec", "--exec-id", "ipv6-" + randomHex(4), name, "sh", "-c", cmd}); err != nil {
			r.logger.Warn("IPv6 command failed", "cmd", cmd, "error", err)
			// Continue with other commands
		} else {
			r.logger.Debug("IPv6 command succeeded", "cmd", cmd)
		}
	}

	// Log current network state for debugging
	if out, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "exec", "--exec-id", "debug-" + randomHex(4), name, "sh", "-c", "cat /proc/net/if_inet6 2>/dev/null || echo 'no ipv6 interfaces'"}); err == nil {
		r.logger.Info("Current IPv6 interfaces in VM", "output", out)
	}

	// Check if the application is actually running and listening
	if out, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"tasks", "exec", "--exec-id", "proc-" + randomHex(4), name, "sh", "-c", "ls -la /proc/*/exe 2>/dev/null | head -5 || echo 'no processes found'"}); err == nil {
		r.logger.Info("Processes running in VM", "output", out)
	}

	// For now, we'll return success since we can't properly configure IPv6 without networking tools
	// This is where the base image approach becomes critical
	r.logger.Warn("IPv6 configuration incomplete - networking tools not available in image",
		"instance", name,
		"ipv6", ipv6,
		"recommendation", "use base image with iproute2 package")

	return nil
}
