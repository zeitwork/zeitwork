#!/bin/bash

# Utility functions for Zeitwork CLI

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Logging functions
log() {
    if [ "$QUIET" != "true" ]; then
        echo -e "${BLUE}[INFO]${NC} $1"
    fi
}

success() {
    if [ "$QUIET" != "true" ]; then
        echo -e "${GREEN}[SUCCESS]${NC} $1"
    fi
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1" >&2
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

debug() {
    if [ "$VERBOSE" == "true" ]; then
        echo -e "${CYAN}[DEBUG]${NC} $1"
    fi
}

# Progress indicator
show_progress() {
    local message="$1"
    if [ "$QUIET" != "true" ]; then
        echo -ne "${BLUE}⣾${NC} $message..."
    fi
}

end_progress() {
    if [ "$QUIET" != "true" ]; then
        echo -e "\r${GREEN}✓${NC} $1"
    fi
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check if running as root
is_root() {
    [ "$EUID" -eq 0 ]
}

# Require root privileges
require_root() {
    if ! is_root; then
        error "This command requires root privileges. Please run with sudo."
        exit 1
    fi
}

# Check if service is running
service_running() {
    local service="$1"
    systemctl is-active --quiet "$service"
}

# Wait for service to be ready
wait_for_service() {
    local service="$1"
    local max_attempts="${2:-30}"
    local attempt=0
    
    show_progress "Waiting for $service to be ready"
    
    while [ $attempt -lt $max_attempts ]; do
        if service_running "$service"; then
            end_progress "$service is ready"
            return 0
        fi
        sleep 1
        ((attempt++))
    done
    
    error "$service failed to start within $max_attempts seconds"
    return 1
}

# Check port availability
is_port_open() {
    local host="$1"
    local port="$2"
    nc -z -w1 "$host" "$port" 2>/dev/null
}

# Wait for port to be available
wait_for_port() {
    local host="$1"
    local port="$2"
    local max_attempts="${3:-30}"
    local attempt=0
    
    show_progress "Waiting for $host:$port to be available"
    
    while [ $attempt -lt $max_attempts ]; do
        if is_port_open "$host" "$port"; then
            end_progress "$host:$port is available"
            return 0
        fi
        sleep 1
        ((attempt++))
    done
    
    error "$host:$port not available after $max_attempts seconds"
    return 1
}

# Execute remote command via SSH
remote_exec() {
    local host="$1"
    local command="$2"
    local ssh_opts="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
    
    debug "Executing on $host: $command"
    ssh $ssh_opts "$host" "$command"
}

# Copy file to remote host
remote_copy() {
    local source="$1"
    local host="$2"
    local dest="$3"
    local scp_opts="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
    
    debug "Copying $source to $host:$dest"
    scp $scp_opts "$source" "$host:$dest"
}

# Check database connectivity
check_database() {
    local db_url="$1"
    
    if command_exists psql; then
        psql "$db_url" -c "SELECT 1" >/dev/null 2>&1
    else
        warning "psql not found, skipping database check"
        return 0
    fi
}

# Generate random string
random_string() {
    local length="${1:-16}"
    cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w "$length" | head -n 1
}

# Confirm action
confirm() {
    local message="${1:-Are you sure?}"
    local default="${2:-n}"
    
    if [ "$QUIET" == "true" ]; then
        return 0
    fi
    
    local prompt
    if [ "$default" == "y" ]; then
        prompt="$message [Y/n]: "
    else
        prompt="$message [y/N]: "
    fi
    
    read -p "$prompt" -n 1 -r
    echo
    
    if [ "$default" == "y" ]; then
        [[ ! $REPLY =~ ^[Nn]$ ]]
    else
        [[ $REPLY =~ ^[Yy]$ ]]
    fi
}

# Validate IP address
is_valid_ip() {
    local ip="$1"
    local regex='^([0-9]{1,3}\.){3}[0-9]{1,3}$'
    
    if [[ $ip =~ $regex ]]; then
        IFS='.' read -ra OCTETS <<< "$ip"
        for octet in "${OCTETS[@]}"; do
            if ((octet > 255)); then
                return 1
            fi
        done
        return 0
    fi
    return 1
}

# Get system info
get_system_info() {
    local key="$1"
    
    case "$key" in
        cpu_count)
            nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo "1"
            ;;
        memory_mb)
            if [ -f /proc/meminfo ]; then
                awk '/MemTotal/ {print int($2/1024)}' /proc/meminfo
            else
                echo "4096"
            fi
            ;;
        disk_free_gb)
            df -BG / | awk 'NR==2 {print int($4)}'
            ;;
        kernel)
            uname -r
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

