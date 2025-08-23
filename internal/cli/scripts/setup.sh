#!/bin/bash

# Setup command for Zeitwork CLI

setup_usage() {
    cat << EOF
Setup a new Zeitwork cluster

Usage: zeitwork setup [options]

Options:
    --operator <ip>       IP address of the operator node
    --workers <ips>       Comma-separated list of worker node IPs
    --database <url>      PostgreSQL database URL
    --region <name>       Region name (default: us-east-1)
    --ssh-key <path>      Path to SSH private key
    --user <username>     SSH username (default: root)
    --from-config        Use settings from current context
    --force              Force rebuild and redeploy (cleans everything)
    --build               Build binaries before setup
    --skip-database       Skip database setup
    --dry-run            Show what would be done without executing
    -h, --help           Show this help message

Examples:
    # Setup operator only
    zeitwork setup --operator 10.0.1.10 --database postgres://user:pass@localhost/zeitwork

    # Setup operator and workers
    zeitwork setup --operator 10.0.1.10 --workers 10.0.1.11,10.0.1.12 --database postgres://...

    # Setup with custom SSH key
    zeitwork setup --operator 10.0.1.10 --ssh-key ~/.ssh/id_rsa --user ubuntu

EOF
}

setup_command() {
    # Load environment variables for current context if available
    load_env_file 2>/dev/null || true
    
    local operator_ip=""
    local operator_ips=""  # For multiple operators
    local worker_ips=""
    local database_url=""
    local region="us-east-1"
    local ssh_key=""
    local ssh_user=""
    local build_first=false
    local skip_database=false
    local dry_run=false
    local use_config=false
    local force=false

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --operator)
                operator_ip="$2"
                shift 2
                ;;
            --workers)
                worker_ips="$2"
                shift 2
                ;;
            --database)
                database_url="$2"
                shift 2
                ;;
            --region)
                region="$2"
                shift 2
                ;;
            --ssh-key)
                ssh_key="$2"
                shift 2
                ;;
            --user)
                ssh_user="$2"
                shift 2
                ;;
            --from-config)
                use_config=true
                shift
                ;;
            --force)
                force=true
                shift
                ;;
            --build)
                build_first=true
                shift
                ;;
            --skip-database)
                skip_database=true
                shift
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            -h|--help)
                setup_usage
                return 0
                ;;
            *)
                error "Unknown option: $1"
                setup_usage
                return 1
                ;;
        esac
    done
    
    # If --from-config, load settings from current context
    if [ "$use_config" == "true" ]; then
        log "Loading configuration from current context: $(get_current_context)"
        
        # Always build fresh Linux binaries when using --from-config or force
        show_progress "Building fresh Linux binaries"
        make clean >/dev/null 2>&1
        if make build >/dev/null 2>&1; then
            end_progress "Linux binaries built successfully"
        else
            error "Failed to build binaries"
            return 1
        fi
        
        # Get cluster name from current context
        local context=$(get_current_context)
        local config_file="${ZEITWORK_CONFIG:-$HOME/.zeitwork/config.yaml}"
        local cluster_name=$(grep -A3 "name: $context" "$config_file" | grep "cluster:" | cut -d: -f2 | tr -d ' ')
        
        # Get region from cluster configuration
        if [ -n "$cluster_name" ]; then
            region=$(awk "/^  $cluster_name:/,/^[a-z]|^$/" "$config_file" | grep "region:" | cut -d: -f2 | tr -d ' ')
        fi
        
        # Get ALL operators from config (not just the first one)
        local operators=$(get_all_operators)
        # Store all operator IPs in a comma-separated list
        operator_ip=$(echo "$operators" | sed 's|http://||g' | sed 's|:[0-9]*||g' | tr '\n' ',' | sed 's/,$//')
        
        # Get workers from config
        worker_ips=$(get_nodes_from_config | tr '\n' ',' | sed 's/,$//')
        
        # Get database URL from environment
        database_url="${DATABASE_URL}"
        
        # Get SSH config
        local ssh_config=$(get_ssh_config)
        # Parse SSH key from config (this is simplified, might need adjustment)
        ssh_key=$(echo "$ssh_config" | grep -o '\-i [^ ]*' | cut -d' ' -f2)
        ssh_user=$(echo "$ssh_config" | grep -o '\-l [^ ]*' | cut -d' ' -f2)
    fi
    
    # Set defaults if not specified
    [ -z "$ssh_key" ] && ssh_key="$HOME/.ssh/id_rsa"
    [ -z "$ssh_user" ] && ssh_user="root"

    # Validate required parameters
    if [ -z "$operator_ip" ]; then
        error "Operator IP is required"
        setup_usage
        return 1
    fi

    if [ -z "$database_url" ] && [ "$skip_database" != "true" ]; then
        error "Database URL is required unless --skip-database is specified"
        setup_usage
        return 1
    fi

    # Validate operator IP addresses
    IFS=',' read -ra OPS <<< "$operator_ip"
    for op in "${OPS[@]}"; do
        if [ -n "$op" ] && ! is_valid_ip "$op"; then
            error "Invalid operator IP address: $op"
            return 1
        fi
    done

    if [ -n "$worker_ips" ]; then
        IFS=',' read -ra WORKERS <<< "$worker_ips"
        for worker in "${WORKERS[@]}"; do
            if ! is_valid_ip "$worker"; then
                error "Invalid worker IP address: $worker"
                return 1
            fi
        done
    fi

    # Check SSH key
    if [ ! -f "$ssh_key" ]; then
        error "SSH key not found: $ssh_key"
        return 1
    fi

    log "Starting Zeitwork cluster setup"
    log "Operators: ${operator_ip:-none}"
    [ -n "$worker_ips" ] && log "Workers: $worker_ips"
    log "Region: $region"

    if [ "$dry_run" == "true" ]; then
        warning "DRY RUN MODE - No changes will be made"
    fi

    # Build binaries if requested or force mode
    if [ "$build_first" == "true" ] || [ "$force" == "true" ]; then
        if [ "$dry_run" != "true" ]; then
            # Skip if already built by --from-config
            if [ "$use_config" != "true" ]; then
                show_progress "Building binaries"
                make clean >/dev/null 2>&1
                if make build >/dev/null 2>&1; then
                    end_progress "Binaries built successfully"
                else
                    error "Failed to build binaries"
                    return 1
                fi
            fi
        else
            log "[DRY RUN] Would build binaries"
        fi
    fi

    # Setup database
    if [ "$skip_database" != "true" ]; then
        if [ "$dry_run" != "true" ]; then
            setup_database "$database_url"
        else
            log "[DRY RUN] Would setup database with URL: $database_url"
        fi
    fi

    # Setup ALL operator nodes
    # operator_ip may contain comma-separated IPs when from config
    IFS=',' read -ra OPERATORS <<< "$operator_ip"
    
    for op_ip in "${OPERATORS[@]}"; do
        if [ -n "$op_ip" ]; then
            if [ "$dry_run" != "true" ]; then
                setup_operator_node "$op_ip" "$database_url" "$ssh_key" "$ssh_user"
            else
                log "[DRY RUN] Would setup operator node at $op_ip"
            fi
        fi
    done

    # Setup worker nodes
    if [ -n "$worker_ips" ]; then
        IFS=',' read -ra WORKERS <<< "$worker_ips"
        for worker in "${WORKERS[@]}"; do
            if [ "$dry_run" != "true" ]; then
                setup_worker_node "$worker" "$operator_ip" "$ssh_key" "$ssh_user"
            else
                log "[DRY RUN] Would setup worker node at $worker"
            fi
        done
    fi

    # Register nodes in database
    if [ "$skip_database" != "true" ] && [ "$dry_run" != "true" ]; then
        register_nodes "$database_url" "$operator_ip" "$worker_ips" "$region"
    elif [ "$dry_run" == "true" ]; then
        log "[DRY RUN] Would register nodes in database"
    fi

    success "Zeitwork cluster setup complete!"
    
    if [ "$dry_run" != "true" ]; then
        log ""
        log "Next steps:"
        log "  1. Verify services: zeitwork status"
        log "  2. Deploy an app: zeitwork deploy --project myapp --image myapp:latest"
        log "  3. View logs: zeitwork logs operator"
    fi
}

