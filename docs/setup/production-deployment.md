# Production Deployment Guide

This guide provides step-by-step instructions for deploying Zeitwork in production with 3 regions, 3 operators per region, and 6 workers per region.

## Prerequisites Checklist

Before starting deployment:

- [ ] 9 operator nodes provisioned (3 per region)
- [ ] 18 worker nodes provisioned (6 per region)
- [ ] PlanetScale PostgreSQL database configured
- [ ] Ubuntu 22.04 LTS installed on all nodes
- [ ] Root or sudo access on all nodes
- [ ] Network connectivity between all nodes verified
- [ ] DNS records prepared for zeitwork.com
- [ ] SSL certificates obtained for \*.zeitwork.com

## Phase 1: Database Setup

### PlanetScale PostgreSQL Setup

Zeitwork uses PlanetScale for PostgreSQL database hosting:

**PlanetScale Features:**

- PostgreSQL 17+ compatibility
- Built-in high availability and global replication
- Automated backups with point-in-time recovery
- SSL/TLS encryption by default
- Connection pooling included
- Serverless scaling
- Zero-downtime schema migrations
- Branch-based database development

### Database Initialization

```bash
# Create database in PlanetScale console:
# 1. Go to https://app.planetscale.com
# 2. Create new database "zeitwork-production"
# 3. Get connection string from dashboard

# Connect to your PlanetScale database
psql "postgresql://username:password@host.connect.psdb.cloud/zeitwork-production?sslmode=require"

# PlanetScale handles user management, but if you need specific users:
CREATE USER zeitwork WITH ENCRYPTED PASSWORD 'your-secure-password';
GRANT ALL PRIVILEGES ON DATABASE zeitwork_production TO zeitwork;
\q

# Run migrations
cd zeitwork/packages/database
npm install
export DATABASE_URL="postgresql://username:password@host.connect.psdb.cloud/zeitwork-production?sslmode=require"
npm run db:migrate
```

## Phase 2: Build and Stage Binaries

Build Zeitwork on a build server:

```bash
# Clone and build
git clone https://github.com/zeitwork/zeitwork.git
cd zeitwork
make build

# Create deployment package
tar -czf zeitwork-binaries.tar.gz build/

# Copy to all nodes
for node in $(cat all-nodes.txt); do
    scp zeitwork-binaries.tar.gz $node:/tmp/
done
```

## Phase 3: Deploy Operator Nodes

Deploy services on each operator node in all regions.

### For Each Operator Node:

```bash
# Extract binaries
cd /tmp
tar -xzf zeitwork-binaries.tar.gz

# Install binaries
sudo cp build/zeitwork-operator /usr/local/bin/
sudo cp build/zeitwork-load-balancer /usr/local/bin/
sudo cp build/zeitwork-edge-proxy /usr/local/bin/
sudo chmod +x /usr/local/bin/zeitwork-*

# Create directories
sudo mkdir -p /etc/zeitwork
sudo mkdir -p /var/lib/zeitwork
sudo mkdir -p /var/log/zeitwork

# Create service user
sudo useradd -r -s /bin/false zeitwork
sudo chown -R zeitwork:zeitwork /var/lib/zeitwork /var/log/zeitwork
```

### Configure Services

Copy the configuration templates from the repository:

```bash
# Copy configuration templates
sudo cp zeitwork/deployments/config/operator.env /etc/zeitwork/
sudo cp zeitwork/deployments/config/load-balancer.env /etc/zeitwork/
sudo cp zeitwork/deployments/config/edge-proxy.env /etc/zeitwork/

# Edit operator configuration
sudo vim /etc/zeitwork/operator.env
# Update DATABASE_URL with your PlanetScale connection string:
# DATABASE_URL=postgresql://username:password@host.connect.psdb.cloud/zeitwork-production?sslmode=require

# The load-balancer.env and edge-proxy.env files can typically be used as-is
# Just ensure OPERATOR_URL in load-balancer.env points to localhost:8080
```

### Install SSL Certificates

