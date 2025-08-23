# Platform-Wide Upgrade Guide

This guide orchestrates upgrading the entire Zeitwork platform across all services and regions.

## Overview

A complete platform upgrade involves coordinating updates across:

- Database schema migrations
- 9 Operator services (3 per region)
- 18 Node Agent services (6 per region)
- 9 Load Balancer services (3 per region)
- 9 Edge Proxy services (3 per region)

## Upgrade Order

The correct dependency order for platform upgrades:

```
1. Database Schema Migrations
   ↓
2. Operator Services (control plane)
   ↓
3. Node Agent Services (worker nodes)
   ↓
4. Load Balancer Services
   ↓
5. Edge Proxy Services
```

## Complete Platform Upgrade Process

### Phase 1: Preparation

```bash
#!/bin/bash
# prepare-platform-upgrade.sh

VERSION="v2.0.0"
UPGRADE_DIR="/opt/zeitwork-upgrade-${VERSION}"

echo "=== Platform Upgrade Preparation for ${VERSION} ==="

# Create upgrade directory
mkdir -p ${UPGRADE_DIR}/{binaries,scripts,backups,logs}
cd ${UPGRADE_DIR}

# Clone and build
git clone https://github.com/zeitwork/zeitwork.git
cd zeitwork
git checkout ${VERSION}

# Build all components
echo "Building platform components..."
make build

# Copy binaries
cp build/* ${UPGRADE_DIR}/binaries/

# Generate node lists
cat > ${UPGRADE_DIR}/nodes.txt << EOF
# Operators
us-east-op-1
us-east-op-2
us-east-op-3
eu-west-op-1
eu-west-op-2
eu-west-op-3
ap-south-op-1
ap-south-op-2
ap-south-op-3

# Workers
us-east-worker-1
us-east-worker-2
us-east-worker-3
us-east-worker-4
us-east-worker-5
us-east-worker-6
eu-west-worker-1
eu-west-worker-2
eu-west-worker-3
eu-west-worker-4
eu-west-worker-5
eu-west-worker-6
ap-south-worker-1
ap-south-worker-2
ap-south-worker-3
ap-south-worker-4
ap-south-worker-5
ap-south-worker-6
EOF

# Pre-upgrade health check
echo "=== Pre-Upgrade Health Check ==="
./scripts/health-check-all.sh | tee ${UPGRADE_DIR}/logs/pre-upgrade-health.log

# Backup current configurations
echo "=== Backing up configurations ==="
for node in $(grep -v "^#" nodes.txt); do
    mkdir -p ${UPGRADE_DIR}/backups/${node}
    scp ${node}:/etc/zeitwork/*.env ${UPGRADE_DIR}/backups/${node}/ || true
done

echo "Preparation complete. Upgrade directory: ${UPGRADE_DIR}"
```

### Phase 2: Database Migrations

```bash
#!/bin/bash
# phase2-database-migrations.sh

echo "=== Phase 2: Database Migrations ==="

cd ${UPGRADE_DIR}/zeitwork/packages/database

# Install dependencies
npm install

# Check current schema version
echo "Current schema version:"
psql "$DATABASE_URL" -c "SELECT version, applied_at FROM schema_migrations ORDER BY version DESC LIMIT 1;"

# Run migrations with backup
echo "Creating database backup point..."
# PlanetScale automatically maintains backups

echo "Running migrations..."
npm run db:migrate

# Verify migrations
echo "New schema version:"
psql "$DATABASE_URL" -c "SELECT version, applied_at FROM schema_migrations ORDER BY version DESC LIMIT 1;"

echo "Database migrations complete"
```

### Phase 3: Operator Upgrade

```bash
#!/bin/bash
# phase3-operator-upgrade.sh

echo "=== Phase 3: Operator Services Upgrade ==="

OPERATOR_NODES=(
    "us-east-op-1" "us-east-op-2" "us-east-op-3"
    "eu-west-op-1" "eu-west-op-2" "eu-west-op-3"
    "ap-south-op-1" "ap-south-op-2" "ap-south-op-3"
)

for node in "${OPERATOR_NODES[@]}"; do
    echo "Upgrading operator on ${node}..."

    # Copy and upgrade
    scp ${UPGRADE_DIR}/binaries/zeitwork-operator ${node}:/tmp/

    ssh ${node} << 'ENDSSH'
        sudo systemctl stop zeitwork-operator
        sudo cp /usr/local/bin/zeitwork-operator /usr/local/bin/zeitwork-operator.backup
        sudo mv /tmp/zeitwork-operator /usr/local/bin/
        sudo chmod +x /usr/local/bin/zeitwork-operator
        sudo systemctl start zeitwork-operator
ENDSSH

    # Health check
    sleep 10
    if ! curl -s http://${node}:8080/health | grep -q "healthy"; then
        echo "ERROR: ${node} unhealthy after upgrade"
        exit 1
    fi

    echo "✓ ${node} upgraded successfully"
    sleep 30  # Wait between operators
done

echo "All operators upgraded"
```

