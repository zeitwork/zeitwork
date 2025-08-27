# Zeitwork Implementation Tasks

This document outlines incomplete and inconsistent implementations that need to be addressed to achieve full architecture compliance and production readiness.

## 1. Database Schema Inconsistencies

### GitHub Repository Integration Missing

**Files:** `packages/database/schema/software.ts`, `internal/database/queries/projects.sql`

The projects table is missing critical GitHub integration fields:

```sql
-- In internal/database/queries/projects.sql:22-24
-- TODO: For now, return empty set. Need to add github_repo field to projects table
SELECT * FROM projects WHERE FALSE;
```

**Required Changes:**

- Add `githubRepo` field to projects table in `packages/database/schema/software.ts`
- Add `githubInstallationId` field to link with GitHub App installations
- Update `ProjectFindByGitHubRepo` query in `internal/database/queries/projects.sql`
- Add database migration for existing projects

### Missing Database Queries

**File:** `internal/database/queries/routing.sql`

The routing cache queries are incomplete:

- Missing `RoutingCacheUpsert` implementation (referenced in `internal/operator/deployments.go:324`)
- Missing `InstanceFindByDeployment` query (referenced in `internal/operator/deployments.go:305`)

## 2. API Inconsistencies

### Inconsistent URL Patterns

**Files:** `internal/operator/service.go`, `internal/node-agent/service.go`

```go
// Operator API (correct pattern):
mux.HandleFunc("GET /api/v1/nodes", s.listNodes)

// Node Agent API (inconsistent pattern):
mux.HandleFunc("POST /instances", s.handleCreateInstance)
```

**Required Changes:**

- Standardize all internal APIs to use `/api/v1/` prefix
- Update node agent routes in `internal/node-agent/service.go:241-249`

### Missing API Error Handling

**Files:** Multiple API handlers in `internal/api/`

Inconsistent error responses across endpoints:

```go
// Some endpoints return proper JSON errors
json.NewEncoder(w).Encode(map[string]string{"error": "message"})

// Others use plain text
http.Error(w, "Internal server error", http.StatusInternalServerError)
```

**Required Changes:**

- Standardize error response format across all API endpoints
- Add proper error codes and messages
- Implement request validation middleware

## 3. Domain Routing Implementation Gaps

### Incomplete Routing Logic

**File:** `internal/edge-proxy/service.go:346`

```go
// Route based on domain to specific instance
if backend := s.routeByDomain(r.Host); backend != nil {
    // Create a custom proxy for this specific backend
    proxy := httputil.NewSingleHostReverseProxy(backend)
    // ...
}
```

**Issues:**

- `routeByDomain` method exists but routing logic is incomplete
- No integration with `routing_cache` database table
- Missing domain-to-instance mapping logic

**Required Changes:**

- Implement full domain routing in `internal/edge-proxy/service.go`
- Add database queries to fetch routing information
- Implement cache invalidation when deployments change

### Missing DNS Record Updates

**File:** `internal/operator/deployments.go:288`

```go
func (dm *DeploymentManager) updateDNSRecords(ctx context.Context, deployment *database.Deployment) error {
    // In a real implementation, this would update Route53 or similar
    // For now, just update the routing cache in the database
    // ...
}
```

**Required Changes:**

- Implement actual DNS record management (Route53, CloudFlare, etc.)
- Add DNS provider configuration
- Implement domain verification process

## 4. Resource Management Issues

### Database Connection Pool Exhaustion

**Files:** `internal/operator/deployments.go`, `internal/operator/scaling.go`

Multiple services create new database connections per operation:

```go
// In internal/operator/deployments.go:81
db, err := database.NewDB(dm.service.config.DatabaseURL)
if err != nil {
    dm.service.logger.Error("Failed to connect to database", "error", err)
    return
}
defer db.Close()
```

**Required Changes:**

- Reuse service-level database connections
- Implement proper connection pooling
- Add connection health checks

### Placeholder Metrics Implementation

**Files:** `internal/operator/scaling.go:442`, `internal/shared/health/monitor.go:421`

Current metrics are hardcoded placeholders:

```go
// In internal/operator/scaling.go:442-454
func (sm *ScalingManager) getInstanceMetrics(ctx context.Context, instances []*database.Instance) []InstanceMetrics {
    // TODO: Implement actual metrics collection from monitoring system
    var metrics []InstanceMetrics
    for _, inst := range instances {
        metrics = append(metrics, InstanceMetrics{
            InstanceID:   uuid.UUID(inst.ID.Bytes).String(),
            CPUUsage:     50.0, // Placeholder
            MemoryUsage:  60.0, // Placeholder
            RequestCount: 100,  // Placeholder
            Healthy:      inst.State == "running",
        })
    }
    return metrics
}
```

**Required Changes:**

- Implement actual metrics collection from node agents
- Add Prometheus/monitoring system integration
- Implement real-time resource usage tracking

