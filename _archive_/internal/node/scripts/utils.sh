#!/bin/bash
# Common utilities for all scripts

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

log_success() {
    echo -e "${GREEN}✓${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

log_error() {
    echo -e "${RED}✗${NC} $1"
}

# Check if running as root
check_root() {
    if [[ $EUID -eq 0 ]]; then
        echo ""
    else
        if command -v sudo &> /dev/null; then
            echo "sudo"
        else
            log_error "Not running as root and sudo is not available"
            exit 1
        fi
    fi
}

# Get latest version from GitHub
get_latest_version() {
    local repo=$1
    local fallback=$2
    
    local url="https://api.github.com/repos/${repo}/releases/latest"
    local version
    
    if version=$(curl -s "$url" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' 2>/dev/null); then
        if [[ -n "$version" ]]; then
            echo "$version"
        else
            echo "$fallback"
        fi
    else
        echo "$fallback"
    fi
}

# Normalize version string (add/remove 'v' prefix as needed)
normalize_version() {
    local version=$1
    local needs_v=${2:-true}
    
    if [[ "$needs_v" == "true" ]]; then
        if [[ ! "$version" =~ ^v ]]; then
            echo "v$version"
        else
            echo "$version"
        fi
    else
        echo "${version#v}"
    fi
}

# Check if a command exists
command_exists() {
    command -v "$1" &> /dev/null
}

# Download file with retry
download_file() {
    local url=$1
    local output=$2
    local max_retries=3
    local retry=0
    
    while [[ $retry -lt $max_retries ]]; do
        if curl -fsSL "$url" -o "$output" 2>/dev/null; then
            return 0
        fi
        retry=$((retry + 1))
        sleep 2
    done
    
    return 1
}