### Phase 4: Node Agent Upgrade

```bash
#!/bin/bash
# phase4-node-agent-upgrade.sh

echo "=== Phase 4: Node Agent Services Upgrade ==="

# Function to upgrade a region
upgrade_region_workers() {
    local region=$1
    local workers=()

    for i in {1..6}; do
        workers+=("${region}-worker-${i}")
    done

    echo "Upgrading workers in region: ${region}"

    for worker in "${workers[@]}"; do
        echo "Processing ${worker}..."

        # Drain node
        node_id=$(curl -s http://localhost:8080/api/v1/nodes | \
                  jq -r ".[] | select(.hostname==\"${worker}\") | .id")
        curl -X POST "http://localhost:8080/api/v1/nodes/${node_id}/drain"

        # Wait for drain
        while [ $(curl -s "http://localhost:8080/api/v1/nodes/${node_id}" | \
                  jq -r '.active_instances') -gt 0 ]; do
            echo "Waiting for VMs to migrate from ${worker}..."
            sleep 10
        done

        # Upgrade
        scp ${UPGRADE_DIR}/binaries/zeitwork-node-agent ${worker}:/tmp/
        ssh ${worker} << 'ENDSSH'
            sudo systemctl stop zeitwork-node-agent
            sudo cp /usr/local/bin/zeitwork-node-agent /usr/local/bin/zeitwork-node-agent.backup
            sudo mv /tmp/zeitwork-node-agent /usr/local/bin/
            sudo chmod +x /usr/local/bin/zeitwork-node-agent
            sudo systemctl start zeitwork-node-agent
ENDSSH

        # Re-enable node
        sleep 10
        curl -X POST "http://localhost:8080/api/v1/nodes/${node_id}/enable"

        echo "✓ ${worker} upgraded"
        sleep 60  # Allow VM redistribution
    done
}

# Upgrade each region
for region in us-east eu-west ap-south; do
    upgrade_region_workers ${region}
    echo "Region ${region} complete. Waiting 5 minutes before next region..."
    sleep 300
done

echo "All node agents upgraded"
```

### Phase 5: Load Balancer & Edge Proxy Upgrade

```bash
#!/bin/bash
# phase5-lb-edge-upgrade.sh

echo "=== Phase 5: Load Balancer & Edge Proxy Upgrade ==="

OPERATOR_NODES=(
    "us-east-op-1" "us-east-op-2" "us-east-op-3"
    "eu-west-op-1" "eu-west-op-2" "eu-west-op-3"
    "ap-south-op-1" "ap-south-op-2" "ap-south-op-3"
)

for node in "${OPERATOR_NODES[@]}"; do
    echo "Upgrading LB and Edge Proxy on ${node}..."

    # Copy binaries
    scp ${UPGRADE_DIR}/binaries/zeitwork-load-balancer ${node}:/tmp/
    scp ${UPGRADE_DIR}/binaries/zeitwork-edge-proxy ${node}:/tmp/

    # Upgrade both services
    ssh ${node} << 'ENDSSH'
        # Load Balancer
        sudo systemctl stop zeitwork-load-balancer
        sudo cp /usr/local/bin/zeitwork-load-balancer /usr/local/bin/zeitwork-load-balancer.backup
        sudo mv /tmp/zeitwork-load-balancer /usr/local/bin/
        sudo chmod +x /usr/local/bin/zeitwork-load-balancer
        sudo systemctl start zeitwork-load-balancer

        # Edge Proxy
        sudo systemctl stop zeitwork-edge-proxy
        sudo cp /usr/local/bin/zeitwork-edge-proxy /usr/local/bin/zeitwork-edge-proxy.backup
        sudo mv /tmp/zeitwork-edge-proxy /usr/local/bin/
        sudo chmod +x /usr/local/bin/zeitwork-edge-proxy
        sudo systemctl start zeitwork-edge-proxy
ENDSSH

    # Health checks
    sleep 10
    if ! curl -s http://${node}:8082/health | grep -q "healthy"; then
        echo "ERROR: Load balancer on ${node} unhealthy"
        exit 1
    fi

    if ! curl -k -s https://${node}:8083/health | grep -q "healthy"; then
        echo "ERROR: Edge proxy on ${node} unhealthy"
        exit 1
    fi

    echo "✓ ${node} LB and Edge Proxy upgraded"
    sleep 30
done

echo "All load balancers and edge proxies upgraded"
```

