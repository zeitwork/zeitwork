#!/bin/bash

# Script to clean up orphaned Firecracker resources
# This can be run manually when the backend is not running

echo "🧹 Firecracker Resource Cleanup Script"
echo "======================================"
echo ""

# Function to check if running as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        echo "❌ This script must be run as root (use sudo)"
        exit 1
    fi
}

# Function to kill Firecracker processes
cleanup_firecracker_processes() {
    echo "🔍 Looking for Firecracker processes..."
    FC_PIDS=$(pgrep -x firecracker 2>/dev/null || true)
    
    if [ -n "$FC_PIDS" ]; then
        echo "Found Firecracker processes:"
        for PID in $FC_PIDS; do
            echo "  - PID $PID: $(ps -p $PID -o args= 2>/dev/null || echo 'Unknown')"
        done
        
        read -p "Kill all Firecracker processes? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            for PID in $FC_PIDS; do
                echo "  Killing PID $PID..."
                kill -TERM $PID 2>/dev/null || true
            done
            sleep 2
            # Force kill if still running
            for PID in $FC_PIDS; do
                if ps -p $PID > /dev/null 2>&1; then
                    echo "  Force killing PID $PID..."
                    kill -KILL $PID 2>/dev/null || true
                fi
            done
            echo "✅ Firecracker processes cleaned up"
        else
            echo "⏭️  Skipping Firecracker process cleanup"
        fi
    else
        echo "✅ No Firecracker processes found"
    fi
    echo ""
}

# Function to clean up TAP devices
cleanup_tap_devices() {
    echo "🔍 Looking for TAP devices..."
    TAP_DEVICES=$(ip link show type tap 2>/dev/null | grep -oE '^[0-9]+: tap[^:]+' | cut -d' ' -f2 || true)
    
    if [ -n "$TAP_DEVICES" ]; then
        echo "Found TAP devices:"
        for TAP in $TAP_DEVICES; do
            echo "  - $TAP"
        done
        
        read -p "Remove all TAP devices? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            for TAP in $TAP_DEVICES; do
                echo "  Removing $TAP..."
                ip link set $TAP down 2>/dev/null || true
                ip link delete $TAP 2>/dev/null || true
            done
            echo "✅ TAP devices cleaned up"
        else
            echo "⏭️  Skipping TAP device cleanup"
        fi
    else
        echo "✅ No TAP devices found"
    fi
    echo ""
}

# Function to clean up VM directories
cleanup_vm_directories() {
    echo "🔍 Looking for VM directories..."
    VM_DIR="/var/lib/firecracker/vms"
    
    if [ -d "$VM_DIR" ]; then
        VM_INSTANCES=$(ls -d $VM_DIR/instance-* 2>/dev/null || true)
        if [ -n "$VM_INSTANCES" ]; then
            echo "Found VM directories:"
            for DIR in $VM_INSTANCES; do
                echo "  - $DIR ($(du -sh $DIR 2>/dev/null | cut -f1))"
            done
            
            read -p "Remove all VM directories? (y/n) " -n 1 -r
            echo
            if [[ $REPLY =~ ^[Yy]$ ]]; then
                for DIR in $VM_INSTANCES; do
                    echo "  Removing $DIR..."
                    rm -rf "$DIR"
                done
                echo "✅ VM directories cleaned up"
            else
                echo "⏭️  Skipping VM directory cleanup"
            fi
        else
            echo "✅ No VM directories found"
        fi
    else
        echo "✅ VM directory does not exist"
    fi
    echo ""
}

# Function to clean up iptables rules
cleanup_iptables() {
    echo "🔍 Looking for Firecracker-related iptables rules..."
    
    # Check for DNAT rules
    DNAT_RULES=$(iptables -t nat -L PREROUTING -n --line-numbers 2>/dev/null | grep "10.0.0" | wc -l)
    FORWARD_RULES=$(iptables -L FORWARD -n --line-numbers 2>/dev/null | grep "10.0.0" | wc -l)
    
    if [ "$DNAT_RULES" -gt 0 ] || [ "$FORWARD_RULES" -gt 0 ]; then
        echo "Found iptables rules:"
        echo "  - $DNAT_RULES DNAT rules"
        echo "  - $FORWARD_RULES FORWARD rules"
        
        read -p "Clean up iptables rules? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            # Remove DNAT rules
            iptables -t nat -L PREROUTING -n --line-numbers | grep "10.0.0" | awk '{print $1}' | sort -rn | while read line; do
                iptables -t nat -D PREROUTING $line 2>/dev/null || true
            done
            
            # Remove FORWARD rules
            iptables -L FORWARD -n --line-numbers | grep "10.0.0" | awk '{print $1}' | sort -rn | while read line; do
                iptables -D FORWARD $line 2>/dev/null || true
            done
            
            echo "✅ iptables rules cleaned up"
        else
            echo "⏭️  Skipping iptables cleanup"
        fi
    else
        echo "✅ No Firecracker-related iptables rules found"
    fi
    echo ""
}

# Function to check bridge status
check_bridge() {
    echo "🔍 Checking bridge status..."
    if ip link show fcbr0 2>/dev/null > /dev/null; then
        echo "Bridge fcbr0 exists:"
        ip addr show fcbr0 | grep -E "inet|state"
        
        read -p "Remove bridge fcbr0? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            ip link set fcbr0 down 2>/dev/null || true
            ip link delete fcbr0 2>/dev/null || true
            echo "✅ Bridge removed"
        else
            echo "ℹ️  Bridge kept (may be needed for networking)"
        fi
    else
        echo "✅ No bridge found"
    fi
    echo ""
}

# Function to show summary
show_summary() {
    echo "📊 Current Status:"
    echo "=================="
    
    FC_COUNT=$(pgrep -x firecracker 2>/dev/null | wc -l)
    TAP_COUNT=$(ip link show type tap 2>/dev/null | grep -c "^[0-9]:" || echo "0")
    VM_COUNT=$(ls -d /var/lib/firecracker/vms/instance-* 2>/dev/null | wc -l || echo "0")
    
    echo "  Firecracker processes: $FC_COUNT"
    echo "  TAP devices: $TAP_COUNT"
    echo "  VM directories: $VM_COUNT"
    
    if [ "$FC_COUNT" -eq 0 ] && [ "$TAP_COUNT" -eq 0 ] && [ "$VM_COUNT" -eq 0 ]; then
        echo ""
        echo "✅ System is clean!"
    else
        echo ""
        echo "⚠️  Some resources still exist"
    fi
}

# Main execution
main() {
    check_root
    
    echo "This script will help clean up orphaned Firecracker resources."
    echo "It's useful when the backend has crashed or been restarted."
    echo ""
    
    read -p "Continue with cleanup? (y/n) " -n 1 -r
    echo
    echo ""
    
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Cleanup cancelled."
        exit 0
    fi
    
    cleanup_firecracker_processes
    cleanup_tap_devices
    cleanup_vm_directories
    cleanup_iptables
    check_bridge
    
    echo ""
    show_summary
}

# Run main function
main
