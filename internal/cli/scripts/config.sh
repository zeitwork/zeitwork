#!/bin/bash

# Config command for Zeitwork CLI

config_usage() {
    cat << EOF
Manage Zeitwork CLI configuration and contexts

Usage: zeitwork config <subcommand> [options]

Subcommands:
    current-context         Show the current context
    use-context <name>      Switch to a different context
    get-contexts            List all available contexts
    get-clusters            List all configured clusters
    set <key> <value>       Set a configuration value
    init                    Initialize configuration with defaults
    validate                Validate configuration and connectivity
    -h, --help             Show this help message

Examples:
    # Show current context
    zeitwork config current-context

    # Switch to production context
    zeitwork config use-context production

    # List all contexts
    zeitwork config get-contexts

    # Initialize configuration
    zeitwork config init

    # Validate configuration
    zeitwork config validate

EOF
}

config_command() {
    local subcommand="${1:-}"
    shift || true

    case "$subcommand" in
        current-context)
            show_current_context
            ;;
        use-context)
            use_context "$@"
            ;;
        get-contexts)
            list_contexts
            ;;
        get-clusters)
            list_clusters
            ;;
        set)
            set_config "$@"
            ;;
        init)
            init_config "$@"
            ;;
        validate)
            validate_config "$@"
            ;;
        -h|--help|"")
            config_usage
            ;;
        *)
            error "Unknown subcommand: $subcommand"
            config_usage
            return 1
            ;;
    esac
}

# Get config file path
get_config_path() {
    echo "${ZEITWORK_CONFIG:-$HOME/.zeitwork/config.yaml}"
}

# Get env file path for a context
get_env_file_path() {
    local context="$1"
    echo "$HOME/.zeitwork/.env.$context"
}

# Show current context
show_current_context() {
    local config_file=$(get_config_path)
    
    if [ ! -f "$config_file" ]; then
        error "No configuration found. Run 'zeitwork config init' to create one."
        return 1
    fi
    
    local current=$(grep "^current-context:" "$config_file" | cut -d: -f2 | tr -d ' ')
    
    if [ -z "$current" ]; then
        warning "No current context set"
    else
        echo "$current"
    fi
}

# Switch to a different context
use_context() {
    local context="$1"
    
    if [ -z "$context" ]; then
        error "Context name required"
        return 1
    fi
    
    local config_file=$(get_config_path)
    
    if [ ! -f "$config_file" ]; then
        error "No configuration found. Run 'zeitwork config init' to create one."
        return 1
    fi
    
    # Check if context exists
    if ! grep -q "^  - name: $context" "$config_file"; then
        error "Context '$context' not found"
        return 1
    fi
    
    # Update current-context
    if [ "$(uname)" == "Darwin" ]; then
        # macOS sed requires backup extension
        sed -i '' "s/^current-context:.*/current-context: $context/" "$config_file"
    else
        sed -i "s/^current-context:.*/current-context: $context/" "$config_file"
    fi
    
    success "Switched to context: $context"
    
    # Load and validate env file
    local env_file=$(get_env_file_path "$context")
    if [ ! -f "$env_file" ]; then
        warning "Environment file not found: $env_file"
        warning "Create it with your database credentials and other secrets"
    fi
}

