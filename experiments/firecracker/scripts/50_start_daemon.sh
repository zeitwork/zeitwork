#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "${SCRIPT_DIR}/common.sh"

SOCK=/run/firecracker-containerd/containerd.sock

if [[ -S "$SOCK" ]]; then
  if pgrep -f "/usr/local/bin/firecracker-containerd" >/dev/null 2>&1; then
    log "Socket already exists and daemon appears running: $SOCK"
    exit 0
  else
    log "Socket exists but daemon not running; removing stale socket"
    sudo rm -f "$SOCK"
  fi
fi

log "Starting firecracker-containerd"
run "sudo bash -lc 'nohup /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml >/tmp/firecracker-containerd.log 2>&1 &'"

log "Waiting for socket: $SOCK"
wait_for_file "$SOCK" 120
log "Daemon ready"


