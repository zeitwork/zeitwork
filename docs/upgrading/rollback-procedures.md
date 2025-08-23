# Rollback Procedures Guide

This guide provides comprehensive rollback procedures for all Zeitwork platform components when upgrades fail or cause issues.

## Table of Contents

1. [Overview](#overview)
2. [Rollback Decision Matrix](#rollback-decision-matrix)
3. [Service-Specific Rollbacks](#service-specific-rollbacks)
4. [Emergency Procedures](#emergency-procedures)
5. [Data Recovery](#data-recovery)
6. [Post-Rollback Actions](#post-rollback-actions)

## Overview

Rollback procedures are critical for maintaining platform stability. Every upgrade should have a tested rollback plan before execution.

### Rollback Principles

1. **Speed over perfection**: Fast rollback minimizes downtime
2. **Preserve data integrity**: Never lose customer data
3. **Document everything**: Record what triggered the rollback
4. **Test rollback paths**: Practice rollbacks in staging
5. **Communicate clearly**: Keep stakeholders informed

## Rollback Decision Matrix

| Symptom                       | Severity | Rollback Decision          | Timeframe    |
| ----------------------------- | -------- | -------------------------- | ------------ |
| Service won't start           | Critical | Immediate rollback         | < 5 minutes  |
| Health checks failing         | High     | Rollback after 2 retries   | < 10 minutes |
| Performance degradation > 50% | High     | Investigate, then rollback | < 15 minutes |
| Increased error rate > 5%     | Medium   | Monitor, consider rollback | < 30 minutes |
| Minor feature issues          | Low      | Document, fix forward      | Next release |

## Service-Specific Rollbacks

### Operator Service Rollback

```bash
#!/bin/bash
# rollback-operator.sh

NODES=(
    "us-east-op-1" "us-east-op-2" "us-east-op-3"
    "eu-west-op-1" "eu-west-op-2" "eu-west-op-3"
    "ap-south-op-1" "ap-south-op-2" "ap-south-op-3"
)

rollback_operator() {
    local node=$1

    echo "Rolling back operator on ${node}..."

    ssh ${node} << 'ENDSSH'
        # Stop service
        sudo systemctl stop zeitwork-operator

        # Restore binary
        if [ -f /usr/local/bin/zeitwork-operator.backup ]; then
            sudo cp /usr/local/bin/zeitwork-operator.backup /usr/local/bin/zeitwork-operator
        else
            echo "ERROR: No backup found!"
            exit 1
        fi

        # Restore configuration if exists
        if [ -f /etc/zeitwork/operator.env.backup ]; then
            sudo cp /etc/zeitwork/operator.env.backup /etc/zeitwork/operator.env
        fi

        # Start service
        sudo systemctl start zeitwork-operator

        # Verify
        sleep 5
        if systemctl is-active zeitwork-operator | grep -q active; then
            echo "Rollback successful"
        else
            echo "Rollback failed - manual intervention required"
            exit 1
        fi
ENDSSH
}

# Rollback all operators
for node in "${NODES[@]}"; do
    rollback_operator ${node}

    # Health check
    if curl -s http://${node}:8080/health | grep -q healthy; then
        echo "✓ ${node} healthy after rollback"
    else
        echo "✗ ${node} still unhealthy - check logs"
    fi
done
```

### Node Agent Rollback

```bash
#!/bin/bash
# rollback-node-agents.sh

rollback_node_agent() {
    local node=$1

    echo "Rolling back node agent on ${node}..."

    # Drain node first to migrate VMs
    node_id=$(curl -s http://localhost:8080/api/v1/nodes | \
              jq -r ".[] | select(.hostname==\"${node}\") | .id")

    if [ ! -z "${node_id}" ]; then
        curl -X POST "http://localhost:8080/api/v1/nodes/${node_id}/drain"

        # Wait for drain
        while [ $(curl -s "http://localhost:8080/api/v1/nodes/${node_id}" | \
                  jq -r '.active_instances') -gt 0 ]; do
            echo "Waiting for VMs to migrate..."
            sleep 10
        done
    fi

    # Perform rollback
    ssh ${node} << 'ENDSSH'
        sudo systemctl stop zeitwork-node-agent
        sudo cp /usr/local/bin/zeitwork-node-agent.backup /usr/local/bin/zeitwork-node-agent
        sudo systemctl start zeitwork-node-agent
ENDSSH

    # Re-enable node
    if [ ! -z "${node_id}" ]; then
        curl -X POST "http://localhost:8080/api/v1/nodes/${node_id}/enable"
    fi
}

# Rollback all node agents in a region
REGION="us-east"
for i in {1..6}; do
    rollback_node_agent "${REGION}-worker-${i}"
done
```

### Database Migration Rollback

```bash
#!/bin/bash
# rollback-database.sh

echo "=== Database Migration Rollback ==="

# Get last successful migration version
LAST_SAFE_VERSION=$(psql "$DATABASE_URL" -t -c "
    SELECT version FROM migration_history
    WHERE status = 'success'
    ORDER BY applied_at DESC
    OFFSET 1 LIMIT 1;"
)

echo "Rolling back to version: ${LAST_SAFE_VERSION}"

# Method 1: Using migration tool
cd packages/database
npm run db:rollback

# Method 2: Manual rollback with SQL
psql "$DATABASE_URL" << EOF
BEGIN;

-- Example: Revert table changes
ALTER TABLE projects DROP COLUMN IF EXISTS new_column;
ALTER TABLE instances RENAME COLUMN new_name TO old_name;

-- Restore dropped columns from backup
ALTER TABLE nodes ADD COLUMN deprecated_field TEXT;
UPDATE nodes n SET deprecated_field = b.deprecated_field
FROM nodes_backup b WHERE n.id = b.id;

-- Update migration tracking
DELETE FROM schema_migrations
WHERE version > '${LAST_SAFE_VERSION}';

COMMIT;
EOF

echo "Database rolled back to version ${LAST_SAFE_VERSION}"
```

## Emergency Procedures

### Complete Platform Rollback

```bash
#!/bin/bash
# emergency-platform-rollback.sh

set -e

echo "=== EMERGENCY PLATFORM ROLLBACK ==="
echo "This will rollback ALL services to previous version"
read -p "Are you sure? (type 'ROLLBACK' to confirm): " confirm

if [ "$confirm" != "ROLLBACK" ]; then
    echo "Rollback cancelled"
    exit 0
fi

LOG_FILE="/var/log/emergency-rollback-$(date +%Y%m%d-%H%M%S).log"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a ${LOG_FILE}
}

# Step 1: Stop all customer traffic
log "Stopping edge proxies to halt traffic..."
for node in us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3}; do
    ssh ${node} "sudo systemctl stop zeitwork-edge-proxy" || true
done

# Step 2: Stop all services
log "Stopping all services..."
parallel-ssh -h all-nodes.txt "sudo systemctl stop zeitwork-*" || true

# Step 3: Rollback database if needed
log "Rolling back database..."
cd packages/database
npm run db:rollback || log "Database rollback failed - continuing"

# Step 4: Restore all binaries
log "Restoring service binaries..."
for node in $(cat all-nodes.txt); do
    ssh ${node} << 'ENDSSH' || true
        for service in operator node-agent load-balancer edge-proxy; do
            if [ -f /usr/local/bin/zeitwork-${service}.backup ]; then
                sudo cp /usr/local/bin/zeitwork-${service}.backup \
                        /usr/local/bin/zeitwork-${service}
            fi
        done
ENDSSH
done

# Step 5: Restore configurations
log "Restoring configurations..."
for node in $(cat all-nodes.txt); do
    ssh ${node} << 'ENDSSH' || true
        for config in /etc/zeitwork/*.env.backup; do
            if [ -f "$config" ]; then
                sudo cp "$config" "${config%.backup}"
            fi
        done
ENDSSH
done

# Step 6: Start services in order
log "Starting operators..."
for node in us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3}; do
    ssh ${node} "sudo systemctl start zeitwork-operator" || true
done
sleep 30

log "Starting load balancers..."
for node in us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3}; do
    ssh ${node} "sudo systemctl start zeitwork-load-balancer" || true
done

log "Starting node agents..."
parallel-ssh -h worker-nodes.txt "sudo systemctl start zeitwork-node-agent" || true
sleep 30

log "Starting edge proxies..."
for node in us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3}; do
    ssh ${node} "sudo systemctl start zeitwork-edge-proxy" || true
done

log "Emergency rollback complete. Check service health immediately."
log "Log file: ${LOG_FILE}"

# Step 7: Verify health
./verify-platform-health.sh
```

### Service Isolation Rollback

When only specific services are affected:

```bash
#!/bin/bash
# isolate-and-rollback.sh

SERVICE_TYPE=$1  # operator, node-agent, load-balancer, or edge-proxy
AFFECTED_NODES=$2  # comma-separated list

if [ -z "$SERVICE_TYPE" ] || [ -z "$AFFECTED_NODES" ]; then
    echo "Usage: $0 <service-type> <node1,node2,node3>"
    exit 1
fi

IFS=',' read -ra NODES <<< "$AFFECTED_NODES"

for node in "${NODES[@]}"; do
    echo "Isolating and rolling back ${SERVICE_TYPE} on ${node}..."

    # Remove from rotation if edge proxy or load balancer
    if [[ "$SERVICE_TYPE" == "edge-proxy" ]] || [[ "$SERVICE_TYPE" == "load-balancer" ]]; then
        ./remove-from-dns.sh ${node}
    fi

    # Drain if node agent
    if [[ "$SERVICE_TYPE" == "node-agent" ]]; then
        node_id=$(curl -s http://localhost:8080/api/v1/nodes | \
                  jq -r ".[] | select(.hostname==\"${node}\") | .id")
        curl -X POST "http://localhost:8080/api/v1/nodes/${node_id}/drain"
    fi

    # Perform rollback
    ssh ${node} << ENDSSH
        sudo systemctl stop zeitwork-${SERVICE_TYPE}
        sudo cp /usr/local/bin/zeitwork-${SERVICE_TYPE}.backup \
                /usr/local/bin/zeitwork-${SERVICE_TYPE}
        sudo systemctl start zeitwork-${SERVICE_TYPE}
ENDSSH

    # Verify and re-enable
    sleep 5
    if [[ "$SERVICE_TYPE" == "node-agent" ]] && [ ! -z "${node_id}" ]; then
        curl -X POST "http://localhost:8080/api/v1/nodes/${node_id}/enable"
    fi

    if [[ "$SERVICE_TYPE" == "edge-proxy" ]] || [[ "$SERVICE_TYPE" == "load-balancer" ]]; then
        ./add-to-dns.sh ${node}
    fi
done
```

## Data Recovery

### Configuration Recovery

```bash
#!/bin/bash
# recover-configs.sh

BACKUP_DIR="/var/backups/zeitwork/configs"
RECOVERY_DATE=$1  # Format: YYYY-MM-DD

if [ -z "$RECOVERY_DATE" ]; then
    echo "Usage: $0 <YYYY-MM-DD>"
    exit 1
fi

# Find latest backup before specified date
BACKUP_FILE=$(find ${BACKUP_DIR} -name "config-backup-*.tar.gz" \
              -newermt "${RECOVERY_DATE} 00:00:00" \
              -not -newermt "${RECOVERY_DATE} 23:59:59" | head -1)

if [ -z "$BACKUP_FILE" ]; then
    echo "No backup found for ${RECOVERY_DATE}"
    exit 1
fi

echo "Recovering from: ${BACKUP_FILE}"

# Extract and deploy
tar -xzf ${BACKUP_FILE} -C /tmp/
for node in $(cat all-nodes.txt); do
    if [ -d /tmp/configs/${node} ]; then
        scp /tmp/configs/${node}/*.env ${node}:/tmp/
        ssh ${node} "sudo mv /tmp/*.env /etc/zeitwork/"
    fi
done

rm -rf /tmp/configs
echo "Configuration recovery complete"
```

### Database Point-in-Time Recovery

```bash
#!/bin/bash
# database-pitr.sh

RECOVERY_TIME=$1  # Format: "YYYY-MM-DD HH:MM:SS"

echo "=== Database Point-in-Time Recovery ==="
echo "Target time: ${RECOVERY_TIME}"

# With PlanetScale
pscale database restore-request create zeitwork-production \
    --restore-to "${RECOVERY_TIME}" \
    --branch recovery-branch

echo "Recovery branch created. To switch over:"
echo "1. Update DATABASE_URL in all services to use recovery-branch"
echo "2. Restart all operator services"
echo "3. Verify data integrity"
echo "4. Promote recovery-branch to main if successful"
```

## Post-Rollback Actions

### Verification Checklist

```bash
#!/bin/bash
# post-rollback-verify.sh

echo "=== Post-Rollback Verification ==="

ERRORS=0

# 1. Service Health
echo "Checking service health..."
for service in operator node-agent load-balancer edge-proxy; do
    count=$(systemctl list-units --state=running | grep -c zeitwork-${service})
    expected=$(cat all-nodes.txt | wc -l)
    if [ "$service" == "operator" ] || [ "$service" == "load-balancer" ] || [ "$service" == "edge-proxy" ]; then
        expected=9  # Only on operator nodes
    elif [ "$service" == "node-agent" ]; then
        expected=18  # Only on worker nodes
    fi

    if [ $count -lt $expected ]; then
        echo "✗ ${service}: ${count}/${expected} running"
        ERRORS=$((ERRORS + 1))
    else
        echo "✓ ${service}: ${count}/${expected} running"
    fi
done

# 2. Database Connectivity
echo -e "\nChecking database connectivity..."
if psql "$DATABASE_URL" -c "SELECT 1;" > /dev/null 2>&1; then
    echo "✓ Database connection successful"
else
    echo "✗ Database connection failed"
    ERRORS=$((ERRORS + 1))
fi

# 3. Customer Applications
echo -e "\nChecking customer applications..."
for deployment in $(curl -s http://localhost:8080/api/v1/deployments/active | jq -r '.[].id'); do
    status=$(curl -s http://localhost:8080/api/v1/deployments/${deployment}/health | jq -r '.status')
    if [ "$status" == "healthy" ]; then
        echo "✓ Deployment ${deployment}: healthy"
    else
        echo "✗ Deployment ${deployment}: ${status}"
        ERRORS=$((ERRORS + 1))
    fi
done

# 4. API Endpoints
echo -e "\nChecking API endpoints..."
endpoints=(
    "http://localhost:8080/health"
    "http://localhost:8081/health"
    "http://localhost:8082/health"
    "https://localhost:8083/health"
)

for endpoint in "${endpoints[@]}"; do
    if curl -k -s ${endpoint} | grep -q healthy; then
        echo "✓ ${endpoint}"
    else
        echo "✗ ${endpoint}"
        ERRORS=$((ERRORS + 1))
    fi
done

# Summary
echo -e "\n=== Summary ==="
if [ ${ERRORS} -eq 0 ]; then
    echo "✓ All checks passed - rollback successful"
else
    echo "✗ ${ERRORS} checks failed - investigate immediately"
fi

exit ${ERRORS}
```

### Incident Report Template

After a rollback, document the incident:

```markdown
# Rollback Incident Report

**Date**: [YYYY-MM-DD HH:MM]
**Duration**: [XX minutes]
**Severity**: [Critical/High/Medium/Low]
**Services Affected**: [List services]

## Timeline

- **HH:MM** - Upgrade started
- **HH:MM** - Issue detected: [description]
- **HH:MM** - Rollback decision made
- **HH:MM** - Rollback initiated
- **HH:MM** - Services restored
- **HH:MM** - Full functionality verified

## Root Cause

[Describe what caused the failure]

## Impact

- **Customer Impact**: [None/Minor/Major]
- **Affected Customers**: [Number or list]
- **Data Loss**: [None/Description]
- **SLA Impact**: [Yes/No]

## Resolution

[Steps taken to rollback]

## Lessons Learned

1. [What went well]
2. [What could be improved]
3. [Action items]

## Follow-up Actions

- [ ] Fix the root cause
- [ ] Update rollback procedures
- [ ] Improve monitoring
- [ ] Update documentation
- [ ] Schedule retry

**Report By**: [Name]
**Reviewed By**: [Name]
```

### Communication Template

```bash
#!/bin/bash
# notify-rollback.sh

REASON=$1
STATUS=$2  # in-progress, complete, failed

# Slack notification
curl -X POST $SLACK_WEBHOOK_URL \
    -H 'Content-Type: application/json' \
    -d "{
        \"text\": \"🔄 Platform Rollback ${STATUS}\",
        \"blocks\": [
            {
                \"type\": \"section\",
                \"text\": {
                    \"type\": \"mrkdwn\",
                    \"text\": \"*Status:* ${STATUS}\n*Reason:* ${REASON}\n*Time:* $(date)\"
                }
            }
        ]
    }"

# Email notification
cat << EOF | mail -s "Zeitwork Platform Rollback ${STATUS}" ops-team@zeitwork.com
Platform Rollback Notification

Status: ${STATUS}
Reason: ${REASON}
Time: $(date)

Services affected:
- Check dashboard for details

Customer impact:
- Being assessed

For updates, check #platform-incidents channel
EOF

# Update status page
curl -X POST https://api.statuspage.io/v1/pages/$PAGE_ID/incidents \
    -H "Authorization: OAuth $STATUSPAGE_API_KEY" \
    -d "{
        \"incident\": {
            \"name\": \"Platform upgrade rollback\",
            \"status\": \"investigating\",
            \"impact\": \"minor\",
            \"body\": \"We are rolling back a recent platform upgrade. No customer impact expected.\"
        }
    }"
```

## Best Practices

1. **Always backup before upgrade**: Never upgrade without backup files
2. **Test rollback procedures**: Practice in staging environment
3. **Document rollback triggers**: Clear criteria for when to rollback
4. **Communicate early**: Inform stakeholders as soon as rollback starts
5. **Preserve evidence**: Keep logs and configs for analysis
6. **Conduct post-mortems**: Learn from every rollback
7. **Update runbooks**: Incorporate lessons learned

## Related Documentation

- [Platform Upgrade Guide](./platform-upgrade.md)
- [Operator Upgrade Guide](./operator-upgrade.md)
- [Database Migration Guide](./database-migrations.md)
- [Emergency Response Plan](../operations/emergency-response.md)
- [Incident Management](../operations/incident-management.md)