```bash
# Install SSL certificates for zeitwork.com
sudo mkdir -p /etc/zeitwork/certs

# If using Let's Encrypt:
sudo certbot certonly --standalone -d "*.zeitwork.com" -d "zeitwork.com"
sudo ln -s /etc/letsencrypt/live/zeitwork.com/fullchain.pem /etc/zeitwork/certs/server.crt
sudo ln -s /etc/letsencrypt/live/zeitwork.com/privkey.pem /etc/zeitwork/certs/server.key

# Or copy your commercial certificates:
sudo cp /path/to/zeitwork.com.crt /etc/zeitwork/certs/server.crt
sudo cp /path/to/zeitwork.com.key /etc/zeitwork/certs/server.key

# Set permissions
sudo chmod 644 /etc/zeitwork/certs/server.crt
sudo chmod 600 /etc/zeitwork/certs/server.key
sudo chown zeitwork:zeitwork /etc/zeitwork/certs/*
```

### Create Systemd Services

Copy the systemd service files from the repository:

```bash
# Copy service files
sudo cp zeitwork/deployments/systemd/zeitwork-operator.service /etc/systemd/system/
sudo cp zeitwork/deployments/systemd/zeitwork-load-balancer.service /etc/systemd/system/
sudo cp zeitwork/deployments/systemd/zeitwork-edge-proxy.service /etc/systemd/system/

# Note: The service files reference /usr/local/bin paths and /etc/zeitwork configs
# No modifications should be needed if you followed the paths above

# Start services
sudo systemctl daemon-reload
sudo systemctl enable zeitwork-operator zeitwork-load-balancer zeitwork-edge-proxy
sudo systemctl start zeitwork-operator
sleep 5
sudo systemctl start zeitwork-load-balancer
sleep 5
sudo systemctl start zeitwork-edge-proxy

# Verify services
sudo systemctl status zeitwork-*
```

## Phase 4: Deploy Worker Nodes

Deploy Node Agent on each worker node.

### Install Firecracker (on each worker):

```bash
# Enable KVM
sudo modprobe kvm
sudo modprobe kvm_intel  # or kvm_amd
echo "kvm" | sudo tee -a /etc/modules
echo "kvm_intel" | sudo tee -a /etc/modules  # or kvm_amd

# Install Firecracker
FC_VERSION="v1.12.1"
ARCH=$(uname -m)
cd /tmp
wget https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-${ARCH}.tgz
tar -xzf firecracker-${FC_VERSION}-${ARCH}.tgz
sudo cp release-${FC_VERSION}-${ARCH}/firecracker-${FC_VERSION}-${ARCH} /usr/bin/firecracker
sudo chmod +x /usr/bin/firecracker

# Download kernel
sudo mkdir -p /var/lib/firecracker/kernels
cd /var/lib/firecracker/kernels
sudo wget https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/${ARCH}/kernels/vmlinux.bin

# Verify KVM
ls -l /dev/kvm
firecracker --version
```

### Install Node Agent (on each worker):

```bash
# Extract and install binary
cd /tmp
tar -xzf zeitwork-binaries.tar.gz
sudo cp build/zeitwork-node-agent /usr/local/bin/
sudo chmod +x /usr/local/bin/zeitwork-node-agent

# Create directories
sudo mkdir -p /etc/zeitwork
sudo mkdir -p /var/lib/zeitwork
sudo mkdir -p /var/log/zeitwork
sudo mkdir -p /var/lib/firecracker/vms

# Create service user
sudo useradd -r -s /bin/false zeitwork
sudo usermod -aG kvm zeitwork
sudo chown -R zeitwork:zeitwork /var/lib/zeitwork /var/log/zeitwork /var/lib/firecracker

# Copy and configure Node Agent
sudo cp zeitwork/deployments/config/node-agent.env /etc/zeitwork/
sudo vim /etc/zeitwork/node-agent.env
# Update OPERATOR_URL with your operator nodes:
# For US-EAST: OPERATOR_URL=http://us-east-op-1:8080,http://us-east-op-2:8080,http://us-east-op-3:8080
# For EU-WEST: OPERATOR_URL=http://eu-west-op-1:8080,http://eu-west-op-2:8080,http://eu-west-op-3:8080
# For AP-SOUTH: OPERATOR_URL=http://ap-south-op-1:8080,http://ap-south-op-2:8080,http://ap-south-op-3:8080

# Copy service file
sudo cp zeitwork/deployments/systemd/zeitwork-node-agent.service /etc/systemd/system/

# Start service
sudo systemctl daemon-reload
sudo systemctl enable zeitwork-node-agent
sudo systemctl start zeitwork-node-agent

# Verify
sudo systemctl status zeitwork-node-agent
sudo journalctl -u zeitwork-node-agent -f
```

