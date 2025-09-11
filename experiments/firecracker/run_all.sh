#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)

run() { echo "+ $*" >&2; bash -lc "$*"; }

run "chmod +x ${SCRIPT_DIR}/scripts/*.sh || true"

run "${SCRIPT_DIR}/scripts/10_prereqs.sh"
run "${SCRIPT_DIR}/scripts/15_kernel_mods.sh"
run "${SCRIPT_DIR}/scripts/20_build_firecracker_containerd.sh"
run "${SCRIPT_DIR}/scripts/30_devmapper_thinpool.sh"
run "${SCRIPT_DIR}/scripts/40_write_configs.sh"
run "${SCRIPT_DIR}/scripts/42_use_debug_rootfs.sh"
run "${SCRIPT_DIR}/scripts/45_cni_ipv6.sh"
run "${SCRIPT_DIR}/scripts/50_start_daemon.sh"
run "${SCRIPT_DIR}/scripts/60_build_image_import.sh"
run "${SCRIPT_DIR}/scripts/70_run_container_ipv6.sh"
run "${SCRIPT_DIR}/scripts/80_verify_ipv6.sh"

echo "All steps completed."


