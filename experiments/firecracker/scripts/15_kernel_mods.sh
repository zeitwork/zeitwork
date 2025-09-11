#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "${SCRIPT_DIR}/common.sh"

log "Loading required kernel modules (vhost_vsock, tun)"
run "sudo modprobe vhost_vsock || true"
run "sudo modprobe tun || true"

log "Kernel modules status:"
run "lsmod | grep -E 'vhost_vsock|tun' || true"


