# Edge Proxy Upgrade Guide

This guide covers upgrading the Zeitwork Edge Proxy service that handles SSL/TLS termination and HTTP routing.

## Overview

The Edge Proxy is a stateless HTTP/HTTPS proxy running on operator nodes. It handles SSL termination, rate limiting, and routes requests to node agents. Each operator node runs an edge proxy instance (3 per region, 9 total).

## Prerequisites

- [ ] SSL certificates valid and not expiring soon
- [ ] New version tested with current SSL configuration
- [ ] Rate limiting rules documented
- [ ] Customer domain CNAME records verified

## Upgrade Process

### Rolling Upgrade

```bash
#!/bin/bash
# upgrade-edge-proxies.sh

VERSION="v2.0.0"
OPERATOR_NODES=(
    "us-east-op-1" "us-east-op-2" "us-east-op-3"
    "eu-west-op-1" "eu-west-op-2" "eu-west-op-3"
    "ap-south-op-1" "ap-south-op-2" "ap-south-op-3"
)

for node in "${OPERATOR_NODES[@]}"; do
    echo "Upgrading edge proxy on $node..."

    # Copy new binary
    scp "build/zeitwork-edge-proxy" "${node}:/tmp/"

    # Upgrade
    ssh "$node" << 'ENDSSH'
        # Backup current binary
        sudo cp /usr/local/bin/zeitwork-edge-proxy \
                /usr/local/bin/zeitwork-edge-proxy.backup

        # Install new binary
        sudo mv /tmp/zeitwork-edge-proxy /usr/local/bin/
        sudo chmod +x /usr/local/bin/zeitwork-edge-proxy
        sudo chown zeitwork:zeitwork /usr/local/bin/zeitwork-edge-proxy

        # Graceful restart (waits for existing connections)
        sudo systemctl reload zeitwork-edge-proxy
        sleep 5
        sudo systemctl restart zeitwork-edge-proxy
ENDSSH

    # Verify health
    sleep 5
    if curl -k -s "https://${node}:8083/health" | grep -q "healthy"; then
        echo "✓ $node upgraded successfully"
    else
        echo "✗ $node upgrade failed, rolling back..."
        ssh "$node" "sudo cp /usr/local/bin/zeitwork-edge-proxy.backup \
                            /usr/local/bin/zeitwork-edge-proxy && \
                     sudo systemctl restart zeitwork-edge-proxy"
        exit 1
    fi

    # Test SSL termination
    if curl -k -s "https://${node}:443" > /dev/null; then
        echo "✓ SSL termination working"
    else
        echo "✗ SSL termination failed"
        exit 1
    fi

    # Wait before next node
    sleep 30
done

echo "All edge proxies upgraded successfully!"
```

### Graceful Upgrade with Connection Draining

```bash
#!/bin/bash
# graceful-edge-proxy-upgrade.sh

upgrade_with_drain() {
    local node=$1

    # Enable connection draining
    ssh "$node" "echo 'DRAIN_MODE=true' | sudo tee -a /etc/zeitwork/edge-proxy.env"
    sudo systemctl reload zeitwork-edge-proxy

    # Wait for connections to drain (max 60s)
    echo "Draining connections on $node..."
    for i in {1..60}; do
        conn_count=$(ssh "$node" "ss -t | grep -c ':443' || echo 0")
        if [ "$conn_count" -eq 0 ]; then
            break
        fi
        echo "Waiting... $conn_count connections remaining"
        sleep 1
    done

    # Perform upgrade
    ssh "$node" << 'ENDSSH'
        sudo systemctl stop zeitwork-edge-proxy
        sudo cp /usr/local/bin/zeitwork-edge-proxy.backup \
                /usr/local/bin/zeitwork-edge-proxy.backup.old
        sudo mv /tmp/zeitwork-edge-proxy /usr/local/bin/
        sudo chmod +x /usr/local/bin/zeitwork-edge-proxy

        # Remove drain mode
        sudo sed -i '/DRAIN_MODE/d' /etc/zeitwork/edge-proxy.env

        sudo systemctl start zeitwork-edge-proxy
ENDSSH
}

# Upgrade each node with connection draining
for node in "${OPERATOR_NODES[@]}"; do
    upgrade_with_drain "$node"
done
```

## SSL Certificate Management

### Verify Certificates Before Upgrade

```bash
# Check certificate expiration
for node in us-east-op-{1..3}; do
    echo -n "$node cert expires: "
    ssh "$node" "openssl x509 -in /etc/zeitwork/certs/server.crt -noout -enddate"
done

# Test SSL configuration
for node in us-east-op-{1..3}; do
    echo "Testing SSL on $node..."
    openssl s_client -connect ${node}:443 -servername zeitwork.com < /dev/null
done
```

### Update Certificates During Upgrade

```bash
# If certificates need updating
for node in "${OPERATOR_NODES[@]}"; do
    # Copy new certificates
    scp new-cert.crt ${node}:/tmp/
    scp new-cert.key ${node}:/tmp/

    ssh "$node" << 'ENDSSH'
        # Backup old certificates
        sudo cp /etc/zeitwork/certs/server.crt /etc/zeitwork/certs/server.crt.backup
        sudo cp /etc/zeitwork/certs/server.key /etc/zeitwork/certs/server.key.backup

        # Install new certificates
        sudo mv /tmp/new-cert.crt /etc/zeitwork/certs/server.crt
        sudo mv /tmp/new-cert.key /etc/zeitwork/certs/server.key
        sudo chown zeitwork:zeitwork /etc/zeitwork/certs/*
        sudo chmod 600 /etc/zeitwork/certs/server.key

        # Reload to pick up new certificates
        sudo systemctl reload zeitwork-edge-proxy
ENDSSH
done
```

