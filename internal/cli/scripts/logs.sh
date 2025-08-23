#!/bin/bash

# Logs command for Zeitwork CLI

logs_usage() {
    cat << EOF
View service and deployment logs

Usage: zeitwork logs <service|project> [options]

Arguments:
    service|project       Name of service or project to view logs for
                         Services: operator, node-agent, load-balancer, edge-proxy
                         Or: project name from deployment

Options:
    --tail <lines>        Number of lines to show (default: 100)
    --follow              Follow log output (like tail -f)
    --since <time>        Show logs since time (e.g., "5m", "1h", "2024-01-01")
    --node <ip>           Specific node to get logs from
    --container <id>      Specific container ID
    --operator <url>      Operator URL (default: http://localhost:8080)
    --grep <pattern>      Filter logs by pattern
    --no-color           Disable colored output
    -h, --help           Show this help message

Examples:
    # View operator logs
    zeitwork logs operator --tail 50

    # Follow node-agent logs
    zeitwork logs node-agent --follow

    # View project logs from last hour
    zeitwork logs myapp --since 1h

    # Filter error logs
    zeitwork logs operator --grep ERROR

EOF
}

logs_command() {
    # Load environment variables for current context
    load_env_file
    
    local target=""
    local tail_lines=100
    local follow=false
    local since=""
    local node_ip=""
    local container_id=""
    local operator_url=""  # Will be set from config or argument
    local grep_pattern=""
    local no_color=false

    # Get target (service or project name)
    if [ $# -gt 0 ] && [[ ! "$1" =~ ^-- ]]; then
        target="$1"
        shift
    fi

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --tail)
                tail_lines="$2"
                shift 2
                ;;
            --follow|-f)
                follow=true
                shift
                ;;
            --since)
                since="$2"
                shift 2
                ;;
            --node)
                node_ip="$2"
                shift 2
                ;;
            --container)
                container_id="$2"
                shift 2
                ;;
            --operator)
                operator_url="$2"
                shift 2
                ;;
            --grep)
                grep_pattern="$2"
                shift 2
                ;;
            --no-color)
                no_color=true
                shift
                ;;
            -h|--help)
                logs_usage
                return 0
                ;;
            *)
                error "Unknown option: $1"
                logs_usage
                return 1
                ;;
        esac
    done

    # Validate target
    if [ -z "$target" ]; then
        error "Service or project name is required"
        logs_usage
        return 1
    fi

    # If operator URL not explicitly provided, find active operator from config
    if [ -z "$operator_url" ]; then
        operator_url="${OPERATOR_URL:-$(find_active_operator)}"
        # Only need operator URL for project logs, not service logs
        if [ -z "$operator_url" ] && [[ ! "$target" =~ ^(operator|node-agent|load-balancer|edge-proxy|zeitwork-) ]]; then
            error "No operator URL configured and cannot find active operator"
            return 1
        fi
        debug "Using operator: $operator_url"
    fi

    # Determine if target is a service or project
    case "$target" in
        operator|zeitwork-operator)
            # Get operator host from config
            local operator_host=$(echo "${operator_url:-$(get_primary_operator)}" | sed 's|http://||' | cut -d: -f1)
            if [ -n "$operator_host" ] && [ "$operator_host" != "localhost" ]; then
                show_remote_service_logs "$operator_host" "zeitwork-operator" "$tail_lines" "$follow" "$since" "$grep_pattern" "$no_color"
            else
                show_service_logs "zeitwork-operator" "$tail_lines" "$follow" "$since" "$grep_pattern" "$no_color"
            fi
            ;;
        node-agent|zeitwork-node-agent)
            if [ -n "$node_ip" ]; then
                show_remote_service_logs "$node_ip" "zeitwork-node-agent" "$tail_lines" "$follow" "$since" "$grep_pattern" "$no_color"
            else
                # Show logs from all nodes
                local nodes=$(get_nodes_from_config)
                for node in $nodes; do
                    echo -e "${BOLD}Node: $node${NC}"
                    show_remote_service_logs "$node" "zeitwork-node-agent" "$tail_lines" "$follow" "$since" "$grep_pattern" "$no_color"
                    echo
                done
            fi
            ;;
        load-balancer|zeitwork-load-balancer)
            # Get load balancer hosts from config (not implemented yet, using operator for now)
            local operator_host=$(echo "${operator_url:-$(get_primary_operator)}" | sed 's|http://||' | cut -d: -f1)
            if [ -n "$operator_host" ] && [ "$operator_host" != "localhost" ]; then
                show_remote_service_logs "$operator_host" "zeitwork-load-balancer" "$tail_lines" "$follow" "$since" "$grep_pattern" "$no_color"
            else
                show_service_logs "zeitwork-load-balancer" "$tail_lines" "$follow" "$since" "$grep_pattern" "$no_color"
            fi
            ;;
        edge-proxy|zeitwork-edge-proxy)
            # Get edge proxy hosts from config (not implemented yet, using operator for now)
            local operator_host=$(echo "${operator_url:-$(get_primary_operator)}" | sed 's|http://||' | cut -d: -f1)
            if [ -n "$operator_host" ] && [ "$operator_host" != "localhost" ]; then
                show_remote_service_logs "$operator_host" "zeitwork-edge-proxy" "$tail_lines" "$follow" "$since" "$grep_pattern" "$no_color"
            else
                show_service_logs "zeitwork-edge-proxy" "$tail_lines" "$follow" "$since" "$grep_pattern" "$no_color"
            fi
            ;;
        *)
            # Assume it's a project name
            show_project_logs "$target" "$operator_url" "$tail_lines" "$follow" "$since" "$container_id" "$grep_pattern" "$no_color"
            ;;
    esac
}

