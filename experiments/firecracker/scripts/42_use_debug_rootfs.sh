#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "${SCRIPT_DIR}/common.sh"

REPO_DIR="${HOME}/firecracker-containerd"
SRC="${REPO_DIR}/tools/image-builder/rootfs-debug.img"
DST="/var/lib/firecracker-containerd/runtime/default-rootfs.img"

if [[ ! -f "${SRC}" ]]; then
  err "Debug rootfs not found at ${SRC}."
  exit 1
fi

log "Switching runtime rootfs to debug image"
run "sudo cp ${SRC} ${DST}"
log "Switched to debug rootfs"


