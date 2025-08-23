#!/bin/bash

# Status command for Zeitwork CLI

status_usage() {
    cat << EOF
Show cluster and service status

Usage: zeitwork status [options]

Options:
    --services            Show all service statuses
    --nodes               Show all node statuses
    --deployments         Show all deployments
    --deployment <id>     Show specific deployment status
    --operator <url>      Operator URL (default: from config/env)
    --format <format>     Output format: table, json, yaml (default: table)
    --watch               Continuously watch status (updates every 5s)
    -h, --help           Show this help message

Examples:
    # Show all services
    zeitwork status --services

    # Show nodes and their status
    zeitwork status --nodes

    # Show specific deployment
    zeitwork status --deployment abc123

    # Watch all deployments
    zeitwork status --deployments --watch

EOF
}

status_command() {
    # Load environment variables for current context
    load_env_file
    
    local show_services=false
    local show_nodes=false
    local show_deployments=false
    local deployment_id=""
    local operator_url=""  # Will be set from config or argument
    local format="table"
    local watch=false

    # If no specific option provided, show all
    if [ $# -eq 0 ]; then
        show_services=true
        show_nodes=true
        show_deployments=true
    fi

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --services)
                show_services=true
                shift
                ;;
            --nodes)
                show_nodes=true
                shift
                ;;
            --deployments)
                show_deployments=true
                shift
                ;;
            --deployment)
                deployment_id="$2"
                shift 2
                ;;
            --operator)
                operator_url="$2"
                shift 2
                ;;
            --format)
                format="$2"
                shift 2
                ;;
            --watch)
                watch=true
                shift
                ;;
            -h|--help)
                status_usage
                return 0
                ;;
            *)
                error "Unknown option: $1"
                status_usage
                return 1
                ;;
        esac
    done

    # If operator URL not explicitly provided, find active operator from config
    if [ -z "$operator_url" ]; then
        operator_url="${OPERATOR_URL:-$(find_active_operator)}"
        if [ -z "$operator_url" ]; then
            error "No operator URL configured and cannot find active operator"
            return 1
        fi
        debug "Using operator: $operator_url"
    fi

    # Main status display loop
    while true; do
        if [ "$watch" == "true" ]; then
            clear
            echo -e "${BOLD}Zeitwork Cluster Status${NC} - $(date)"
            echo "----------------------------------------"
        fi

        # Show service status
        if [ "$show_services" == "true" ]; then
            show_service_status "$format"
        fi

        # Show node status
        if [ "$show_nodes" == "true" ]; then
            show_node_status "$operator_url" "$format"
        fi

        # Show deployments
        if [ "$show_deployments" == "true" ]; then
            show_deployment_status "$operator_url" "$format"
        fi

        # Show specific deployment
        if [ -n "$deployment_id" ]; then
            show_deployment_details "$operator_url" "$deployment_id" "$format"
        fi

        if [ "$watch" != "true" ]; then
            break
        fi

        sleep 5
    done
}

show_service_status() {
    local format="$1"
    
    echo -e "\n${BOLD}Services:${NC}"
    
    if [ "$format" == "json" ]; then
        echo '{"services":['
    fi
    
    # Get nodes from config
    local nodes=$(get_nodes_from_config)
    local operators=$(get_all_operators)
    local first=true
    
    # Check operator services on operator nodes
    for operator_host in $operators; do
        # Extract just the host from URL
        local host=$(echo "$operator_host" | sed 's|http://||' | cut -d: -f1)
        
        # Check if operator is running
        local status="inactive"
        if ssh_remote "$host" "systemctl is-active --quiet zeitwork-operator" 2>/dev/null; then
            status="active"
        fi
        
        if [ "$format" == "json" ]; then
            if [ "$first" != "true" ]; then echo -n ","; fi
            echo -n '{"name":"zeitwork-operator","host":"'$host'","status":"'$status'","port":"8080"}'
            first=false
        else
            if [ "$status" == "active" ]; then
                printf "  ${GREEN}●${NC} %-25s ${GREEN}active${NC}   Node: %s:8080\n" "zeitwork-operator" "$host"
            else
                printf "  ${RED}●${NC} %-25s ${RED}inactive${NC} Node: %s:8080\n" "zeitwork-operator" "$host"
            fi
        fi
    done
    
    # Check node-agent on worker nodes
    for node in $nodes; do
        local status="inactive"
        if ssh_remote "$node" "systemctl is-active --quiet zeitwork-node-agent" 2>/dev/null; then
            status="active"
        fi
        
        if [ "$format" == "json" ]; then
            if [ "$first" != "true" ]; then echo -n ","; fi
            echo -n '{"name":"zeitwork-node-agent","host":"'$node'","status":"'$status'","port":"8081"}'
            first=false
        else
            if [ "$status" == "active" ]; then
                printf "  ${GREEN}●${NC} %-25s ${GREEN}active${NC}   Node: %s:8081\n" "zeitwork-node-agent" "$node"
            else
                printf "  ${RED}●${NC} %-25s ${RED}inactive${NC} Node: %s:8081\n" "zeitwork-node-agent" "$node"
            fi
        fi
    done
    
    if [ "$format" == "json" ]; then
        echo ']}'
    fi
}

