# EdgeProxy Service

A production-grade HTTP reverse proxy that routes domain traffic to VMs based on database state, with health checking and cross-region routing support.

## Features

- **Database-driven routing**: Loads routing configuration from PostgreSQL (domains → deployments → VMs)
- **Automatic refresh**: Refreshes routing state every 10 seconds
- **Health checking**: Performs HTTP health checks before routing to VMs
- **Cross-region routing**: Routes traffic to other regions' load balancers when VM is in a different region
- **Same-region optimization**: Routes directly to VMs in the same region for lower latency
- **Graceful shutdown**: Handles SIGINT/SIGTERM signals for clean shutdown

## Architecture

### Routing Flow

1. HTTP request arrives with `Host` header
2. EdgeProxy looks up domain in routing table
3. Checks if VM is in same region:
   - **Same region**: Health check VM at `http://{privateIp}:{port}/`, then proxy if healthy
   - **Different region**: Proxy to region load balancer at `http://{regionPublicIp}:80`
4. If no route found or VM unhealthy, return appropriate error

### Database Query

The proxy queries the database to fetch active routes:

```sql
SELECT
    d.name as domain_name,
    v.private_ip as vm_private_ip,
    v.port as vm_port,
    v.region_id as vm_region_id,
    r.public_ipv4 as region_public_ip
FROM domains d
INNER JOIN deployments dep ON d.deployment_id = dep.id
INNER JOIN vms v ON dep.vm_id = v.id
INNER JOIN regions r ON v.region_id = r.id
WHERE d.verified_at IS NOT NULL
  AND dep.status = 'ready'
  AND v.status = 'running'
ORDER BY d.name
```

## Configuration

Environment variables:

| Variable                    | Required | Default | Description                           |
| --------------------------- | -------- | ------- | ------------------------------------- |
| `EDGEPROXY_HTTP_ADDR`       | No       | `:8080` | HTTP listen address                   |
| `EDGEPROXY_DATABASE_URL`    | Yes      | -       | PostgreSQL connection string          |
| `EDGEPROXY_REGION_ID`       | Yes      | -       | UUID of the region this proxy runs in |
| `EDGEPROXY_UPDATE_INTERVAL` | No       | `10s`   | How often to refresh routes           |
| `EDGEPROXY_LOG_LEVEL`       | No       | `info`  | Log level: debug, info, warn, error   |

## Building

```bash
# Build the binary
go build -o edgeproxy ./cmd/edgeproxy

# Or build with optimizations
go build -ldflags="-s -w" -o edgeproxy ./cmd/edgeproxy
```

## Running

```bash
# Set environment variables
export EDGEPROXY_DATABASE_URL="postgres://user:pass@localhost:5432/zeitwork"
export EDGEPROXY_REGION_ID="01234567-89ab-cdef-0123-456789abcdef"
export EDGEPROXY_HTTP_ADDR=":8080"
export EDGEPROXY_LOG_LEVEL="info"

# Run the service
./edgeproxy
```

## Docker

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o edgeproxy ./cmd/edgeproxy

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/edgeproxy .
CMD ["./edgeproxy"]
```

## Health Checking

The proxy performs active health checks before routing to VMs in the same region:

- **Endpoint**: `GET http://{vm_ip}:{vm_port}/`
- **Timeout**: 2 seconds
- **Healthy**: HTTP status code 2xx or 3xx
- **Unhealthy**: Connection error or 4xx/5xx status → returns 503 to client

## Error Responses

| Scenario               | HTTP Status | Response Body                             |
| ---------------------- | ----------- | ----------------------------------------- |
| No route found         | 404         | "Service Not Found"                       |
| VM health check failed | 503         | "Service Unavailable - VM not responding" |
| Proxy connection error | 502         | "Bad Gateway"                             |
| Internal error         | 500         | "Internal Server Error"                   |

## Logging

The service logs in JSON format to stdout:

- **info**: Startup, route updates, shutdown events
- **warn**: Health check failures, missing routes
- **error**: Database errors, proxy errors
- **debug**: Individual request routing, health check results

## Performance

- Routing table is in-memory for fast lookups (O(1) map lookup)
- Background refresh runs every 10 seconds (configurable)
- Health checks have 2-second timeout to avoid blocking
- Concurrent request handling via Go's HTTP server

## Dependencies

- `github.com/jackc/pgx/v5` - PostgreSQL driver
- `github.com/google/uuid` - UUID utilities
- `github.com/caarlos0/env/v11` - Environment variable parsing
