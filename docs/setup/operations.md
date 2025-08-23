# Operations Guide

This guide covers day-to-day operations, monitoring, maintenance, and scaling of your Zeitwork production deployment.

## Daily Operations

### Health Checks

Perform these checks daily or automate with monitoring:

```bash
# Check all services status
for node in $(cat operator-nodes.txt); do
    echo "=== $node ==="
    ssh $node "systemctl status zeitwork-* --no-pager"
done

for node in $(cat worker-nodes.txt); do
    echo "=== $node ==="
    ssh $node "systemctl status zeitwork-node-agent --no-pager"
done

# Check API health
for region in us-east eu-west ap-south; do
    echo "Region: $region"
    curl -s https://$region.zeitwork.com/health | jq .
done

# Check database connectivity (from operator nodes)
for node in $(cat operator-nodes.txt | head -3); do
    echo "=== $node ==="
    ssh $node "psql \$DATABASE_URL -c 'SELECT 1;' > /dev/null && echo 'DB OK' || echo 'DB FAIL'"
done
```

### Log Monitoring

Key logs to monitor:

```bash
# Aggregate logs from all operators
parallel-ssh -h operator-nodes.txt \
    "journalctl -u zeitwork-operator --since '1 hour ago' | grep ERROR"

# Check for node agent issues
parallel-ssh -h worker-nodes.txt \
    "journalctl -u zeitwork-node-agent --since '1 hour ago' | grep -E 'ERROR|WARN'"

# Monitor Edge Proxy rate limiting
parallel-ssh -h operator-nodes.txt \
    "journalctl -u zeitwork-edge-proxy --since '1 hour ago' | grep 'rate limit'"
```

## Monitoring Setup

### Prometheus Configuration

Deploy Prometheus to collect metrics. See [deployments/config/](../../deployments/config/) for service configurations.

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: "zeitwork-operator"
    static_configs:
      - targets:
          - "us-east-op-1:8080"
          - "us-east-op-2:8080"
          - "us-east-op-3:8080"
          - "eu-west-op-1:8080"
          - "eu-west-op-2:8080"
          - "eu-west-op-3:8080"
          - "ap-south-op-1:8080"
          - "ap-south-op-2:8080"
          - "ap-south-op-3:8080"

  - job_name: "zeitwork-node-agent"
    static_configs:
      - targets:
          - "us-east-worker-1:8081"
          - "us-east-worker-2:8081"
          # ... all worker nodes

  - job_name: "zeitwork-load-balancer"
    static_configs:
      - targets:
          - "us-east-op-1:8084"
          - "us-east-op-2:8084"
          # ... all operator nodes
```

### Key Metrics to Monitor

| Metric                  | Alert Threshold  | Action                        |
| ----------------------- | ---------------- | ----------------------------- |
| Service Uptime          | < 99%            | Check logs, restart service   |
| API Latency p99         | > 1s             | Scale operators, check DB     |
| Node Agent Health       | Unhealthy > 5min | SSH to node, check logs       |
| VM Boot Time            | > 200ms          | Check node resources          |
| Database Latency        | > 100ms          | Check DB metrics in console   |
| Deployment Build Time   | > 5min           | Check build logs, optimize    |
| Customer App Response   | > 500ms p95      | Check app instances           |
| Deployment Success Rate | < 95%            | Review failed deployments     |
| Disk Usage              | > 80%            | Clean logs, old images        |
| Memory Usage            | > 90%            | Add nodes or restart services |
| Error Rate              | > 1%             | Check logs, investigate       |

### Alerting Rules

```yaml
# alerts.yml
groups:
  - name: zeitwork
    rules:
      - alert: ServiceDown
        expr: up{job=~"zeitwork-.*"} == 0
        for: 5m
        annotations:
          summary: "Service {{ $labels.job }} is down on {{ $labels.instance }}"

      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.01
        for: 10m
        annotations:
          summary: "High error rate on {{ $labels.instance }}"

      - alert: DatabaseConnectionFailure
        expr: postgres_up == 0
        for: 1m
        annotations:
          summary: "Cannot connect to PostgreSQL database"
