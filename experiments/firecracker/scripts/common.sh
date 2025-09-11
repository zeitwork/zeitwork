#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
ROOT_DIR=$(cd -- "${SCRIPT_DIR}/.." &>/dev/null && pwd)

log() {
  echo "[+] $*"
}

err() {
  echo "[!] $*" >&2
}

run() {
  echo "+ $*" >&2
  bash -lc "$*"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { err "Missing command: $1"; return 1; }
}

detect_pm() {
  if command -v apt-get >/dev/null 2>&1; then echo apt; return; fi
  if command -v yum >/dev/null 2>&1; then echo yum; return; fi
  if command -v dnf >/dev/null 2>&1; then echo dnf; return; fi
  echo unknown
}

require_kvm() {
  if [[ ! -e /dev/kvm ]]; then
    err "/dev/kvm missing. Enable KVM and rerun."
    return 1
  fi
  if ! (exec 3<>/dev/kvm) 2>/dev/null; then
    err "/dev/kvm not accessible for read/write; continuing but Firecracker may fail"
  else
    exec 3>&-
  fi
}

wait_for_file() {
  local file="$1"; shift
  local timeout="${1:-60}"
  local i=0
  while [[ $i -lt $timeout ]]; do
    if [[ -S "$file" || -f "$file" ]]; then return 0; fi
    sleep 1
    i=$((i+1))
  done
  err "Timed out waiting for $file"
  return 1
}


