#!/bin/bash
set -euo pipefail

# Load and export environment variables from .env
set -a
source .env
set +a

# Run the Go application
go run cmd/reconciler/reconciler.go