show_node_status() {
    local operator_url="$1"
    local format="$2"
    
    echo -e "\n${BOLD}Nodes:${NC}"
    
    # Get nodes from operator API
    local response=$(curl -s "$operator_url/api/v1/nodes" 2>/dev/null)
    
    if [ -z "$response" ] || [[ "$response" == *"error"* ]]; then
        warning "Could not fetch node information from operator"
        return
    fi
    
    case "$format" in
        json)
            echo "$response"
            ;;
        yaml)
            # Simple JSON to YAML conversion
            echo "$response" | sed 's/[{}]//g' | sed 's/,/\n/g' | sed 's/"//g' | sed 's/:/: /g' | sed 's/^/  /'
            ;;
        *)
            # Parse and display as table using jq if available, otherwise use grep
            if command -v jq >/dev/null 2>&1; then
                # Use jq for proper JSON parsing
                local count=$(echo "$response" | jq -r 'length')
                if [ "$count" -eq 0 ] || [ -z "$count" ]; then
                    echo "  No nodes registered"
                else
                    printf "  %-40s %-16s %-20s %-10s\n" "Hostname" "IP Address" "Type" "Status"
                    echo "  --------------------------------------------------------------------------------"
                    
                    echo "$response" | jq -r '.[] | "\(.hostname) \(.ip_address) \(.resources.type // "unknown") \(.state)"' | \
                    while IFS=' ' read -r hostname ip type state; do
                        if [ "$state" == "active" ] || [ "$state" == "ready" ]; then
                            printf "  %-40s %-16s %-20s ${GREEN}%-10s${NC}\n" "$hostname" "$ip" "$type" "$state"
                        else
                            printf "  %-40s %-16s %-20s ${YELLOW}%-10s${NC}\n" "$hostname" "$ip" "$type" "$state"
                        fi
                    done
                fi
            else
                # Fallback to grep-based parsing
                local nodes=$(echo "$response" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
                if [ -z "$nodes" ]; then
                    echo "  No nodes registered"
                else
                    printf "  %-40s %-16s %-10s\n" "Node ID" "IP Address" "Status"
                    echo "  ------------------------------------------------------------"
                    
                    while IFS= read -r node_id; do
                        local node_json=$(echo "$response" | grep -o "{[^}]*\"id\":\"$node_id\"[^}]*}" | head -1)
                        local ip=$(echo "$node_json" | grep -o '"ip_address":"[^"]*' | cut -d'"' -f4)
                        local state=$(echo "$node_json" | grep -o '"state":"[^"]*' | cut -d'"' -f4)
                        local hostname=$(echo "$node_json" | grep -o '"hostname":"[^"]*' | cut -d'"' -f4)
                        
                        if [ "$state" == "active" ] || [ "$state" == "ready" ]; then
                            printf "  %-40s %-16s ${GREEN}%-10s${NC}\n" "${hostname:-$node_id}" "$ip" "$state"
                        else
                            printf "  %-40s %-16s ${YELLOW}%-10s${NC}\n" "${hostname:-$node_id}" "$ip" "$state"
                        fi
                    done <<< "$nodes"
                fi
            fi
            ;;
    esac
}

