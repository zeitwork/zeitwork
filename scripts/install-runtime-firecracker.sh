#!/usr/bin/env bash
set -euo pipefail

# Zeitwork Firecracker runtime setup wrapper
# - Runs internal setup to prepare kernel/rootfs and IPv6 bridge
# - Symlinks Firecracker/jailer into /usr/local/bin if needed
# - Optionally writes a sample nodeagent env (if missing)

require_root() {
  if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
    echo "[INFO] Re-executing with sudo..."
    exec sudo -E -H bash "$0" "$@"
  fi
}

main() {
  require_root "$@"

  if [[ "$(uname -s)" != "Linux" ]]; then
    echo "[ERROR] Firecracker setup is only supported on Linux" >&2
    exit 1
  fi

  local script_dir repo_root
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  repo_root="$(cd "${script_dir}/.." && pwd)"

  local setup_script="${repo_root}/internal/nodeagent/runtime/firecracker/setup/setup.sh"
  if [[ ! -x "${setup_script}" ]]; then
    chmod +x "${setup_script}"
  fi

  echo "[INFO] Running Firecracker runtime setup..."
  "${setup_script}"
  echo "[OK] Firecracker runtime setup completed"

  # Attempt to expose firecracker/jailer on PATH for the runtime
  local work_dir="${WORK_DIR:-/var/lib/zeitwork/firecracker}"
  local bin_dir="${work_dir}/binaries"
  if [[ -x "${bin_dir}/firecracker" && ! -e "/usr/local/bin/firecracker" ]]; then
    ln -sf "${bin_dir}/firecracker" /usr/local/bin/firecracker
    echo "[OK] Linked firecracker -> /usr/local/bin/firecracker"
  fi
  if [[ -x "${bin_dir}/jailer" && ! -e "/usr/local/bin/jailer" ]]; then
    ln -sf "${bin_dir}/jailer" /usr/local/bin/jailer
    echo "[OK] Linked jailer -> /usr/local/bin/jailer"
  fi

  # Do not create env here; installers will copy from templates explicitly

  echo "[DONE] Firecracker runtime is ready. You can now (re)start the nodeagent."
}

main "$@"
