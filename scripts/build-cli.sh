#!/usr/bin/env bash
set -euo pipefail

# Build the Zeitwork CLI tool
# Usage: ./scripts/build-cli.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "Building Zeitwork CLI..."
go build -ldflags="-s -w" -o zeitwork-cli ./cmd/cli

echo "✓ CLI built successfully: ./zeitwork-cli"
echo ""
echo "Usage examples:"
echo "  ./zeitwork-cli deploy                              # Deploy all services"
echo "  ./zeitwork-cli deploy --services builder           # Deploy only builder"
echo "  ./zeitwork-cli deploy --env-file .env.staging      # Use staging environment"
echo ""

