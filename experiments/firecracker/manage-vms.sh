#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK_DIR="${SCRIPT_DIR}/zeitwork-build"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${BLUE}[$(date '+%Y-%m-%d %H:%M:%S')] $1${NC}"
}

log_success() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')] ✓ $1${NC}"
}

log_error() {
    echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] ✗ $1${NC}"
}

show_status() {
    log "ZeitWork VM Status:"
    echo
    
    if [[ ! -d "${WORK_DIR}/vm-configs" ]]; then
        log_error "No VMs found. Run main.sh first."
        return 1
    fi
    
    for vm_dir in "${WORK_DIR}/vm-configs"/vm*; do
        if [[ -d "$vm_dir" ]]; then
            local vm_id=$(basename "$vm_dir" | sed 's/vm//')
            local pid_file="${vm_dir}/firecracker.pid"
            local ipv6_file="${vm_dir}/ipv6-addr"
            
            if [[ -f "$ipv6_file" ]]; then
                local vm_ipv6=$(cat "$ipv6_file")
                echo -n "VM ${vm_id}: [${vm_ipv6}]:3000 - "
                
                if [[ -f "$pid_file" ]]; then
                    local pid=$(cat "$pid_file")
                    if kill -0 "$pid" 2>/dev/null; then
                        echo -e "${GREEN}RUNNING${NC} (PID: $pid)"
                        
                        # Test HTTP connectivity
                        if curl -6 --connect-timeout 2 --max-time 5 "http://[${vm_ipv6}]:3000" >/dev/null 2>&1; then
                            echo "  └─ HTTP server: ${GREEN}RESPONDING${NC}"
                        else
                            echo "  └─ HTTP server: ${RED}NOT RESPONDING${NC}"
                        fi
                    else
                        echo -e "${RED}STOPPED${NC} (stale PID: $pid)"
                    fi
                else
                    echo -e "${YELLOW}UNKNOWN${NC} (no PID file)"
                fi
            fi
        fi
    done
    echo
}

test_connectivity() {
    log "Testing connectivity to all VMs..."
    
    local success_count=0
    local total_count=0
    
    for vm_dir in "${WORK_DIR}/vm-configs"/vm*; do
        if [[ -d "$vm_dir" ]]; then
            local vm_id=$(basename "$vm_dir" | sed 's/vm//')
            local ipv6_file="${vm_dir}/ipv6-addr"
            
            if [[ -f "$ipv6_file" ]]; then
                local vm_ipv6=$(cat "$ipv6_file")
                total_count=$((total_count + 1))
                
                echo -n "Testing VM ${vm_id} [${vm_ipv6}]:3000... "
                
                if response=$(curl -6 --connect-timeout 5 --max-time 10 "http://[${vm_ipv6}]:3000" 2>/dev/null); then
                    echo -e "${GREEN}SUCCESS${NC}"
                    echo "  Response: $response"
                    success_count=$((success_count + 1))
                else
                    echo -e "${RED}FAILED${NC}"
                fi
            fi
        fi
    done
    
    echo
    log_success "Connectivity test completed: ${success_count}/${total_count} VMs responding"
}

stop_vms() {
    log "Stopping all ZeitWork VMs..."
    
    # Kill Firecracker processes
    sudo pkill -f firecracker || true
    
    # Clean up PID files
    for vm_dir in "${WORK_DIR}/vm-configs"/vm*; do
        if [[ -d "$vm_dir" ]]; then
            rm -f "${vm_dir}/firecracker.pid"
        fi
    done
    
    log_success "All VMs stopped"
}

clean_networking() {
    log "Cleaning up networking..."
    
    # Remove TAP devices
    for tap in $(ip link show | grep tap-zeitwork | awk -F: '{print $2}' | tr -d ' '); do
        sudo ip link del "$tap" 2>/dev/null || true
    done
    
    # Remove bridge
    sudo ip link del br-zeitwork 2>/dev/null || true
    
    log_success "Networking cleaned up"
}

full_cleanup() {
    stop_vms
    clean_networking
    
    # Remove work directory
    if [[ -d "${WORK_DIR}" ]]; then
        log "Removing work directory..."
        sudo rm -rf "${WORK_DIR}"
        log_success "Work directory removed"
    fi
}

show_help() {
    cat << EOF
Zeitwork Firecracker VM Management

Usage: $0 <command>

Commands:
  status      Show status of all VMs
  test        Test HTTP connectivity to all VMs
  stop        Stop all running VMs
  clean-net   Clean up networking (TAP devices, bridge)
  cleanup     Full cleanup (stop VMs, clean networking, remove work dir)
  debug       Show debug information for troubleshooting
  help        Show this help message

Examples:
  $0 status                    # Show VM status
  $0 test                      # Test connectivity
  $0 stop                      # Stop all VMs
  $0 cleanup                   # Complete cleanup
  $0 debug                     # Show debug info

For detailed debugging, use: ./debug-vm.sh
EOF
}

show_debug() {
    log "Debug Information"
    echo "================="
    
    # Check if debug script exists
    if [[ -f "${SCRIPT_DIR}/debug-vm.sh" ]]; then
        log "For detailed debugging, run: ./debug-vm.sh"
        echo
    fi
    
    # Show quick debug info
    echo "Host Network:"
    echo "  IPv6 forwarding: $(cat /proc/sys/net/ipv6/conf/all/forwarding)"
    echo "  Bridge br-zeitwork: $(ip link show br-zeitwork >/dev/null 2>&1 && echo "UP" || echo "DOWN")"
    
    echo
    echo "VM Status Summary:"
    show_status
    
    echo
    echo "Recent logs (if available):"
    for vm_dir in "${WORK_DIR}/vm-configs"/vm*; do
        if [[ -d "$vm_dir" ]]; then
            local vm_id=$(basename "$vm_dir" | sed 's/vm//')
            echo "VM ${vm_id} console (last 5 lines):"
            tail -5 "${vm_dir}/logs/console.log" 2>/dev/null || echo "  No console log"
            echo
        fi
    done
}

main() {
    case "${1:-help}" in
        "status")
            show_status
            ;;
        "test")
            test_connectivity
            ;;
        "stop")
            stop_vms
            ;;
        "clean-net")
            clean_networking
            ;;
        "cleanup")
            full_cleanup
            ;;
        "debug")
            show_debug
            ;;
        "help"|"-h"|"--help")
            show_help
            ;;
        *)
            log_error "Unknown command: $1"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
