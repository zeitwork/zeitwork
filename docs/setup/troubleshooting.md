# Troubleshooting Guide

This guide helps diagnose and resolve common issues with Zeitwork deployments.

## Service Issues

### Operator Service

#### Operator won't start

**Symptoms:**

- `systemctl status zeitwork-operator` shows failed state
- API endpoint not responding

**Diagnosis:**

```bash
# Check logs
sudo journalctl -u zeitwork-operator -n 100

# Common issues to look for:
# - Database connection errors
# - Port already in use
# - Permission denied
```

**Solutions:**

1. **Database connection failed**

```bash
# Test database connection to PlanetScale
psql $DATABASE_URL -c "SELECT 1;"

# If connection fails, check:
# - Network connectivity to PlanetScale endpoint
# - Firewall rules allow outbound connection to port 3306
# - Database credentials are correct
# - Connection string format is correct

# Verify DATABASE_URL format
cat /etc/zeitwork/operator.env | grep DATABASE_URL
# Should be: postgresql://username:password@host.connect.psdb.cloud/zeitwork-production?sslmode=require

sudo systemctl restart zeitwork-operator
```

2. **Port already in use**

```bash
# Check what's using port 8080
sudo lsof -i :8080

# Kill the process or change port in /etc/zeitwork/operator.env
sudo systemctl restart zeitwork-operator
```

3. **Permission denied**

```bash
# Check file permissions
ls -la /var/lib/zeitwork
ls -la /etc/zeitwork

# Fix permissions
sudo chown -R zeitwork:zeitwork /var/lib/zeitwork
sudo chown zeitwork:zeitwork /etc/zeitwork/*.env
sudo chmod 640 /etc/zeitwork/*.env
```

#### Operator API slow

**Symptoms:**

- API calls take > 1 second
- Timeouts on operations

**Diagnosis:**

```bash
# Check database performance in PlanetScale dashboard
# Go to: https://app.planetscale.com/[org]/zeitwork-production/insights

# Check query performance
psql $DATABASE_URL -c "SELECT * FROM pg_stat_statements ORDER BY mean_exec_time DESC LIMIT 10;"

# Check operator resource usage
top -p $(pgrep zeitwork-operator)
```

**Solutions:**

```bash
# Add database indexes if missing
psql $DATABASE_URL << EOF
CREATE INDEX IF NOT EXISTS idx_nodes_state ON nodes(state);
CREATE INDEX IF NOT EXISTS idx_instances_node_id ON instances(node_id);
CREATE INDEX IF NOT EXISTS idx_instances_state ON instances(state);
VACUUM ANALYZE;
EOF

# Increase operator resources
sudo systemctl edit zeitwork-operator
# Add:
# [Service]
# LimitNOFILE=65536
# LimitNPROC=32768

sudo systemctl restart zeitwork-operator
```

### Node Agent

#### Node Agent won't register

**Symptoms:**

- Node doesn't appear in operator's node list
- Node agent logs show registration failures

**Diagnosis:**

```bash
# Check node agent logs
sudo journalctl -u zeitwork-node-agent -n 50

# Test operator connectivity
curl http://operator-host:8080/health

# Check node agent config
cat /etc/zeitwork/node-agent.env
```

**Solutions:**

1. **Network connectivity issue**

```bash
# Test connectivity to all operators
for op in operator-1 operator-2 operator-3; do
    ping -c 3 $op
    curl http://$op:8080/health
done

# Check firewall
sudo iptables -L -n | grep 8080
sudo ufw status

# Fix firewall if needed
sudo ufw allow from operator-subnet to any port 8081
```

2. **Wrong operator URL**

```bash
# Update configuration
sudo vim /etc/zeitwork/node-agent.env
# Set correct OPERATOR_URL with comma-separated values
# OPERATOR_URL=http://op1:8080,http://op2:8080,http://op3:8080

sudo systemctl restart zeitwork-node-agent
```

#### VMs won't start on node

**Symptoms:**

- Instance creation fails
- Firecracker process doesn't start

**Diagnosis:**

```bash
# Check KVM support
ls -l /dev/kvm
lsmod | grep kvm

# Check Firecracker
firecracker --version
which firecracker

# Check permissions
groups zeitwork

# Check disk space
df -h /var/lib/firecracker
```

**Solutions:**

1. **KVM not available**

```bash
# Enable KVM modules
sudo modprobe kvm
sudo modprobe kvm_intel  # or kvm_amd

# Make persistent
echo "kvm" | sudo tee -a /etc/modules
echo "kvm_intel" | sudo tee -a /etc/modules

# Add user to kvm group
sudo usermod -aG kvm zeitwork
```

