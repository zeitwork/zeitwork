#!/bin/bash

# Cleanup command for Zeitwork CLI

cleanup_usage() {
    cat << EOF
Clean up Zeitwork resources

Usage: zeitwork cleanup [options]

Options:
    --all                 Remove everything (requires confirmation)
    --deployments         Remove all deployments
    --deployment <id>     Remove specific deployment
    --nodes               Unregister all nodes
    --node <ip>           Unregister specific node
    --services            Stop and disable all services
    --data                Remove all data directories
    --logs                Clear all log files
    --operator <url>      Operator URL (default: http://localhost:8080)
    --force               Skip confirmation prompts
    --dry-run            Show what would be removed without doing it
    -h, --help           Show this help message

Examples:
    # Remove specific deployment
    zeitwork cleanup --deployment abc123

    # Stop all services
    zeitwork cleanup --services

    # Full cleanup (dangerous!)
    zeitwork cleanup --all --force

    # See what would be cleaned
    zeitwork cleanup --all --dry-run

EOF
}

cleanup_command() {
    # Load environment variables for current context
    load_env_file
    
    local cleanup_all=false
    local cleanup_deployments=false
    local deployment_id=""
    local cleanup_nodes=false
    local node_ip=""
    local cleanup_services=false
    local cleanup_data=false
    local cleanup_logs=false
    local operator_url=""  # Will be set from config or argument
    local force=false
    local dry_run=false

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --all)
                cleanup_all=true
                shift
                ;;
            --deployments)
                cleanup_deployments=true
                shift
                ;;
            --deployment)
                deployment_id="$2"
                shift 2
                ;;
            --nodes)
                cleanup_nodes=true
                shift
                ;;
            --node)
                node_ip="$2"
                shift 2
                ;;
            --services)
                cleanup_services=true
                shift
                ;;
            --data)
                cleanup_data=true
                shift
                ;;
            --logs)
                cleanup_logs=true
                shift
                ;;
            --operator)
                operator_url="$2"
                shift 2
                ;;
            --force)
                force=true
                shift
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            -h|--help)
                cleanup_usage
                return 0
                ;;
            *)
                error "Unknown option: $1"
                cleanup_usage
                return 1
                ;;
        esac
    done

    # If --all is specified, enable all cleanup options
    if [ "$cleanup_all" == "true" ]; then
        cleanup_deployments=true
        cleanup_nodes=true
        cleanup_services=true
        cleanup_data=true
        cleanup_logs=true
    fi

    # Confirm dangerous operations
    if [ "$force" != "true" ] && [ "$dry_run" != "true" ]; then
        if [ "$cleanup_all" == "true" ]; then
            warning "This will remove ALL Zeitwork resources and data!"
            if ! confirm "Are you absolutely sure?" "n"; then
                log "Cleanup cancelled"
                return 0
            fi
        elif [ "$cleanup_data" == "true" ]; then
            warning "This will remove all data directories!"
            if ! confirm "Are you sure?" "n"; then
                log "Cleanup cancelled"
                return 0
            fi
        fi
    fi

    if [ "$dry_run" == "true" ]; then
        warning "DRY RUN MODE - No changes will be made"
    fi

    # If operator URL not explicitly provided, find active operator from config
    if [ -z "$operator_url" ]; then
        operator_url="${OPERATOR_URL:-$(find_active_operator)}"
        # Only need operator URL for deployment/node cleanup
        if [ -z "$operator_url" ] && ( [ "$cleanup_deployments" == "true" ] || [ "$cleanup_nodes" == "true" ] || [ -n "$deployment_id" ] || [ -n "$node_ip" ] ); then
            error "No operator URL configured and cannot find active operator"
            return 1
        fi
        debug "Using operator: $operator_url"
    fi

    # Cleanup deployments
    if [ "$cleanup_deployments" == "true" ] || [ -n "$deployment_id" ]; then
        cleanup_deployments_func "$operator_url" "$deployment_id" "$dry_run"
    fi

    # Cleanup nodes
    if [ "$cleanup_nodes" == "true" ] || [ -n "$node_ip" ]; then
        cleanup_nodes_func "$operator_url" "$node_ip" "$dry_run"
    fi

    # Cleanup services
    if [ "$cleanup_services" == "true" ]; then
        cleanup_services_func "$dry_run"
    fi

    # Cleanup data
    if [ "$cleanup_data" == "true" ]; then
        cleanup_data_func "$dry_run"
    fi

    # Cleanup logs
    if [ "$cleanup_logs" == "true" ]; then
        cleanup_logs_func "$dry_run"
    fi

    success "Cleanup complete!"
}