show_deployment_status() {
    local operator_url="$1"
    local format="$2"
    
    echo -e "\n${BOLD}Deployments:${NC}"
    
    # Get deployments from operator API
    local response=$(curl -s "$operator_url/api/deployments" 2>/dev/null)
    
    if [ -z "$response" ] || [[ "$response" == *"error"* ]]; then
        warning "Could not fetch deployment information from operator"
        return
    fi
    
    case "$format" in
        json)
            echo "$response"
            ;;
        yaml)
            # Simple JSON to YAML conversion
            echo "$response" | sed 's/[{}]//g' | sed 's/,/\n/g' | sed 's/"//g' | sed 's/:/: /g' | sed 's/^/  /'
            ;;
        *)
            # Parse and display as table
            local deployments=$(echo "$response" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
            if [ -z "$deployments" ]; then
                echo "  No deployments found"
            else
                printf "  %-12s %-20s %-15s %-10s %-10s\n" "ID" "Project" "Image" "Replicas" "Status"
                echo "  -------------------------------------------------------------------------"
                
                while IFS= read -r dep_id; do
                    local project=$(echo "$response" | grep -o "\"$dep_id\".*?\"project_name\":\"[^\"]*" | grep -o '"project_name":"[^"]*' | cut -d'"' -f4)
                    local image=$(echo "$response" | grep -o "\"$dep_id\".*?\"image\":\"[^\"]*" | grep -o '"image":"[^"]*' | cut -d'"' -f4 | cut -d':' -f2)
                    local replicas=$(echo "$response" | grep -o "\"$dep_id\".*?\"replicas\":[0-9]*" | grep -o '"replicas":[0-9]*' | cut -d':' -f2)
                    local status=$(echo "$response" | grep -o "\"$dep_id\".*?\"status\":\"[^\"]*" | grep -o '"status":"[^"]*' | cut -d'"' -f4)
                    
                    case "$status" in
                        "running")
                            printf "  %-12s %-20s %-15s %-10s ${GREEN}%s${NC}\n" "$dep_id" "$project" "$image" "$replicas" "$status"
                            ;;
                        "pending"|"creating")
                            printf "  %-12s %-20s %-15s %-10s ${YELLOW}%s${NC}\n" "$dep_id" "$project" "$image" "$replicas" "$status"
                            ;;
                        "failed"|"error")
                            printf "  %-12s %-20s %-15s %-10s ${RED}%s${NC}\n" "$dep_id" "$project" "$image" "$replicas" "$status"
                            ;;
                        *)
                            printf "  %-12s %-20s %-15s %-10s %s\n" "$dep_id" "$project" "$image" "$replicas" "$status"
                            ;;
                    esac
                done <<< "$deployments"
            fi
            ;;
    esac
}

show_deployment_details() {
    local operator_url="$1"
    local deployment_id="$2"
    local format="$3"
    
    echo -e "\n${BOLD}Deployment Details: $deployment_id${NC}"
    
    # Get deployment details from operator API
    local response=$(curl -s "$operator_url/api/deployments/$deployment_id" 2>/dev/null)
    
    if [ -z "$response" ] || [[ "$response" == *"error"* ]]; then
        error "Could not fetch deployment details"
        return
    fi
    
    case "$format" in
        json)
            echo "$response" | python -m json.tool 2>/dev/null || echo "$response"
            ;;
        yaml)
            # Simple JSON to YAML conversion
            echo "$response" | sed 's/[{}]//g' | sed 's/,/\n/g' | sed 's/"//g' | sed 's/:/: /g'
            ;;
        *)
            # Parse and display details
            local project=$(echo "$response" | grep -o '"project_name":"[^"]*' | cut -d'"' -f4)
            local image=$(echo "$response" | grep -o '"image":"[^"]*' | cut -d'"' -f4)
            local replicas=$(echo "$response" | grep -o '"replicas":[0-9]*' | cut -d':' -f2)
            local status=$(echo "$response" | grep -o '"status":"[^"]*' | cut -d'"' -f4)
            local created=$(echo "$response" | grep -o '"created_at":"[^"]*' | cut -d'"' -f4)
            
            echo "  Project:    $project"
            echo "  Image:      $image"
            echo "  Replicas:   $replicas"
            echo "  Status:     $status"
            echo "  Created:    $created"
            
            # Show instances if available
            local instances=$(echo "$response" | grep -o '"instances":\[[^]]*\]')
            if [ -n "$instances" ]; then
                echo -e "\n  ${BOLD}Instances:${NC}"
                local instance_ids=$(echo "$instances" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
                while IFS= read -r inst_id; do
                    echo "    - $inst_id"
                done <<< "$instance_ids"
            fi
            ;;
    esac
}