```

## Maintenance Tasks

### Database Maintenance

With PlanetScale, database maintenance is fully managed:

- **Backups**: Automated daily backups with point-in-time recovery
- **Updates**: Zero-downtime updates handled by PlanetScale
- **Monitoring**: Built-in dashboard at app.planetscale.com
- **Scaling**: Automatic scaling based on usage
- **Branching**: Database branches for safe schema changes

For application-level maintenance:

```bash
# Analyze query performance
psql $DATABASE_URL -c "
SELECT query, mean_exec_time, calls
FROM pg_stat_statements
ORDER BY mean_exec_time DESC
LIMIT 10;"

# Check table sizes
psql $DATABASE_URL -c "
SELECT schemaname, tablename,
       pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;"
```

### Log Rotation

Configure logrotate for Zeitwork logs:

```bash
# /etc/logrotate.d/zeitwork
/var/log/zeitwork/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0640 zeitwork zeitwork
    sharedscripts
    postrotate
        systemctl reload rsyslog
    endscript
}
```

### Certificate Renewal

For Let's Encrypt certificates:

```bash
#!/bin/bash
# renew-certs.sh

# Renew certificates
certbot renew --quiet

# Distribute to all operator nodes
for node in $(cat operator-nodes.txt); do
    scp /etc/letsencrypt/live/zeitwork.com/fullchain.pem \
        $node:/etc/zeitwork/certs/server.crt
    scp /etc/letsencrypt/live/zeitwork.com/privkey.pem \
        $node:/etc/zeitwork/certs/server.key

    # Restart Edge Proxy to load new certs
    ssh $node "sudo systemctl restart zeitwork-edge-proxy"
done
```

## Scaling Operations

### Adding Worker Nodes

To add a new worker node to a region:

```bash
# 1. Provision new node
# 2. Install dependencies and Firecracker (see deployment guide)
# 3. Deploy Node Agent

NEW_NODE="us-east-worker-7"

# Copy binaries and configuration from existing worker
scp us-east-worker-1:/usr/local/bin/zeitwork-node-agent \
    $NEW_NODE:/tmp/

ssh $NEW_NODE << 'EOF'
sudo mv /tmp/zeitwork-node-agent /usr/local/bin/
sudo chmod +x /usr/local/bin/zeitwork-node-agent

# Copy configuration template
sudo cp /tmp/node-agent.env /etc/zeitwork/

# Start service
sudo systemctl enable zeitwork-node-agent
sudo systemctl start zeitwork-node-agent
EOF

# Verify registration
curl http://us-east-op-1:8080/api/v1/nodes | jq '.[] | select(.hostname=="'$NEW_NODE'")'
```

### Adding a New Region

To add a fourth region:

```bash
# 1. Provision infrastructure (3 operators, 6 workers)
# 2. Ensure database connectivity from new region
# 3. Deploy operator services on operator nodes
# 4. Deploy node agents on worker nodes
# 5. Update DNS records for new region

# Add region to database
psql $DATABASE_URL << EOF
INSERT INTO regions (id, name, code, country)
VALUES (gen_random_uuid(), 'US West', 'us-west', 'United States');
EOF
```

### Removing a Node

To safely remove a node:

```bash
# 1. Drain the node (move VMs to other nodes)
NODE_ID="node-uuid-here"
curl -X POST http://operator:8080/api/v1/nodes/$NODE_ID/drain