## 5. Security Vulnerabilities

### Missing Input Validation

**Files:** All API handlers in `internal/api/`

Limited validation across API endpoints:

```go
// In internal/api/projects.go:84
if req.Name == "" || req.Slug == "" || req.OrganizationID == "" {
    http.Error(w, "name, slug, and organization_id are required", http.StatusBadRequest)
    return
}
```

**Missing:**

- SQL injection prevention
- XSS protection
- Input sanitization
- Length limits on string fields
- Format validation for emails, URLs, etc.

### Authentication Gaps

**File:** `internal/api/auth.go`

Basic JWT implementation missing enterprise features:

- Token revocation mechanism
- Rate limiting on authentication endpoints
- Brute force protection
- Session management improvements

**Required Changes:**

- Implement token blacklisting/revocation
- Add rate limiting middleware
- Implement account lockout policies
- Add 2FA support

## 6. Build and Deployment Gaps

### Incomplete GitHub Webhook Processing

**File:** `internal/api/webhooks.go:158`

```go
// TODO: Fix this when github_repo field is added to projects table
// For now, return empty set
projects, err := s.db.Queries().ProjectFindByGitHubRepo(ctx)
```

**Required Changes:**

- Complete GitHub webhook processing
- Implement automatic build triggering
- Add support for multiple branches
- Implement PR preview deployments

### Missing Build Queue Processing

**Files:** `internal/operator/`, `internal/node-agent/`

Build queue exists in database but no processing:

- No build queue consumer implementation
- No build status updates
- No build artifact management

**Required Changes:**

- Implement build queue processor in operator
- Add build status tracking
- Implement build artifact cleanup

## 7. Monitoring and Observability

### Incomplete Health Checks

**File:** `internal/shared/health/monitor.go:369`

```go
// TODO: Implement actual health check (HTTP request to instance)
// For now, mark as healthy based on last update time
if instance.UpdatedAt.Time.Before(time.Now().Add(-5 * time.Minute)) {
    // Instance hasn't been updated in 5 minutes, might be unhealthy
}
```

**Required Changes:**

- Implement actual HTTP health checks for instances
- Add application-level health endpoints
- Implement circuit breaker patterns

### Missing Distributed Tracing

**Files:** All service files

No distributed tracing implementation:

- No request correlation IDs
- No trace propagation between services
- No performance monitoring

**Required Changes:**

- Add OpenTelemetry integration
- Implement request tracing
- Add performance metrics collection

## 8. Configuration Management

### Hardcoded Service URLs

**File:** `internal/shared/health/monitor.go:569`

```go
func getServiceURL(service string) string {
    // In production, this would come from service discovery
    switch service {
    case "operator":
        return "localhost:8080"
    case "load-balancer":
        return "localhost:8082"
    // ...
    }
}
```

**Required Changes:**

- Implement service discovery mechanism
- Add configuration for service endpoints
- Support dynamic service registration

### Missing Environment-Specific Configurations

**Files:** `internal/shared/config/config.go`

Limited environment handling:

- No staging/production configuration differences
- Missing feature flags
- No configuration validation

## 9. Error Handling and Logging

### Inconsistent Error Patterns

**Files:** Multiple service files

Mixed error handling approaches:

```go
// Some places wrap errors properly
return fmt.Errorf("failed to create instance: %w", err)

// Others don't provide context
return err
```

**Required Changes:**

- Standardize error handling patterns
- Add error codes and categorization
- Implement error aggregation and reporting

### Missing Structured Logging Context

**Files:** All service files

Logging lacks consistent context:

- Missing correlation IDs
- Inconsistent field naming
- No log aggregation configuration

## 10. Performance and Scalability

### Synchronous Operations in Critical Paths

**File:** `internal/operator/scaling.go:86`

Blocking operations in monitoring loops:

```go
for _, deployment := range deployments {
    // This could block the entire monitoring cycle
    instances, err := db.Queries().InstanceFindByDeployment(ctx, deployment.ID)
    // Process each deployment synchronously
}
```

**Required Changes:**

- Implement concurrent processing
- Add timeout controls
- Implement backpressure mechanisms

### Missing Caching Layer

**Files:** `internal/edge-proxy/service.go`, `internal/operator/`

No caching implementation:

- Domain routing lookups hit database every time
- No Redis or in-memory caching
- No cache invalidation strategy

**Required Changes:**

- Implement Redis caching layer
- Add cache invalidation logic
- Implement cache warming strategies

## Priority Levels

### High Priority (Blocks Production)

1. Database schema fixes for GitHub integration
2. Domain routing implementation completion
3. Database connection pool management
4. Input validation and security hardening

### Medium Priority (Improves Reliability)

1. Metrics implementation replacement
2. Health check completion
3. Error handling standardization
4. Build queue processing

### Low Priority (Technical Debt)

1. API URL consistency
2. Logging improvements
3. Configuration management
4. Performance optimizations