2. **Firecracker not found**

```bash
# Reinstall Firecracker
FC_VERSION="v1.12.1"
ARCH=$(uname -m)
wget https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-${ARCH}.tgz
tar -xzf firecracker-${FC_VERSION}-${ARCH}.tgz
sudo cp release-${FC_VERSION}-${ARCH}/firecracker-${FC_VERSION}-${ARCH} /usr/bin/firecracker
sudo chmod +x /usr/bin/firecracker
```

3. **Disk space issue**

```bash
# Clean up old VMs
sudo rm -rf /var/lib/firecracker/vms/old-*

# Clean up logs
sudo journalctl --vacuum-time=7d
```

### Load Balancer

#### Load Balancer not routing traffic

**Symptoms:**

- Connections timeout
- No backends available error

**Diagnosis:**

```bash
# Check load balancer status
sudo journalctl -u zeitwork-load-balancer -n 50

# Check health endpoint
curl http://localhost:8084/health

# Check backend discovery
curl http://localhost:8084/backends
```

**Solutions:**

1. **No backends discovered**

```bash
# Check operator connection
curl http://operator:8080/api/v1/instances

# Update configuration
sudo vim /etc/zeitwork/load-balancer.env
# Ensure OPERATOR_URL is correct

sudo systemctl restart zeitwork-load-balancer
```

2. **Backend health checks failing**

```bash
# Test backend directly
curl http://backend-ip:port/health

# Check network connectivity
ping backend-ip

# Review firewall rules
sudo iptables -L -n
```

### Edge Proxy

#### SSL certificate errors

**Symptoms:**

- Browser shows certificate warning for zeitwork.com
- curl returns SSL error

**Diagnosis:**

```bash
# Check certificate
openssl x509 -in /etc/zeitwork/certs/server.crt -text -noout | grep -E "(Subject:|Not After)"

# Test SSL
openssl s_client -connect localhost:443 -servername zeitwork.com < /dev/null

# Check certificate paths
ls -la /etc/zeitwork/certs/
```

**Solutions:**

1. **Certificate expired**

```bash
# Renew certificate (Let's Encrypt)
sudo certbot renew --cert-name zeitwork.com

# Copy new certificate
sudo cp /etc/letsencrypt/live/zeitwork.com/fullchain.pem /etc/zeitwork/certs/server.crt
sudo cp /etc/letsencrypt/live/zeitwork.com/privkey.pem /etc/zeitwork/certs/server.key

# Fix permissions
sudo chmod 644 /etc/zeitwork/certs/server.crt
sudo chmod 600 /etc/zeitwork/certs/server.key
sudo chown zeitwork:zeitwork /etc/zeitwork/certs/*

# Restart edge proxy
sudo systemctl restart zeitwork-edge-proxy
```

2. **Wrong certificate path**

```bash
# Update configuration (see deployments/config/edge-proxy.env)
sudo vim /etc/zeitwork/edge-proxy.env
# Fix SSL_CERT_PATH and SSL_KEY_PATH

sudo systemctl restart zeitwork-edge-proxy
```

#### Rate limiting too aggressive

**Symptoms:**

- Legitimate requests blocked
- 429 Too Many Requests errors

**Diagnosis:**

```bash
# Check current rate limit
grep RATE_LIMIT /etc/zeitwork/edge-proxy.env

# Check logs for rate limit hits
sudo journalctl -u zeitwork-edge-proxy | grep "rate limit"
```

**Solutions:**

```bash
# Increase rate limit
sudo vim /etc/zeitwork/edge-proxy.env
# RATE_LIMIT_RPS=500  # Increase from 100

sudo systemctl restart zeitwork-edge-proxy
```

## Database Issues

### Connection Problems

**Symptoms:**

- Services can't connect to PostgreSQL
- "connection refused" or timeout errors

**Diagnosis:**

```bash
# Test connection from operator node
psql $DATABASE_URL -c "SELECT version();"

# Check network connectivity to PlanetScale
telnet host.connect.psdb.cloud 3306  # PlanetScale uses MySQL port

# Test with curl
curl -v telnet://host.connect.psdb.cloud:3306
```

**Solutions:**

1. **Network/Security Issues**

