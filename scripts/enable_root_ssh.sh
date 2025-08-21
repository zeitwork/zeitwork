#!/bin/bash
# Script to enable root SSH access on remote Ubuntu server
# This script reads the configuration from config.yaml and sets up root SSH access

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

# Function to parse YAML config
parse_yaml() {
    local config_file="${1:-config.yaml}"
    
    # Check if config file exists
    if [[ ! -f "$config_file" ]]; then
        log_error "Configuration file not found: $config_file"
        exit 1
    fi
    
    # Parse the YAML file for server details (ignore commented lines)
    SERVER_USER=$(grep -v '^[[:space:]]*#' "$config_file" | grep "fallback_user:" | cut -d':' -f2 | sed 's/[" ]//g' || echo "")
    SERVER_HOST=$(grep -v '^[[:space:]]*#' "$config_file" | grep "host:" | head -1 | cut -d':' -f2 | sed 's/[" ]//g' || echo "")
    SERVER_PORT=$(grep -v '^[[:space:]]*#' "$config_file" | grep "port:" | cut -d':' -f2 | sed 's/[" ]//g' || echo "")
    SSH_KEY_PATH=$(grep -v '^[[:space:]]*#' "$config_file" | grep "ssh_key_path:" | cut -d':' -f2 | sed 's/[" ]//g' || echo "")
    
    # Set defaults if not found
    SERVER_PORT=${SERVER_PORT:-22}
    SSH_KEY_PATH=${SSH_KEY_PATH:-~/.ssh/firecracker_manager_rsa}
    
    # Expand tilde in SSH key path
    SSH_KEY_PATH="${SSH_KEY_PATH/#\~/$HOME}"
    
    # Validate required fields
    if [[ -z "$SERVER_USER" ]]; then
        log_error "Server fallback_user not found in config.yaml"
        exit 1
    fi
    
    if [[ -z "$SERVER_HOST" ]]; then
        log_error "Server host not found in config.yaml"
        exit 1
    fi
}

# Function to check SSH connectivity
check_ssh_connectivity() {
    log_info "Testing SSH connectivity to ${SERVER_USER}@${SERVER_HOST}..."
    
    local ssh_opts="-o ConnectTimeout=5 -o StrictHostKeyChecking=no -o BatchMode=no -p ${SERVER_PORT}"
    
    # Add SSH key option if key exists
    if [[ -f "$SSH_KEY_PATH" ]]; then
        ssh_opts="$ssh_opts -i $SSH_KEY_PATH"
        log_info "Using SSH key: $SSH_KEY_PATH"
    else
        log_info "No SSH key found at $SSH_KEY_PATH, will prompt for password"
    fi
    
    # Try to connect - this will prompt for password if needed
    if ssh $ssh_opts "${SERVER_USER}@${SERVER_HOST}" "echo 'SSH connection successful'"; then
        log_success "SSH connection established successfully"
        return 0
    else
        log_error "Failed to establish SSH connection"
        log_info "Please ensure:"
        log_info "  - The server is reachable at ${SERVER_HOST}:${SERVER_PORT}"
        log_info "  - User '${SERVER_USER}' exists on the server"
        log_info "  - SSH key is set up or password authentication is enabled"
        return 1
    fi
}

