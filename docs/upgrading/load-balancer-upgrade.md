# Load Balancer Upgrade Guide

This guide covers upgrading the Zeitwork Load Balancer service that handles Layer 4 TCP load balancing.

## Overview

The Load Balancer is a stateless service running on operator nodes, making upgrades straightforward with minimal risk. Each operator node runs its own load balancer instance (3 per region, 9 total).

## Prerequisites

- [ ] New version tested in staging
- [ ] DNS failover configured
- [ ] Monitoring active on all endpoints

## Upgrade Process

### Rolling Upgrade

```bash
#!/bin/bash
# upgrade-load-balancers.sh

VERSION="v2.0.0"
OPERATOR_NODES=(
    "us-east-op-1" "us-east-op-2" "us-east-op-3"
    "eu-west-op-1" "eu-west-op-2" "eu-west-op-3"
    "ap-south-op-1" "ap-south-op-2" "ap-south-op-3"
)

for node in "${OPERATOR_NODES[@]}"; do
    echo "Upgrading load balancer on $node..."

    # Copy new binary
    scp "build/zeitwork-load-balancer" "${node}:/tmp/"

    # Upgrade
    ssh "$node" << 'ENDSSH'
        # Backup current binary
        sudo cp /usr/local/bin/zeitwork-load-balancer \
                /usr/local/bin/zeitwork-load-balancer.backup

        # Install new binary
        sudo mv /tmp/zeitwork-load-balancer /usr/local/bin/
        sudo chmod +x /usr/local/bin/zeitwork-load-balancer
        sudo chown zeitwork:zeitwork /usr/local/bin/zeitwork-load-balancer

        # Restart service (brief connection reset)
        sudo systemctl restart zeitwork-load-balancer
ENDSSH

    # Verify health
    sleep 5
    if curl -s "http://${node}:8082/health" | grep -q "healthy"; then
        echo "✓ $node upgraded successfully"
    else
        echo "✗ $node upgrade failed, rolling back..."
        ssh "$node" "sudo cp /usr/local/bin/zeitwork-load-balancer.backup \
                            /usr/local/bin/zeitwork-load-balancer && \
                     sudo systemctl restart zeitwork-load-balancer"
        exit 1
    fi

    # Wait before next node
    sleep 10
done

echo "All load balancers upgraded successfully!"
```

### Zero-Downtime Upgrade with DNS

For true zero-downtime, use DNS failover:

```bash
# 1. Remove node from DNS rotation
./dns-remove.sh us-east-op-1

# 2. Wait for DNS propagation
sleep 60

# 3. Upgrade the node
./upgrade-single-lb.sh us-east-op-1

# 4. Re-add to DNS
./dns-add.sh us-east-op-1

# 5. Repeat for each node
```

## Verification

```bash
# Check all load balancer health
for node in us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3}; do
    echo -n "$node: "
    curl -s "http://${node}:8082/health" | jq -r '.status'
done

# Verify load balancing algorithms
curl http://us-east-op-1:8082/api/v1/config | jq '.algorithm'

# Check backend pools
curl http://us-east-op-1:8082/api/v1/backends | jq '.[].status'
```

## Configuration Updates

If configuration changes are needed:

```bash
# Update load balancer configuration
sudo vim /etc/zeitwork/load-balancer.env

# Key configuration options:
# ALGORITHM=round-robin|least-connections|ip-hash
# HEALTH_CHECK_INTERVAL=10s
# BACKEND_TIMEOUT=30s

# Restart to apply
sudo systemctl restart zeitwork-load-balancer
```

## Rollback

```bash
#!/bin/bash
# rollback-load-balancer.sh

NODE=$1
ssh "$NODE" << 'ENDSSH'
    sudo cp /usr/local/bin/zeitwork-load-balancer.backup \
            /usr/local/bin/zeitwork-load-balancer
    sudo systemctl restart zeitwork-load-balancer
ENDSSH
```

## Monitoring

During upgrade, monitor:

- Active connections: `curl http://node:8082/metrics | grep active_connections`
- Error rates: `curl http://node:8082/metrics | grep errors_total`
- Backend health: `curl http://node:8082/api/v1/backends`

## Related Documentation

- [Edge Proxy Upgrade Guide](./edge-proxy-upgrade.md)
- [Platform Upgrade Guide](./platform-upgrade.md)