```bash
# Check firewall allows outbound to PlanetScale
sudo iptables -L -n | grep 3306

# PlanetScale requires SSL - verify it's enabled
psql "$DATABASE_URL" -c "SHOW ssl;"

# Test with SSL explicitly disabled (should fail)
psql "${DATABASE_URL/sslmode=require/sslmode=disable}" -c "SELECT 1;"
```

2. **Connection Pool Exhausted**

```bash
# Check connection count
psql $DATABASE_URL -c "SELECT count(*) FROM pg_stat_activity;"

# PlanetScale handles connection pooling automatically
# Check current connections in PlanetScale dashboard:
# https://app.planetscale.com/[org]/zeitwork-production/settings/connections

# PlanetScale has generous connection limits but you can increase if needed
```

### Performance Issues

**Symptoms:**

- Slow queries
- High database CPU usage

**Diagnosis:**

```bash
# Check slow queries (if pg_stat_statements is enabled)
psql $DATABASE_URL -c "
SELECT query, mean_exec_time, calls
FROM pg_stat_statements
WHERE mean_exec_time > 1000
ORDER BY mean_exec_time DESC
LIMIT 10;"

# Check table bloat
psql $DATABASE_URL -c "
SELECT schemaname, tablename,
       pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;"
```

**Solutions:**

```bash
# Run VACUUM and ANALYZE
psql $DATABASE_URL -c "VACUUM ANALYZE;"

# Add missing indexes
psql $DATABASE_URL << EOF
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_instances_created_at ON instances(created_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_deployments_name ON deployments(name);
EOF

# For persistent issues, consider:
# - Upgrading database instance size
# - Enabling read replicas for read-heavy workloads
# - Implementing caching layer (Redis)
```

## Network Issues

### High latency between services

**Symptoms:**

- Slow cross-region operations
- Timeouts on internal calls

**Diagnosis:**

```bash
# Test latency between nodes
for node in node1 node2 node3; do
    ping -c 10 $node | tail -1
done

# Check DNS resolution
nslookup api.zeitwork.com
dig us-east.zeitwork.com

# Trace route to identify bottlenecks
traceroute api.zeitwork.com
```

**Solutions:**

- Ensure services are in same availability zone where possible
- Use regional endpoints instead of cross-region calls
- Implement caching for frequently accessed data
- Consider CDN for static assets

## Performance Issues

### Slow VM boot times

**Symptoms:**

- VMs take > 200ms to start
- Instance creation timeouts

**Diagnosis:**

```bash
# Check node resources
ssh worker-node "top -bn1 | head -20"
ssh worker-node "iostat -x 1 5"

# Check Firecracker logs
ssh worker-node "sudo journalctl -u zeitwork-node-agent | grep -i firecracker | tail -20"
```

**Solutions:**

```bash
# Pre-cache kernel in memory
ssh worker-node "sudo dd if=/var/lib/firecracker/kernels/vmlinux.bin of=/dev/null bs=1M"

# Use faster storage for VMs
# Move to NVMe if using SATA
ssh worker-node "sudo mv /var/lib/firecracker /mnt/nvme/"
ssh worker-node "sudo ln -s /mnt/nvme/firecracker /var/lib/firecracker"

# Reduce VM memory if over-provisioned
# Edit deployment to use appropriate resources
```

### High memory usage

**Symptoms:**

- Services using excessive memory
- Out of memory errors

**Diagnosis:**

```bash
# Check memory usage
free -h
ps aux --sort=-%mem | head -20

# Check for memory leaks
sudo pmap $(pgrep zeitwork-operator) | tail -1
```

**Solutions:**

```bash
# Restart service to clear memory
sudo systemctl restart zeitwork-operator

# Adjust memory limits
sudo systemctl edit zeitwork-operator
# Add:
# [Service]
# MemoryMax=4G
# MemoryHigh=3G

# Enable swap as emergency buffer
sudo fallocate -l 4G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
```

## Recovery Procedures

### Service recovery after crash

```bash
#!/bin/bash
# recover-service.sh

SERVICE=$1

# Stop service
sudo systemctl stop $SERVICE

# Clear any locks or temp files
sudo rm -f /var/lib/zeitwork/.lock
sudo rm -f /tmp/zeitwork-*

# Check and fix permissions
sudo chown -R zeitwork:zeitwork /var/lib/zeitwork
sudo chown zeitwork:zeitwork /etc/zeitwork/*.env

# Start service with increased logging
sudo systemctl edit $SERVICE --runtime
# Add: Environment="LOG_LEVEL=debug"

sudo systemctl start $SERVICE

# Monitor for 1 minute
sudo journalctl -u $SERVICE -f
```