# 2. Wait for VMs to migrate
while [ $(curl -s http://operator:8080/api/v1/nodes/$NODE_ID | jq '.instances') -gt 0 ]; do
    echo "Waiting for VMs to migrate..."
    sleep 10
done

# 3. Stop the node agent
ssh $NODE "sudo systemctl stop zeitwork-node-agent"

# 4. Remove from operator
curl -X DELETE http://operator:8080/api/v1/nodes/$NODE_ID
```

## Customer Deployment Operations

### Managing Customer Projects

```bash
# Register a new customer project
curl -X POST http://localhost:8080/api/v1/projects \
  -H "Content-Type: application/json" \
  -d '{
    "name": "webapp",
    "org": "acme",
    "github_repo": "acme-corp/webapp",
    "branch": "main",
    "auto_deploy": true,
    "resources": {
      "vcpu": 2,
      "memory": 1024
    }
  }'

# List all customer projects
curl http://localhost:8080/api/v1/projects

# Get project details
curl http://localhost:8080/api/v1/projects/acme/webapp

# Update project settings
curl -X PATCH http://localhost:8080/api/v1/projects/acme/webapp \
  -H "Content-Type: application/json" \
  -d '{
    "auto_deploy": false,
    "resources": {
      "vcpu": 4,
      "memory": 2048
    }
  }'
```

### Managing Deployments

```bash
# List all deployments for a project
curl http://localhost:8080/api/v1/projects/dokedu/dokedu/deployments

# Get current production deployment
curl http://localhost:8080/api/v1/projects/dokedu/dokedu/deployments/current

# Manually trigger deployment from specific commit
curl -X POST http://localhost:8080/api/v1/deployments \
  -H "Content-Type: application/json" \
  -d '{
    "project": "dokedu",
    "org": "dokedu",
    "image": "ghcr.io/dokedu/app:sha-abc123",
    "commit_sha": "abc123def456",
    "commit_message": "Fix: Updated API endpoints",
    "instances_per_region": 3
  }'

# Rollback to previous deployment
curl -X POST http://localhost:8080/api/v1/projects/dokedu/dokedu/rollback

# Promote staging to production
curl -X POST http://localhost:8080/api/v1/projects/dokedu/dokedu/promote \
  -H "Content-Type: application/json" \
  -d '{"from": "staging", "to": "production"}'
```

### Deployment URLs and Routing

Each deployment automatically gets a unique URL:

```bash
# Format: <project>-<nanoid>-<org>.zeitwork.app

# Example deployments for customer "dokedu":
dokedu-a7x9k2m-dokedu.zeitwork.app  # Current production
dokedu-b8y3n5p-dokedu.zeitwork.app  # Previous version
dokedu-c9z4m6q-dokedu.zeitwork.app  # Staging branch

# Check which deployment is serving production traffic
curl http://localhost:8080/api/v1/projects/dokedu/dokedu/routing

# The customer's domain (app.dokedu.org) always points to the current production
```

### GitHub Integration

```bash
# Configure GitHub webhook for automatic deployments
curl -X POST http://localhost:8080/api/v1/github/webhooks \
  -H "Content-Type: application/json" \
  -d '{
    "org": "dokedu",
    "repo": "app",
    "events": ["push", "pull_request"],
    "secret": "webhook-secret-key"
  }'

# View webhook deliveries
curl http://localhost:8080/api/v1/github/webhooks/dokedu/app/deliveries

# Manually trigger build from GitHub
curl -X POST http://localhost:8080/api/v1/github/trigger \
  -H "Content-Type: application/json" \
  -d '{
    "org": "dokedu",
    "repo": "app",
    "ref": "refs/heads/main"
  }'
```

### Deployment Process

When a GitHub push event is received:

1. **Image Building**:

   ```bash
   # Operator pulls the container image
   docker pull ghcr.io/dokedu/app:sha-abc123

   # Optimize for Firecracker microVM
   # - Minimize image size
   # - Remove unnecessary packages
   # - Create rootfs for Firecracker
   ```

2. **Global Deployment**:

   ```bash
   # Deploy to all regions simultaneously
   for region in us-east eu-west ap-south; do
     # Create 3 instances per region (9 total)
     deploy_to_region $region 3
   done
   ```

3. **URL Assignment**:

   ```bash
   # Generate unique deployment URL
   DEPLOYMENT_URL="dokedu-$(generate_nanoid)-dokedu.zeitwork.app"

   # Update routing table
   update_routing "dokedu" "$DEPLOYMENT_URL" "production"
   ```

4. **Traffic Switching**:
   ```bash
   # Customer's app.dokedu.org (CNAME to edge.zeitwork.com)
   # automatically routes to new deployment
   ```

### Zero-Downtime Deployments

Deployments are zero-downtime by default:

```bash
# New deployment process
1. Build and deploy new version (dokedu-newid-dokedu.zeitwork.app)
2. Run health checks on new instances
3. Update routing table to point to new deployment
4. Keep old deployment running for 10 minutes (rollback window)
5. Terminate old deployment instances

