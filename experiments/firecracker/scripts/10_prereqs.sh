#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "${SCRIPT_DIR}/common.sh"

log "Checking KVM"
require_kvm

pm=$(detect_pm)
log "Detected package manager: $pm"

case "$pm" in
  apt)
    run "sudo DEBIAN_FRONTEND=noninteractive apt-get update"
    run "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y make git curl e2fsprogs util-linux bc gnupg gcc dmsetup jq iproute2 ca-certificates"
    ;;
  yum)
    run "sudo yum -y install make git curl e2fsprogs util-linux bc gnupg gcc device-mapper jq iproute ca-certificates"
    ;;
  dnf)
    run "sudo dnf -y install make git curl e2fsprogs util-linux bc gnupg2 gcc device-mapper jq iproute ca-certificates"
    ;;
  *)
    err "Unknown package manager; skipping package install"
    ;;
esac

if ! command -v docker >/dev/null 2>&1; then
  log "Installing Docker via get.docker.com"
  run "curl -fsSL https://get.docker.com | sh"
  run "sudo systemctl enable --now docker"
fi

log "Prerequisites complete"


