#!/usr/bin/env bash
set -euo pipefail

# Install Go on Linux using the official method from go.dev
# - Detects architecture (amd64/arm64)
# - Downloads latest version unless GO_VERSION is set (e.g., GO_VERSION=go1.22.6)
# - Installs into /usr/local/go and ensures PATH update via /etc/profile.d/go.sh

require_root() {
  if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
    echo "[INFO] Re-executing with sudo..."
    exec sudo -E -H bash "$0" "$@"
  fi
}

main() {
  require_root "$@"

  if command -v go >/dev/null 2>&1; then
    echo "[OK] Go already installed: $(go version)"
    exit 0
  fi

  local arch goarch
  arch="$(uname -m)"
  case "${arch}" in
    x86_64|amd64)
      goarch="amd64"
      ;;
    aarch64|arm64)
      goarch="arm64"
      ;;
    *)
      echo "[ERROR] Unsupported architecture: ${arch}" >&2
      exit 1
      ;;
  esac

  local tmpdir goversion tarball_url
  tmpdir="$(mktemp -d)"
  # Use default expansion to avoid set -u errors if tmpdir goes out of scope
  trap 'rm -rf "${tmpdir:-}"' EXIT

  goversion="${GO_VERSION:-}"
  if [[ -z "${goversion}" ]]; then
    if command -v curl >/dev/null 2>&1; then
      # Fetch latest version text and sanitize to first token/line
      goversion="$(curl -fsSL https://go.dev/VERSION?m=text | head -n1 | awk '{print $1}' | tr -d '\r' || true)"
    fi
  fi
  # Validate/normalize version string; fallback if malformed
  if [[ -n "${goversion}" ]]; then
    if ! [[ "${goversion}" =~ ^go[0-9]+(\.[0-9]+){1,2}$ ]]; then
      goversion=""
    fi
  fi
  if [[ -z "${goversion}" ]]; then
    goversion="go1.25.1"
  fi

  tarball_url="https://go.dev/dl/${goversion}.linux-${goarch}.tar.gz"
  echo "[INFO] Downloading ${tarball_url}"
  curl -fL "${tarball_url}" -o "${tmpdir}/go.tgz"

  rm -rf /usr/local/go
  tar -C /usr/local -xzf "${tmpdir}/go.tgz"

  export PATH="/usr/local/go/bin:${PATH}"
  if [[ ! -f /etc/profile.d/go.sh ]]; then
    echo 'export PATH="$PATH:/usr/local/go/bin"' > /etc/profile.d/go.sh
    chmod 0644 /etc/profile.d/go.sh
  fi

  echo "[OK] Installed $(/usr/local/go/bin/go version)"
}

main "$@"


