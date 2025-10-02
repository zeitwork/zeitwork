#!/bin/bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Default values
HOST=""
USER="root"
NODE_ID=""
NODE_REGION_ID=""
NODE_IP_ADDRESS=""
NODE_RUNTIME_MODE="firecracker"

# 1Password configuration
OP_ACCOUNT="dokedu.1password.eu"
OP_VAULT="Server"
OP_ITEM="Zeitwork"

# Function to print colored output
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to show usage
usage() {
    cat << EOF
Usage: $0 --host <ip> --user <user> [OPTIONS]

Required:
  --host <ip>              Target host IP address
  --user <user>            SSH user (default: root)
  --node-id <uuid>         Node UUID
  --region-id <uuid>       Region UUID
  --ip <ip>                Node IP address (defaults to --host value)

Optional:
  --runtime <mode>         Runtime mode: firecracker or docker (default: firecracker)
  --help                   Show this help message

Example:
  $0 --host 67.213.124.173 --user root \\
     --node-id 0199bf39-abb2-7482-81fd-c447ba9cf96b \\
     --region-id 01996e48-2c25-7a67-9507-2126d85bb007 \\
     --ip 67.213.124.173

EOF
    exit 1
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --host)
            HOST="$2"
            shift 2
            ;;
        --user)
            USER="$2"
            shift 2
            ;;
        --node-id)
            NODE_ID="$2"
            shift 2
            ;;
        --region-id)
            NODE_REGION_ID="$2"
            shift 2
            ;;
        --ip)
            NODE_IP_ADDRESS="$2"
            shift 2
            ;;
        --runtime)
            NODE_RUNTIME_MODE="$2"
            shift 2
            ;;
        --help)
            usage
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            ;;
    esac
done

# Validate required arguments
if [[ -z "$HOST" ]]; then
    log_error "Missing required argument: --host"
    usage
fi

if [[ -z "$NODE_ID" ]]; then
    log_error "Missing required argument: --node-id"
    usage
fi

if [[ -z "$NODE_REGION_ID" ]]; then
    log_error "Missing required argument: --region-id"
    usage
fi

# Default NODE_IP_ADDRESS to HOST if not specified
if [[ -z "$NODE_IP_ADDRESS" ]]; then
    NODE_IP_ADDRESS="$HOST"
fi

log_info "Starting nodeagent deployment to $USER@$HOST"
log_info "Node ID: $NODE_ID"
log_info "Region ID: $NODE_REGION_ID"
log_info "IP Address: $NODE_IP_ADDRESS"
log_info "Runtime Mode: $NODE_RUNTIME_MODE"

# Check if 1Password CLI is installed
if ! command -v op &> /dev/null; then
    log_error "1Password CLI (op) is not installed"
    log_error "Install it with: brew install --cask 1password-cli"
    exit 1
fi

# Check if logged in to 1Password
if ! op account get --account "$OP_ACCOUNT" &> /dev/null; then
    log_warn "Not logged in to 1Password account: $OP_ACCOUNT"
    log_info "Attempting to sign in..."
    eval $(op signin --account "$OP_ACCOUNT")
fi

# Fetch secrets from 1Password
log_info "Fetching secrets from 1Password..."

# Function to fetch a field from 1Password
fetch_secret() {
    local field_name="$1"
    local value
    value=$(op item get "$OP_ITEM" --vault "$OP_VAULT" --account "$OP_ACCOUNT" --fields "$field_name" --reveal 2>/dev/null || echo "")
    
    if [[ -z "$value" ]]; then
        log_warn "Could not fetch secret: $field_name" >&2
    fi
    
    echo "$value"
}

# Fetch required secrets
NODE_DATABASE_URL=$(fetch_secret "NUXT_DSN")
NODE_REGISTRY_URL=$(fetch_secret "BUILDER_REGISTRY_URL")
NODE_REGISTRY_USER=$(fetch_secret "BUILDER_REGISTRY_USER")
NODE_REGISTRY_PASS=$(fetch_secret "BUILDER_REGISTRY_PASS")
TAILSCALE_AUTH_KEY=$(fetch_secret "TAILSCALE_AUTH_KEY")

# Validate critical secrets
if [[ -z "$NODE_DATABASE_URL" ]]; then
    log_error "Failed to fetch NODE_DATABASE_URL from 1Password"
    log_error "Please add this field to $OP_VAULT/$OP_ITEM"
    exit 1
fi

if [[ -z "$NODE_REGISTRY_URL" ]]; then
    log_error "Failed to fetch BUILDER_REGISTRY_URL from 1Password"
    exit 1
fi

log_info "Successfully fetched secrets from 1Password"

# Build the nodeagent binary
log_info "Building nodeagent binary for Linux..."
cd "$PROJECT_ROOT"

GOOS=linux GOARCH=amd64 go build -o "$PROJECT_ROOT/nodeagent" ./cmd/nodeagent

