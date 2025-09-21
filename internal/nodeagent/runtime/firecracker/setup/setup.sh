#!/usr/bin/env bash
set -euo pipefail

# Idempotent Firecracker setup for Zeitwork nodes (IPv6-first)
# Run as root before starting the nodeagent.

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'
log() { echo -e "${BLUE}[$(date '+%Y-%m-%d %H:%M:%S')] $*${NC}"; }
ok() { echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')] ✓ $*${NC}"; }
warn() { echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] ⚠ $*${NC}"; }
err() { echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] ✗ $*${NC}"; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

KERNEL_VERSION="${KERNEL_VERSION:-6.1}"
ALPINE_VERSION="${ALPINE_VERSION:-3.19}"
IPV6_PREFIX="${IPV6_PREFIX:-fd00:42::}"
WORK_DIR="${WORK_DIR:-/var/lib/zeitwork/firecracker}"
DEFAULT_KERNEL="${DEFAULT_KERNEL:-${WORK_DIR}/zeitwork-vmlinux.bin}"
DEFAULT_ROOTFS="${DEFAULT_ROOTFS:-${WORK_DIR}/zeitwork-rootfs.img}"
KERNEL_DEFAULT_CONFIG="${KERNEL_DEFAULT_CONFIG:-${SCRIPT_DIR}/kernel.default.config}"
KERNEL_PATCH_CONFIG="${KERNEL_PATCH_CONFIG:-${SCRIPT_DIR}/kernel.zwpatch.config}"

require_root() { if [[ $EUID -ne 0 ]]; then err "Please run as root"; exit 1; fi; }

cleanup_existing() {
  log "Cleaning up existing Firecracker instances, taps, sockets, and jailer dirs"
  # Make cleanup fully idempotent and avoid self-termination
  set +e
  pkill -x firecracker 2>/dev/null || true
  pkill -x jailer 2>/dev/null || true
  for tap in $(ip -o link show | awk -F: '{print $2}' | tr -d ' ' | grep -E '^tap-' || true); do ip link del "$tap" 2>/dev/null || true; done
  rm -f /tmp/firecracker*.socket /run/firecracker*.socket 2>/dev/null || true
  rm -rf /srv/jailer/firecracker/* 2>/dev/null || true
  set -e
  ok "Cleanup completed"
}

install_deps() {
  log "Installing dependencies"
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y
  apt-get install -y --no-install-recommends \
    build-essential libssl-dev pkg-config curl git bc flex bison libelf-dev \
    squashfs-tools docker.io socat bridge-utils iptables iproute2 jq wget file busybox-static
  systemctl enable --now docker || true
  ok "Dependencies installed"
}

download_firecracker() {
  log "Downloading Firecracker binaries"
  mkdir -p "${WORK_DIR}/binaries"
  local arch; arch=$(uname -m)
  local rel=https://github.com/firecracker-microvm/firecracker/releases
  local latest; latest=$(basename $(curl -fsSLI -o /dev/null -w %{url_effective} ${rel}/latest))
  cd "${WORK_DIR}/binaries"
  if [[ ! -f firecracker ]]; then
    curl -L ${rel}/download/${latest}/firecracker-${latest}-${arch}.tgz | tar -xz
    mv release-${latest}-${arch}/firecracker-${latest}-${arch} firecracker
    mv release-${latest}-${arch}/jailer-${latest}-${arch} jailer || true
    chmod +x firecracker jailer || true
    rm -rf release-${latest}-${arch}
  fi
  ok "Firecracker ready"
}

build_kernel() {
  log "Building kernel ${KERNEL_VERSION} (Firecracker + Zeitwork optimized)"
  if [[ ! -f "${KERNEL_DEFAULT_CONFIG}" ]]; then
    err "Default kernel config not found at ${KERNEL_DEFAULT_CONFIG}"
    exit 1
  fi
  if [[ ! -f "${KERNEL_PATCH_CONFIG}" ]]; then
    err "Zeitwork patch config not found at ${KERNEL_PATCH_CONFIG}"
    exit 1
  fi
  mkdir -p "${WORK_DIR}/kernel"
  cd "${WORK_DIR}/kernel"
  if [[ ! -d linux ]]; then
    git clone --depth 1 --branch v${KERNEL_VERSION} https://github.com/torvalds/linux.git
  fi
  cd linux
  # Combine default Firecracker config with Zeitwork patches
  cat "${KERNEL_DEFAULT_CONFIG}" "${KERNEL_PATCH_CONFIG}" > .config
  make olddefconfig
  make -j"$(nproc)" vmlinux
  install -Dm644 vmlinux "${DEFAULT_KERNEL}"
  ok "Kernel installed at ${DEFAULT_KERNEL}"
}

create_rootfs() {
  log "Creating base Alpine rootfs ${ALPINE_VERSION}"
  mkdir -p "${WORK_DIR}/rootfs"
  cd "${WORK_DIR}/rootfs"
  if [[ ! -f "${DEFAULT_ROOTFS}" ]]; then
    dd if=/dev/zero of="${DEFAULT_ROOTFS}" bs=1M count=10240
    mkfs.ext4 "${DEFAULT_ROOTFS}"
    mkdir -p rootfs-mount
    mount "${DEFAULT_ROOTFS}" rootfs-mount
    docker run --rm -v "$(pwd)/rootfs-mount:/my-rootfs" alpine:${ALPINE_VERSION} sh -c '
      apk add --no-cache openrc util-linux busybox-extras iproute2 iputils wget curl
      ln -s agetty /etc/init.d/agetty.ttyS0
      echo ttyS0 > /etc/securetty
      rc-update add agetty.ttyS0 default
      rc-update add devfs boot; rc-update add procfs boot; rc-update add sysfs boot
      for d in bin etc lib root sbin usr; do tar c "/$d" | tar x -C /my-rootfs; done
      for dir in dev proc run sys var tmp; do mkdir -p /my-rootfs/${dir}; done
      chmod +x /my-rootfs/sbin/init || true
      # Create OpenRC service for Zeitwork IPv6 configuration
      cat > /my-rootfs/etc/init.d/zeitwork-network <<"EOF"
#!/sbin/openrc-run
name="zeitwork-network"
description="Zeitwork IPv6 Network Configuration"
depend() {
    need localmount
    before net
}
start() {
    ebegin "Configuring Zeitwork IPv6 network"
    ip link set lo up
    ip link set eth0 up
    if [ -f /etc/ipv6-addr ]; then
        IPV6_ADDR=$(cat /etc/ipv6-addr)
        einfo "Setting IPv6 address: ${IPV6_ADDR}"
        ip -6 addr add ${IPV6_ADDR}/64 dev eth0 2>/dev/null || true
        ip -6 route add default via fd00:42::1 dev eth0 2>/dev/null || true
    fi
    eend $?
}
EOF
      chmod +x /my-rootfs/etc/init.d/zeitwork-network
      chroot /my-rootfs rc-update add zeitwork-network boot
      # Minimal interfaces file
      cat > /my-rootfs/etc/network/interfaces <<"EOF"
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet manual
EOF
      # Base image contains only networking service; app will be layered at runtime
    '
    umount rootfs-mount
    rmdir rootfs-mount
  fi
  ok "Rootfs ready at ${DEFAULT_ROOTFS}"
}

setup_ipv6() {
  log "Configuring IPv6 bridge br-zeitwork (${IPV6_PREFIX}/64)"
  ip link set br-zeitwork down 2>/dev/null || true
  ip link del br-zeitwork 2>/dev/null || true
  ip link add name br-zeitwork type bridge
  ip link set br-zeitwork up
  ip -6 addr add ${IPV6_PREFIX}1/64 dev br-zeitwork || true
  sysctl -w net.ipv6.conf.all.forwarding=1 >/dev/null
  if command -v ip6tables >/dev/null 2>&1; then
    ip6tables -t nat -D POSTROUTING -s ${IPV6_PREFIX}/64 -j MASQUERADE 2>/dev/null || true
    ip6tables -D FORWARD -i br-zeitwork -j ACCEPT 2>/dev/null || true
    ip6tables -D FORWARD -o br-zeitwork -j ACCEPT 2>/dev/null || true
    ip6tables -t nat -A POSTROUTING -s ${IPV6_PREFIX}/64 -j MASQUERADE || true
    ip6tables -A FORWARD -i br-zeitwork -j ACCEPT || true
    ip6tables -A FORWARD -o br-zeitwork -j ACCEPT || true
  fi
  ok "IPv6 bridge configured"
}

ensure_dirs() {
  log "Ensuring work directories"
  mkdir -p "${WORK_DIR}" "${WORK_DIR}/kernel" "${WORK_DIR}/rootfs" "${WORK_DIR}/binaries"
  ok "Directories ready"
}

main() {
  require_root
  ensure_dirs
  cleanup_existing
  install_deps
  download_firecracker
  build_kernel
  create_rootfs
  setup_ipv6
  ok "Zeitwork Firecracker setup complete"
}

main "$@"


