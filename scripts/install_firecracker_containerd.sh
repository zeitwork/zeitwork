#!/usr/bin/env bash

# Automated setup for Firecracker + containerd following the project guide.
# - Installs dependencies (Docker, Go, build tools, dmsetup, etc.)
# - Clones and builds firecracker-containerd and Firecracker
# - Builds Debian rootfs image and installs it to default path
# - Downloads kernel image to default path
# - Sets up devmapper thinpool for snapshotter
# - Writes firecracker-containerd config + runtime JSON
# - Creates and starts a systemd unit for firecracker-containerd
# - Optional: CNI demo network + default_network_interfaces
# - Optional: Smoke test with busybox

set -Eeuo pipefail
shopt -s extglob

SCRIPT_NAME=$(basename "$0")
START_TIME=$(date +%s)

REPO_URL="https://github.com/firecracker-microvm/firecracker-containerd"
REPO_DIR="/opt/firecracker-containerd"
INSTALLROOT="/usr/local"

FC_BASE_DIR="/var/lib/firecracker-containerd"
FC_RUNTIME_DIR="${FC_BASE_DIR}/runtime"
FC_SNAPSHOTTER_DIR="${FC_BASE_DIR}/snapshotter/devmapper"
FC_KERNEL_PATH="${FC_RUNTIME_DIR}/hello-vmlinux.bin"
FC_ROOTFS_PATH="${FC_RUNTIME_DIR}/default-rootfs.img"

FCCD_CONFIG_DIR="/etc/firecracker-containerd"
FCCD_CONFIG_TOML="${FCCD_CONFIG_DIR}/config.toml"
FCCD_SOCKET="/run/firecracker-containerd/containerd.sock"
FCCD_SERVICE="firecracker-containerd.service"
FCCD_SERVICE_PATH="/etc/systemd/system/${FCCD_SERVICE}"

RUNTIME_JSON_DIR="/etc/containerd"
RUNTIME_JSON_PATH="${RUNTIME_JSON_DIR}/firecracker-runtime.json"

DOCKER_GROUP="docker"

# Defaults (can be overridden by flags)
WITH_CNI="false"
WITH_SMOKE="false"
NONINTERACTIVE="true"

log_info()  { printf "\033[1;34m[INFO]\033[0m %s\n" "$*"; }
log_warn()  { printf "\033[1;33m[WARN]\033[0m %s\n" "$*"; }
log_error() { printf "\033[1;31m[ERROR]\033[0m %s\n" "$*" 1>&2; }
log_success(){ printf "\033[1;32m[DONE]\033[0m %s\n" "$*"; }

on_error() {
  local exit_code=$1
  local line_no=$2
  log_error "Failed at line ${line_no} with exit code ${exit_code}"
  log_error "See logs above for context. ${SCRIPT_NAME} aborted."
  exit "${exit_code}"
}
trap 'on_error $? $LINENO' ERR

usage() {
  cat <<EOF
${SCRIPT_NAME} - Firecracker + containerd automated setup (Ubuntu only)

Usage: sudo bash ${SCRIPT_NAME} [options]

Options:
  --with-cni        Install demo CNI setup and set as default network (optional)
  --with-smoke      Run a smoke test (pull busybox + run a command)
  --repo-dir DIR    Clone/build directory (default: ${REPO_DIR})
  --no-noninteractive  Allow interactive apt behavior
  -h, --help        Show this help
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --with-cni)
        WITH_CNI="true"; shift ;;
      --with-smoke)
        WITH_SMOKE="true"; shift ;;
      --repo-dir)
        REPO_DIR="$2"; shift 2 ;;
      --no-noninteractive)
        NONINTERACTIVE="false"; shift ;;
      -h|--help)
        usage; exit 0 ;;
      *)
        log_error "Unknown option: $1"; usage; exit 2 ;;
    esac
  done
}

require_root() {
  if [[ $EUID -ne 0 ]]; then
    log_info "Re-executing with sudo..."
    exec sudo -E -H bash "$0" "$@"
  fi
}