# Function to enable root SSH access
enable_root_ssh() {
    log_info "Enabling root SSH access on ${SERVER_HOST}..."
    
    local ssh_opts="-o StrictHostKeyChecking=no -p ${SERVER_PORT}"
    
    # Add SSH key option if key exists
    if [[ -f "$SSH_KEY_PATH" ]]; then
        ssh_opts="$ssh_opts -i $SSH_KEY_PATH"
    fi
    
    # Create a script to run on the remote server
    local remote_script='#!/bin/bash
set -e

echo "=== Enabling root SSH access ==="

# Check if running with sudo privileges
if [[ $EUID -ne 0 ]]; then
    echo "This script needs to be run with sudo privileges"
    exit 1
fi

# Backup the original sshd_config
echo "Backing up SSH configuration..."
cp /etc/ssh/sshd_config /etc/ssh/sshd_config.backup.$(date +%Y%m%d_%H%M%S)

# Enable root login in SSH configuration
echo "Configuring SSH to allow root login..."
sed -i "s/^#*PermitRootLogin.*/PermitRootLogin yes/" /etc/ssh/sshd_config

# If PermitRootLogin line does not exist, add it
if ! grep -q "^PermitRootLogin" /etc/ssh/sshd_config; then
    echo "PermitRootLogin yes" >> /etc/ssh/sshd_config
fi

# Enable password authentication for root (optional, can be disabled later)
sed -i "s/^#*PasswordAuthentication.*/PasswordAuthentication yes/" /etc/ssh/sshd_config

# If PasswordAuthentication line does not exist, add it
if ! grep -q "^PasswordAuthentication" /etc/ssh/sshd_config; then
    echo "PasswordAuthentication yes" >> /etc/ssh/sshd_config
fi

# Set root password if not already set
echo "Setting up root account..."
if ! passwd -S root | grep -q "P "; then
    echo "Please set a password for root user:"
    passwd root
fi

# Create .ssh directory for root if it does not exist
mkdir -p /root/.ssh
chmod 700 /root/.ssh

# Copy authorized_keys from current user to root (if exists)
if [[ -f ~/.ssh/authorized_keys ]]; then
    echo "Copying SSH authorized keys to root account..."
    cp ~/.ssh/authorized_keys /root/.ssh/authorized_keys
    chmod 600 /root/.ssh/authorized_keys
    chown -R root:root /root/.ssh
    echo "SSH keys copied successfully"
fi

# Restart SSH service
echo "Restarting SSH service..."
if systemctl is-active --quiet ssh; then
    systemctl restart ssh
elif systemctl is-active --quiet sshd; then
    systemctl restart sshd
else
    service ssh restart || service sshd restart
fi

echo "=== Root SSH access has been enabled ==="
echo ""
echo "You can now SSH as root using:"
echo "  - SSH key authentication (if keys were copied)"
echo "  - Password authentication (if password was set)"
echo ""
echo "For security, consider:"
echo "  1. Disabling password authentication after setting up SSH keys"
echo "  2. Using SSH keys exclusively for root access"
echo "  3. Implementing fail2ban or similar security measures"
'
    
    # Execute the script on the remote server
    log_info "Executing configuration on remote server..."
    
    # First, create the script file on the remote server
    ssh $ssh_opts "${SERVER_USER}@${SERVER_HOST}" "cat > /tmp/enable_root_ssh.sh" <<< "$remote_script"
    
    # Make it executable
    ssh $ssh_opts "${SERVER_USER}@${SERVER_HOST}" "chmod +x /tmp/enable_root_ssh.sh"
    
    # Execute with sudo (will prompt for password if needed)
    log_info "You may be prompted for the sudo password for user '${SERVER_USER}'..."
    ssh -t $ssh_opts "${SERVER_USER}@${SERVER_HOST}" "sudo /tmp/enable_root_ssh.sh"
    
    # Clean up the temporary script
    ssh $ssh_opts "${SERVER_USER}@${SERVER_HOST}" "rm -f /tmp/enable_root_ssh.sh"
    
    log_success "Root SSH access configuration completed"
}

# Function to test root SSH access
test_root_ssh() {
    log_info "Testing root SSH access..."
    
    local ssh_opts="-o ConnectTimeout=5 -o StrictHostKeyChecking=no -p ${SERVER_PORT}"
    
    # Add SSH key option if key exists
    if [[ -f "$SSH_KEY_PATH" ]]; then
        ssh_opts="$ssh_opts -i $SSH_KEY_PATH"
    fi
    
    if ssh $ssh_opts "root@${SERVER_HOST}" "echo 'Root SSH access successful'" 2>/dev/null; then
        log_success "Root SSH access is working!"
        return 0
    else
        log_warning "Root SSH access test failed"
        log_info "This might be normal if:"
        log_info "  - You haven't set a root password yet"
        log_info "  - SSH keys haven't been copied to root account"
        log_info "  - The SSH service hasn't fully restarted"
        log_info ""
        log_info "Try connecting manually with:"
        log_info "  ssh -p ${SERVER_PORT} root@${SERVER_HOST}"
        return 1
    fi
}

# Main execution
main() {
    log_info "Starting root SSH enablement script"
    echo ""
    
    # Change to script directory to find config.yaml
    SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
    cd "$SCRIPT_DIR/.."
    
    # Parse configuration
    parse_yaml "config.yaml"
    
    log_info "Configuration loaded:"
    log_info "  Server: ${SERVER_HOST}:${SERVER_PORT}"
    log_info "  User: ${SERVER_USER}"
    log_info "  SSH Key: ${SSH_KEY_PATH}"
    echo ""
    
    # Check SSH connectivity
    if ! check_ssh_connectivity; then
        exit 1
    fi
    echo ""
    
    # Enable root SSH access
    enable_root_ssh
    echo ""
    
    # Test root SSH access
    test_root_ssh
    echo ""
    
    log_success "Script completed successfully!"
    log_info "Next steps:"
    log_info "  1. Test root SSH access: ssh -p ${SERVER_PORT} root@${SERVER_HOST}"
    log_info "  2. Consider disabling password authentication after confirming key-based access works"
    log_info "  3. Review and harden SSH configuration as needed"
}

# Run main function
main "$@"