setup_database() {
    local db_url="$1"
    
    show_progress "Setting up database"
    
    if ! check_database "$db_url"; then
        error "Cannot connect to database"
        return 1
    fi
    
    # Run migrations
    if [ -d "packages/database" ]; then
        (cd packages/database && bun run db:migrate) >/dev/null 2>&1
        end_progress "Database setup complete"
    else
        warning "Database migration scripts not found"
    fi
}

setup_operator_node() {
    local ip="$1"
    local db_url="$2"
    local ssh_key="$3"
    local ssh_user="$4"
    
    log "Setting up operator node at $ip"
    
    # Create tar archive of binaries
    show_progress "Packaging binaries"
    tar -czf /tmp/zeitwork-binaries.tar.gz -C build . 2>/dev/null
    end_progress "Binaries packaged"
    
    # Copy binaries to operator
    show_progress "Copying binaries to operator"
    scp_remote "/tmp/zeitwork-binaries.tar.gz" "$ip" "/tmp/"
    end_progress "Binaries copied"
    
    # Copy setup script
    show_progress "Copying setup script"
    # Check if the old script exists, otherwise use a simplified version
    if [ -f "$SCRIPT_DIR/../zeitwork-cli/scripts/setup_operator.sh" ]; then
        scp_remote "$SCRIPT_DIR/../zeitwork-cli/scripts/setup_operator.sh" "$ip" "/tmp/"
    else
        # Create a minimal setup script
        cat > /tmp/setup_operator_minimal.sh << 'EOF'
#!/bin/bash
set -e
DATABASE_URL="$1"
cd /tmp
tar -xzf zeitwork-binaries.tar.gz
sudo cp -f zeitwork-* /usr/local/bin/
sudo chmod +x /usr/local/bin/zeitwork-*
echo "Operator setup complete"
EOF
        scp_remote "/tmp/setup_operator_minimal.sh" "$ip" "/tmp/setup_operator.sh"
        rm -f /tmp/setup_operator_minimal.sh
    fi
    end_progress "Setup script copied"
    
    # Run setup on operator
    show_progress "Running operator setup"
    ssh_remote "$ip" "chmod +x /tmp/setup_operator.sh && sudo /tmp/setup_operator.sh '$db_url'"
    end_progress "Operator setup complete"
    
    # Wait for operator to be ready
    wait_for_port "$ip" 8080 60
    
    rm -f /tmp/zeitwork-binaries.tar.gz
}