retry() {
  local -r max_attempts=${3:-5}
  local -r delay_seconds=${4:-2}
  local attempt=1
  local cmd_desc="$1"
  shift
  local cmd=("$@")
  until "${cmd[@]}"; do
    local rc=$?
    if (( attempt >= max_attempts )); then
      log_error "${cmd_desc} failed after ${attempt} attempts (exit ${rc})"
      return ${rc}
    fi
    log_warn "${cmd_desc} failed (attempt ${attempt}/${max_attempts}), retrying in ${delay_seconds}s..."
    sleep "${delay_seconds}"
    ((attempt++))
  done
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

install_packages() {
  local -a pkgs=(git make gcc curl ca-certificates gnupg bc dmsetup util-linux lsb-release tar)
  export DEBIAN_FRONTEND=noninteractive
  [[ "$NONINTERACTIVE" == "true" ]] && APT_FLAGS=(-y -o Dpkg::Options::=--force-confnew) || APT_FLAGS=()
  retry "apt-get update" apt-get update
  retry "apt-get install base packages" apt-get install "${APT_FLAGS[@]}" "${pkgs[@]}"
}

ensure_docker() {
  if command_exists docker; then
    log_success "Docker already installed"
    return
  fi
  log_info "Installing Docker CE..."
  . /etc/os-release || true
  local id=${ID:-ubuntu}
  local codename=${VERSION_CODENAME:-$(lsb_release -cs 2>/dev/null || echo focal)}
  install -m 0755 -d /etc/apt/keyrings
  retry "download docker gpg key" bash -lc "curl -fsSL https://download.docker.com/linux/${id}/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg"
  chmod a+r /etc/apt/keyrings/docker.gpg
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${id} ${codename} stable" \
    | tee /etc/apt/sources.list.d/docker.list >/dev/null
  export DEBIAN_FRONTEND=noninteractive
  retry "apt-get update (docker repo)" apt-get update
  retry "apt-get install docker" apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  systemctl enable --now docker
}

ensure_user_in_docker_group() {
  # Add the invoking non-root user to docker group if possible
  local target_user=${SUDO_USER:-${USER}}
  if id -nG "$target_user" 2>/dev/null | grep -qw "$DOCKER_GROUP"; then
    return
  fi
  if getent group "$DOCKER_GROUP" >/dev/null; then
    usermod -aG "$DOCKER_GROUP" "$target_user" || true
  else
    groupadd "$DOCKER_GROUP"
    usermod -aG "$DOCKER_GROUP" "$target_user" || true
  fi
}

go_version_ok() {
  if ! command_exists go; then return 1; fi
  local gv
  gv=$(go version | awk '{print $3}')
  # Expect form go1.X[.Y]
  if [[ "$gv" =~ ^go([0-9]+)\.([0-9]+) ]]; then
    local major=${BASH_REMATCH[1]}
    local minor=${BASH_REMATCH[2]}
    if (( major > 1 || (major == 1 && minor >= 23) )); then
      return 0
    fi
  fi
  return 1
}

install_go() {
  if go_version_ok; then
    log_success "Go $(go version | awk '{print $3}') already installed and sufficient"
    return
  fi
  log_info "Installing Go >= 1.23..."
  local ver
  ver=$(curl -fsSL https://go.dev/VERSION?m=text | head -n1 || true)
  if [[ ! "$ver" =~ ^go1\.[0-9]+(\.[0-9]+)?$ ]]; then
    ver="go1.23.0"
  fi
  if [[ "$ver" =~ ^go1\.([0-9]+) ]]; then
    local minor=${BASH_REMATCH[1]}
    if (( minor < 23 )); then
      ver="go1.23.0"
    fi
  fi
  local url="https://go.dev/dl/${ver}.linux-amd64.tar.gz"
  local tmp_tgz
  tmp_tgz=$(mktemp -t go.XXXXXXXX.tar.gz)
  retry "download ${ver}" curl -fsSL -o "$tmp_tgz" "$url"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "$tmp_tgz"
  rm -f "$tmp_tgz"
  mkdir -p /etc/profile.d
  cat >/etc/profile.d/golang.sh <<'EOS'
export PATH=/usr/local/go/bin:$PATH
export GOPATH=${GOPATH:-/usr/local/go}
export PATH=$GOPATH/bin:$PATH
EOS
  export PATH=/usr/local/go/bin:$PATH
}

check_prereqs() {
  local err=""
  if [[ "$(uname) $(uname -m)" != "Linux x86_64" ]]; then
    err+=$'ERROR: your system is not Linux x86_64.\n'
  fi
  if [[ ! -e /dev/kvm || ! -r /dev/kvm || ! -w /dev/kvm ]]; then
    err+=$'ERROR: /dev/kvm is inaccessible.\n'
  fi
  local kmajor kminor
  kmajor=$(uname -r | cut -d. -f1)
  kminor=$(uname -r | cut -d. -f2)
  if (( kmajor*1000 + kminor < 4014 )); then
    err+=$"ERROR: kernel version $(uname -r) is too old (need >= 4.14).\n"
  fi
  if dmesg | grep -iq "hypervisor detected"; then
    log_warn "Running inside a VM; nested virtualization may be unstable."
  fi
  if [[ -n "$err" ]]; then
    printf "%s" "$err" >&2
    exit 1
  fi
  log_success "Host looks ready for Firecracker"
}

clone_and_build() {
  mkdir -p "$REPO_DIR"
  if [[ ! -d "${REPO_DIR}/.git" ]]; then
    log_info "Cloning ${REPO_URL} into ${REPO_DIR}..."
    retry "git clone" git clone --recurse-submodules "$REPO_URL" "$REPO_DIR"
  else
    log_info "Updating repository at ${REPO_DIR}..."
    git -C "$REPO_DIR" fetch --all --tags --recurse-submodules
    git -C "$REPO_DIR" pull --recurse-submodules
    git -C "$REPO_DIR" submodule update --init --recursive
  fi

  log_info "Building firecracker-containerd components..."
  (cd "$REPO_DIR" && \
    export GO111MODULE=on && \
    make all)

  log_info "Installing firecracker-containerd binaries to ${INSTALLROOT}..."
  (cd "$REPO_DIR" && make install INSTALLROOT="$INSTALLROOT")

  log_info "Building + installing Firecracker + jailer..."
  (cd "$REPO_DIR" && make install-firecracker INSTALLROOT="$INSTALLROOT")
}

build_rootfs_image() {
  log_info "Building VM rootfs image (Docker required)..."
  # Pin image-builder to Debian bookworm to satisfy agent GLIBC requirements
  if [[ -f "$REPO_DIR/tools/image-builder/Makefile" ]]; then
    if grep -q '\bbullseye\b' "$REPO_DIR/tools/image-builder/Makefile"; then
      log_info "Switching debootstrap release from bullseye to bookworm"
      perl -0777 -pe 's/\bbullseye\b/bookworm/g' -i "$REPO_DIR/tools/image-builder/Makefile"
    fi
  fi
  if [[ -f "$REPO_DIR/tools/image-builder/Dockerfile.debian-image" ]]; then
    if grep -q 'debian:bullseye-slim' "$REPO_DIR/tools/image-builder/Dockerfile.debian-image"; then
      log_info "Switching base image to debian:bookworm-slim"
      perl -0777 -pe 's/debian:bullseye-slim/debian:bookworm-slim/g' -i "$REPO_DIR/tools/image-builder/Dockerfile.debian-image"
    fi
  fi
  (cd "$REPO_DIR" && make image)
  mkdir -p "$FC_RUNTIME_DIR"
  install -m 0644 "$REPO_DIR/tools/image-builder/rootfs.img" "$FC_ROOTFS_PATH"
  log_success "Installed rootfs image to ${FC_ROOTFS_PATH}"
}

download_kernel() {
  mkdir -p "$FC_RUNTIME_DIR"
  local url="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
  retry "download kernel" curl -fsSL -o "$FC_KERNEL_PATH" "$url"
  log_success "Downloaded kernel to ${FC_KERNEL_PATH}"
}

write_firecracker_containerd_config() {
  mkdir -p "$FCCD_CONFIG_DIR"
  cat >"$FCCD_CONFIG_TOML" <<'EOT'
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
EOT
  log_success "Wrote ${FCCD_CONFIG_TOML}"
}

setup_devmapper_thinpool() {
  mkdir -p "$FC_SNAPSHOTTER_DIR"
  log_info "Setting up devmapper thinpool (loopback, not for production)..."

  local data_file="${FC_SNAPSHOTTER_DIR}/data"
  local meta_file="${FC_SNAPSHOTTER_DIR}/metadata"
  [[ -f "$data_file" ]] || truncate -s 100G "$data_file"
  [[ -f "$meta_file" ]] || truncate -s 2G "$meta_file"

  local datadev
  datadev=$(losetup --output NAME --noheadings --associated "$data_file" || true)
  if [[ -z "$datadev" ]]; then
    datadev=$(losetup --find --show "$data_file")
  fi

  local metadev
  metadev=$(losetup --output NAME --noheadings --associated "$meta_file" || true)
  if [[ -z "$metadev" ]]; then
    metadev=$(losetup --find --show "$meta_file")
  fi

  local SECTORSIZE=512
  local DATASIZE
  DATASIZE=$(blockdev --getsize64 -q "$datadev")
  local LENGTH_SECTORS
  LENGTH_SECTORS=$(bc <<< "${DATASIZE}/${SECTORSIZE}")
  local DATA_BLOCK_SIZE=128
  local LOW_WATER_MARK=32768
  local POOL="fc-dev-thinpool"
  local THINP_TABLE="0 ${LENGTH_SECTORS} thin-pool ${metadev} ${datadev} ${DATA_BLOCK_SIZE} ${LOW_WATER_MARK} 1 skip_block_zeroing"

  dmsetup reload "$POOL" --table "$THINP_TABLE" 2>/dev/null || dmsetup create "$POOL" --table "$THINP_TABLE"
  log_success "Thinpool ${POOL} ready"
}

write_runtime_json() {
  mkdir -p "$RUNTIME_JSON_DIR"
  if [[ "$WITH_CNI" == "true" ]]; then
    cat >"$RUNTIME_JSON_PATH" <<'EOJ'
{
  "firecracker_binary_path": "/usr/local/bin/firecracker",
  "kernel_image_path": "/var/lib/firecracker-containerd/runtime/hello-vmlinux.bin",
  "kernel_args": "console=ttyS0 noapic reboot=k panic=1 pci=off nomodules ro systemd.unified_cgroup_hierarchy=0 systemd.journald.forward_to_console systemd.unit=firecracker.target init=/sbin/overlay-init",
  "root_drive": "/var/lib/firecracker-containerd/runtime/default-rootfs.img",
  "cpu_template": "T2",
  "log_fifo": "fc-logs.fifo",
  "log_levels": ["debug"],
  "metrics_fifo": "fc-metrics.fifo",
  "default_network_interfaces": [
    {
      "CNIConfig": {
        "NetworkName": "fcnet",
        "InterfaceName": "veth0"
      }
    }
  ]
}
EOJ
  else
    cat >"$RUNTIME_JSON_PATH" <<'EOJ'
{
  "firecracker_binary_path": "/usr/local/bin/firecracker",
  "kernel_image_path": "/var/lib/firecracker-containerd/runtime/hello-vmlinux.bin",
  "kernel_args": "console=ttyS0 noapic reboot=k panic=1 pci=off nomodules ro systemd.unified_cgroup_hierarchy=0 systemd.journald.forward_to_console systemd.unit=firecracker.target init=/sbin/overlay-init",
  "root_drive": "/var/lib/firecracker-containerd/runtime/default-rootfs.img",
  "cpu_template": "T2",
  "log_fifo": "fc-logs.fifo",
  "log_levels": ["debug"],
  "metrics_fifo": "fc-metrics.fifo"
}
EOJ
  fi
  log_success "Wrote ${RUNTIME_JSON_PATH}"
}

setup_cni_demo() {
  if [[ "$WITH_CNI" != "true" ]]; then return; fi
  log_info "Installing demo CNI network (fcnet)..."
  (cd "$REPO_DIR" && make demo-network)
  log_success "Installed demo CNI (fcnet). Runtime configured to use it by default."
}

create_systemd_service() {
  cat >"$FCCD_SERVICE_PATH" <<EOF
[Unit]
Description=Firecracker-enabled containerd (firecracker-containerd)
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=${INSTALLROOT}/bin/firecracker-containerd --config ${FCCD_CONFIG_TOML}
Restart=on-failure
RestartSec=2
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now "$FCCD_SERVICE"
}

wait_for_socket() {
  local path="$1"; shift
  local timeout_sec=${1:-60}
  local waited=0
  while [[ ! -S "$path" ]]; do
    sleep 1
    ((waited++))
    if (( waited >= timeout_sec )); then
      log_error "Timeout waiting for socket: $path"
      return 1
    fi
  done
}

smoke_test() {
  if [[ "$WITH_SMOKE" != "true" ]]; then return; fi
  log_info "Running smoke test with busybox..."
  wait_for_socket "$FCCD_SOCKET" 90
  local CTR="${INSTALLROOT}/bin/firecracker-ctr"
  retry "pull busybox" "$CTR" --address "$FCCD_SOCKET" images pull --snapshotter devmapper docker.io/library/busybox:latest
  "$CTR" --address "$FCCD_SOCKET" run --rm --tty --net-host --snapshotter devmapper --runtime aws.firecracker \
    docker.io/library/busybox:latest busybox-test sh -lc 'echo fc-ok'
  log_success "Smoke test completed"
}

print_summary() {
  local end_time=$(date +%s)
  cat <<EOF

============================================================
Firecracker + containerd setup completed successfully.

- Repo:            ${REPO_DIR}
- Kernel:          ${FC_KERNEL_PATH}
- Rootfs:          ${FC_ROOTFS_PATH}
- Thinpool:        fc-dev-thinpool (loopback)
- Config (toml):   ${FCCD_CONFIG_TOML}
- Runtime (json):  ${RUNTIME_JSON_PATH}
- Service:         ${FCCD_SERVICE} (socket: ${FCCD_SOCKET})
- CNI enabled:     ${WITH_CNI}
- Smoke test:      ${WITH_SMOKE}

Elapsed: $((end_time - START_TIME))s
============================================================
EOF
}

main() {
  parse_args "$@"
  require_root "$@"
  check_prereqs
  install_packages
  ensure_docker
  ensure_user_in_docker_group || true
  install_go

  mkdir -p "$FC_BASE_DIR" "$FC_RUNTIME_DIR" "$FC_SNAPSHOTTER_DIR"

  clone_and_build
  build_rootfs_image
  download_kernel
  write_firecracker_containerd_config
  setup_devmapper_thinpool
  write_runtime_json
  setup_cni_demo
  create_systemd_service
  smoke_test
  print_summary
}

main "$@"