# List all contexts
list_contexts() {
    local config_file=$(get_config_path)
    
    if [ ! -f "$config_file" ]; then
        error "No configuration found. Run 'zeitwork config init' to create one."
        return 1
    fi
    
    local current=$(show_current_context)
    
    echo "CURRENT   NAME          CLUSTER              USER"
    echo "-------   -----------   -----------------    ---------------"
    
    # Parse contexts from YAML
    local in_contexts=false
    local name=""
    local cluster=""
    local user=""
    
    while IFS= read -r line; do
        if [[ "$line" == "contexts:" ]]; then
            in_contexts=true
        elif [[ "$line" == "clusters:" ]] || [[ "$line" == "users:" ]]; then
            in_contexts=false
        elif [ "$in_contexts" == "true" ]; then
            if [[ "$line" =~ ^[[:space:]]*-[[:space:]]name:[[:space:]](.+) ]]; then
                name="${BASH_REMATCH[1]}"
            elif [[ "$line" =~ ^[[:space:]]*cluster:[[:space:]](.+) ]]; then
                cluster="${BASH_REMATCH[1]}"
            elif [[ "$line" =~ ^[[:space:]]*user:[[:space:]](.+) ]]; then
                user="${BASH_REMATCH[1]}"
                
                # Print the context line
                if [ "$name" == "$current" ]; then
                    printf "*         %-13s %-20s %s\n" "$name" "$cluster" "$user"
                else
                    printf "          %-13s %-20s %s\n" "$name" "$cluster" "$user"
                fi
                
                # Reset for next context
                name=""
                cluster=""
                user=""
            fi
        fi
    done < "$config_file"
}

# List all clusters
list_clusters() {
    local config_file=$(get_config_path)
    
    if [ ! -f "$config_file" ]; then
        error "No configuration found. Run 'zeitwork config init' to create one."
        return 1
    fi
    
    echo "CLUSTER              REGION        OPERATORS    NODES"
    echo "------------------   -----------   ----------   ------"
    
    # This is a simplified version - you might want to use a proper YAML parser
    local cluster_name=""
    local region=""
    local operator_count=0
    local node_count=0
    local in_clusters=false
    
    while IFS= read -r line; do
        if [[ "$line" == "clusters:" ]]; then
            in_clusters=true
        elif [[ "$line" == "users:" ]]; then
            in_clusters=false
        elif [ "$in_clusters" == "true" ]; then
            if [[ "$line" =~ ^[[:space:]]{2}[^[:space:]].*:$ ]]; then
                # New cluster
                if [ -n "$cluster_name" ]; then
                    printf "%-20s %-13s %-12d %d\n" "$cluster_name" "$region" "$operator_count" "$node_count"
                fi
                cluster_name=$(echo "$line" | cut -d: -f1 | tr -d ' ')
                operator_count=0
                node_count=0
                region="unknown"
            elif [[ "$line" =~ region:[[:space:]](.+) ]]; then
                region="${BASH_REMATCH[1]}"
            elif [[ "$line" =~ -[[:space:]]host: ]] && [[ "$prev_line" == *"operators:"* ]]; then
                ((operator_count++))
            elif [[ "$line" =~ -[[:space:]]host: ]] && [[ "$prev_line" == *"nodes:"* ]]; then
                ((node_count++))
            fi
        fi
        prev_line="$line"
    done < "$config_file"
    
    # Print last cluster
    if [ -n "$cluster_name" ]; then
        printf "%-20s %-13s %-12d %d\n" "$cluster_name" "$region" "$operator_count" "$node_count"
    fi
}

