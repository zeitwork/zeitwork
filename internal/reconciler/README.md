# Reconciler Service

A production-grade reconciler service that continuously monitors and reconciles the desired state of domains, deployments, builds, and VMs with their actual state in the database.

## Overview

The reconciler runs in a continuous loop, ensuring that the system converges to its desired state by:

- Verifying domain ownership via DNS
- Managing deployment lifecycle transitions
- Monitoring build timeouts
- Maintaining a pool of ready VMs

## Architecture

The reconciler operates on four main resources:

### 1. Domain Reconciliation

**Purpose**: Verify domain ownership via DNS TXT records

**Flow**:

1. Query domains where `verified_at IS NULL` and `updated_at` within last 48 hours
2. Look up DNS TXT record: `{base58(domain.id)}-zeitwork.{domain.name}`
3. If record contains verification token, mark domain as verified

**Example DNS record**:

```
xyz123-zeitwork.example.com. TXT "abc123verificationtoken"
```

### 2. Deployment Reconciliation

**Purpose**: Manage deployment lifecycle from queued → ready

**State Machine**:

```
queued → building → ready → inactive
                  ↓
                failed
```

**State Transitions**:

**Queued → Building**:

- No `build_id` assigned
- Creates new build with status "queued"
- Updates deployment with `build_id`

**Building (waiting for image)**:

- Has `build_id` but no `image_id`
- Monitors build status
- If build ready: copies `image_id` to deployment
- If build error/canceled: marks deployment as failed

**Building (waiting for VM)**:

- Has `image_id` but no `vm_id`
- Looks for available pool VM
- If available: assigns VM, transitions to ready
- If unavailable: waits for VM pool to refill

**Ready (supersession check)**:

- Checks for newer deployments in same project+environment
- If newer deployment ready for 5+ minutes: marks old deployment as inactive

**Inactive/Failed (cleanup)**:

- Returns VM to pool
- Clears `vm_id` from deployment

### 3. Build Reconciliation

**Purpose**: Monitor build timeouts

**Flow**:

1. Query builds in "building" state for 10+ minutes
2. Mark as "error" (timeout)
3. Cascade effect: deployment transitions to failed

### 4. VM Reconciliation

**Purpose**: Maintain pool of ready VMs

**Flow**:

1. Count VMs with status "pooling"
2. If count < configured pool size: create new VMs
3. Distribute new VMs across regions (round-robin)

**VM Lifecycle**:

- `pooling` → Available for assignment
- `initializing` → Being created
- `starting` → Assigned to deployment, booting
- `running` → Actively serving deployment
- `stopping` → Being shut down
- `deleting` → Being removed
- `off` → Stopped but not deleted

## Configuration

Environment variables:

| Variable                             | Required | Default | Description                                 |
| ------------------------------------ | -------- | ------- | ------------------------------------------- |
| `RECONCILER_DATABASE_URL`            | Yes      | -       | PostgreSQL connection string                |
| `RECONCILER_INTERVAL`                | No       | `5s`    | How often to run reconciliation loop        |
| `RECONCILER_VM_POOL_SIZE`            | No       | `3`     | Minimum number of VMs in pool               |
| `RECONCILER_BUILD_TIMEOUT`           | No       | `10m`   | Build timeout duration                      |
| `RECONCILER_DEPLOYMENT_GRACE_PERIOD` | No       | `5m`    | Grace period before superseding deployments |
| `RECONCILER_LOG_LEVEL`               | No       | `info`  | Log level: debug, info, warn, error         |

## Building

```bash
# Build the binary
go build -o reconciler ./cmd/reconciler

# Or build with optimizations
go build -ldflags="-s -w" -o reconciler ./cmd/reconciler
```

## Running

```bash
# Set environment variables
export RECONCILER_DATABASE_URL="postgres://user:pass@localhost:5432/zeitwork"
export RECONCILER_INTERVAL="5s"
export RECONCILER_VM_POOL_SIZE="3"
export RECONCILER_LOG_LEVEL="info"

# Run the service
./reconciler
```

## Docker

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o reconciler ./cmd/reconciler

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/reconciler .
CMD ["./reconciler"]
```

## Reconciliation Logic Details

### Domain Verification

The reconciler uses Base58-encoded domain IDs in DNS TXT records to prevent confusion with similar-looking characters:

```go
// Expected DNS record format
txtRecordName := fmt.Sprintf("%s-zeitwork.%s", base58.EncodeUUID(domainID), domainName)
// Example: "5HpjkF8qKvW-zeitwork.example.com"
```

### Deployment Supersession

When multiple deployments exist for the same project+environment:

1. Newest deployment is always kept active
2. Second-newest deployment has 5-minute grace period
3. Older deployments (N-2, N-3, etc.) are immediately marked inactive

This allows for:

- Zero-downtime deployments
- Quick rollback capability
- Automatic cleanup of old deployments

### VM Pool Management

The reconciler maintains a pool of ready VMs to enable fast deployment:

- **Pool size**: Configurable minimum number of VMs
- **Distribution**: Round-robin across regions
- **IP allocation**: Uses 10.77.0.0/16 range with /29 subnets per VM
- **Lifecycle**: VMs can be reused or marked for deletion based on policy

## Observability

The reconciler emits structured JSON logs with the following levels:

- **debug**: Individual reconciliation decisions, DNS lookups, state checks
- **info**: State transitions, VM creation, domain verification
- **warn**: Build timeouts, missing resources, health check failures
- **error**: Database errors, failed state transitions

### Example Log Output

```json
{
  "time": "2025-10-19T12:34:56.789Z",
  "level": "INFO",
  "msg": "deployment transitioned to ready",
  "deployment_id": "01234567-89ab-cdef-0123-456789abcdef",
  "vm_id": "fedcba98-7654-3210-fedc-ba9876543210"
}
```

## Error Handling

The reconciler is designed to be resilient:

- **Database errors**: Logged and skipped, reconciliation continues
- **DNS lookup failures**: Logged and skipped, retry on next cycle
- **State inconsistencies**: Logged for investigation, safe defaults applied
- **Idempotency**: All operations can be safely retried

## Performance Considerations

- **Reconciliation interval**: 5 seconds by default, adjust based on load
- **Batch operations**: Processes all resources of each type together
- **Database queries**: Optimized with proper indexes
- **Memory footprint**: Minimal, processes resources in streaming fashion

## Dependencies

- `github.com/jackc/pgx/v5` - PostgreSQL driver
- `github.com/caarlos0/env/v11` - Environment variable parsing
- `internal/shared/uuid` - UUID utilities
- `internal/shared/base58` - Base58 encoding for domain verification
