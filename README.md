# Zeitwork Platform

A production-ready platform for running containerized applications using Firecracker microVMs.

## Architecture

The Zeitwork platform consists of four main services that work together to provide a complete container hosting solution:

### Services

1. **Operator** (Port 8080)

   - Central orchestration service
   - Manages cluster state in PostgreSQL
   - Provides REST API for platform operations
   - Handles image building, deployments, and instance management

2. **Node Agent** (Port 8081)

   - Runs on each compute node
   - Manages Firecracker VMs locally
   - Reports health and resources to operator
   - Handles VM lifecycle operations

3. **Load Balancer** (Port 8082)

   - Distributes traffic across instances
   - Service discovery from operator
   - Multiple algorithms: round-robin, least-connections, ip-hash
   - Health checking for backends

4. **Edge Proxy** (Port 443/8083)
   - External entry point for applications
   - SSL/TLS termination
   - Rate limiting per IP
   - Security headers and CORS handling

## Quick Start

### Prerequisites

- Go 1.23+
- PostgreSQL 14+
- Linux with KVM support (for node agents)
- Firecracker (installed by node agent setup)

### Building

```bash
# Build all services for Linux
make build

# Build for local development
make build-local
```

### Development

Run services locally for development:

```bash
# Set up PostgreSQL database first
createdb zeitwork_dev

# Run migrations (from packages/database)
npm run db:migrate

# Run services (in separate terminals)
make run-operator
make run-node-agent
make run-load-balancer
make run-edge-proxy
```

### Production Deployment

1. Build the binaries:

```bash
make build
```

2. Install on target servers:

```bash
sudo make install
```

3. Configure services:

```bash
# Edit configuration files in /etc/zeitwork/
sudo vim /etc/zeitwork/operator.env
sudo vim /etc/zeitwork/node-agent.env
# etc...
```

4. Start services:

```bash
sudo systemctl start zeitwork-operator
sudo systemctl start zeitwork-node-agent
sudo systemctl start zeitwork-load-balancer
sudo systemctl start zeitwork-edge-proxy
```

## API Usage

### Nodes

```bash
# List nodes
curl http://localhost:8080/api/v1/nodes

# Add a node (done automatically by node-agent)
curl -X POST http://localhost:8080/api/v1/nodes \
  -H "Content-Type: application/json" \
  -d '{"hostname": "node1", "ip_address": "10.0.1.1"}'
```

### Instances

```bash
# List instances
curl http://localhost:8080/api/v1/instances

# Create an instance
curl -X POST http://localhost:8080/api/v1/instances \
  -H "Content-Type: application/json" \
  -d '{
    "node_id": "...",
    "image_id": "...",
    "resources": {"vcpu": 2, "memory": 2048}
  }'
```

### Images

```bash
# List images
curl http://localhost:8080/api/v1/images

# Build an image from GitHub (TODO)
curl -X POST http://localhost:8080/api/v1/images \
  -H "Content-Type: application/json" \
  -d '{"github_repo": "owner/repo"}'
```

## Configuration

All services use environment variables for configuration. See the `deployments/config/` directory for templates.

Key configuration:

- `DATABASE_URL`: PostgreSQL connection string (operator only)
- `OPERATOR_URL`: URL to operator service (other services)
- `PORT`: Service port
- `LOG_LEVEL`: Logging level (debug, info, warn, error)
- `ENVIRONMENT`: Environment name (development, staging, production)

## Database

The platform uses PostgreSQL with sqlc for type-safe queries. The schema is managed using Drizzle ORM in TypeScript.

```bash
# Generate SQL code after schema changes
make sqlc
```

## Monitoring

Each service exposes:

- `/health` - Health check endpoint
- `/metrics` - Metrics endpoint (TODO)

## Security

- Services run as non-root user (except node-agent which needs root for VM management)
- systemd security hardening (PrivateTmp, ProtectSystem, etc.)
- Rate limiting at edge proxy
- SSL/TLS termination at edge proxy
- Security headers (HSTS, X-Frame-Options, etc.)

## Development

### Project Structure

```
.
├── cmd/                    # Service entry points
│   ├── operator/
│   ├── node-agent/
│   ├── load-balancer/
│   └── edge-proxy/
├── internal/               # Business logic
│   ├── operator/
│   ├── node-agent/
│   ├── load-balancer/
│   ├── edge-proxy/
│   ├── database/          # Generated SQL code
│   └── shared/            # Shared utilities
├── packages/
│   └── database/          # TypeScript schema
├── deployments/           # Deployment configs
│   ├── systemd/          # Service files
│   └── config/           # Environment templates
└── scripts/              # Build and install scripts
```

### Testing

```bash
# Run all tests
make test

# Run specific package tests
go test ./internal/operator/...
```
