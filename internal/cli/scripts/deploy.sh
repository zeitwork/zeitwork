#!/bin/bash

# Deploy command for Zeitwork CLI

deploy_usage() {
    cat << EOF
Deploy an application to the Zeitwork cluster

Usage: zeitwork deploy [options]

Options:
    --project <name>      Project name
    --image <image>       Docker image to deploy
    --replicas <count>    Number of replicas (default: 1)
    --cpu <cores>         CPU cores per instance (default: 1)
    --memory <mb>         Memory in MB per instance (default: 512)
    --disk <gb>           Disk size in GB per instance (default: 10)
    --env <key=value>     Environment variables (can be repeated)
    --port <port>         Port to expose (can be repeated)
    --domain <domain>     Domain name for the deployment
    --operator <url>      Operator URL (default: from config/env)
    --wait                Wait for deployment to be ready
    --timeout <seconds>   Deployment timeout (default: 300)
    -h, --help           Show this help message

Examples:
    # Simple deployment
    zeitwork deploy --project myapp --image myapp:latest

    # Deployment with resources
    zeitwork deploy --project api --image api:v1.0 --replicas 3 --cpu 2 --memory 1024

    # Deployment with environment and ports
    zeitwork deploy --project web --image web:latest --env DB_URL=postgres://... --port 8080 --domain app.example.com

EOF
}

deploy_command() {
    # Load environment variables for current context
    load_env_file
    
    local project=""
    local image=""
    local replicas=1
    local cpu=1
    local memory=512
    local disk=10
    local env_vars=()
    local ports=()
    local domain=""
    local operator_url=""  # Will be set from config or argument
    local wait=false
    local timeout=300

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --project)
                project="$2"
                shift 2
                ;;
            --image)
                image="$2"
                shift 2
                ;;
            --replicas)
                replicas="$2"
                shift 2
                ;;
            --cpu)
                cpu="$2"
                shift 2
                ;;
            --memory)
                memory="$2"
                shift 2
                ;;
            --disk)
                disk="$2"
                shift 2
                ;;
            --env)
                env_vars+=("$2")
                shift 2
                ;;
            --port)
                ports+=("$2")
                shift 2
                ;;
            --domain)
                domain="$2"
                shift 2
                ;;
            --operator)
                operator_url="$2"
                shift 2
                ;;
            --wait)
                wait=true
                shift
                ;;
            --timeout)
                timeout="$2"
                shift 2
                ;;
            -h|--help)
                deploy_usage
                return 0
                ;;
            *)
                error "Unknown option: $1"
                deploy_usage
                return 1
                ;;
        esac
    done

    # Validate required parameters
    if [ -z "$project" ]; then
        error "Project name is required"
        deploy_usage
        return 1
    fi

    if [ -z "$image" ]; then
        error "Docker image is required"
        deploy_usage
        return 1
    fi

    # If operator URL not explicitly provided, find active operator from config
    if [ -z "$operator_url" ]; then
        operator_url="${OPERATOR_URL:-$(find_active_operator)}"
        if [ -z "$operator_url" ]; then
            error "No operator URL configured and cannot find active operator"
            return 1
        fi
        debug "Using operator: $operator_url"
    fi

    log "Deploying $project with image $image"
    debug "Replicas: $replicas, CPU: $cpu, Memory: ${memory}MB, Disk: ${disk}GB"
    debug "Operator: $operator_url"

    # Build deployment JSON
    local deployment_json=$(build_deployment_json "$project" "$image" "$replicas" "$cpu" "$memory" "$disk" "${env_vars[@]}" "${ports[@]}" "$domain")
    
    debug "Deployment JSON: $deployment_json"

    # Create deployment
    show_progress "Creating deployment"
    
    local response=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -d "$deployment_json" \
        "$operator_url/api/deployments")
    
    local deployment_id=$(echo "$response" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
    
    if [ -z "$deployment_id" ]; then
        error "Failed to create deployment"
        debug "Response: $response"
        return 1
    fi
    
    end_progress "Deployment created: $deployment_id"

    # Wait for deployment if requested
    if [ "$wait" == "true" ]; then
        wait_for_deployment "$operator_url" "$deployment_id" "$timeout"
    fi

    success "Deployment successful!"
    log ""
    log "Deployment ID: $deployment_id"
    log "Project: $project"
    log "Image: $image"
    [ -n "$domain" ] && log "Domain: $domain"
    log ""
    log "Check status: zeitwork status --deployment $deployment_id"
    log "View logs: zeitwork logs $project"
}

build_deployment_json() {
    local project="$1"
    local image="$2"
    local replicas="$3"
    local cpu="$4"
    local memory="$5"
    local disk="$6"
    shift 6
    
    # Start building JSON
    local json='{'
    json+='"project_name":"'$project'",'
    json+='"image":"'$image'",'
    json+='"replicas":'$replicas','
    json+='"resources":{'
    json+='"cpu":'$cpu','
    json+='"memory":'$memory','
    json+='"disk":'$disk
    json+='},'
    
    # Add environment variables
    json+='"environment":{'
    local first=true
    for env in "$@"; do
        if [[ "$env" == *"="* ]]; then
            if [ "$first" != "true" ]; then
                json+=','
            fi
            local key="${env%%=*}"
            local value="${env#*=}"
            json+='"'$key'":"'$value'"'
            first=false
        fi
    done
    json+='},'
    
    # Add ports
    json+='"ports":['
    first=true
    for item in "$@"; do
        if [[ "$item" =~ ^[0-9]+$ ]]; then
            if [ "$first" != "true" ]; then
                json+=','
            fi
            json+=$item
            first=false
        fi
    done
    json+='],'
    
    # Add domain if provided
    for item in "$@"; do
        if [[ "$item" =~ \. ]] && [[ ! "$item" == *"="* ]]; then
            json+='"domain":"'$item'",'
            break
        fi
    done
    
    # Add metadata
    json+='"metadata":{'
    json+='"created_by":"zeitwork-cli",'
    json+='"created_at":"'$(date -u +"%Y-%m-%dT%H:%M:%SZ")'"'
    json+='}'
    
    json+='}'
    
    echo "$json"
}

wait_for_deployment() {
    local operator_url="$1"
    local deployment_id="$2"
    local timeout="$3"
    
    show_progress "Waiting for deployment to be ready"
    
    local start_time=$(date +%s)
    local current_time
    local elapsed
    
    while true; do
        current_time=$(date +%s)
        elapsed=$((current_time - start_time))
        
        if [ $elapsed -gt $timeout ]; then
            error "Deployment timeout after ${timeout} seconds"
            return 1
        fi
        
        local status=$(curl -s "$operator_url/api/deployments/$deployment_id" | grep -o '"status":"[^"]*' | cut -d'"' -f4)
        
        case "$status" in
            "running")
                end_progress "Deployment is running"
                return 0
                ;;
            "failed")
                error "Deployment failed"
                return 1
                ;;
            "pending"|"creating")
                debug "Status: $status (${elapsed}s elapsed)"
                ;;
            *)
                warning "Unknown status: $status"
                ;;
        esac
        
        sleep 5
    done
}