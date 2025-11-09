# EdgeProxy Service

A production-grade HTTPS reverse proxy that routes domain traffic to VMs based on database state, with automatic Let's Encrypt SSL certificate management, health checking, and cross-region routing support.

## Features

- **Automatic HTTPS with Let's Encrypt**: Automatically obtains and renews SSL certificates using ACME TLS-ALPN-01 challenge
- **Database-driven routing**: Loads routing configuration from PostgreSQL (domains → deployments → VMs)
- **Proactive certificate acquisition**: Acquires certificates for verified domains before first request
- **HTTP to HTTPS redirect**: Automatically redirects all HTTP traffic to HTTPS
- **PostgreSQL certificate storage**: Stores certificates in database for multi-instance deployment
- **Automatic refresh**: Refreshes routing state every 10 seconds
- **Health checking**: Performs HTTP health checks before routing to VMs
- **Cross-region routing**: Routes traffic to other regions' load balancers when VM is in a different region
- **Same-region optimization**: Routes directly to VMs in the same region for lower latency
- **Graceful shutdown**: Handles SIGINT/SIGTERM signals for clean shutdown
- **Configurable ACME environment**: Supports both Let's Encrypt staging and production

## Architecture

### Traffic Flow

1. **HTTP Request (Port 80)**:
   - If path is `/.well-known/acme-challenge/*`: Serve ACME challenge response
   - Otherwise: Redirect to HTTPS (301 Moved Permanently)

2. **HTTPS Request (Port 443)**:
   - TLS termination with certificate from certmagic
   - EdgeProxy looks up domain in routing table
   - Checks if VM is in same region:
     - **Same region**: Health check VM at `http://{publicIp}:{port}/`, then proxy if healthy
     - **Different region**: Proxy to region load balancer at `http://{regionPublicIp}:80`
   - If no route found or VM unhealthy, return appropriate error

### Certificate Management

1. **On Startup**:
   - Load all verified domains from database
   - **Immediately** start certificate acquisition for all domains needing certificates
   - Start background loop to check for new domains every hour

2. **Certificate Acquisition** (with Rate Limiting):
   - Query database for domains where `verified_at IS NOT NULL` and certificate is missing/expiring
   - Process domains sequentially with configurable delay between each (default 3s)
   - Rate limiting prevents hitting Let's Encrypt's 300 orders per 3 hours limit
   - For each domain:
     - Update status to "pending"
     - Use certmagic to obtain certificate via TLS-ALPN-01 challenge
     - Update status to "active" on success or "failed" on error
     - Store certificate in PostgreSQL

3. **Certificate Storage**:
   - Certificates stored in `certmagic_data` table (base64 encoded)
   - Distributed locks in `certmagic_locks` table
   - Domain certificate status tracked in `domains` table

### Database Queries

The proxy uses several SQL queries:

**Active Routes:**

```sql
SELECT
    d.name as domain_name,
    v.public_ip as vm_public_ip,
    v.port as vm_port,
    v.region_id as vm_region_id,
    r.load_balancer_ipv4 as region_load_balancer_ip
FROM domains d
INNER JOIN deployments dep ON d.deployment_id = dep.id
INNER JOIN vms v ON dep.vm_id = v.id
INNER JOIN regions r ON v.region_id = r.id
WHERE d.verified_at IS NOT NULL
  AND dep.status = 'ready'
  AND v.status = 'running'
ORDER BY d.name
```

**Domains Needing Certificates:**

```sql
SELECT
    id,
    name,
    ssl_certificate_status,
    ssl_certificate_expires_at
FROM domains
WHERE verified_at IS NOT NULL
  AND (
    ssl_certificate_status IS NULL
    OR ssl_certificate_status != 'active'
    OR ssl_certificate_expires_at IS NULL
    OR ssl_certificate_expires_at < NOW() + INTERVAL '30 days'
  )
ORDER BY name
```

## Configuration

Environment variables:

| Variable                             | Required | Default | Description                                        |
| ------------------------------------ | -------- | ------- | -------------------------------------------------- |
| `EDGEPROXY_HTTP_ADDR`                | No       | `:8080` | HTTP listen address                                |
| `EDGEPROXY_HTTPS_ADDR`               | No       | `:8443` | HTTPS listen address                               |
| `EDGEPROXY_DATABASE_URL`             | Yes      | -       | PostgreSQL connection string                       |
| `EDGEPROXY_REGION_ID`                | Yes      | -       | UUID of the region this proxy runs in              |
| `EDGEPROXY_ACME_EMAIL`               | Yes      | -       | Email for Let's Encrypt account                    |
| `EDGEPROXY_ACME_STAGING`             | No       | `false` | Use Let's Encrypt staging (for testing)            |
| `EDGEPROXY_ACME_CERT_CHECK_INTERVAL` | No       | `1h`    | How often to check for certificates to renew       |
| `EDGEPROXY_ACME_RATE_LIMIT_DELAY`    | No       | `3s`    | Delay between cert requests (respects rate limits) |
| `EDGEPROXY_UPDATE_INTERVAL`          | No       | `10s`   | How often to refresh routes                        |
| `EDGEPROXY_LOG_LEVEL`                | No       | `info`  | Log level: debug, info, warn, error                |

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
export EDGEPROXY_ACME_EMAIL="admin@example.com"
export EDGEPROXY_ACME_STAGING="false"  # Set to "true" for testing
export EDGEPROXY_HTTP_ADDR=":80"
export EDGEPROXY_HTTPS_ADDR=":443"
export EDGEPROXY_LOG_LEVEL="info"