if [[ ! -f "$PROJECT_ROOT/nodeagent" ]]; then
    log_error "Failed to build nodeagent binary"
    exit 1
fi

log_info "Successfully built nodeagent binary"

# Create temporary environment file
TEMP_ENV_FILE=$(mktemp)
trap "rm -f $TEMP_ENV_FILE" EXIT

cat > "$TEMP_ENV_FILE" << EOF
# Zeitwork NodeAgent Environment Configuration
# Generated at $(date)

# Node Configuration
NODE_ID=$NODE_ID
NODE_REGION_ID=$NODE_REGION_ID
NODE_IP_ADDRESS=$NODE_IP_ADDRESS
NODE_RUNTIME_MODE=$NODE_RUNTIME_MODE

# Database
NODE_DATABASE_URL=$NODE_DATABASE_URL

# Registry Configuration
NODE_REGISTRY_URL=$NODE_REGISTRY_URL
NODE_REGISTRY_USER=$NODE_REGISTRY_USER
NODE_REGISTRY_PASS=$NODE_REGISTRY_PASS

# Tailscale
TAILSCALE_AUTH_KEY=$TAILSCALE_AUTH_KEY
EOF

log_info "Created environment configuration"

# Deploy to target host
log_info "Deploying to $USER@$HOST..."

# Copy files to temporary location on remote host
log_info "Copying files to remote host..."
scp "$PROJECT_ROOT/nodeagent" "$USER@$HOST:/tmp/nodeagent"
scp "$TEMP_ENV_FILE" "$USER@$HOST:/tmp/nodeagent.env"
scp "$PROJECT_ROOT/config/systemd/nodeagent.service" "$USER@$HOST:/tmp/nodeagent.service"

# Set permissions and start service
log_info "Installing and starting service..."
ssh "$USER@$HOST" << 'REMOTE_SCRIPT'
# Create directories with sudo
sudo mkdir -p /etc/zeitwork /usr/local/bin

# Move files to final location
sudo mv /tmp/nodeagent /usr/local/bin/nodeagent
sudo mv /tmp/nodeagent.env /etc/zeitwork/nodeagent.env
sudo mv /tmp/nodeagent.service /etc/systemd/system/nodeagent.service

# Make binary executable
sudo chmod +x /usr/local/bin/nodeagent

# Secure the environment file
sudo chmod 600 /etc/zeitwork/nodeagent.env

# Set proper ownership
sudo chown root:root /usr/local/bin/nodeagent
sudo chown root:root /etc/zeitwork/nodeagent.env
sudo chown root:root /etc/systemd/system/nodeagent.service

# Configure firewall to allow port 8080 (reverse proxy)
# if command -v ufw &> /dev/null; then
#     echo "Configuring UFW firewall..."
#     sudo ufw allow 8080/tcp comment 'Zeitwork Reverse Proxy'
# elif command -v firewall-cmd &> /dev/null; then
#     echo "Configuring firewalld..."
#     sudo firewall-cmd --permanent --add-port=8080/tcp
#     sudo firewall-cmd --reload
# else
#     echo "No supported firewall found (ufw/firewalld), skipping firewall configuration"
# fi

# Reload systemd
sudo systemctl daemon-reload

# Enable and restart service
sudo systemctl enable nodeagent
sudo systemctl restart nodeagent

# Wait a moment for service to start
sleep 2

# Check service status
sudo systemctl status nodeagent --no-pager || true
REMOTE_SCRIPT

# Check if service is running
log_info "Verifying deployment..."
if ssh "$USER@$HOST" "sudo systemctl is-active --quiet nodeagent"; then
    log_info "✓ Nodeagent service is running"
else
    log_error "✗ Nodeagent service failed to start"
    log_error "Check logs with: ssh $USER@$HOST 'sudo journalctl -u nodeagent -f'"
    exit 1
fi

# Verify port 8080 is listening
log_info "Verifying reverse proxy port..."
sleep 2  # Give proxy a moment to start
if ssh "$USER@$HOST" "sudo ss -tlnp | grep ':8080' > /dev/null"; then
    log_info "✓ Port 8080 is listening (reverse proxy active)"
else
    log_warn "✗ Port 8080 is not listening yet (proxy may still be starting)"
fi

# Show recent logs
log_info "Recent logs:"
ssh "$USER@$HOST" "sudo journalctl -u nodeagent -n 20 --no-pager"

log_info "✓ Deployment completed successfully!"
log_info ""
log_info "Useful commands:"
log_info "  Check status:     ssh $USER@$HOST 'sudo systemctl status nodeagent'"
log_info "  View logs:        ssh $USER@$HOST 'sudo journalctl -u nodeagent -f'"
log_info "  Check port 8080:  ssh $USER@$HOST 'sudo ss -tlnp | grep 8080'"
log_info "  Restart:          ssh $USER@$HOST 'sudo systemctl restart nodeagent'"
log_info "  Stop:             ssh $USER@$HOST 'sudo systemctl stop nodeagent'"

