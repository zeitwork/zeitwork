#!/usr/bin/env bash
set -euo pipefail

# Zeitwork Nodeagent installer
# - Builds the nodeagent binary
# - Installs to /usr/local/bin/zeitwork-nodeagent
# - Copies env and systemd unit from scripts/deploy templates
# - Enables and starts the service

require_root() {
  if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
    echo "[INFO] Re-executing with sudo..."
    exec sudo -E -H bash "$0" "$@"
  fi
}

usage() {
  cat <<EOF
Usage: sudo bash scripts/install-nodeagent.sh [--no-build]

Options:
  --no-build   Skip building; use existing build/zeitwork-nodeagent artifact
EOF
}

main() {
  local do_build=1
  if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage; exit 0
  fi
  if [[ "${1:-}" == "--no-build" ]]; then
    do_build=0
  fi

  require_root "$@"

  local script_dir repo_root
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  repo_root="$(cd "${script_dir}/.." && pwd)"

  local service_name="zeitwork-nodeagent"
  local service_file="/etc/systemd/system/${service_name}.service"
  local bin_target="/usr/local/bin/${service_name}"
  local build_artifact="${repo_root}/build/${service_name}"
  local env_dir="/etc/zeitwork"
  local env_file="${env_dir}/nodeagent.env"
  local work_dir="/var/lib/zeitwork"

  mkdir -p "${work_dir}" "${env_dir}"

  # Build strategy:
  # 1) If an artifact already exists, prefer it and skip building
  # 2) Ensure Go is available (install from go.dev if missing)
  # 3) Build locally with the host Go toolchain
  if (( do_build == 1 )); then
    if [[ -f "${build_artifact}" ]]; then
      echo "[INFO] Found existing artifact at ${build_artifact}; skipping build."
      do_build=0
    fi
  fi

  if (( do_build == 1 )); then
    if ! command -v go >/dev/null 2>&1; then
      echo "[INFO] Go toolchain not found; running scripts/install-go.sh"
      "${repo_root}/scripts/install-go.sh"
    fi
    echo "[INFO] Building ${service_name} with local Go toolchain..."
    (cd "${repo_root}" && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o "${build_artifact}" ./cmd/nodeagent)
  else
    echo "[INFO] Skipping build (using existing artifact if present)"
  fi

  if [[ -f "${build_artifact}" ]]; then
    install -m 0755 "${build_artifact}" "${bin_target}"
    echo "[OK] Installed binary to ${bin_target}"
  else
    # Fallback: if user already has binary at target, keep it
    if [[ ! -x "${bin_target}" ]]; then
      echo "[ERROR] No build artifact at ${build_artifact} and no existing ${bin_target}"
      exit 1
    fi
  fi

  # Copy env from template; do not inline-create
  if [[ ! -f "${env_file}" ]]; then
    local tmpl="${repo_root}/scripts/deploy/env/.env.example.nodeagent"
    if [[ ! -f "${tmpl}" ]]; then
      echo "[ERROR] Template not found: ${tmpl}. Please add it and re-run." >&2
      exit 1
    fi
    install -m 0644 "${tmpl}" "${env_file}"
    echo "[OK] Installed env to ${env_file}"
  fi

  # Copy systemd unit from template; do not inline-create
  local unit_tmpl="${repo_root}/scripts/deploy/systemd/zeitwork-nodeagent.service"
  if [[ ! -f "${unit_tmpl}" ]]; then
    echo "[ERROR] Systemd template not found: ${unit_tmpl}. Please add it and re-run." >&2
    exit 1
  fi
  install -m 0644 "${unit_tmpl}" "${service_file}"

  echo "[OK] Wrote ${service_file}"

  if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable --now "${service_name}"
    systemctl status --no-pager "${service_name}" || true
    echo "[DONE] ${service_name} enabled and started"
  else
    echo "[WARN] systemctl not found; service file installed at ${service_file}. Start it manually."
  fi
}

main "$@"