setup_worker_node() {
    local ip="$1"
    local operator_ip="$2"
    local ssh_key="$3"
    local ssh_user="$4"
    
    log "Setting up worker node at $ip"
    
    # Ensure operator_ip is a proper URL
    # If it contains multiple IPs (comma-separated), use the first one
    local first_operator_ip=$(echo "$operator_ip" | cut -d',' -f1)
    local operator_url=""
    
    # Check if it's already a URL or just an IP
    if [[ "$first_operator_ip" == http* ]]; then
        operator_url="$first_operator_ip"
    else
        operator_url="http://${first_operator_ip}:8080"
    fi
    
    debug "Using operator URL: $operator_url"
    
    # Clean any stale files
    rm -rf /tmp/worker-build /tmp/zeitwork-node-agent /tmp/worker-binaries.tar.gz
    
    # Create minimal tar with just node-agent
    show_progress "Packaging node agent"
    mkdir -p /tmp/worker-build/build
    cp build/zeitwork-node-agent /tmp/worker-build/build/
    tar -czf /tmp/worker-binaries.tar.gz -C /tmp/worker-build . 2>/dev/null
    end_progress "Node agent packaged"
    
    # Copy binary to worker
    show_progress "Copying binary to worker"
    scp_remote "/tmp/worker-binaries.tar.gz" "$ip" "/tmp/"
    end_progress "Binary copied"
    
    # Copy setup script
    show_progress "Copying setup script"
    if [ -f "$SCRIPT_DIR/../zeitwork-cli/scripts/setup_worker.sh" ]; then
        scp_remote "$SCRIPT_DIR/../zeitwork-cli/scripts/setup_worker.sh" "$ip" "/tmp/"
    else
        # Create a minimal worker setup script
        cat > /tmp/setup_worker_minimal.sh << 'EOF'
#!/bin/bash
set -e
OPERATOR_URL="$1"
cd /tmp
tar -xzf worker-binaries.tar.gz
sudo cp -f build/zeitwork-node-agent /usr/local/bin/
sudo chmod +x /usr/local/bin/zeitwork-node-agent
echo "Worker setup complete"
EOF
        scp_remote "/tmp/setup_worker_minimal.sh" "$ip" "/tmp/setup_worker.sh"
        rm -f /tmp/setup_worker_minimal.sh
    fi
    end_progress "Setup script copied"
    
    # Run setup on worker - pass the full operator URL
    show_progress "Running worker setup"
    ssh_remote "$ip" "chmod +x /tmp/setup_worker.sh && sudo /tmp/setup_worker.sh '$operator_url'"
    end_progress "Worker setup complete"
    
    # Wait for node agent to be ready
    wait_for_port "$ip" 8081 60
    
    rm -rf /tmp/worker-build /tmp/worker-binaries.tar.gz
}