# Create directory if it doesn't exist
ensure_directory() {
    local dir="$1"
    if [ ! -d "$dir" ]; then
        debug "Creating directory: $dir"
        mkdir -p "$dir"
    fi
}

# Backup file before modifying
backup_file() {
    local file="$1"
    if [ -f "$file" ]; then
        local backup="${file}.backup.$(date +%Y%m%d_%H%M%S)"
        debug "Backing up $file to $backup"
        cp "$file" "$backup"
    fi
}

# Cleanup temporary files
cleanup_temp() {
    local temp_dir="${1:-/tmp/zeitwork-*}"
    debug "Cleaning up temporary files: $temp_dir"
    rm -rf $temp_dir
}

# Trap for cleanup on exit
setup_trap() {
    trap 'cleanup_temp; exit' INT TERM EXIT
}

# ============================================
# Configuration and Context Functions
# ============================================

# Get current context from config
get_current_context() {
    local config_file="${ZEITWORK_CONFIG:-$HOME/.zeitwork/config.yaml}"
    if [ -f "$config_file" ]; then
        grep "^current-context:" "$config_file" | cut -d: -f2 | tr -d ' '
    else
        echo "local"
    fi
}

# Load environment variables from .env file
load_env_file() {
    local context="${1:-$(get_current_context)}"
    local env_file="$HOME/.zeitwork/.env.$context"
    
    if [ -f "$env_file" ]; then
        debug "Loading environment from $env_file"
        set -a  # Export all variables
        source "$env_file"
        set +a  # Stop exporting
        return 0
    else
        warning "Environment file not found: $env_file"
        return 1
    fi
}

# Get primary operator for current context
get_primary_operator() {
    local context="${1:-$(get_current_context)}"
    local config_file="${ZEITWORK_CONFIG:-$HOME/.zeitwork/config.yaml}"
    
    # Load environment to get operator from there if available
    if load_env_file "$context"; then
        if [ -n "$OPERATOR_URL" ]; then
            echo "$OPERATOR_URL"
            return 0
        fi
    fi
    
    # Otherwise try to parse from config
    # This is a simplified version - ideally use a proper YAML parser
    local cluster=$(grep -A2 "name: $context" "$config_file" | grep "cluster:" | cut -d: -f2 | tr -d ' ')
    if [ -n "$cluster" ]; then
        # Find primary operator for this cluster
        local in_cluster=false
        local in_operators=false
        while IFS= read -r line; do
            if [[ "$line" == *"$cluster:"* ]]; then
                in_cluster=true
            elif [ "$in_cluster" == "true" ] && [[ "$line" == *"operators:"* ]]; then
                in_operators=true
            elif [ "$in_operators" == "true" ] && [[ "$line" == *"host:"* ]]; then
                local host=$(echo "$line" | grep -o 'host:.*' | cut -d: -f2 | tr -d ' ')
                local port=$(grep -A1 "$line" "$config_file" | grep "port:" | cut -d: -f2 | tr -d ' ')
                echo "http://$host:${port:-8080}"
                return 0
            elif [ "$in_cluster" == "true" ] && [[ "$line" =~ ^[[:space:]]{0,2}[^[:space:]] ]] && [[ "$line" != *"operators:"* ]]; then
                break
            fi
        done < "$config_file"
    fi
    
    # Default to localhost
    echo "http://localhost:8080"
}

# Get all operators for current context
get_all_operators() {
    local context="${1:-$(get_current_context)}"
    local config_file="${ZEITWORK_CONFIG:-$HOME/.zeitwork/config.yaml}"
    
    # Get cluster name from context
    local cluster=$(grep -A3 "name: $context" "$config_file" | grep "cluster:" | cut -d: -f2 | tr -d ' ')
    
    if [ -n "$cluster" ]; then
        # Extract all operator hosts for this cluster
        # Use a pattern that works even when cluster is near end of file
        awk "/^  $cluster:/,/^[a-z]/" "$config_file" | \
            awk '/operators:/,/nodes:/' | \
            grep "host:" | \
            sed 's/.*host: *//' | \
            awk '{print "http://" $0 ":8080"}'
    else
        # Fall back to primary operator
        get_primary_operator "$context"
    fi
}

