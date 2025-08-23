#!/bin/bash
# Cleanup script to ensure a completely fresh deployment
# Removes ALL existing Zeitwork services, binaries, configuration, and data

echo "Zeitwork Complete Cleanup Script"
echo "================================"
echo "Removing all Zeitwork components for fresh deployment..."
echo ""

# Stop all Zeitwork services
echo "Stopping Zeitwork services..."
sudo systemctl stop zeitwork-operator 2>/dev/null || true
sudo systemctl stop zeitwork-load-balancer 2>/dev/null || true
sudo systemctl stop zeitwork-edge-proxy 2>/dev/null || true
sudo systemctl stop zeitwork-node-agent 2>/dev/null || true

# Disable services
echo "Disabling Zeitwork services..."
sudo systemctl disable zeitwork-operator 2>/dev/null || true
sudo systemctl disable zeitwork-load-balancer 2>/dev/null || true
sudo systemctl disable zeitwork-edge-proxy 2>/dev/null || true
sudo systemctl disable zeitwork-node-agent 2>/dev/null || true

# Remove service files
echo "Removing service files..."
sudo rm -f /etc/systemd/system/zeitwork-*.service
sudo systemctl daemon-reload
sudo systemctl reset-failed 2>/dev/null || true

# Kill any remaining processes
echo "Killing any remaining Zeitwork processes..."
sudo pkill -f zeitwork- 2>/dev/null || true
sleep 2
sudo pkill -9 -f zeitwork- 2>/dev/null || true

# Remove binaries
echo "Removing binaries..."
sudo rm -f /usr/local/bin/zeitwork-*

# ALWAYS remove configuration and data for clean deployment
echo "Removing ALL configuration and data..."
sudo rm -rf /etc/zeitwork
sudo rm -rf /var/lib/zeitwork
sudo rm -rf /var/log/zeitwork
sudo rm -rf /var/lib/firecracker/vms

# Remove temporary files
echo "Cleaning temporary files..."
sudo rm -rf /tmp/zeitwork-binaries.tar.gz
sudo rm -rf /tmp/build
sudo rm -rf /tmp/firecracker-*

# Remove any leftover Firecracker sockets
echo "Cleaning Firecracker sockets..."
sudo rm -f /tmp/firecracker.socket*

echo ""
echo "✓ Complete cleanup done! The node is ready for a fresh deployment."
echo "  - All services stopped and removed"
echo "  - All configuration deleted"
echo "  - All data directories cleaned"
echo "  - All temporary files removed"
