# L4 → L7 Architecture Refactor

## Summary

We have successfully refactored the Zeitwork platform architecture to implement the correct traffic flow:

**Before (incorrect):**

```
User → Edge Proxy (L7) → Load Balancer (L4) → Worker Nodes
```

**After (correct):**

```
User → Load Balancer (L4) → Edge Proxy (L7) → Worker Nodes
```

## Why This Architecture is Better

### 1. High Availability

- Multiple Edge Proxies can run behind a single L4 Load Balancer
- If one Edge Proxy fails, traffic automatically routes to healthy ones
- No single point of failure for L7 processing

### 2. Horizontal Scaling

- Add more Edge Proxies as traffic grows
- L4 Load Balancer efficiently distributes TCP connections
- Edge Proxies can be scaled independently of Worker Nodes

### 3. Performance

- L4 load balancing is faster (operates at TCP level)
- TLS passthrough avoids double encryption
- Connection pooling between Edge Proxies and Workers

### 4. Separation of Concerns

- L4 Load Balancer: Simple TCP distribution
- L7 Edge Proxy: Complex HTTP routing, TLS termination
- Worker Nodes: Application execution

## Implementation Changes

### Load Balancer (L4)

**Changed from:** Discovering and routing to Worker Nodes
**Changed to:** Discovering and routing to Edge Proxies

```go
// Now discovers Edge Proxies instead of Workers
func (s *Service) discoverFromOperator(ctx context.Context) error {
    req, err := http.NewRequestWithContext(ctx, "GET",
        s.config.OperatorURL+"/api/v1/edge-proxies", nil)
    // ... discovers edge proxy instances
}
```

### Edge Proxy (L7)

**Changed from:** Proxying all traffic to Load Balancer
**Changed to:** Directly connecting to Worker Nodes

```go
// Now discovers and connects directly to Workers
func (s *Service) discoverBackends(ctx context.Context) {
    req, err := http.NewRequestWithContext(ctx, "GET",
        s.config.OperatorURL+"/api/v1/nodes?type=worker&state=ready", nil)
    // ... discovers worker node instances
}
```

## Traffic Flow

### 1. External Request Arrives

- User makes HTTPS request to `app.zeitwork.com`
- DNS resolves to L4 Load Balancer IP address

### 2. L4 Load Balancer Processing

- Receives TCP connection on port 443
- Selects healthy Edge Proxy using round-robin
- Passes through TLS traffic without inspection
- Maintains TCP connection to Edge Proxy

### 3. L7 Edge Proxy Processing

- Terminates TLS using Let's Encrypt certificate
- Reads HTTP headers (Host, Path, etc.)
- Applies rate limiting per client IP
- Routes based on domain to specific Worker
- Establishes mTLS connection to Worker

### 4. Worker Node Processing

- Validates Edge Proxy's client certificate
- Routes to specific VM instance
- Executes application logic
- Returns response through same path

## Service Discovery

### L4 Load Balancer Discovery

```
Operator API → /api/v1/edge-proxies
Returns: List of Edge Proxy instances
```

### L7 Edge Proxy Discovery

```
Operator API → /api/v1/nodes?type=worker
Returns: List of Worker Node instances
```

## Configuration Updates

### Environment Variables

**L4 Load Balancer:**

```bash
OPERATOR_URL=http://operator:8080  # To discover Edge Proxies
PORT=80                             # HTTP traffic
TLS_PORT=443                        # HTTPS traffic (passthrough)
```

**L7 Edge Proxy:**

```bash
OPERATOR_URL=http://operator:8080  # To discover Worker Nodes
DATABASE_URL=postgres://...        # For domain routing
SSL_CERT_PATH=/etc/ssl/cert.pem   # Let's Encrypt cert
ENABLE_MTLS=true                   # For Worker communication
```

## Benefits Achieved

1. **Scalability**: Can now scale Edge Proxies independently
2. **Reliability**: Multiple Edge Proxies provide redundancy
3. **Performance**: L4 distribution is more efficient
4. **Security**: mTLS between Edge Proxy and Workers
5. **Flexibility**: Easy to add/remove Edge Proxies

## Testing the New Architecture

```bash
# Start L4 Load Balancer (listens on :80/:443)
./build/zeitwork-load-balancer

# Start multiple Edge Proxies (discovered by L4)
EDGE_PROXY_PORT=8083 ./build/zeitwork-edge-proxy
EDGE_PROXY_PORT=8084 ./build/zeitwork-edge-proxy

# Start Worker Nodes (discovered by Edge Proxies)
NODE_PORT=8081 ./build/zeitwork-node-agent
NODE_PORT=8082 ./build/zeitwork-node-agent

# Test request flow
curl https://app.zeitwork.com
```

## Monitoring Points

- L4 Load Balancer: `/health` - Shows Edge Proxy backend health
- L7 Edge Proxy: `/health` - Shows Worker Node backend health
- Worker Node: `/health` - Shows VM instance health

## Future Enhancements

1. **TCP Multiplexing**: Use HTTP/2 between Edge Proxy and Workers
2. **Connection Pooling**: Persistent connections to Workers
3. **Geographic Load Balancing**: Route to closest Edge Proxy
4. **Circuit Breakers**: Automatic failure detection and recovery
5. **Request Tracing**: Distributed tracing across all layers