# The customer's domain never experiences downtime
# app.dokedu.org → edge.zeitwork.com → new deployment
```

## Customer Deployment Monitoring

### Monitoring Customer Applications

```bash
# Check health of all deployments for a customer
curl http://localhost:8080/api/v1/projects/dokedu/dokedu/health

# Get metrics for specific deployment
curl http://localhost:8080/api/v1/deployments/dokedu-a7x9k2m-dokedu/metrics

# View deployment logs
curl http://localhost:8080/api/v1/deployments/dokedu-a7x9k2m-dokedu/logs

# Check instance distribution
curl http://localhost:8080/api/v1/deployments/dokedu-a7x9k2m-dokedu/instances
```

### Deployment Analytics

```bash
# Deployment frequency by customer
curl http://localhost:8080/api/v1/analytics/deployments/frequency

# Average deployment time
curl http://localhost:8080/api/v1/analytics/deployments/duration

# Success rate by project
curl http://localhost:8080/api/v1/analytics/deployments/success-rate

# Resource usage by customer
curl http://localhost:8080/api/v1/analytics/resources/by-org
```

### Customer Domain Monitoring

```bash
#!/bin/bash
# monitor-customer-domains.sh

# Check that customer domains resolve correctly
CUSTOMERS="dokedu.org acme.com webapp.io"