# Get nodes for current context
get_nodes_from_config() {
    local context="${1:-$(get_current_context)}"
    local config_file="${ZEITWORK_CONFIG:-$HOME/.zeitwork/config.yaml}"
    
    # This is simplified - in production you'd parse YAML properly
    local cluster=$(grep -A2 "name: $context" "$config_file" | grep "cluster:" | cut -d: -f2 | tr -d ' ')
    
    if [ -n "$cluster" ]; then
        # Extract nodes for this cluster
        # First get the cluster section up to the next top-level section
        # Then extract the nodes subsection and get all host values
        awk "/^  $cluster:/,/^[a-zA-Z]/" "$config_file" | \
            sed -n '/nodes:/,/^[[:space:]]{4}[a-zA-Z]/p' | \
            grep "host:" | \
            sed 's/.*host: *//' | \
            tr -d ' '
    fi
}

# Get SSH configuration for current context
get_ssh_config() {
    local context="${1:-$(get_current_context)}"
    local config_file="${ZEITWORK_CONFIG:-$HOME/.zeitwork/config.yaml}"
    
    # Get user for this context
    local user=$(grep -A3 "name: $context" "$config_file" | grep "user:" | cut -d: -f2 | tr -d ' ')
    
    if [ -n "$user" ]; then
        # Get SSH details for this user
        # Use a pattern that works even when user is the last entry in the file
        local ssh_key=$(awk "/^  $user:/,/^[a-z]|^$/" "$config_file" | grep "ssh-key:" | cut -d: -f2- | tr -d ' ')
        local ssh_user=$(awk "/^  $user:/,/^[a-z]|^$/" "$config_file" | grep "ssh-user:" | cut -d: -f2 | tr -d ' ')
        
        # Expand tilde in ssh-key path
        ssh_key="${ssh_key/#\~/$HOME}"
        
        echo "-i $ssh_key -l ${ssh_user:-root}"
    else
        echo "-l root"
    fi
}

# Execute command on remote host using context-aware SSH config
ssh_remote() {
    local host="$1"
    local command="$2"
    
    # Get SSH config components
    local ssh_config=$(get_ssh_config)
    local ssh_key=$(echo "$ssh_config" | grep -o '\-i [^ ]*' | cut -d' ' -f2)
    local ssh_user=$(echo "$ssh_config" | grep -o '\-l [^ ]*' | cut -d' ' -f2)
    
    # Build SSH command with proper option handling
    local ssh_cmd="ssh"
    [ -n "$ssh_key" ] && ssh_cmd="$ssh_cmd -i $ssh_key"
    [ -n "$ssh_user" ] && ssh_cmd="$ssh_cmd -l $ssh_user"
    ssh_cmd="$ssh_cmd -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
    
    debug "SSH to $host: $command"
    $ssh_cmd "$host" "$command"
}

# Copy file to remote host using context-aware SSH config
scp_remote() {
    local source="$1"
    local host="$2"
    local dest="$3"
    
    # Get SSH config components
    local ssh_config=$(get_ssh_config)
    local ssh_key=$(echo "$ssh_config" | grep -o '\-i [^ ]*' | cut -d' ' -f2)
    local ssh_user=$(echo "$ssh_config" | grep -o '\-l [^ ]*' | cut -d' ' -f2)
    
    # Build SCP command with proper option handling
    local scp_cmd="scp"
    [ -n "$ssh_key" ] && scp_cmd="$scp_cmd -i $ssh_key"
    scp_cmd="$scp_cmd -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
    
    # Add user to host if specified
    local target="$host"
    [ -n "$ssh_user" ] && target="${ssh_user}@${host}"
    
    debug "SCP $source to $target:$dest"
    $scp_cmd "$source" "$target:$dest"
}

# Find active operator from list (with failover)
find_active_operator() {
    local operators=$(get_all_operators)
    
    for operator in $operators; do
        if curl -s -f -m 2 "$operator/health" >/dev/null 2>&1; then
            debug "Active operator found: $operator"
            echo "$operator"
            return 0
        else
            debug "Operator $operator is not responding"
        fi
    done
    
    error "No active operators found"
    return 1
}