## Phase 5: Configure DNS

Configure your DNS provider with the following records:

### Platform DNS (zeitwork.com)

```
# GeoDNS for edge endpoint (most important)
edge.zeitwork.com    GeoDNS:
  - Americas  → <US-EAST-OP-1-PUBLIC-IP>, <US-EAST-OP-2-PUBLIC-IP>, <US-EAST-OP-3-PUBLIC-IP>
  - Europe    → <EU-WEST-OP-1-PUBLIC-IP>, <EU-WEST-OP-2-PUBLIC-IP>, <EU-WEST-OP-3-PUBLIC-IP>
  - Asia      → <AP-SOUTH-OP-1-PUBLIC-IP>, <AP-SOUTH-OP-2-PUBLIC-IP>, <AP-SOUTH-OP-3-PUBLIC-IP>

# Regional endpoints (for direct access)
us-east.zeitwork.com    A    <US-EAST-OP-1-PUBLIC-IP>
us-east.zeitwork.com    A    <US-EAST-OP-2-PUBLIC-IP>
us-east.zeitwork.com    A    <US-EAST-OP-3-PUBLIC-IP>

eu-west.zeitwork.com    A    <EU-WEST-OP-1-PUBLIC-IP>
eu-west.zeitwork.com    A    <EU-WEST-OP-2-PUBLIC-IP>
eu-west.zeitwork.com    A    <EU-WEST-OP-3-PUBLIC-IP>

ap-south.zeitwork.com   A    <AP-SOUTH-OP-1-PUBLIC-IP>
ap-south.zeitwork.com   A    <AP-SOUTH-OP-2-PUBLIC-IP>
ap-south.zeitwork.com   A    <AP-SOUTH-OP-3-PUBLIC-IP>

# API endpoint for management
api.zeitwork.com        A    <ALL-OPERATOR-PUBLIC-IPS>
```

### Customer Application DNS (zeitwork.app)

```
# Wildcard for all customer deployments
*.zeitwork.app          CNAME edge.zeitwork.com

# This enables:
# - dokedu-a7x9k2m-dokedu.zeitwork.app
# - webapp-b8y3n5p-acme.zeitwork.app
# - any <project>-<nanoid>-<org>.zeitwork.app
```

## Phase 6: Verification

### Test Each Component

```bash
# Test Operator API (from operator node)
curl http://localhost:8080/health

# Test Load Balancer health
curl http://localhost:8084/health

# Test Edge Proxy (should return SSL cert info)
curl -k https://localhost:443/health

# Test Node Agent (from worker node)
curl http://localhost:8081/health

# Check registered nodes (from operator node)
curl http://localhost:8080/api/v1/nodes

# Test end-to-end (from external)
curl https://api.zeitwork.com/health
```

### Verify Cross-Region Communication

```bash
# From US-EAST operator
curl http://eu-west-op-1:8080/health
curl http://ap-south-op-1:8080/health

# Check database connectivity
psql $DATABASE_URL -c "SELECT version();"
```

### Monitor Logs

```bash
# On operator nodes
sudo journalctl -u zeitwork-operator -f
sudo journalctl -u zeitwork-load-balancer -f
sudo journalctl -u zeitwork-edge-proxy -f

# On worker nodes
sudo journalctl -u zeitwork-node-agent -f
```

## Phase 7: Initial Configuration

### Register Regions in Database

```bash
# Connect to database
psql $DATABASE_URL

# Insert regions
INSERT INTO regions (id, name, code, country) VALUES
  (gen_random_uuid(), 'US East', 'us-east', 'United States'),
  (gen_random_uuid(), 'EU West', 'eu-west', 'Germany'),
  (gen_random_uuid(), 'AP South', 'ap-south', 'Singapore');
```

### Configure GitHub Integration