for customer in $CUSTOMERS; do
    echo "Checking app.$customer..."

    # Check CNAME
    cname=$(dig +short app.$customer CNAME)
    if [ "$cname" = "edge.zeitwork.com." ]; then
        echo "  ✓ CNAME correct"
    else
        echo "  ✗ CNAME incorrect: $cname"
    fi

    # Check response
    response=$(curl -s -o /dev/null -w "%{http_code}" https://app.$customer)
    if [ "$response" = "200" ]; then
        echo "  ✓ Site responding"
    else
        echo "  ✗ HTTP $response"
    fi
done
```

## Troubleshooting Operations

### Service Issues

```bash
# Service won't start
sudo journalctl -u zeitwork-operator -n 100 --no-pager
sudo systemctl status zeitwork-operator

# High memory usage
sudo systemctl restart zeitwork-operator
# or increase memory limits in systemd service file

# Connection refused
# Check firewall rules
sudo iptables -L -n
sudo ufw status
```

### Performance Issues

```bash
# Slow API responses
# Check database performance in PlanetScale dashboard
# Go to: https://app.planetscale.com/[org]/[database]/insights

# Check node resources
for node in $(cat worker-nodes.txt); do
    ssh $node "top -bn1 | head -5"
done

# Check network latency
for region in us-east eu-west ap-south; do
    ping -c 10 ${region}-op-1 | tail -1
done
```

### VM Issues

```bash
# VM won't start
# Check Firecracker logs
ssh worker-node "sudo journalctl -u zeitwork-node-agent | grep firecracker"

# Check KVM
ssh worker-node "ls -l /dev/kvm"

# Check available resources
ssh worker-node "free -h; df -h /var/lib/firecracker"
```

## Disaster Recovery

### Region Failure

If an entire region fails:

```bash
# 1. Update DNS to remove failed region
# Remove A records for failed region at zeitwork.com

# 2. Scale up other regions to handle load
for region in surviving-regions; do
    # Add more worker nodes
    ./scale-workers.sh $region 3  # Add 3 workers
done

# 3. When region recovers:
# - Redeploy services
# - Re-add to DNS
# - Rebalance load
```

### Database Issues

PlanetScale handles database failures automatically:

1. **Automatic Failover**: Built-in high availability with automatic failover
2. **Point-in-time Recovery**: Restore to any point in the last 30 days
3. **Global Replication**: Data replicated across regions automatically

If you need to switch to a different database branch or endpoint:

```bash
# Update all operators with new PlanetScale endpoint
NEW_DB_ENDPOINT="host.connect.psdb.cloud"

for node in $(cat operator-nodes.txt); do
    ssh $node "sudo sed -i 's/old-endpoint/new-endpoint/' /etc/zeitwork/operator.env"
    ssh $node "sudo systemctl restart zeitwork-operator"
done
```

## Security Operations

### Security Updates

```bash
# Apply security updates monthly
for node in $(cat all-nodes.txt); do
    echo "Updating $node..."
    ssh $node "sudo apt-get update && sudo apt-get upgrade -y"
    # Rolling restart if kernel was updated
done
```

### Audit Logs

```bash
# Collect audit logs
for node in $(cat all-nodes.txt); do
    ssh $node "sudo grep 'Failed password' /var/log/auth.log | tail -20"
done

# Check for unauthorized API access
grep "401\|403" /var/log/zeitwork/edge-proxy.log
```

### Rotate Secrets

```bash
# Rotate database password
# 1. Update password in managed database console
# 2. Update all services
for node in $(cat operator-nodes.txt); do
    ssh $node "sudo vim /etc/zeitwork/operator.env"
    # Update DATABASE_URL with new password
    ssh $node "sudo systemctl restart zeitwork-operator"
done
```

## Performance Tuning

### Network Tuning

```bash
# Optimize network for high throughput
cat << EOF | sudo tee /etc/sysctl.d/network-tuning.conf
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.ipv4.tcp_rmem = 4096 87380 134217728
net.ipv4.tcp_wmem = 4096 65536 134217728
net.core.netdev_max_backlog = 5000
net.ipv4.tcp_congestion_control = bbr
net.core.default_qdisc = fq
EOF

sudo sysctl -p /etc/sysctl.d/network-tuning.conf
```

## Reporting

### Generate Monthly Report

```bash
#!/bin/bash
# monthly-report.sh

REPORT_DATE=$(date +%Y-%m)
REPORT_FILE="zeitwork-report-$REPORT_DATE.txt"

cat > $REPORT_FILE << EOF
Zeitwork Operations Report - $REPORT_DATE
==========================================

=== System Uptime ===
EOF

for service in operator load-balancer edge-proxy node-agent; do
    echo "zeitwork-$service:" >> $REPORT_FILE
    for node in $(cat all-nodes.txt); do
        ssh $node "systemctl status zeitwork-$service 2>/dev/null | grep 'Active:' | head -1" >> $REPORT_FILE 2>/dev/null
    done
done

echo "" >> $REPORT_FILE
echo "=== Node Count ===" >> $REPORT_FILE
curl -s http://localhost:8080/api/v1/nodes | jq '. | length' >> $REPORT_FILE

echo "" >> $REPORT_FILE
echo "=== Customer Projects ===" >> $REPORT_FILE
curl -s http://localhost:8080/api/v1/projects | jq '. | length' >> $REPORT_FILE

echo "" >> $REPORT_FILE
echo "=== Active Deployments ===" >> $REPORT_FILE
curl -s http://localhost:8080/api/v1/deployments | jq '. | length' >> $REPORT_FILE

echo "" >> $REPORT_FILE
echo "=== Deployments by Customer ===" >> $REPORT_FILE
curl -s http://localhost:8080/api/v1/deployments | jq 'group_by(.org) | map({org: .[0].org, count: length})' >> $REPORT_FILE

echo "Report generated: $REPORT_FILE"
```

---

_Last updated: December 2024_
