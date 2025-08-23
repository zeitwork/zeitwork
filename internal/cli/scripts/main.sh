#!/bin/bash

# Zeitwork CLI - Main entry point
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get script directory
SCRIPT_DIR="${ZEITWORK_SCRIPTS_DIR:-$(dirname "$0")}"

# Source utilities
source "$SCRIPT_DIR/utils.sh"

# Display usage
usage() {
    cat << EOF
Zeitwork CLI - Platform Management Tool

Usage: zeitwork <command> [options]

Commands:
    setup       Set up a new Zeitwork cluster
    deploy      Deploy an application
    status      Show cluster and service status
    logs        View service logs
    cleanup     Clean up resources
    config      Manage contexts and configuration
    help        Show this help message

Options:
    -h, --help     Show help for a specific command
    -v, --verbose  Enable verbose output
    -q, --quiet    Suppress non-error output

Examples:
    zeitwork setup --operator 10.0.1.10 --database postgres://user:pass@localhost/zeitwork
    zeitwork deploy --project myapp --image myapp:latest
    zeitwork status --services
    zeitwork logs operator --tail 50
    zeitwork cleanup --all

For more information on a specific command, run:
    zeitwork <command> --help

EOF
}

# Parse global options
VERBOSE=false
QUIET=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -q|--quiet)
            QUIET=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            break
            ;;
    esac
done

# Check if command was provided
if [ $# -eq 0 ]; then
    error "No command specified"
    usage
    exit 1
fi

# Get command
COMMAND=$1
shift

# Export global options for subscripts
export VERBOSE
export QUIET
export SCRIPT_DIR

# Execute command
case $COMMAND in
    setup)
        source "$SCRIPT_DIR/setup.sh"
        setup_command "$@"
        ;;
    deploy)
        source "$SCRIPT_DIR/deploy.sh"
        deploy_command "$@"
        ;;
    status)
        source "$SCRIPT_DIR/status.sh"
        status_command "$@"
        ;;
    logs)
        source "$SCRIPT_DIR/logs.sh"
        logs_command "$@"
        ;;
    cleanup)
        source "$SCRIPT_DIR/cleanup.sh"
        cleanup_command "$@"
        ;;
    config)
        source "$SCRIPT_DIR/config.sh"
        config_command "$@"
        ;;
    help)
        usage
        exit 0
        ;;
    *)
        error "Unknown command: $COMMAND"
        usage
        exit 1
        ;;
esac