### Phase 6: Verification

```bash
#!/bin/bash
# phase6-verification.sh

echo "=== Phase 6: Platform Verification ==="

# Function to check service health
check_service() {
    local service=$1
    local port=$2
    local nodes=("${@:3}")

    echo "Checking ${service}..."
    for node in "${nodes[@]}"; do
        if curl -s http://${node}:${port}/health 2>/dev/null | grep -q "healthy"; then
            echo "  ✓ ${node}"
        else
            echo "  ✗ ${node} - UNHEALTHY"
            ERRORS=$((ERRORS + 1))
        fi
    done
}

ERRORS=0

# Check all operators
OPERATORS=(us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3})
check_service "Operators" 8080 "${OPERATORS[@]}"

# Check all node agents
WORKERS=(us-east-worker-{1..6} eu-west-worker-{1..6} ap-south-worker-{1..6})
check_service "Node Agents" 8081 "${WORKERS[@]}"

# Check load balancers
check_service "Load Balancers" 8082 "${OPERATORS[@]}"

# Check edge proxies (HTTPS)
echo "Checking Edge Proxies..."
for node in "${OPERATORS[@]}"; do
    if curl -k -s https://${node}:8083/health 2>/dev/null | grep -q "healthy"; then
        echo "  ✓ ${node}"
    else
        echo "  ✗ ${node} - UNHEALTHY"
        ERRORS=$((ERRORS + 1))
    fi
done

# Check customer applications
echo -e "\nChecking Customer Applications..."
for deployment in $(curl -s http://localhost:8080/api/v1/deployments/active | jq -r '.[].id'); do
    status=$(curl -s http://localhost:8080/api/v1/deployments/${deployment}/health | jq -r '.status')
    echo "  Deployment ${deployment}: ${status}"
done

# Summary
echo -e "\n=== Upgrade Summary ==="
if [ ${ERRORS} -eq 0 ]; then
    echo "✓ Platform upgrade completed successfully!"
else
    echo "✗ Platform upgrade completed with ${ERRORS} errors"
    echo "  Review logs and take corrective action"
fi

# Save verification report
echo "Detailed report saved to: ${UPGRADE_DIR}/logs/post-upgrade-verification.log"
```

## Automated Platform Upgrade

For fully automated upgrades, use the master script:

```bash
#!/bin/bash
# master-platform-upgrade.sh

set -e  # Exit on any error

VERSION="v2.0.0"
UPGRADE_LOG="/var/log/zeitwork-upgrade-${VERSION}.log"

# Function to log with timestamp
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a ${UPGRADE_LOG}
}

# Main upgrade orchestration
main() {
    log "Starting Zeitwork Platform Upgrade to ${VERSION}"

    # Phase 1: Preparation
    log "Phase 1: Preparation"
    ./prepare-platform-upgrade.sh 2>&1 | tee -a ${UPGRADE_LOG}

    # Phase 2: Database
    log "Phase 2: Database Migrations"
    ./phase2-database-migrations.sh 2>&1 | tee -a ${UPGRADE_LOG}

    # Phase 3: Operators
    log "Phase 3: Operator Services"
    ./phase3-operator-upgrade.sh 2>&1 | tee -a ${UPGRADE_LOG}

    # Phase 4: Node Agents
    log "Phase 4: Node Agent Services"
    ./phase4-node-agent-upgrade.sh 2>&1 | tee -a ${UPGRADE_LOG}

    # Phase 5: LB & Edge Proxy
    log "Phase 5: Load Balancers and Edge Proxies"
    ./phase5-lb-edge-upgrade.sh 2>&1 | tee -a ${UPGRADE_LOG}

    # Phase 6: Verification
    log "Phase 6: Verification"
    ./phase6-verification.sh 2>&1 | tee -a ${UPGRADE_LOG}

    log "Platform upgrade complete! Log: ${UPGRADE_LOG}"
}

# Confirmation prompt
read -p "Upgrade Zeitwork Platform to ${VERSION}? This will affect all services. (yes/no): " confirm
if [ "$confirm" != "yes" ]; then
    echo "Upgrade cancelled"
    exit 0
fi

# Run upgrade
main
```

## Rollback Procedures

### Complete Platform Rollback

