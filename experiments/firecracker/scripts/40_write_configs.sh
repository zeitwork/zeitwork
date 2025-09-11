#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "${SCRIPT_DIR}/common.sh"

log "Writing firecracker-containerd config.toml"
run "sudo mkdir -p /etc/firecracker-containerd /var/lib/firecracker-containerd/containerd /run/firecracker-containerd"
sudo tee /etc/firecracker-containerd/config.toml >/dev/null <<'EOF'
version = 2
disabled_plugins = ["io.containerd.grpc.v1.cri"]
root = "/var/lib/firecracker-containerd/containerd"
state = "/run/firecracker-containerd"
[grpc]
  address = "/run/firecracker-containerd/containerd.sock"
[plugins]
  [plugins."io.containerd.snapshotter.v1.devmapper"]
    pool_name = "fc-dev-thinpool"
    base_image_size = "10GB"
    root_path = "/var/lib/firecracker-containerd/snapshotter/devmapper"

[debug]
  level = "debug"
EOF

log "Writing runtime JSON with IPv6 default network fcnet6"
run "sudo mkdir -p /etc/containerd"
sudo tee /etc/containerd/firecracker-runtime.json >/dev/null <<'EOF'
{
  "firecracker_binary_path": "/usr/local/bin/firecracker",
  "kernel_image_path": "/var/lib/firecracker-containerd/runtime/default-vmlinux.bin",
  "kernel_args": "console=ttyS0 noapic reboot=k panic=1 pci=off nomodules ro systemd.unified_cgroup_hierarchy=0 systemd.journald.forward_to_console systemd.unit=firecracker.target init=/sbin/overlay-init",
  "root_drive": "/var/lib/firecracker-containerd/runtime/default-rootfs.img",
  "cpu_template": "C3",
  "log_fifo": "fc-logs.fifo",
  "log_levels": ["debug"],
  "metrics_fifo": "fc-metrics.fifo",
  "debug": true,
  "shim_base_dir": "/var/lib/firecracker-containerd/shim-base",
  "default_network_interfaces": [
    {
      "CNIConfig": { "NetworkName": "fcnet6", "InterfaceName": "veth0" }
    }
  ]
}
EOF

log "Configs written"