show_service_logs() {
    local service="$1"
    local tail_lines="$2"
    local follow="$3"
    local since="$4"
    local grep_pattern="$5"
    local no_color="$6"

    local journalctl_cmd="journalctl -u $service"

    # Add tail option
    if [ -n "$tail_lines" ]; then
        journalctl_cmd="$journalctl_cmd -n $tail_lines"
    fi

    # Add follow option
    if [ "$follow" == "true" ]; then
        journalctl_cmd="$journalctl_cmd -f"
    fi

    # Add since option
    if [ -n "$since" ]; then
        journalctl_cmd="$journalctl_cmd --since=\"$since\""
    fi

    # Add no-pager for non-follow mode
    if [ "$follow" != "true" ]; then
        journalctl_cmd="$journalctl_cmd --no-pager"
    fi

    # Execute command with optional grep
    if [ -n "$grep_pattern" ]; then
        if [ "$no_color" == "true" ]; then
            eval "$journalctl_cmd" | grep "$grep_pattern"
        else
            eval "$journalctl_cmd" | grep --color=auto "$grep_pattern"
        fi
    else
        if [ "$no_color" == "true" ]; then
            eval "$journalctl_cmd" | sed 's/\x1b\[[0-9;]*m//g'
        else
            eval "$journalctl_cmd"
        fi
    fi
}

show_remote_service_logs() {
    local node_ip="$1"
    local service="$2"
    local tail_lines="$3"
    local follow="$4"
    local since="$5"
    local grep_pattern="$6"
    local no_color="$7"

    log "Fetching logs from node $node_ip"

    local remote_cmd="journalctl -u $service"

    # Add tail option
    if [ -n "$tail_lines" ]; then
        remote_cmd="$remote_cmd -n $tail_lines"
    fi

    # Add since option
    if [ -n "$since" ]; then
        remote_cmd="$remote_cmd --since='$since'"
    fi

    # Add no-pager for non-follow mode
    if [ "$follow" != "true" ]; then
        remote_cmd="$remote_cmd --no-pager"
    fi

    # Execute remote command using ssh_remote
    if [ "$follow" == "true" ]; then
        # For follow mode, need to use ssh directly with the config
        local ssh_opts="$(get_ssh_config) -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
        if [ -n "$grep_pattern" ]; then
            ssh $ssh_opts "$node_ip" "$remote_cmd" | grep --color=auto "$grep_pattern"
        else
            ssh $ssh_opts "$node_ip" "$remote_cmd"
        fi
    else
        # For non-follow mode, use ssh_remote function
        local output=$(ssh_remote "$node_ip" "$remote_cmd")
        if [ -n "$grep_pattern" ]; then
            echo "$output" | grep --color=auto "$grep_pattern"
        else
            echo "$output"
        fi
    fi
}