```bash
#!/bin/bash
# platform-rollback.sh

echo "=== EMERGENCY PLATFORM ROLLBACK ==="

# Stop all services first
parallel-ssh -h all-nodes.txt "sudo systemctl stop zeitwork-*"

# Restore binaries
for node in $(cat all-nodes.txt); do
    echo "Restoring binaries on ${node}..."
    ssh ${node} << 'ENDSSH'
        for service in operator node-agent load-balancer edge-proxy; do
            if [ -f /usr/local/bin/zeitwork-${service}.backup ]; then
                sudo cp /usr/local/bin/zeitwork-${service}.backup /usr/local/bin/zeitwork-${service}
            fi
        done
ENDSSH
done

# Restore configurations
for node in $(cat all-nodes.txt); do
    echo "Restoring configs on ${node}..."
    if [ -d ${UPGRADE_DIR}/backups/${node} ]; then
        scp ${UPGRADE_DIR}/backups/${node}/*.env ${node}:/tmp/
        ssh ${node} "sudo mv /tmp/*.env /etc/zeitwork/"
    fi
done

# Start services in order
# 1. Operators
for node in us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3}; do
    ssh ${node} "sudo systemctl start zeitwork-operator"
done
sleep 30

# 2. Load Balancers and Edge Proxies
for node in us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3}; do
    ssh ${node} "sudo systemctl start zeitwork-load-balancer zeitwork-edge-proxy"
done

# 3. Node Agents
parallel-ssh -h worker-nodes.txt "sudo systemctl start zeitwork-node-agent"

echo "Rollback complete. Verify all services are healthy."
```

## Monitoring During Upgrade

### Real-Time Dashboard

```bash
#!/bin/bash
# monitor-upgrade.sh

while true; do
    clear
    echo "=== ZEITWORK PLATFORM UPGRADE MONITOR ==="
    echo "Time: $(date)"
    echo

    # Service Status
    echo "OPERATORS:"
    for node in us-east-op-{1..3}; do
        status=$(curl -s http://${node}:8080/health 2>/dev/null | jq -r '.status' || echo "DOWN")
        printf "  %-15s: %s\n" "${node}" "${status}"
    done

    echo -e "\nNODE AGENTS:"
    for node in us-east-worker-{1..3}; do
        status=$(curl -s http://${node}:8081/health 2>/dev/null | jq -r '.status' || echo "DOWN")
        vms=$(curl -s http://localhost:8080/api/v1/nodes 2>/dev/null | \
              jq -r ".[] | select(.hostname==\"${node}\") | .active_instances" || echo "?")
        printf "  %-20s: %-8s (VMs: %s)\n" "${node}" "${status}" "${vms}"
    done

    echo -e "\nCUSTOMER IMPACT:"
    for domain in app.dokedu.org app.acme.com; do
        response=$(curl -o /dev/null -s -w "%{http_code}" https://${domain})
        time=$(curl -o /dev/null -s -w "%{time_total}" https://${domain})
        printf "  %-20s: HTTP %s (%.3fs)\n" "${domain}" "${response}" "${time}"
    done

    sleep 5
done
```

## Best Practices

1. **Test in Staging**: Always run the complete upgrade in staging first
2. **Gradual Rollout**: Consider upgrading one region at a time for critical updates
3. **Monitor Actively**: Keep monitoring dashboards open during the entire process
4. **Document Issues**: Record any issues for post-mortem analysis
5. **Communicate**: Keep stakeholders informed of progress
6. **Plan Rollback**: Have rollback procedures ready and tested
7. **Verify Backups**: Ensure database backups are current before starting

## Troubleshooting

### Service Dependencies Failed

If services fail to start due to dependencies:

```bash
# Check service dependencies
systemctl list-dependencies zeitwork-operator

# Start in correct order
systemctl start postgresql
systemctl start zeitwork-operator
systemctl start zeitwork-node-agent
```

### Database Migration Failed

```bash
# Check migration status
cd packages/database
npm run db:status

# Rollback migration
npm run db:rollback

# Fix and retry
npm run db:migrate
```

### VM Migration Stuck

```bash
# Force migrate VMs
curl -X POST http://localhost:8080/api/v1/nodes/${NODE_ID}/drain \
    -d '{"force": true, "timeout": 300}'

# Or manually evacuate
for vm in $(curl -s http://localhost:8080/api/v1/nodes/${NODE_ID}/instances | jq -r '.[].id'); do
    curl -X POST http://localhost:8080/api/v1/instances/${vm}/migrate \
        -d '{"target": "auto"}'
done
```

## Related Documentation

- [Operator Upgrade Guide](./operator-upgrade.md)
- [Node Agent Upgrade Guide](./node-agent-upgrade.md)
- [Load Balancer Upgrade Guide](./load-balancer-upgrade.md)
- [Edge Proxy Upgrade Guide](./edge-proxy-upgrade.md)
- [Database Migration Guide](./database-migrations.md)
- [Rollback Procedures](./rollback-procedures.md)
