#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "${SCRIPT_DIR}/common.sh"

HOME_DIR=${HOME}
REPO_DIR="${HOME_DIR}/firecracker-containerd"

if [[ ! -d "${REPO_DIR}" ]]; then
  log "Cloning firecracker-containerd"
  run "cd ${HOME_DIR} && git clone https://github.com/firecracker-microvm/firecracker-containerd.git"
fi

log "Building firecracker-containerd components (static agent)"
run "cd ${REPO_DIR} && sudo -E make clean || true"
# Build agent statically on host to avoid glibc mismatch in guest
run "cd ${REPO_DIR} && env STATIC_AGENT=on CGO_ENABLED=0 GOOS=linux sudo -E make agent"
# Build rootfs image and firecracker
run "cd ${REPO_DIR} && sudo -E make image firecracker"

log "Installing firecracker-containerd, firecracker, and demo-network"
run "cd ${REPO_DIR} && sudo make install install-firecracker demo-network"

log "Ensuring runtime assets"
run "sudo mkdir -p /var/lib/firecracker-containerd/runtime"

ROOTFS_SRC="${REPO_DIR}/tools/image-builder/rootfs.img"
ROOTFS_DST="/var/lib/firecracker-containerd/runtime/default-rootfs.img"
if [[ ! -f "${ROOTFS_SRC}" ]]; then
  err "rootfs.img not found at ${ROOTFS_SRC}. Did 'make image' succeed?"
  exit 1
fi
run "sudo cp ${ROOTFS_SRC} ${ROOTFS_DST}"

KERNEL_DST="/var/lib/firecracker-containerd/runtime/default-vmlinux.bin"
if [[ ! -f "${KERNEL_DST}" ]]; then
  TMP_KERNEL="$(mktemp)"
  run "curl -fsSL -o ${TMP_KERNEL} https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
  run "sudo cp ${TMP_KERNEL} ${KERNEL_DST}"
fi

log "Build and install complete"