# Run the service
./edgeproxy
```

## Docker

```bash
# Build the image
docker build -f docker/edgeproxy/Dockerfile -t zeitwork/edgeproxy .

# Run with docker-compose
docker-compose up edgeproxy
```

**Note**: When running in Docker as a non-root user, binding to ports < 1024 requires special configuration. Either:

- Use port mapping: `-p 80:8080 -p 443:8443` and set `EDGEPROXY_HTTP_ADDR=:8080` and `EDGEPROXY_HTTPS_ADDR=:8443`
- Run container with `--cap-add=NET_BIND_SERVICE`
- Run behind a load balancer that handles privileged ports

## Let's Encrypt

### Rate Limits

Let's Encrypt has rate limits to prevent abuse. The edgeproxy includes built-in rate limiting to respect these limits:

- **Staging environment** (recommended for testing):
  - No rate limits
  - Issues test certificates (not trusted by browsers)
  - Set `EDGEPROXY_ACME_STAGING=true`

- **Production environment**:
  - 50 certificates per registered domain per week
  - 5 duplicate certificates per week
  - **300 new orders per account per 3 hours** (most restrictive)
  - Set `EDGEPROXY_ACME_STAGING=false`

**Built-in Rate Limiting**:

- Edgeproxy adds a configurable delay between certificate requests (default: 3 seconds)
- For 19 domains with 3s delay = ~60 seconds total startup time
- This prevents hitting the 300 orders/3 hours limit
- Adjust with `EDGEPROXY_ACME_RATE_LIMIT_DELAY` if you have many domains

### Certificate Lifecycle

1. **Initial Acquisition**: When a domain is verified, certificate is requested proactively
2. **Renewal**: Certificates are automatically renewed when they expire in < 30 days
3. **Storage**: Certificates stored in PostgreSQL for persistence across restarts
4. **Validity**: Let's Encrypt certificates are valid for 90 days

### ACME TLS-ALPN-01 Challenge

The proxy uses TLS-ALPN-01 challenge, which requires:

- Port 443 accessible from the internet
- DNS records pointing to the proxy
- Domain must be verified in the database (`verified_at IS NOT NULL`)

Challenge flow:

1. Certmagic requests certificate from Let's Encrypt
2. Let's Encrypt responds with a challenge
3. Let's Encrypt makes TLS connection to `https://domain:443` with ALPN protocol "acme-tls/1"
4. Certmagic responds with the challenge certificate via TLS handshake
5. Let's Encrypt validates and issues certificate

**Advantages of TLS-ALPN-01**:

- Works through port 443 (HTTPS) which we already handle
- No need for port 80 to be accessible
- Challenge handled automatically in the TLS handshake
- Works seamlessly with TCP load balancers forwarding 443→8443

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

- **info**: Startup, route updates, certificate acquisition, shutdown events
- **warn**: Health check failures, missing routes, certificate errors
- **error**: Database errors, proxy errors, ACME failures
- **debug**: Individual request routing, health check results, ACME challenges

Example log entries:

```json
{"time":"2025-01-01T12:00:00Z","level":"INFO","msg":"acquiring certificate","domain":"example.com"}
{"time":"2025-01-01T12:00:05Z","level":"INFO","msg":"certificate obtained successfully","domain":"example.com"}
{"time":"2025-01-01T12:00:10Z","level":"DEBUG","msg":"serving ACME challenge","path":"/.well-known/acme-challenge/abc123","host":"example.com"}
{"time":"2025-01-01T12:00:15Z","level":"DEBUG","msg":"redirecting to HTTPS","from":"http://example.com/path","to":"https://example.com/path"}
{"time":"2025-01-01T12:00:20Z","level":"DEBUG","msg":"proxying request","host":"example.com","path":"/api","target":"http://10.0.1.5:8080","same_region":true}
```

## Performance

- Routing table is in-memory for fast lookups (O(1) map lookup)
- Certificate lookups use certmagic's in-memory cache backed by PostgreSQL
- Background refresh runs every 10 seconds (configurable)
- Certificate checks run every 1 hour (configurable)
- Health checks have 2-second timeout to avoid blocking
- Concurrent request handling via Go's HTTP server
- TLS session resumption and HTTP/2 support via Go's standard library

## Security

- TLS 1.2+ only (configured by Go's crypto/tls)
- Modern cipher suites
- Certificate private keys stored in PostgreSQL (base64 encoded)
- Distributed locking prevents concurrent certificate requests
- Non-root user in Docker container
- No credentials in logs

## Troubleshooting

### Certificate acquisition fails

1. **Check DNS**: Ensure domain points to proxy's public IP
2. **Check port 80**: Verify port 80 is accessible from internet
3. **Check domain verification**: Ensure `verified_at` is set in database
4. **Check rate limits**: Use staging environment for testing
5. **Check logs**: Look for ACME errors in service logs

```bash
# Check domain verification
SELECT name, verified_at, ssl_certificate_status, ssl_certificate_error
FROM domains
WHERE name = 'example.com';
```

### HTTP redirect loop

- Ensure proxy is terminating TLS, not a load balancer upstream
- Check that application isn't forcing HTTPS redirect

### Certificate not found

- Wait for certificate acquisition (check logs)
- Verify certificate in database: `SELECT key FROM certmagic_data WHERE key LIKE '%example.com%'`
- Check certificate status: `SELECT ssl_certificate_status FROM domains WHERE name = 'example.com'`

## Dependencies

- `github.com/jackc/pgx/v5` - PostgreSQL driver
- `github.com/caddyserver/certmagic` - Automatic HTTPS with ACME
- `github.com/caarlos0/env/v11` - Environment variable parsing