cleanup_deployments_func() {
    local operator_url="$1"
    local deployment_id="$2"
    local dry_run="$3"

    if [ -n "$deployment_id" ]; then
        # Remove specific deployment
        show_progress "Removing deployment $deployment_id"
        if [ "$dry_run" != "true" ]; then
            local response=$(curl -s -X DELETE "$operator_url/api/deployments/$deployment_id" 2>/dev/null)
            if [[ "$response" == *"error"* ]]; then
                error "Failed to remove deployment: $deployment_id"
                debug "Response: $response"
            else
                end_progress "Deployment removed: $deployment_id"
            fi
        else
            log "[DRY RUN] Would remove deployment: $deployment_id"
        fi
    else
        # Remove all deployments
        show_progress "Removing all deployments"
        if [ "$dry_run" != "true" ]; then
            local deployments=$(curl -s "$operator_url/api/deployments" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
            local count=0
            while IFS= read -r dep_id; do
                if [ -n "$dep_id" ]; then
                    curl -s -X DELETE "$operator_url/api/deployments/$dep_id" >/dev/null 2>&1
                    ((count++))
                fi
            done <<< "$deployments"
            end_progress "Removed $count deployments"
        else
            log "[DRY RUN] Would remove all deployments"
        fi
    fi
}

cleanup_nodes_func() {
    local operator_url="$1"
    local node_ip="$2"
    local dry_run="$3"

    if [ -n "$node_ip" ]; then
        # Unregister specific node
        show_progress "Unregistering node $node_ip"
        if [ "$dry_run" != "true" ]; then
            # First, stop node-agent on the node
            ssh_remote "$node_ip" "systemctl stop zeitwork-node-agent" 2>/dev/null || true
            
            # Then unregister from operator
            local response=$(curl -s -X DELETE "$operator_url/api/nodes/$node_ip" 2>/dev/null)
            if [[ "$response" == *"error"* ]]; then
                warning "Failed to unregister node: $node_ip"
            else
                end_progress "Node unregistered: $node_ip"
            fi
        else
            log "[DRY RUN] Would unregister node: $node_ip"
        fi
    else
        # Unregister all nodes
        show_progress "Unregistering all nodes"
        if [ "$dry_run" != "true" ]; then
            local nodes=$(curl -s "$operator_url/api/nodes" | grep -o '"ip_address":"[^"]*' | cut -d'"' -f4)
            local count=0
            while IFS= read -r ip; do
                if [ -n "$ip" ]; then
                    # Stop node-agent
                    ssh_remote "$ip" "systemctl stop zeitwork-node-agent" 2>/dev/null || true
                    # Unregister
                    curl -s -X DELETE "$operator_url/api/nodes/$ip" >/dev/null 2>&1
                    ((count++))
                fi
            done <<< "$nodes"
            end_progress "Unregistered $count nodes"
        else
            log "[DRY RUN] Would unregister all nodes"
        fi
    fi
}

cleanup_services_func() {
    local dry_run="$1"

    show_progress "Stopping and disabling services on all nodes"
    
    # Get all nodes and operators from config
    local nodes=$(get_nodes_from_config)
    local operators=$(get_all_operators)
    
    # Stop operator services on operator nodes
    for operator_url in $operators; do
        local host=$(echo "$operator_url" | sed 's|http://||' | cut -d: -f1)
        if [ "$dry_run" != "true" ]; then
            debug "Stopping services on operator node $host"
            ssh_remote "$host" "systemctl stop zeitwork-operator zeitwork-load-balancer zeitwork-edge-proxy 2>/dev/null || true" || true
            ssh_remote "$host" "systemctl disable zeitwork-operator zeitwork-load-balancer zeitwork-edge-proxy 2>/dev/null || true" || true
        else
            log "[DRY RUN] Would stop services on operator node $host"
        fi
    done
    
    # Stop node-agent on worker nodes
    for node in $nodes; do
        if [ "$dry_run" != "true" ]; then
            debug "Stopping node-agent on $node"
            ssh_remote "$node" "systemctl stop zeitwork-node-agent 2>/dev/null || true" || true
            ssh_remote "$node" "systemctl disable zeitwork-node-agent 2>/dev/null || true" || true
        else
            log "[DRY RUN] Would stop node-agent on $node"
        fi
    done
    
    if [ "$dry_run" != "true" ]; then
        end_progress "Services stopped and disabled on all nodes"
    fi
}

cleanup_data_func() {
    local dry_run="$1"

    show_progress "Removing data directories"
    
    local dirs=("/var/lib/zeitwork" "/etc/zeitwork" "/var/log/zeitwork" "/var/log/firecracker")
    
    for dir in "${dirs[@]}"; do
        if [ -d "$dir" ]; then
            if [ "$dry_run" != "true" ]; then
                debug "Removing $dir"
                sudo rm -rf "$dir"
            else
                log "[DRY RUN] Would remove directory: $dir"
            fi
        fi
    done
    
    # Remove binaries
    local binaries=("/usr/local/bin/zeitwork-operator" "/usr/local/bin/zeitwork-node-agent" 
                   "/usr/local/bin/zeitwork-load-balancer" "/usr/local/bin/zeitwork-edge-proxy")
    
    for binary in "${binaries[@]}"; do
        if [ -f "$binary" ]; then
            if [ "$dry_run" != "true" ]; then
                debug "Removing $binary"
                sudo rm -f "$binary"
            else
                log "[DRY RUN] Would remove binary: $binary"
            fi
        fi
    done
    
    if [ "$dry_run" != "true" ]; then
        end_progress "Data directories removed"
    fi
}

cleanup_logs_func() {
    local dry_run="$1"

    show_progress "Clearing log files"
    
    if [ "$dry_run" != "true" ]; then
        # Clear systemd logs for our services
        local services=("zeitwork-operator" "zeitwork-node-agent" "zeitwork-load-balancer" "zeitwork-edge-proxy")
        for service in "${services[@]}"; do
            sudo journalctl --vacuum-time=1s -u "$service" 2>/dev/null || true
        done
        
        # Clear log directories
        if [ -d "/var/log/zeitwork" ]; then
            sudo find /var/log/zeitwork -type f -name "*.log" -delete 2>/dev/null || true
        fi
        
        if [ -d "/var/log/firecracker" ]; then
            sudo find /var/log/firecracker -type f -name "*.log" -delete 2>/dev/null || true
        fi
        
        end_progress "Log files cleared"
    else
        log "[DRY RUN] Would clear all log files"
    fi
}

# Helper function to remove firecracker VMs
cleanup_vms() {
    local dry_run="$1"
    
    show_progress "Stopping all Firecracker VMs"
    
    if [ "$dry_run" != "true" ]; then
        # Kill all firecracker processes
        sudo pkill -9 firecracker 2>/dev/null || true
        
        # Clean up socket files
        sudo rm -f /tmp/firecracker-*.sock 2>/dev/null || true
        
        # Clean up TAP interfaces
        local tap_interfaces=$(ip link show | grep -o 'tap[0-9]*' || true)
        while IFS= read -r tap; do
            if [ -n "$tap" ]; then
                sudo ip link delete "$tap" 2>/dev/null || true
            fi
        done <<< "$tap_interfaces"
        
        end_progress "VMs stopped and cleaned up"
    else
        log "[DRY RUN] Would stop all Firecracker VMs"
    fi
}