show_project_logs() {
    local project="$1"
    local operator_url="$2"
    local tail_lines="$3"
    local follow="$4"
    local since="$5"
    local container_id="$6"
    local grep_pattern="$7"
    local no_color="$8"

    # Get deployment info from operator
    local deployment_info=$(curl -s "$operator_url/api/deployments?project=$project" 2>/dev/null)

    if [ -z "$deployment_info" ] || [[ "$deployment_info" == *"error"* ]]; then
        error "Could not find deployment for project: $project"
        return 1
    fi

    # Extract container information
    local containers=$(echo "$deployment_info" | grep -o '"container_id":"[^"]*' | cut -d'"' -f4)

    if [ -z "$containers" ]; then
        warning "No containers found for project: $project"
        return 1
    fi

    # If specific container requested
    if [ -n "$container_id" ]; then
        if ! echo "$containers" | grep -q "$container_id"; then
            error "Container $container_id not found in project $project"
            return 1
        fi
        containers="$container_id"
    fi

    # Show logs for each container
    local first=true
    while IFS= read -r container; do
        if [ "$first" != "true" ]; then
            echo -e "\n${CYAN}---${NC}\n"
        fi
        first=false

        # Get node IP for container
        local node_info=$(curl -s "$operator_url/api/instances/$container" 2>/dev/null)
        local node_ip=$(echo "$node_info" | grep -o '"node_ip":"[^"]*' | cut -d'"' -f4)

        if [ -z "$node_ip" ]; then
            warning "Could not determine node for container $container"
            continue
        fi

        echo -e "${BOLD}Container: $container (Node: $node_ip)${NC}"

        # Get container logs via node agent
        show_container_logs "$node_ip" "$container" "$tail_lines" "$follow" "$since" "$grep_pattern" "$no_color"
    done <<< "$containers"
}

show_container_logs() {
    local node_ip="$1"
    local container_id="$2"
    local tail_lines="$3"
    local follow="$4"
    local since="$5"
    local grep_pattern="$6"
    local no_color="$7"

    # Build firecracker log command
    local log_cmd="cat /var/log/firecracker/$container_id.log"

    # For tail
    if [ -n "$tail_lines" ] && [ "$follow" != "true" ]; then
        log_cmd="tail -n $tail_lines /var/log/firecracker/$container_id.log"
    fi

    # For follow
    if [ "$follow" == "true" ]; then
        log_cmd="tail -f /var/log/firecracker/$container_id.log"
    fi

    # Execute remote command using ssh with config
    local ssh_opts="$(get_ssh_config) -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
    
    if [ -n "$grep_pattern" ]; then
        if [ "$no_color" == "true" ]; then
            ssh $ssh_opts "$node_ip" "$log_cmd" | grep "$grep_pattern"
        else
            ssh $ssh_opts "$node_ip" "$log_cmd" | grep --color=auto "$grep_pattern"
        fi
    else
        if [ "$no_color" == "true" ]; then
            ssh $ssh_opts "$node_ip" "$log_cmd" | sed 's/\x1b\[[0-9;]*m//g'
        else
            ssh $ssh_opts "$node_ip" "$log_cmd"
        fi
    fi
}

# Helper to colorize log levels
colorize_logs() {
    while IFS= read -r line; do
        case "$line" in
            *ERROR*|*FATAL*|*PANIC*)
                echo -e "${RED}$line${NC}"
                ;;
            *WARN*|*WARNING*)
                echo -e "${YELLOW}$line${NC}"
                ;;
            *INFO*)
                echo -e "${BLUE}$line${NC}"
                ;;
            *DEBUG*)
                echo -e "${CYAN}$line${NC}"
                ;;
            *)
                echo "$line"
                ;;
        esac
    done
}