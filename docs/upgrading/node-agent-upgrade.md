# Node Agent Upgrade Guide

This guide covers upgrading the Zeitwork Node Agent service that runs on worker nodes and manages Firecracker VMs.

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Upgrade Process](#upgrade-process)
4. [VM Migration](#vm-migration)
5. [Verification](#verification)
6. [Rollback](#rollback)
7. [Best Practices](#best-practices)

## Overview

The Node Agent runs on each worker node (6 per region, 18 total) and manages local Firecracker VMs. Upgrades must be coordinated to ensure running customer workloads are not interrupted.

### Key Considerations

- **VM Migration**: Move running VMs before upgrading the node agent
- **Operator Compatibility**: Ensure compatibility with operator version
- **Firecracker Version**: May require Firecracker upgrade as well
- **Customer Impact**: Zero-downtime requires careful VM migration

## Prerequisites

- [ ] New version tested with current operator version
- [ ] VM migration tested in staging
- [ ] Firecracker compatibility verified
- [ ] Sufficient capacity on other nodes for VM migration
- [ ] Monitoring of customer applications active

## Upgrade Process

### 1. Build the New Version

```bash
# On build server
cd /path/to/zeitwork
git checkout v2.0.0

# Build node agent
make build

# Create deployment package
tar -czf zeitwork-node-agent-v2.0.0.tar.gz build/zeitwork-node-agent
```

### 2. Rolling Upgrade Script

```bash
#!/bin/bash
# upgrade-node-agents.sh

VERSION="v2.0.0"
WORKER_NODES=(
    "us-east-worker-1" "us-east-worker-2" "us-east-worker-3"
    "us-east-worker-4" "us-east-worker-5" "us-east-worker-6"
    "eu-west-worker-1" "eu-west-worker-2" "eu-west-worker-3"
    "eu-west-worker-4" "eu-west-worker-5" "eu-west-worker-6"
    "ap-south-worker-1" "ap-south-worker-2" "ap-south-worker-3"
    "ap-south-worker-4" "ap-south-worker-5" "ap-south-worker-6"
)

# Function to drain node (migrate VMs)
drain_node() {
    local node=$1
    local node_id=$(curl -s http://operator:8080/api/v1/nodes | \
                    jq -r ".[] | select(.hostname==\"$node\") | .id")

    echo "Draining node $node (ID: $node_id)..."
    curl -X POST "http://operator:8080/api/v1/nodes/${node_id}/drain"

    # Wait for VMs to migrate
    while true; do
        vm_count=$(curl -s "http://operator:8080/api/v1/nodes/${node_id}" | \
                   jq -r '.active_instances')
        if [ "$vm_count" = "0" ]; then
            echo "Node drained successfully"
            break
        fi
        echo "Waiting for $vm_count VMs to migrate..."
        sleep 10
    done
}

# Function to upgrade node
upgrade_node() {
    local node=$1

    echo "Upgrading $node to version $VERSION..."

    # Copy new binary
    scp "zeitwork-node-agent-${VERSION}.tar.gz" "${node}:/tmp/"

    # Perform upgrade
    ssh "$node" << 'ENDSSH'
        set -e

        # Extract new binary
        cd /tmp
        tar -xzf zeitwork-node-agent-*.tar.gz

        # Stop service
        sudo systemctl stop zeitwork-node-agent

        # Backup and replace binary
        sudo cp /usr/local/bin/zeitwork-node-agent \
                /usr/local/bin/zeitwork-node-agent.backup
        sudo mv build/zeitwork-node-agent /usr/local/bin/
        sudo chmod +x /usr/local/bin/zeitwork-node-agent
        sudo chown zeitwork:zeitwork /usr/local/bin/zeitwork-node-agent

        # Start service
        sudo systemctl start zeitwork-node-agent

        # Cleanup
        rm -rf /tmp/zeitwork-node-agent-*.tar.gz /tmp/build
ENDSSH

    # Wait for node to register
    sleep 10

    # Verify node is healthy
    if curl -s "http://${node}:8081/health" | grep -q "healthy"; then
        echo "✓ $node upgraded successfully"

        # Re-enable node for scheduling
        node_id=$(curl -s http://operator:8080/api/v1/nodes | \
                  jq -r ".[] | select(.hostname==\"$node\") | .id")
        curl -X POST "http://operator:8080/api/v1/nodes/${node_id}/enable"
    else
        echo "✗ $node health check failed"
        return 1
    fi
}

# Main upgrade loop
for node in "${WORKER_NODES[@]}"; do
    echo "Processing $node..."

    # Drain node
    drain_node "$node"

    # Upgrade node
    if ! upgrade_node "$node"; then
        echo "Upgrade failed on $node, attempting rollback..."
        ssh "$node" << 'ENDSSH'
            sudo cp /usr/local/bin/zeitwork-node-agent.backup \
                    /usr/local/bin/zeitwork-node-agent
            sudo systemctl restart zeitwork-node-agent
ENDSSH
        exit 1
    fi

    # Wait before next node to allow VM redistribution
    echo "Waiting 60 seconds for VM redistribution..."
    sleep 60
done

echo "All node agents upgraded successfully!"
```

### 3. Upgrade with Minimal VM Migration

For faster upgrades with brief VM downtime:

```bash
#!/bin/bash
# fast-upgrade-node-agent.sh

# Upgrade nodes in parallel within each region
# VMs will briefly restart on the same node

upgrade_region() {
    local region=$1
    local nodes=("${region}-worker-1" "${region}-worker-2" "${region}-worker-3"
                 "${region}-worker-4" "${region}-worker-5" "${region}-worker-6")

    echo "Upgrading region: $region"

    # Stop all node agents in region simultaneously
    for node in "${nodes[@]}"; do
        ssh "$node" "sudo systemctl stop zeitwork-node-agent" &
    done
    wait

    # Upgrade all nodes
    for node in "${nodes[@]}"; do
        (
            scp "zeitwork-node-agent-new.tar.gz" "${node}:/tmp/"
            ssh "$node" << 'ENDSSH'
                cd /tmp && tar -xzf zeitwork-node-agent-new.tar.gz
                sudo mv build/zeitwork-node-agent /usr/local/bin/
                sudo chmod +x /usr/local/bin/zeitwork-node-agent
                rm -rf /tmp/zeitwork-node-agent-new.tar.gz /tmp/build
ENDSSH
        ) &
    done
    wait

    # Start all node agents
    for node in "${nodes[@]}"; do
        ssh "$node" "sudo systemctl start zeitwork-node-agent" &
    done
    wait

    echo "Region $region upgraded"
}

# Upgrade each region
for region in us-east eu-west ap-south; do
    upgrade_region "$region"
    sleep 30
done
```

## VM Migration

### Pre-Migration Checks

```bash
# Check VM distribution across nodes
curl http://operator:8080/api/v1/nodes | \
    jq '.[] | {hostname: .hostname, vms: .active_instances}'

# Verify target nodes have capacity
curl http://operator:8080/api/v1/nodes | \
    jq '.[] | select(.active_instances < .max_instances) | .hostname'
```

### Manual VM Migration

If automatic draining fails:

```bash
# List VMs on a node
NODE_ID="node-uuid"
curl "http://operator:8080/api/v1/nodes/${NODE_ID}/instances"

# Migrate specific VM
INSTANCE_ID="instance-uuid"
TARGET_NODE="target-node-uuid"
curl -X POST "http://operator:8080/api/v1/instances/${INSTANCE_ID}/migrate" \
    -H "Content-Type: application/json" \
    -d "{\"target_node\": \"${TARGET_NODE}\"}"
```

## Verification

### Health Checks

```bash
#!/bin/bash
# verify-node-agents.sh

echo "=== Node Agent Health Status ==="
for i in {1..6}; do
    for region in us-east eu-west ap-south; do
        node="${region}-worker-${i}"
        printf "%-20s: " "$node"
        if curl -s "http://${node}:8081/health" 2>/dev/null | grep -q "healthy"; then
            echo "✓ Healthy"
        else
            echo "✗ Unhealthy"
        fi
    done
done

echo -e "\n=== Version Check ==="
for i in {1..6}; do
    for region in us-east eu-west ap-south; do
        node="${region}-worker-${i}"
        printf "%-20s: " "$node"
        ssh "$node" "/usr/local/bin/zeitwork-node-agent --version" 2>/dev/null
    done
done

echo -e "\n=== VM Distribution ==="
curl -s http://operator:8080/api/v1/nodes | \
    jq -r '.[] | "\(.hostname): \(.active_instances)/\(.max_instances) VMs"'
```

### Firecracker Verification

```bash
# Check Firecracker version on nodes
for node in us-east-worker-{1..6}; do
    echo -n "$node: "
    ssh "$node" "firecracker --version"
done

# Verify VM functionality
INSTANCE_ID=$(curl -s http://operator:8080/api/v1/instances | jq -r '.[0].id')
curl "http://operator:8080/api/v1/instances/${INSTANCE_ID}/health"
```

## Rollback

### Single Node Rollback

```bash
#!/bin/bash
# rollback-node-agent.sh

NODE=$1

# Drain node first
NODE_ID=$(curl -s http://operator:8080/api/v1/nodes | \
          jq -r ".[] | select(.hostname==\"$NODE\") | .id")
curl -X POST "http://operator:8080/api/v1/nodes/${NODE_ID}/drain"

# Wait for drain
while [ $(curl -s "http://operator:8080/api/v1/nodes/${NODE_ID}" | \
          jq -r '.active_instances') -gt 0 ]; do
    sleep 5
done

# Rollback
ssh "$NODE" << 'ENDSSH'
    sudo systemctl stop zeitwork-node-agent
    sudo cp /usr/local/bin/zeitwork-node-agent.backup \
            /usr/local/bin/zeitwork-node-agent
    sudo systemctl start zeitwork-node-agent
ENDSSH

# Re-enable node
curl -X POST "http://operator:8080/api/v1/nodes/${NODE_ID}/enable"
```

### Regional Rollback

```bash
# Rollback entire region
REGION="us-east"
for i in {1..6}; do
    ./rollback-node-agent.sh "${REGION}-worker-${i}"
done
```

## Best Practices

### Upgrade Schedule

1. **Start with one node per region**: Test the upgrade on worker-1 in each region
2. **Monitor for 30 minutes**: Check VM health and performance
3. **Proceed with remaining nodes**: Upgrade workers 2-6 in each region
4. **Stagger regions**: Complete one region before starting the next

### Capacity Planning

- Ensure at least 20% spare capacity for VM migration
- Consider peak hours - upgrade during low traffic
- Have emergency nodes ready if migration fails

### Testing

Always test the upgrade path:

```bash
# In staging environment
./upgrade-node-agents.sh --dry-run
./test-vm-migration.sh
./verify-customer-apps.sh
```

### Monitoring During Upgrade

```bash
# Watch VM migrations in real-time
watch -n 2 'curl -s http://operator:8080/api/v1/nodes | \
    jq -r ".[] | \"\(.hostname): \(.active_instances) VMs, \
    Status: \(.status), Drain: \(.draining)\""'

# Monitor customer application health
for deployment in $(curl -s http://operator:8080/api/v1/deployments | \
                   jq -r '.[].id'); do
    curl -s "http://operator:8080/api/v1/deployments/${deployment}/health"
done
```

### Communication

Before starting:

- Notify customers of maintenance (even though zero-downtime)
- Alert on-call team
- Update status page

During upgrade:

- Post updates to #platform-upgrades Slack
- Monitor customer support channels
- Be ready to pause/rollback

## Troubleshooting

### Node Agent Won't Start

```bash
# Check logs
sudo journalctl -u zeitwork-node-agent -n 100

# Verify Firecracker setup
sudo systemctl status firecracker
ls -la /dev/kvm
lsmod | grep kvm

# Check connectivity to operator
curl http://operator:8080/health
```

### VMs Won't Migrate

```bash
# Force migration
curl -X POST "http://operator:8080/api/v1/instances/${INSTANCE_ID}/migrate" \
    -d '{"force": true}'

# Check node capacity
curl http://operator:8080/api/v1/nodes | jq '.[] | select(.available_capacity > 0)'
```

### Performance Degradation

```bash
# Check resource usage
ssh $NODE "top -b -n 1 | head -20"
ssh $NODE "df -h"
ssh $NODE "free -h"

# Review VM resource allocation
curl "http://${NODE}:8081/metrics" | grep -E "cpu|memory|disk"
```

## Related Documentation

- [Operator Upgrade Guide](./operator-upgrade.md)
- [Platform Upgrade Guide](./platform-upgrade.md)
- [VM Migration Guide](../operations/vm-migration.md)
- [Firecracker Management](../operations/firecracker.md)
