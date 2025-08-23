# Zeitwork Production Deployment Guide

Zeitwork is a platform for running containerized applications using Firecracker microVMs. This guide covers the standard production deployment model and how customer applications are deployed.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Customer Deployment Model](#customer-deployment-model)
3. [Production Infrastructure](#production-infrastructure)
4. [Installation](#installation)
5. [Network Configuration](#network-configuration)
6. [Production Deployment Guide](setup/production-deployment.md)
7. [Operations Guide](setup/operations.md)
8. [Troubleshooting](setup/troubleshooting.md)

## Architecture Overview

Zeitwork orchestrates containerized workloads in Firecracker microVMs through a distributed system of services:

### Core Services

**1. Operator Service (Port 8080)**

- Central control plane and API
- Manages cluster state in PostgreSQL
- Handles GitHub webhooks for deployments
- Builds and optimizes container images for microVMs
- Coordinates deployments across regions

**2. Node Agent (Port 8081)**

- Runs on each worker node
- Manages local Firecracker VMs
- Reports health to operator
- Executes instance lifecycle operations

**3. Load Balancer (Port 8082)**

- Layer 4 TCP load balancer
- Entry point from the internet
- Routes raw TCP connections to edge proxies
- Supports round-robin, least-connections, and IP-hash algorithms

**4. Edge Proxy (Port 443/8083)**

- HTTP/HTTPS application layer proxy
- SSL/TLS termination
- Rate limiting per IP
- Routes HTTP requests to node agents based on deployment

### Request Flow

```
    Customer Domain (app.dokedu.org)
            │
            ▼ (CNAME)
    edge.zeitwork.com
            │
            ▼ (GeoDNS to closest region)
    L4 Load Balancer (Port 8082)
    (TCP connection routing)
            │
            ▼
    Edge Proxy (Port 443/8083)
    (SSL termination, HTTP routing, rate limiting)
            │
            ▼
    Node Agents (Port 8081)
    (on worker nodes)
            │
            ▼
    Firecracker VMs
    (customer application)

    Operator Service ← (manages all components)
            │
            ▼
    PostgreSQL Database
    (cluster state)
```

## Customer Deployment Model

### How Customer Applications are Deployed

1. **GitHub Integration**: Customer connects their GitHub repository to Zeitwork
2. **Automatic Builds**: Each commit triggers a new deployment:
   - Webhook received from GitHub
   - Pull the container image
   - Build and optimize for Firecracker microVM
   - Create deployment across all regions
3. **Global Distribution**: Minimum 3 VM instances per region (9 total globally)
4. **Unique Endpoints**: Each deployment gets a versioned URL:
   - Format: `<project>-<nanoid>-<org>.zeitwork.app`
   - Example: `dokedu-a7x9k2m-dokedu.zeitwork.app`
5. **Customer Domain**: Customer points their domain to `edge.zeitwork.com`:
   - `app.dokedu.org` → CNAME → `edge.zeitwork.com`
   - Always routes to the latest deployment automatically
   - GeoDNS ensures users connect to the nearest region

### Deployment Flow

```
1. Developer pushes to GitHub
        ↓
2. GitHub webhook to Zeitwork Operator
        ↓
3. Pull container image from registry
        ↓
4. Build and optimize for Firecracker microVM
        ↓
5. Deploy to all 3 regions (min 3 instances each)
        ↓
6. Register deployment as: project-nanoid-org.zeitwork.app
        ↓
7. Update routing tables to point to new deployment
        ↓
8. Customer domain (via CNAME) automatically serves new version
```

### Example Customer Setup

For a customer "Dokedu" with application at `app.dokedu.org`:

1. **DNS Configuration** (customer side):

   ```
   app.dokedu.org  CNAME  edge.zeitwork.com
   ```

2. **Deployment URLs** (Zeitwork generates):

   ```
   Latest:    dokedu-a7x9k2m-dokedu.zeitwork.app  (current production)
   Previous:  dokedu-b8y3n5p-dokedu.zeitwork.app  (previous version)
   Staging:   dokedu-c9z4m6q-dokedu.zeitwork.app  (staging branch)
   ```

3. **Traffic Flow**:
   - User visits `app.dokedu.org`
   - DNS resolves to `edge.zeitwork.com`
   - GeoDNS routes to nearest region (US, EU, or Asia)
   - Edge proxy routes to the latest deployment
   - Request served from nearest Firecracker VM

## Production Infrastructure

Zeitwork requires a minimum production deployment of **3 regions** for high availability and geographic distribution.

### Required Infrastructure

#### Per Region (minimum):

- **3 Operator nodes**: High availability control plane
- **6 Worker nodes**: Compute capacity for Firecracker VMs

#### Total Minimum Infrastructure:

- **9 Operator nodes** (3 per region × 3 regions)
- **18 Worker nodes** (6 per region × 3 regions)
- **PlanetScale PostgreSQL database** (managed service)

### Service Distribution

#### Operator Nodes (3 per region)

Each operator node runs:

- 1× Operator Service
- 1× Load Balancer
- 1× Edge Proxy

This provides redundancy for all control plane services.

#### Worker Nodes (6 per region)

Each worker node runs:

- 1× Node Agent
- Multiple Firecracker VMs (based on resources)

### Network Architecture

```
Region 1 (e.g., US-EAST)
├── Operator Nodes (3)
│   ├── operator-1: Operator + LB + Edge Proxy
│   ├── operator-2: Operator + LB + Edge Proxy
│   └── operator-3: Operator + LB + Edge Proxy
└── Worker Nodes (6)
    ├── worker-1: Node Agent + VMs
    ├── worker-2: Node Agent + VMs
    ├── worker-3: Node Agent + VMs
    ├── worker-4: Node Agent + VMs
    ├── worker-5: Node Agent + VMs
    └── worker-6: Node Agent + VMs

External: PlanetScale PostgreSQL Database

Region 2 (e.g., EU-WEST)
├── [Same structure as Region 1]

Region 3 (e.g., AP-SOUTH)
├── [Same structure as Region 1]
```

### Hardware Requirements

#### Operator Nodes

**Minimum specifications:**

- CPU: 8 cores
- RAM: 32 GB
- Storage: 250 GB SSD
- Network: 10 Gbps

**Recommended specifications:**

- CPU: 16 cores
- RAM: 64 GB
- Storage: 500 GB NVMe SSD
- Network: 25 Gbps

#### Worker Nodes

**Minimum specifications:**

- CPU: 16 cores with Intel VT-x or AMD-V
- RAM: 64 GB
- Storage: 500 GB SSD
- Network: 10 Gbps
- KVM support required

**Recommended specifications:**

- CPU: 32 cores with Intel VT-x or AMD-V
- RAM: 128 GB
- Storage: 1 TB NVMe SSD
- Network: 25 Gbps
- KVM support required

#### Database Requirements

**PlanetScale PostgreSQL:**

- PostgreSQL 17 or later
- Built-in high availability and global replication
- Automated backups with point-in-time recovery
- SSL/TLS encryption enforced
- Connection pooling included
- Serverless scaling
- Zero-downtime schema migrations

### IP Address Requirements

**Per Region:**

- Public IPs: 3 (for Load Balancers on operator nodes)
- Private subnet: /24 minimum (256 IPs)
  - 3 IPs for operator nodes
  - 6 IPs for worker nodes
  - Remaining for Firecracker VMs

## Prerequisites

### Software Requirements

- **Operating System**: Ubuntu 22.04 LTS
- **Kernel**: Linux 5.10+ with KVM modules
- **Go**: 1.25+ (for building from source)
- **PostgreSQL**: 17+
- **Firecracker**: 1.12+
- **Node.js**: 22+ (for database migrations)

### Network Requirements

- Inter-region connectivity with <100ms latency
- Intra-region connectivity with <1ms latency
- Firewall rules configured for service ports
- DNS configured for zeitwork.com and zeitwork.app domains

## Installation

### 1. Build Zeitwork

```bash
# Clone repository
git clone https://github.com/zeitwork/zeitwork.git
cd zeitwork

# Build all services
make build
```

### 2. Database Setup

```bash
# Prerequisites: PlanetScale PostgreSQL database
# Create database at: https://app.planetscale.com
# Requirements: PostgreSQL 14+ compatibility mode

# Connect to your PlanetScale database
psql "postgresql://username:password@host.connect.psdb.cloud/zeitwork-production?sslmode=require"

# Create user and permissions (if not using PlanetScale's built-in user management)
CREATE USER zeitwork WITH ENCRYPTED PASSWORD 'your-secure-password';
GRANT ALL PRIVILEGES ON DATABASE zeitwork_production TO zeitwork;
\q

# Run migrations
cd packages/database
npm install
export DATABASE_URL="postgresql://username:password@host.connect.psdb.cloud/zeitwork-production?sslmode=require"
npm run db:migrate
```

### 3. Install Firecracker (on worker nodes)

```bash
# Download Firecracker
FC_VERSION="v1.12.1"
ARCH=$(uname -m)
wget https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-${ARCH}.tgz
tar -xzf firecracker-${FC_VERSION}-${ARCH}.tgz

# Install binary
sudo cp release-${FC_VERSION}-${ARCH}/firecracker-${FC_VERSION}-${ARCH} /usr/bin/firecracker
sudo chmod +x /usr/bin/firecracker

# Download kernel
sudo mkdir -p /var/lib/firecracker/kernels
cd /var/lib/firecracker/kernels
sudo wget https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/${ARCH}/kernels/vmlinux.bin

# Verify KVM access
ls -l /dev/kvm
```

### 4. Configure Services

Create configuration files for each service:

#### Operator Configuration

Copy configuration from [deployments/config/operator.env](../deployments/config/operator.env):

```bash
sudo cp deployments/config/operator.env /etc/zeitwork/
# Edit to set your PlanetScale DATABASE_URL:
# DATABASE_URL=postgresql://username:password@host.connect.psdb.cloud/zeitwork-production?sslmode=require
```

#### Node Agent Configuration

Copy configuration from [deployments/config/node-agent.env](../deployments/config/node-agent.env):

```bash
sudo cp deployments/config/node-agent.env /etc/zeitwork/
# Edit OPERATOR_URL to point to your operator nodes
```

#### Load Balancer Configuration

Copy configuration from [deployments/config/load-balancer.env](../deployments/config/load-balancer.env):

```bash
sudo cp deployments/config/load-balancer.env /etc/zeitwork/
```

#### Edge Proxy Configuration

Copy configuration from [deployments/config/edge-proxy.env](../deployments/config/edge-proxy.env):

```bash
sudo cp deployments/config/edge-proxy.env /etc/zeitwork/
# Update SSL_CERT_PATH and SSL_KEY_PATH to your certificate locations
```

### 5. Deploy Services

```bash
# Install binaries and systemd services
sudo make install

# On operator nodes, start all three services:
sudo systemctl enable --now zeitwork-operator
sudo systemctl enable --now zeitwork-load-balancer
sudo systemctl enable --now zeitwork-edge-proxy

# On worker nodes, start node agent:
sudo systemctl enable --now zeitwork-node-agent

# Verify services
sudo systemctl status zeitwork-*
```

## Network Configuration

### DNS Setup

#### Platform DNS (zeitwork.com)

```
# Global edge entry point with GeoDNS
edge.zeitwork.com → GeoDNS routing to closest region:
  - US-EAST Load Balancers (for Americas)
  - EU-WEST Load Balancers (for Europe/Africa)
  - AP-SOUTH Load Balancers (for Asia/Pacific)

# Regional direct access endpoints
us-east.zeitwork.com → [US-EAST Load Balancers]
eu-west.zeitwork.com → [EU-WEST Load Balancers]
ap-south.zeitwork.com → [AP-SOUTH Load Balancers]

# API endpoint for management
api.zeitwork.com → [All regional Operators]
```

#### Customer Application DNS (zeitwork.app)

```
# Each deployment gets a unique versioned endpoint
<project>-<nanoid>-<org>.zeitwork.app → [Specific deployment]

# Examples:
dokedu-a7x9k2m-dokedu.zeitwork.app → [Production deployment]
dokedu-b8y3n5p-dokedu.zeitwork.app → [Previous version]
webapp-c9z4m6q-acme.zeitwork.app → [Different customer]

# Wildcard DNS for all deployments
*.zeitwork.app → [Edge Proxy routing]
```

#### Customer Domain Configuration

```
# Customer configures their DNS (one-time setup)
app.customer.com  CNAME  edge.zeitwork.com

# Traffic flow:
app.customer.com → edge.zeitwork.com → Closest region → Latest deployment
```

### Firewall Rules

Configure firewall rules for each node type:

#### Operator Nodes

```bash
# Inbound
- 8080/tcp from internal network (Operator API)
- 8082/tcp from internet (Load Balancer)
- 443/tcp from internet (Edge Proxy HTTPS)
- 8083/tcp from internal network (Edge Proxy HTTP)

# Outbound
- 8081/tcp to worker nodes (Node Agent)
- 5432/tcp to external PostgreSQL database
```

#### Worker Nodes

```bash
# Inbound
- 8081/tcp from operator nodes (Node Agent API)
- 22/tcp from management network (SSH)

# Outbound
- 8080/tcp to operator nodes (Operator API)
- 443/tcp to internet (for pulling images)
```

## Security Considerations

1. **Network Segmentation**

   - Separate VLANs for operator, worker, and database tiers
   - Strict firewall rules between tiers

2. **TLS/SSL**

   - Valid certificates on Edge Proxies
   - TLS for database connections
   - Internal service communication can use private CA

3. **Access Control**

   - SSH key-based authentication only
   - Service accounts with minimal privileges
   - Regular security updates

4. **Secrets Management**
   - Database passwords in environment files with restricted permissions
   - SSL certificates with proper file permissions (600)
   - Consider using a secrets management system for production

## Monitoring

Essential metrics to monitor:

- **System Health**

  - Service status (all Zeitwork services)
  - Node availability
  - Database replication lag

- **Performance**

  - Request latency (p50, p95, p99)
  - Error rates
  - VM boot times
  - Deployment build times

- **Resources**

  - CPU and memory usage per node
  - Disk I/O and space
  - Network throughput

- **Application**
  - Active VM count per deployment
  - Deployment success rate
  - API response times
  - Customer application health

## Maintenance

### Regular Tasks

- Database backups (daily)
- Log rotation (configured via logrotate)
- Security updates (monthly)
- Certificate renewal (before expiry)

### Scaling

- Add worker nodes: Deploy new nodes with node agent
- Add regions: Replicate the 3-operator, 6-worker setup
- Increase capacity: Scale worker nodes horizontally

## Next Steps

1. **[Production Deployment Guide](setup/production-deployment.md)** - Detailed step-by-step deployment
2. **[Operations Guide](setup/operations.md)** - Day-to-day operations and maintenance
3. **[Troubleshooting](setup/troubleshooting.md)** - Common issues and solutions

## Support

- GitHub Issues: https://github.com/zeitwork/zeitwork/issues
- Documentation: https://docs.zeitwork.com
- Customer Portal: https://app.zeitwork.com
- Email: support@zeitwork.com