register_nodes() {
    local db_url="$1"
    local operator_ips="$2"
    local worker_ips="$3"
    local region_name="$4"
    
    show_progress "Registering nodes in database"
    
    # First, create or get the region
    local region_id=""
    if command_exists psql; then
        # Check if region exists
        region_id=$(psql "$db_url" -t -c "SELECT id FROM regions WHERE code = '$region_name'" 2>/dev/null | tr -d ' ')
        
        if [ -z "$region_id" ]; then
            # Create region with a new UUID
            region_id=$(uuidgen | tr '[:upper:]' '[:lower:]')
            local region_sql="INSERT INTO regions (id, name, code, country) 
                             VALUES ('$region_id', '$region_name', '$region_name', 'Unknown')
                             ON CONFLICT (code) DO UPDATE SET code = EXCLUDED.code
                             RETURNING id;"
            region_id=$(psql "$db_url" -t -c "$region_sql" 2>/dev/null | tr -d ' ')
            debug "Created region $region_name with ID $region_id"
        else
            debug "Using existing region $region_name with ID $region_id"
        fi
    fi
    
    # Register all operator nodes
    IFS=',' read -ra OPERATORS <<< "$operator_ips"
    for op_ip in "${OPERATORS[@]}"; do
        if [ -n "$op_ip" ]; then
            # Get system info from the remote operator
            local cpu_count=$(ssh_remote "$op_ip" "nproc 2>/dev/null || echo 4")
            local memory_mb=$(ssh_remote "$op_ip" "free -m 2>/dev/null | awk '/^Mem:/ {print \$2}' || echo 8192")
            local disk_gb=$(ssh_remote "$op_ip" "df -h / 2>/dev/null | awk 'NR==2 {print int(\$2)}' || echo 100")
            
            # Create JSONB resources object
            local resources='{"cpu_cores": '$cpu_count', "memory_mb": '$memory_mb', "disk_gb": '$disk_gb', "type": "operator"}'
            
            local node_id=$(uuidgen | tr '[:upper:]' '[:lower:]')
            local hostname="operator-${op_ip//./-}"
            
            local register_sql="INSERT INTO nodes (id, region_id, hostname, ip_address, state, resources) 
                               VALUES ('$node_id', '$region_id', '$hostname', '$op_ip', 'active', '$resources'::jsonb)
                               ON CONFLICT (hostname) DO UPDATE 
                               SET ip_address = EXCLUDED.ip_address,
                                   state = EXCLUDED.state,
                                   resources = EXCLUDED.resources,
                                   updated_at = now();"
            
            if command_exists psql; then
                psql "$db_url" -c "$register_sql" >/dev/null 2>&1
                debug "Registered operator node: $op_ip"
            fi
        fi
    done
    
    # Register worker nodes
    if [ -n "$worker_ips" ]; then
        IFS=',' read -ra WORKERS <<< "$worker_ips"
        for worker_ip in "${WORKERS[@]}"; do
            if [ -n "$worker_ip" ]; then
                # Get system info from the remote worker
                local cpu_count=$(ssh_remote "$worker_ip" "nproc 2>/dev/null || echo 4")
                local memory_mb=$(ssh_remote "$worker_ip" "free -m 2>/dev/null | awk '/^Mem:/ {print \$2}' || echo 8192")
                local disk_gb=$(ssh_remote "$worker_ip" "df -h / 2>/dev/null | awk 'NR==2 {print int(\$2)}' || echo 100")
                
                # Create JSONB resources object
                local resources='{"cpu_cores": '$cpu_count', "memory_mb": '$memory_mb', "disk_gb": '$disk_gb', "type": "worker"}'
                
                local node_id=$(uuidgen | tr '[:upper:]' '[:lower:]')
                local hostname="worker-${worker_ip//./-}"
                
                local register_sql="INSERT INTO nodes (id, region_id, hostname, ip_address, state, resources) 
                                   VALUES ('$node_id', '$region_id', '$hostname', '$worker_ip', 'active', '$resources'::jsonb)
                                   ON CONFLICT (hostname) DO UPDATE 
                                   SET ip_address = EXCLUDED.ip_address,
                                       state = EXCLUDED.state,
                                       resources = EXCLUDED.resources,
                                       updated_at = now();"
                
                if command_exists psql; then
                    psql "$db_url" -c "$register_sql" >/dev/null 2>&1
                    debug "Registered worker node: $worker_ip"
                fi
            fi
        done
    fi
    
    end_progress "Nodes registered"
}