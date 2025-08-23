# Zeitwork L4 Load Balancer

A Layer 4 (TCP) load balancer that distributes raw TCP connections across backend instances.

## Overview

The Zeitwork Load Balancer operates at the transport layer (Layer 4 of the OSI model), providing:

- Raw TCP connection forwarding without application-layer inspection
- Multiple load balancing algorithms (round-robin, least-connections, IP-hash)
- Automatic backend discovery via the Operator service
- TCP-based health checking
- Minimal latency overhead

## Features

### Layer 4 Operation

- **TCP Forwarding**: Direct TCP connection proxying without HTTP parsing
- **Protocol Agnostic**: Works with any TCP-based protocol (HTTP, HTTPS, WebSocket, gRPC, custom protocols)
- **Low Latency**: Minimal processing overhead as no application data inspection occurs
- **Connection Persistence**: Maintains TCP connection state between client and backend

### Load Balancing Algorithms

- **Round-Robin**: Distributes connections evenly across all healthy backends
- **Least-Connections**: Routes to the backend with the fewest active connections
- **IP-Hash**: Consistent routing based on client IP address for session affinity

### Health Checking

- TCP connection-based health checks (not HTTP)
- Configurable check intervals
- Automatic backend removal/addition based on health status

### Monitoring

- HTTP health endpoint for monitoring the load balancer itself (separate port)
- Backend status and connection metrics
- Real-time backend discovery from Operator service

## Configuration

The load balancer is configured via environment variables:

```bash
# TCP load balancing port
PORT=8082

# HTTP health check endpoint port (for monitoring the LB)
HEALTH_PORT=8083

# Logging
LOG_LEVEL=info
ENVIRONMENT=production

# Backend discovery
OPERATOR_URL=http://localhost:8080

# Algorithm: round-robin, least-connections, or ip-hash
LB_ALGORITHM=round-robin
```

## Architecture

```
Client → [L4 Load Balancer:8082] → Backend Instance
                    ↓
            [Health API:8083]
                    ↓
              Monitoring
```

### TCP Flow

1. Client establishes TCP connection to load balancer
2. Load balancer selects backend using configured algorithm
3. Load balancer establishes TCP connection to backend
4. Bidirectional byte stream copying between client and backend
5. Connection remains until either side closes

### Backend Discovery

- Periodically queries Operator service for running instances
- Updates backend pool dynamically
- No manual backend configuration required

## API Endpoints

The load balancer exposes HTTP endpoints on the health port (default 8083) for monitoring:

### GET /health

Returns the health status of the load balancer:

```json
{
  "status": "healthy",
  "type": "L4",
  "algorithm": "round-robin",
  "total_backends": 5,
  "healthy_backends": 4
}
```

### GET /backends

Returns the list of discovered backends:

```json
[
  {
    "id": "inst-123",
    "address": "10.0.1.5:8080",
    "healthy": true,
    "connections": 12,
    "last_check": "2024-01-15T10:30:00Z"
  }
]
```

## Usage

### Running the Load Balancer

```bash
# Set environment variables
export PORT=8082
export HEALTH_PORT=8083
export OPERATOR_URL=http://localhost:8080
export LB_ALGORITHM=round-robin

# Run the load balancer
./load-balancer
```

### Systemd Service

```bash
# Copy service file
sudo cp deployments/systemd/zeitwork-load-balancer.service /etc/systemd/system/

# Copy configuration
sudo cp deployments/config/load-balancer.env /etc/zeitwork/

# Start service
sudo systemctl enable zeitwork-load-balancer
sudo systemctl start zeitwork-load-balancer
```

## Performance Considerations

### L4 vs L7 Load Balancing

**Layer 4 (This Implementation)**

- ✅ Lower latency (no HTTP parsing)
- ✅ Higher throughput
- ✅ Protocol agnostic
- ✅ WebSocket/gRPC friendly
- ❌ No HTTP-specific features (path routing, headers)
- ❌ No request inspection or modification

**Layer 7 (HTTP Load Balancers)**

- ✅ HTTP-aware routing (path, headers, cookies)
- ✅ Request/response modification
- ✅ HTTP-specific health checks
- ❌ Higher latency
- ❌ Limited to HTTP/HTTPS

### Connection Pooling

- Each client connection maps to one backend connection
- No connection multiplexing
- Suitable for long-lived connections

### Resource Usage

- Minimal CPU usage (no parsing)
- Memory usage proportional to concurrent connections
- Network I/O bound rather than CPU bound

## Monitoring and Debugging

### Logs

The load balancer logs important events:

- Backend discovery updates
- Health check failures
- Connection errors
- Algorithm decisions (in debug mode)

### Metrics

Monitor via the health endpoint:

- Total vs healthy backends
- Connections per backend
- Health check status

### Troubleshooting

**No backends available**

- Check Operator service is running
- Verify OPERATOR_URL is correct
- Check instances are in "running" state

**Uneven load distribution**

- Verify algorithm setting
- Check for backend health issues
- Consider connection duration patterns

**High latency**

- Check network connectivity to backends
- Monitor backend response times
- Verify health check isn't too aggressive

## Development

### Building

```bash
go build -o load-balancer cmd/load-balancer/balancer.go
```

### Testing

```bash
# Start test backends
for i in 8001 8002 8003; do
  nc -l -k -p $i &
done

# Configure and start load balancer
export PORT=8000
export HEALTH_PORT=8083
./load-balancer

# Test connection
echo "test" | nc localhost 8000
```

## Security Considerations

- No TLS termination (pass-through for encrypted connections)
- No authentication/authorization (relies on network security)
- Recommended to run behind a firewall
- Use IP whitelisting for production deployments