### Emergency database failover

PlanetScale handles failover automatically with zero downtime. If you need to switch branches or handle an emergency:

**PlanetScale Automatic Handling:**

```bash
# PlanetScale provides automatic failover with no action required
# Database remains available during failures

# If you need to switch to a different branch:
# 1. Create a new branch in PlanetScale dashboard
# 2. Update connection string
NEW_DATABASE_URL="postgresql://username:password@host.connect.psdb.cloud/zeitwork-production-backup?sslmode=require"
```

**Update all services if connection string changes:**

```bash
for node in $(cat operator-nodes.txt); do
    ssh $node "sudo sed -i 's/old-endpoint/new-endpoint/' /etc/zeitwork/operator.env"
    ssh $node "sudo systemctl restart zeitwork-operator"
done
```

### Full cluster restart

```bash
#!/bin/bash
# cluster-restart.sh

echo "Starting cluster restart..."

# 1. Stop all services
parallel-ssh -h all-nodes.txt "sudo systemctl stop zeitwork-*"

# 2. Verify database connectivity
psql $DATABASE_URL -c "SELECT 1;" || exit 1

# 3. Start operators
for op in $(cat operator-nodes.txt); do
    ssh $op "sudo systemctl start zeitwork-operator"
done
sleep 10

# 4. Start load balancers and edge proxies
for op in $(cat operator-nodes.txt); do
    ssh $op "sudo systemctl start zeitwork-load-balancer zeitwork-edge-proxy"
done
sleep 5

# 5. Start node agents
parallel-ssh -h worker-nodes.txt "sudo systemctl start zeitwork-node-agent"

# 6. Verify
for node in $(cat all-nodes.txt); do
    echo "=== $node ==="
    ssh $node "systemctl status zeitwork-* --no-pager | grep Active:"
done
```

## Debug Commands

### Useful debugging commands

```bash
# Check all service statuses at once
for s in operator load-balancer edge-proxy node-agent; do
    systemctl status zeitwork-$s --no-pager | grep -E "(●|Active:)"
done

# Get all error logs from last hour
journalctl -u 'zeitwork-*' --since '1 hour ago' | grep -E "(ERROR|FATAL|panic)"

# Check port bindings
sudo ss -tlnp | grep zeitwork

# Monitor real-time logs from all services
sudo journalctl -u 'zeitwork-*' -f

# Check resource usage of all Zeitwork processes
ps aux | grep zeitwork | grep -v grep

# Test internal API endpoints
for port in 8080 8081 8082 8083; do
    echo "Port $port:"
    curl -s http://localhost:$port/health | jq . || echo "Failed"
done

# Database connection test
psql $DATABASE_URL -c "SELECT version();"

# KVM functionality test
sudo kvm-ok

# Firecracker test
sudo firecracker --version && echo "Firecracker OK"

# Network connectivity matrix
for src in node1 node2 node3; do
    for dst in node1 node2 node3; do
        [ "$src" != "$dst" ] && echo "$src -> $dst:" && ssh $src "ping -c 1 -W 1 $dst > /dev/null 2>&1 && echo 'OK' || echo 'FAIL'"
    done
done

# Check DNS resolution for zeitwork.com and zeitwork.app
for domain in edge.zeitwork.com api.zeitwork.com us-east.zeitwork.com; do
    echo "$domain: $(dig +short $domain | head -1)"
done

# Check wildcard resolution for customer deployments
for deployment in dokedu-a7x9k2m-dokedu.zeitwork.app webapp-b8y3n5p-acme.zeitwork.app; do
    echo "$deployment: $(dig +short $deployment | head -1)"
done
```

## Getting Help

If you can't resolve an issue:

1. Collect diagnostic information:

```bash
# Generate diagnostic bundle
tar czf diagnostics-$(date +%Y%m%d-%H%M%S).tar.gz \
    /var/log/zeitwork/ \
    /etc/zeitwork/*.env \
    <(journalctl -u 'zeitwork-*' --since '24 hours ago')
```

2. Check configuration templates in repository:

- [deployments/config/](../../deployments/config/) - Configuration templates
- [deployments/systemd/](../../deployments/systemd/) - Service files

3. Contact support with:

- Diagnostic bundle
- Error logs
- Steps to reproduce
- Expected vs actual behavior

Support channels:

- GitHub Issues: https://github.com/zeitwork/zeitwork/issues
- Email: support@zeitwork.com
- Documentation: https://docs.zeitwork.com

---

_Last updated: December 2024_
