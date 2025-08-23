# Operator Service Upgrade Guide

This guide provides detailed instructions for upgrading the Zeitwork Operator service to a new version with zero downtime.

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Upgrade Strategies](#upgrade-strategies)
4. [Step-by-Step Upgrade Process](#step-by-step-upgrade-process)
5. [Configuration Updates](#configuration-updates)
6. [Database Migrations](#database-migrations)
7. [Verification](#verification)
8. [Rollback Procedures](#rollback-procedures)
9. [Troubleshooting](#troubleshooting)
10. [Best Practices](#best-practices)

## Overview

The Zeitwork Operator is the central control plane service that manages the entire cluster. With 3 operator nodes per region (9 total across 3 regions), upgrades can be performed with zero downtime using a rolling update strategy.

### Architecture Considerations

- **High Availability**: Always maintain at least 2 operators running per region during upgrades
- **Database Compatibility**: Ensure database schema migrations are backward compatible
- **API Compatibility**: New operators must be compatible with existing node agents
- **State Management**: Operators are stateless; state is maintained in PostgreSQL

## Prerequisites

Before starting the upgrade:

- [ ] New version tested in staging environment
- [ ] Release notes reviewed for breaking changes
- [ ] Database backup completed (automatic with PlanetScale)
- [ ] Maintenance window scheduled (optional, but recommended)
- [ ] Rollback plan prepared
- [ ] Monitoring alerts configured
- [ ] All operator nodes accessible via SSH
- [ ] Build server has latest code

## Upgrade Strategies

### Strategy 1: Rolling Update (Recommended)

Upgrade one operator at a time across all regions. This provides:

- Zero downtime
- Immediate rollback capability
- Gradual rollout across the cluster

### Strategy 2: Region-by-Region

Upgrade all operators in one region before moving to the next. This provides:

- Regional isolation of potential issues
- Extended monitoring between regions
- Easier rollback scope

### Strategy 3: Blue-Green (Advanced)

Deploy new operators alongside old ones, then switch traffic. This requires:

- Additional infrastructure
- Load balancer reconfiguration
- More complex but safest approach

## Step-by-Step Upgrade Process

### 1. Build the New Version

```bash
# On build server
cd /path/to/zeitwork

# Checkout new version
git fetch --all
git checkout v2.0.0  # or specific tag/branch

# Build the operator binary
make build

# Verify build
./build/zeitwork-operator --version

# Create deployment archive
tar -czf zeitwork-operator-v2.0.0.tar.gz build/zeitwork-operator
```

### 2. Prepare Upgrade Script

Create `upgrade-operator.sh`:

```bash
#!/bin/bash
set -e

# Configuration
VERSION="v2.0.0"
BINARY_ARCHIVE="zeitwork-operator-${VERSION}.tar.gz"
OPERATOR_NODES=(
    "us-east-op-1" "us-east-op-2" "us-east-op-3"
    "eu-west-op-1" "eu-west-op-2" "eu-west-op-3"
    "ap-south-op-1" "ap-south-op-2" "ap-south-op-3"
)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Health check function
check_health() {
    local node=$1
    local max_attempts=30
    local attempt=1

    while [ $attempt -le $max_attempts ]; do
        if curl -s "http://${node}:8080/health" | grep -q "healthy"; then
            return 0
        fi
        echo -n "."
        sleep 2
        ((attempt++))
    done
    return 1
}

# Upgrade single node
upgrade_node() {
    local node=$1
    log_info "Starting upgrade on ${node}..."

    # Copy new binary
    log_info "Copying new binary to ${node}..."
    scp "${BINARY_ARCHIVE}" "${node}:/tmp/"

    # Perform upgrade
    ssh "${node}" << 'ENDSSH'
        set -e

        # Extract new binary
        cd /tmp
        tar -xzf zeitwork-operator-*.tar.gz

        # Create backup
        sudo cp /usr/local/bin/zeitwork-operator \
                /usr/local/bin/zeitwork-operator.backup

        # Install new binary
        sudo mv build/zeitwork-operator /usr/local/bin/zeitwork-operator
        sudo chmod +x /usr/local/bin/zeitwork-operator
        sudo chown zeitwork:zeitwork /usr/local/bin/zeitwork-operator

        # Restart service
        sudo systemctl restart zeitwork-operator

        # Cleanup
        rm -rf /tmp/zeitwork-operator-*.tar.gz /tmp/build
ENDSSH

    # Verify health
    log_info "Checking health of ${node}..."
    if check_health "${node}"; then
        log_info "✓ ${node} upgraded successfully"
        return 0
    else
        log_error "✗ ${node} health check failed"
        return 1
    fi
}

# Rollback single node
rollback_node() {
    local node=$1
    log_warn "Rolling back ${node}..."

    ssh "${node}" << 'ENDSSH'
        sudo cp /usr/local/bin/zeitwork-operator.backup \
                /usr/local/bin/zeitwork-operator
        sudo systemctl restart zeitwork-operator
ENDSSH

    if check_health "${node}"; then
        log_info "✓ ${node} rolled back successfully"
    else
        log_error "✗ ${node} rollback failed - manual intervention required"
        exit 1
    fi
}

# Main upgrade loop
main() {
    log_info "Starting operator upgrade to ${VERSION}"
    log_info "Total nodes to upgrade: ${#OPERATOR_NODES[@]}"

    for node in "${OPERATOR_NODES[@]}"; do
        if upgrade_node "${node}"; then
            log_info "Waiting 30 seconds before next node..."
            sleep 30
        else
            log_error "Upgrade failed on ${node}"
            rollback_node "${node}"
            log_error "Upgrade aborted. Successfully upgraded nodes may need manual rollback."
            exit 1
        fi
    done

    log_info "✓ All operators upgraded successfully to ${VERSION}"
}

# Run main function
main
```

### 3. Execute the Upgrade

```bash
# Make script executable
chmod +x upgrade-operator.sh

# Run the upgrade
./upgrade-operator.sh

# Monitor progress
watch -n 2 'for node in us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3}; do \
    echo -n "$node: "; \
    curl -s http://$node:8080/health 2>/dev/null | jq -r .status || echo "unreachable"; \
done'
```

### 4. Regional Upgrade Approach (Alternative)

For a more cautious approach, upgrade by region:

```bash
#!/bin/bash
# regional-upgrade.sh

REGIONS=("us-east" "eu-west" "ap-south")
NODES_PER_REGION=3

for region in "${REGIONS[@]}"; do
    echo "Upgrading region: $region"

    for i in $(seq 1 $NODES_PER_REGION); do
        node="${region}-op-${i}"
        ./upgrade-operator.sh "$node"
    done

    echo "Region $region complete. Monitoring for 1 hour..."
    sleep 3600

    # Check metrics/alerts before proceeding
    read -p "Continue to next region? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Upgrade paused. Resume manually."
        exit 0
    fi
done
```

## Configuration Updates

### Updating Configuration Files

If the new version requires configuration changes:

```bash
#!/bin/bash
# update-operator-config.sh

OPERATOR_NODES=(
    "us-east-op-1" "us-east-op-2" "us-east-op-3"
    "eu-west-op-1" "eu-west-op-2" "eu-west-op-3"
    "ap-south-op-1" "ap-south-op-2" "ap-south-op-3"
)

CONFIG_CHANGES=(
    "s/LOG_LEVEL=info/LOG_LEVEL=debug/"
    "s/MAX_CONNECTIONS=100/MAX_CONNECTIONS=200/"
    "s/OLD_PARAM=value/NEW_PARAM=newvalue/"
)

for node in "${OPERATOR_NODES[@]}"; do
    echo "Updating configuration on $node..."

    ssh "$node" << 'ENDSSH'
        # Backup current config
        sudo cp /etc/zeitwork/operator.env /etc/zeitwork/operator.env.backup

        # Apply changes
        for change in "${CONFIG_CHANGES[@]}"; do
            sudo sed -i "$change" /etc/zeitwork/operator.env
        done

        # Restart service
        sudo systemctl restart zeitwork-operator
ENDSSH
done
```

### Environment Variable Changes

For new environment variables:

```bash
# Add new variables to /etc/zeitwork/operator.env
echo "NEW_FEATURE_FLAG=enabled" | sudo tee -a /etc/zeitwork/operator.env
echo "PERFORMANCE_MODE=optimized" | sudo tee -a /etc/zeitwork/operator.env

# Restart to apply
sudo systemctl restart zeitwork-operator
```

## Database Migrations

### Pre-Upgrade Migrations

Run database migrations before upgrading operators:

```bash
# 1. Check migration status
cd packages/database
npm run db:status

# 2. Backup database (automatic with PlanetScale)
# PlanetScale maintains automatic backups

# 3. Run migrations
export DATABASE_URL="postgresql://username:password@host.connect.psdb.cloud/zeitwork-production?sslmode=require"
npm run db:migrate

# 4. Verify migrations
psql "$DATABASE_URL" -c "SELECT version, applied_at FROM schema_migrations ORDER BY version DESC LIMIT 5;"
```

### Post-Upgrade Migrations

For non-backward-compatible changes (rare):

```bash
# 1. Upgrade all operators first
./upgrade-operator.sh

# 2. Run post-upgrade migrations
npm run db:migrate:post

# 3. Restart operators to pick up schema changes
for node in "${OPERATOR_NODES[@]}"; do
    ssh "$node" "sudo systemctl restart zeitwork-operator"
done
```

## Verification

### Health Checks

```bash
#!/bin/bash
# verify-upgrade.sh

echo "=== Operator Health Status ==="
for node in us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3}; do
    printf "%-20s: " "$node"
    if curl -s "http://${node}:8080/health" 2>/dev/null | grep -q "healthy"; then
        echo "✓ Healthy"
    else
        echo "✗ Unhealthy"
    fi
done

echo -e "\n=== Version Information ==="
for node in us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3}; do
    printf "%-20s: " "$node"
    ssh "$node" "/usr/local/bin/zeitwork-operator --version" 2>/dev/null || echo "Error"
done

echo -e "\n=== API Endpoints ==="
# Test critical API endpoints
curl -s http://us-east-op-1:8080/api/v1/nodes | jq '.[] | .hostname' | head -5
curl -s http://us-east-op-1:8080/api/v1/instances | jq '.[] | .id' | head -5
```

### Monitoring Checks

```bash
# Check metrics
curl -s http://us-east-op-1:8080/metrics | grep -E "^zeitwork_operator_"

# Check logs for errors
for node in us-east-op-{1..3}; do
    echo "=== $node recent errors ==="
    ssh "$node" "sudo journalctl -u zeitwork-operator --since '10 minutes ago' | grep -i error" || true
done

# Database connectivity
for node in us-east-op-{1..3}; do
    echo -n "$node DB connection: "
    ssh "$node" "sudo journalctl -u zeitwork-operator --since '5 minutes ago' | grep -c 'database connection established'" || echo "0"
done
```

## Rollback Procedures

### Immediate Rollback

If issues are detected during upgrade:

```bash
#!/bin/bash
# rollback-operator.sh

NODE=$1

if [ -z "$NODE" ]; then
    echo "Usage: $0 <node-hostname>"
    exit 1
fi

echo "Rolling back $NODE..."

ssh "$NODE" << 'ENDSSH'
    set -e

    # Restore binary
    if [ -f /usr/local/bin/zeitwork-operator.backup ]; then
        sudo cp /usr/local/bin/zeitwork-operator.backup /usr/local/bin/zeitwork-operator
    else
        echo "Error: Backup binary not found!"
        exit 1
    fi

    # Restore configuration if exists
    if [ -f /etc/zeitwork/operator.env.backup ]; then
        sudo cp /etc/zeitwork/operator.env.backup /etc/zeitwork/operator.env
    fi

    # Restart service
    sudo systemctl restart zeitwork-operator

    # Check status
    sleep 5
    sudo systemctl status zeitwork-operator --no-pager
ENDSSH

# Verify rollback
if curl -s "http://${NODE}:8080/health" | grep -q "healthy"; then
    echo "✓ Rollback successful"
else
    echo "✗ Rollback failed - manual intervention required"
    exit 1
fi
```

### Full Cluster Rollback

For complete rollback of all operators:

```bash
#!/bin/bash
# rollback-all-operators.sh

OPERATOR_NODES=(
    "us-east-op-1" "us-east-op-2" "us-east-op-3"
    "eu-west-op-1" "eu-west-op-2" "eu-west-op-3"
    "ap-south-op-1" "ap-south-op-2" "ap-south-op-3"
)

for node in "${OPERATOR_NODES[@]}"; do
    ./rollback-operator.sh "$node"
done

echo "All operators rolled back"
```

## Troubleshooting

### Common Issues

#### Service Won't Start

```bash
# Check service status
sudo systemctl status zeitwork-operator

# Check for port conflicts
sudo netstat -tlnp | grep 8080

# Verify binary permissions
ls -la /usr/local/bin/zeitwork-operator

# Check service logs
sudo journalctl -u zeitwork-operator -n 100 --no-pager
```

#### Database Connection Issues

```bash
# Test database connectivity
psql "$DATABASE_URL" -c "SELECT 1;"

# Check operator database configuration
sudo grep DATABASE_URL /etc/zeitwork/operator.env

# Verify PlanetScale status
# Check https://status.planetscale.com
```

#### Health Check Failures

```bash
# Detailed health check
curl -v http://localhost:8080/health

# Check specific subsystems
curl http://localhost:8080/health/ready
curl http://localhost:8080/health/live

# Review error logs
sudo journalctl -u zeitwork-operator | grep -i "health check"
```

### Emergency Procedures

#### Single Node Recovery

```bash
# If a node is completely broken
NODE="us-east-op-1"

# Stop the service
ssh $NODE "sudo systemctl stop zeitwork-operator"

# Clean reinstall
ssh $NODE << 'ENDSSH'
    # Remove current installation
    sudo rm -f /usr/local/bin/zeitwork-operator*

    # Copy from working node
    scp us-east-op-2:/usr/local/bin/zeitwork-operator /tmp/
    sudo mv /tmp/zeitwork-operator /usr/local/bin/
    sudo chmod +x /usr/local/bin/zeitwork-operator
    sudo chown zeitwork:zeitwork /usr/local/bin/zeitwork-operator

    # Copy working configuration
    scp us-east-op-2:/etc/zeitwork/operator.env /tmp/
    sudo mv /tmp/operator.env /etc/zeitwork/

    # Start service
    sudo systemctl start zeitwork-operator
ENDSSH
```

## Best Practices

### Pre-Upgrade Checklist

- [ ] Test upgrade procedure in staging environment
- [ ] Review changelogs and breaking changes
- [ ] Ensure database backups are current
- [ ] Notify team of maintenance window
- [ ] Prepare rollback scripts
- [ ] Monitor key metrics baseline

### During Upgrade

- **Monitor actively**: Keep monitoring dashboards open
- **Upgrade slowly**: Wait between nodes to detect issues
- **Test continuously**: Run health checks after each node
- **Document issues**: Note any unexpected behavior
- **Communicate**: Keep team informed of progress

### Post-Upgrade

- [ ] Verify all nodes running new version
- [ ] Run comprehensive health checks
- [ ] Monitor metrics for 24 hours
- [ ] Update documentation with any issues encountered
- [ ] Clean up backup files after stable period
- [ ] Schedule post-mortem if issues occurred

### Version Compatibility Matrix

| Operator Version | Node Agent Version | Database Schema | Notes                     |
| ---------------- | ------------------ | --------------- | ------------------------- |
| v1.0.x           | v1.0.x             | v1              | Initial release           |
| v1.1.x           | v1.0.x - v1.1.x    | v1              | Backward compatible       |
| v2.0.x           | v1.1.x - v2.0.x    | v2              | Schema migration required |

### Maintenance Windows

Recommended windows for upgrades:

- **Lowest traffic**: Tuesday-Thursday, 2-6 AM local time
- **Avoid**: Mondays (high deployment activity), Fridays (reduced support availability)
- **Duration**: Allow 2 hours for full cluster upgrade
- **Notification**: 48 hours advance notice to customers for major versions

## Related Documentation

- [Node Agent Upgrade Guide](./node-agent-upgrade.md)
- [Load Balancer Upgrade Guide](./load-balancer-upgrade.md)
- [Edge Proxy Upgrade Guide](./edge-proxy-upgrade.md)
- [Database Migration Guide](./database-migrations.md)
- [Rollback Procedures](./rollback-procedures.md)