# Initialize configuration
init_config() {
    local config_dir="$HOME/.zeitwork"
    local config_file="$config_dir/config.yaml"
    
    # Create directory if it doesn't exist
    if [ ! -d "$config_dir" ]; then
        log "Creating configuration directory: $config_dir"
        mkdir -p "$config_dir"
    fi
    
    # Check if config already exists
    if [ -f "$config_file" ]; then
        if ! confirm "Configuration already exists. Overwrite?" "n"; then
            log "Initialization cancelled"
            return 0
        fi
    fi
    
    # Create default configuration
    cat > "$config_file" << 'EOF'
current-context: local

contexts:
  - name: local
    cluster: local-cluster
    user: local-admin
    env-file: ~/.zeitwork/.env.local
  
  - name: production
    cluster: prod-cluster
    user: prod-admin
    env-file: ~/.zeitwork/.env.production
  
  - name: staging
    cluster: staging-cluster
    user: staging-admin
    env-file: ~/.zeitwork/.env.staging

clusters:
  local-cluster:
    region: local
    operators:
      - host: localhost
        port: 8080
        primary: true
    nodes:
      - host: localhost
        port: 8081
  
  prod-cluster:
    region: us-east-1
    operators:
      - host: 10.0.1.10
        port: 8080
        primary: true
      - host: 10.0.1.11
        port: 8080
        primary: false
    load-balancers:
      - host: 10.0.1.12
        port: 8082
      - host: 10.0.1.13
        port: 8082
    edge-proxies:
      - host: 10.0.1.14
        port: 443
    nodes:
      - host: 10.0.1.20
        port: 8081
        region: us-east-1a
        labels:
          type: compute
          tier: standard
      - host: 10.0.1.21
        port: 8081
        region: us-east-1b
        labels:
          type: compute
          tier: standard
  
  staging-cluster:
    region: us-west-2
    operators:
      - host: 10.0.2.10
        port: 8080
        primary: true
    nodes:
      - host: 10.0.2.20
        port: 8081
      - host: 10.0.2.21
        port: 8081

users:
  local-admin:
    ssh-key: ~/.ssh/id_rsa
    ssh-user: root
  
  prod-admin:
    ssh-key: ~/.ssh/id_rsa_prod
    ssh-user: ubuntu
    ssh-options: "-o StrictHostKeyChecking=no"
  
  staging-admin:
    ssh-key: ~/.ssh/id_rsa
    ssh-user: root
EOF
    
    success "Configuration initialized at: $config_file"
    
    # Create sample env files
    for env in local production staging; do
        local env_file="$config_dir/.env.$env"
        if [ ! -f "$env_file" ]; then
            cat > "$env_file" << EOF
# Zeitwork environment configuration for $env
# Add your actual credentials here

DATABASE_URL=postgres://user:password@host:5432/zeitwork_$env
OPERATOR_API_KEY=your-api-key-here
AWS_ACCESS_KEY_ID=your-aws-key
AWS_SECRET_ACCESS_KEY=your-aws-secret
DOCKER_REGISTRY_PASSWORD=your-registry-password
EOF
            log "Created sample env file: $env_file"
        fi
    done
    
    echo ""
    log "Next steps:"
    log "  1. Edit $config_file to add your cluster details"
    log "  2. Update the .env files in $config_dir with your credentials"
    log "  3. Run 'zeitwork config validate' to test connectivity"
    log "  4. Use 'zeitwork config use-context <name>' to switch contexts"
}

# Validate configuration
validate_config() {
    local config_file=$(get_config_path)
    
    if [ ! -f "$config_file" ]; then
        error "No configuration found. Run 'zeitwork config init' to create one."
        return 1
    fi
    
    local context=$(show_current_context)
    log "Validating configuration for context: $context"
    
    # Check env file
    local env_file=$(get_env_file_path "$context")
    if [ -f "$env_file" ]; then
        success "Environment file found: $env_file"
        
        # Check for required variables
        local required_vars=("DATABASE_URL")
        for var in "${required_vars[@]}"; do
            if grep -q "^$var=" "$env_file"; then
                success "  ✓ $var is set"
            else
                error "  ✗ $var is missing"
            fi
        done
    else
        error "Environment file not found: $env_file"
    fi
    
    # Get cluster info from config
    log "Checking cluster connectivity..."
    
    # This is a simplified check - in reality you'd parse the YAML properly
    # and test each operator/node
    local operators=$(grep -A10 "^  $context:" "$config_file" | grep "host:" | awk '{print $3}')
    
    for host in $operators; do
        if [ -n "$host" ]; then
            show_progress "Testing operator at $host"
            if curl -s -f -m 5 "http://$host:8080/health" >/dev/null 2>&1; then
                end_progress "Operator $host is reachable"
            else
                error "Cannot reach operator at $host:8080"
            fi
        fi
    done
    
    success "Configuration validation complete"
}

# Set a configuration value
set_config() {
    local key="$1"
    local value="$2"
    
    if [ -z "$key" ] || [ -z "$value" ]; then
        error "Both key and value are required"
        echo "Usage: zeitwork config set <key> <value>"
        return 1
    fi
    
    # For now, just handle current-context
    if [ "$key" == "current-context" ]; then
        use_context "$value"
    else
        error "Setting '$key' is not yet supported"
    fi
}