```bash
# Set up GitHub webhook endpoint
curl -X POST http://localhost:8080/api/v1/github/webhook \
  -H "Content-Type: application/json" \
  -d '{
    "secret": "your-webhook-secret"
  }'

# Register a customer project
curl -X POST http://localhost:8080/api/v1/projects \
  -H "Content-Type: application/json" \
  -d '{
    "name": "dokedu",
    "org": "dokedu",
    "github_repo": "dokedu/app",
    "branch": "main",
    "auto_deploy": true
  }'
```

### Create Test Deployment

```bash
# Manually trigger a deployment (usually done via GitHub webhook)
curl -X POST http://localhost:8080/api/v1/deployments \
  -H "Content-Type: application/json" \
  -d '{
    "project": "dokedu",
    "org": "dokedu",
    "image": "ghcr.io/dokedu/app:latest",
    "commit_sha": "abc123def456",
    "instances_per_region": 3,
    "resources": {
      "vcpu": 2,
      "memory": 1024
    }
  }'

# This will create:
# - Deployment URL: dokedu-<nanoid>-dokedu.zeitwork.app
# - 9 total instances (3 per region)
# - Automatic routing via edge.zeitwork.com
```

### Customer DNS Setup

Instruct customers to add this CNAME record:

```
# Customer's DNS provider
app.customer.com    CNAME    edge.zeitwork.com

# Example for Dokedu:
app.dokedu.org      CNAME    edge.zeitwork.com
```

## Troubleshooting Deployment

### Service Won't Start

```bash
# Check logs
sudo journalctl -u zeitwork-operator -n 100

# Check permissions
ls -la /var/lib/zeitwork
ls -la /etc/zeitwork

# Verify configuration
cat /etc/zeitwork/operator.env
```

### Database Connection Issues

```bash
# Test connection
psql $DATABASE_URL -c "SELECT 1;"

# Check network connectivity to PlanetScale
telnet host.connect.psdb.cloud 3306  # PlanetScale uses MySQL port

# Verify SSL requirement (PlanetScale requires SSL)
psql "postgresql://username:password@host.connect.psdb.cloud/zeitwork-production?sslmode=disable"
# Should fail as PlanetScale enforces SSL
```

### Node Agent Not Registering

```bash
# Check operator is reachable
curl http://operator-host:8080/health

# Check node agent logs
sudo journalctl -u zeitwork-node-agent -n 100

# Verify KVM access
ls -l /dev/kvm
groups zeitwork
```

## Post-Deployment Checklist

- [ ] All services running on all nodes
- [ ] Database connection verified
- [ ] Node agents registered with operators
- [ ] Load balancers discovering backends
- [ ] Edge proxies serving HTTPS with zeitwork.com certificates
- [ ] DNS resolving correctly for zeitwork.com domains
- [ ] End-to-end connectivity verified
- [ ] Monitoring configured
- [ ] Database backups configured in PlanetScale
- [ ] Documentation updated with actual IPs/hostnames

## Configuration Files Reference

All configuration templates are available in the repository:

- **Operator**: [deployments/config/operator.env](../../deployments/config/operator.env)
- **Node Agent**: [deployments/config/node-agent.env](../../deployments/config/node-agent.env)
- **Load Balancer**: [deployments/config/load-balancer.env](../../deployments/config/load-balancer.env)
- **Edge Proxy**: [deployments/config/edge-proxy.env](../../deployments/config/edge-proxy.env)

Systemd service files:

- **Operator**: [deployments/systemd/zeitwork-operator.service](../../deployments/systemd/zeitwork-operator.service)
- **Node Agent**: [deployments/systemd/zeitwork-node-agent.service](../../deployments/systemd/zeitwork-node-agent.service)
- **Load Balancer**: Not provided in repo (create similar to operator)
- **Edge Proxy**: [deployments/systemd/zeitwork-edge-proxy.service](../../deployments/systemd/zeitwork-edge-proxy.service)

## Next Steps

1. Configure monitoring (Prometheus, Grafana)
2. Set up log aggregation (ELK stack)
3. Configure automated backups (handled automatically by PlanetScale)
4. Set up CI/CD pipeline
5. Deploy first production workload

---

_Last updated: December 2024_
