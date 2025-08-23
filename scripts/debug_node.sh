#!/bin/bash

# Debug script to inspect node-agent issues on remote nodes
# Usage: ./debug_node.sh <node-ip> [ssh-key] [ssh-user]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get arguments
NODE_IP="${1:-213.239.207.93}"
SSH_KEY="${2:-$HOME/.zeitwork/.ssh/id_rsa}"
SSH_USER="${3:-root}"

if [ -z "$NODE_IP" ]; then
    echo "Usage: $0 <node-ip> [ssh-key] [ssh-user]"
    exit 1
fi

echo -e "${BLUE}=== Debugging Node Agent on $NODE_IP ===${NC}"
echo ""

# SSH options
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
SSH_CMD="ssh $SSH_OPTS -i $SSH_KEY ${SSH_USER}@${NODE_IP}"

# Check SSH connectivity
echo -e "${YELLOW}1. Testing SSH connectivity...${NC}"
if $SSH_CMD "echo 'SSH connection successful'" 2>/dev/null; then
    echo -e "${GREEN}✓ SSH connection successful${NC}"
else
    echo -e "${RED}✗ Cannot connect via SSH${NC}"
    exit 1
fi
echo ""

# Check if node-agent binary exists
echo -e "${YELLOW}2. Checking node-agent binary...${NC}"
$SSH_CMD "ls -la /usr/local/bin/zeitwork-node-agent 2>/dev/null || echo 'Binary not found'"
echo ""

# Check service status
echo -e "${YELLOW}3. Checking service status...${NC}"
$SSH_CMD "systemctl status zeitwork-node-agent --no-pager 2>/dev/null || echo 'Service not found'"
echo ""

# Check if service file exists
echo -e "${YELLOW}4. Checking service file...${NC}"
$SSH_CMD "cat /etc/systemd/system/zeitwork-node-agent.service 2>/dev/null || echo 'Service file not found'"
echo ""

# Check configuration
echo -e "${YELLOW}5. Checking configuration...${NC}"
$SSH_CMD "cat /etc/zeitwork/node-agent.env 2>/dev/null || echo 'Config file not found'"
echo ""

# Check journal logs
echo -e "${YELLOW}6. Recent journal logs...${NC}"
$SSH_CMD "journalctl -u zeitwork-node-agent --no-pager -n 50 2>/dev/null || echo 'No logs found'"
echo ""

# Check if port 8081 is listening
echo -e "${YELLOW}7. Checking port 8081...${NC}"
$SSH_CMD "netstat -tlnp | grep 8081 2>/dev/null || ss -tlnp | grep 8081 2>/dev/null || echo 'Port 8081 not listening'"
echo ""

# Check firewall rules
echo -e "${YELLOW}8. Checking firewall...${NC}"
$SSH_CMD "iptables -L INPUT -n | grep 8081 2>/dev/null || echo 'No specific firewall rules for 8081'"
$SSH_CMD "ufw status 2>/dev/null | grep 8081 || echo 'UFW not active or no rules for 8081'"
echo ""

# Check if process is running
echo -e "${YELLOW}9. Checking running processes...${NC}"
$SSH_CMD "ps aux | grep zeitwork-node-agent | grep -v grep || echo 'No node-agent process found'"
echo ""

# Try to start the service manually if it's not running
echo -e "${YELLOW}10. Attempting manual start (if not running)...${NC}"
$SSH_CMD "if ! systemctl is-active zeitwork-node-agent >/dev/null 2>&1; then
    echo 'Service is not active, attempting to start...'
    systemctl start zeitwork-node-agent 2>&1 || true
    sleep 2
    systemctl status zeitwork-node-agent --no-pager 2>/dev/null || true
    echo ''
    echo 'Latest logs after start attempt:'
    journalctl -u zeitwork-node-agent --no-pager -n 20
else
    echo 'Service is already active'
fi"
echo ""

# Test connectivity to operator
echo -e "${YELLOW}11. Testing connectivity to operator...${NC}"
OPERATOR_URL=$($SSH_CMD "grep OPERATOR_URL /etc/zeitwork/node-agent.env 2>/dev/null | cut -d= -f2" || echo "")
if [ -n "$OPERATOR_URL" ]; then
    echo "Operator URL: $OPERATOR_URL"
    $SSH_CMD "curl -s -m 5 $OPERATOR_URL/health || echo 'Cannot reach operator at $OPERATOR_URL'"
else
    echo "No OPERATOR_URL found in configuration"
fi
echo ""

# Check system resources
echo -e "${YELLOW}12. System resources...${NC}"
$SSH_CMD "free -h"
echo ""
$SSH_CMD "df -h /"
echo ""

# Check for required dependencies
echo -e "${YELLOW}13. Checking dependencies...${NC}"
echo -n "Firecracker: "
$SSH_CMD "which firecracker 2>/dev/null || echo 'Not found'"
echo -n "KVM module: "
$SSH_CMD "lsmod | grep kvm >/dev/null 2>&1 && echo 'Loaded' || echo 'Not loaded'"
echo -n "Kernel image: "
$SSH_CMD "ls /var/lib/firecracker/kernels/vmlinux.bin 2>/dev/null || echo 'Not found'"
echo ""

# Summary
echo -e "${BLUE}=== Debug Summary ===${NC}"
echo "Node IP: $NODE_IP"
echo "SSH User: $SSH_USER"
echo "SSH Key: $SSH_KEY"

# Final connectivity test
echo ""
echo -e "${YELLOW}14. Final port test from local machine...${NC}"
nc -zv -w2 $NODE_IP 8081 2>&1 || echo "Port 8081 not reachable from local machine"

echo ""
echo -e "${GREEN}Debug complete!${NC}"