## Configuration Updates

### Rate Limiting Configuration

```bash
# Update rate limiting rules
sudo vim /etc/zeitwork/edge-proxy.env

# Example configuration:
RATE_LIMIT_ENABLED=true
RATE_LIMIT_REQUESTS_PER_MINUTE=100
RATE_LIMIT_BURST=20
RATE_LIMIT_BY_IP=true

# Apply changes
sudo systemctl reload zeitwork-edge-proxy
```

### Security Headers

```bash
# Configure security headers
cat << EOF | sudo tee -a /etc/zeitwork/edge-proxy.env
SECURITY_HEADERS_ENABLED=true
HSTS_MAX_AGE=31536000
CSP_POLICY="default-src 'self'"
X_FRAME_OPTIONS=DENY
EOF

sudo systemctl restart zeitwork-edge-proxy
```

## Verification

### Health Checks

```bash
#!/bin/bash
# verify-edge-proxies.sh

echo "=== Edge Proxy Health ==="
for node in us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3}; do
    printf "%-20s: " "$node"
    if curl -k -s "https://${node}:8083/health" | grep -q "healthy"; then
        echo "✓ Healthy"
    else
        echo "✗ Unhealthy"
    fi
done

echo -e "\n=== SSL Certificate Status ==="
for node in us-east-op-{1..3} eu-west-op-{1..3} ap-south-op-{1..3}; do
    printf "%-20s: " "$node"
    echo | openssl s_client -connect ${node}:443 -servername zeitwork.com 2>/dev/null | \
        openssl x509 -noout -dates | grep notAfter | cut -d= -f2
done

echo -e "\n=== Rate Limiting Status ==="
for node in us-east-op-{1..3}; do
    echo "$node:"
    curl -s "http://${node}:8083/api/v1/rate-limit/status" | jq '.'
done
```

### Customer Domain Testing

```bash
# Test customer domains through edge proxy
CUSTOMERS=("app.dokedu.org" "app.acme.com" "webapp.io")

for domain in "${CUSTOMERS[@]}"; do
    echo "Testing $domain..."

    # DNS resolution
    dig +short $domain

    # HTTPS connectivity
    curl -I -s https://$domain | head -n 1

    # Response time
    curl -w "Response time: %{time_total}s\n" -o /dev/null -s https://$domain
done
```

## Rollback

### Quick Rollback

```bash
#!/bin/bash
# rollback-edge-proxy.sh

NODE=$1
ssh "$NODE" << 'ENDSSH'
    # Restore binary
    sudo cp /usr/local/bin/zeitwork-edge-proxy.backup \
            /usr/local/bin/zeitwork-edge-proxy

    # Restore certificates if needed
    if [ -f /etc/zeitwork/certs/server.crt.backup ]; then
        sudo cp /etc/zeitwork/certs/server.crt.backup /etc/zeitwork/certs/server.crt
        sudo cp /etc/zeitwork/certs/server.key.backup /etc/zeitwork/certs/server.key
    fi

    # Restart service
    sudo systemctl restart zeitwork-edge-proxy
ENDSSH

# Verify
curl -k -s "https://${NODE}:8083/health"
```

## Monitoring During Upgrade

```bash
# Real-time monitoring
watch -n 2 'for node in us-east-op-{1..3}; do
    echo -n "$node: "
    curl -s http://$node:8083/metrics | grep -E "requests_total|errors_total|latency_p99"
done'

# Customer impact monitoring
while true; do
    for domain in app.dokedu.org app.acme.com; do
        response_time=$(curl -w "%{time_total}" -o /dev/null -s https://$domain)
        echo "$(date): $domain response time: ${response_time}s"
    done
    sleep 5
done
```

## Troubleshooting

### SSL Issues

```bash
# Debug SSL handshake
openssl s_client -connect node:443 -debug -msg

# Check certificate chain
openssl s_client -connect node:443 -showcerts

# Verify certificate permissions
ssh $node "ls -la /etc/zeitwork/certs/"
```

### Connection Issues

```bash
# Check listening ports
ssh $node "sudo netstat -tlnp | grep -E '443|8083'"

# Review error logs
ssh $node "sudo journalctl -u zeitwork-edge-proxy | grep -i error | tail -20"

# Check rate limiting
curl -v https://node:443 2>&1 | grep -i "rate"
```

## Best Practices

1. **Test SSL thoroughly**: Verify certificates work before and after upgrade
2. **Monitor customer domains**: Keep customer domain monitoring active during upgrade
3. **Use connection draining**: Allow existing connections to complete
4. **Coordinate with CDN**: If using CDN, coordinate cache purging
5. **Update documentation**: Document any configuration changes

## Related Documentation

- [Load Balancer Upgrade Guide](./load-balancer-upgrade.md)
- [SSL Certificate Management](../operations/ssl-certificates.md)
- [Rate Limiting Configuration](../configuration/rate-limiting.md)
