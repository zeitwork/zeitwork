# TODO List - Zeitwork Platform

This document tracks all TODO items found in the codebase, organized by component with detailed explanations of what needs to be done.

## Table of Contents

- [API & Documentation](#api--documentation)
- [Operator Service](#operator-service)
- [Node Agent Service](#node-agent-service)
- [Archive](#archive)

---

## API & Documentation

### 1. Build an image from GitHub endpoint

**Location:** `README.md:143`

```bash
# Build an image from GitHub (TODO)
curl -X POST http://localhost:8080/api/v1/images \
```

**What needs to be done:**

- Implement the complete API endpoint for building images from GitHub repositories
- Add support for GitHub authentication (OAuth, personal access tokens)
- Implement webhook handling for automatic builds on push
- Add build queue management (use postgres "skip locked")
- Handle build status updates and notifications
- We need to build the images using Docker running in a Firecracker VM (2vcpus, 4GB ram)

### 2. Metrics endpoint

**Location:** `README.md:175`

```
- `/metrics` - Metrics endpoint (TODO)
```

**What needs to be done:**

- Implement Prometheus-compatible metrics endpoint
- Export key metrics: request counts, latency, error rates, resource usage
- Add custom business metrics (deployments, instances, active users)
- Configure metric collection middleware
- Document available metrics

---

## Operator Service

### 3. Node registration notification

**Location:** `internal/operator/nodes.go:123`

```go
// TODO: Notify the node agent on the new node to register itself
```

**What needs to be done:**

- Implement async notification system to contact new nodes
- Create registration protocol between operator and node agents
- Add retry logic for failed registration attempts
- Implement health checks to verify successful registration
- Add timeout handling for unresponsive nodes

### 4. Image creation endpoint

**Location:** `internal/operator/images.go:52`

```go
// TODO: Implement image creation
```

**What needs to be done:**

- Implement logic to build container images from source
- Implement build caching and layer optimization
- Add image validation and security scanning
- Store image metadata in database
- Handle build failures gracefully

### 5. Image status update endpoint

**Location:** `internal/operator/images.go:58`

```go
// TODO: Implement image status update
```

**What needs to be done:**

- Implement status tracking for image builds (building, ready, failed)
- Add webhook notifications for status changes
- Update database with current status
- Implement status history tracking
- Add support for progress updates during builds

### 6. Image deletion endpoint

**Location:** `internal/operator/images.go:64`

```go
// TODO: Implement image deletion
```

**What needs to be done:**

- Implement safe image deletion with dependency checking
- Verify no active deployments use the image
- Clean up image artifacts from storage
- Remove image metadata from database
- Add soft-delete option for recovery
- Implement cascade deletion for related resources

### 7. Deployment creation endpoint

**Location:** `internal/operator/deployments.go:52`

```go
// TODO: Implement deployment creation
```

**What needs to be done:**

- Implement deployment orchestration logic
- Add support for different deployment strategies (rolling, blue-green)
- Implement resource allocation and scheduling
- Add deployment validation and pre-flight checks
- Create deployment rollback capabilities
- Store deployment configuration in database

### 8. Deployment status update endpoint

**Location:** `internal/operator/deployments.go:58`

```go
// TODO: Implement deployment status update
```

**What needs to be done:**

- Track deployment lifecycle states (pending, running, succeeded, failed)
- Implement real-time status updates from instances
- Add deployment health monitoring
- Implement automatic rollback on failure
- Store status history for audit trail

### 9. IP address allocation

**Location:** `internal/operator/instances.go:144`

```go
IpAddress: "10.0.0.2", // TODO: Allocate IP address properly
```

**What needs to be done:**

- Have a look at docs/decisions/2025-08-25-routing.md
- Add support for IPv6 addresses
- Prevent IP conflicts and duplicates
- Track IP usage and availability

### 10. Send instance creation request to node agent

**Location:** `internal/operator/instances.go:155`

```go
// TODO: Send request to node agent to actually create the instance
```

**What needs to be done:**

- Implement HTTP client to communicate with node agents
- Add request queuing and retry logic
- Implement timeout handling
- Add response validation
- Handle partial failures and rollback
- Log all communication for debugging

### 11. Send instance deletion request to node agent

**Location:** `internal/operator/instances.go:225`

```go
// TODO: Send request to node agent to stop and remove the instance
```

**What needs to be done:**

- Implement instance deletion protocol with node agent
- Add graceful shutdown with configurable timeout
- Clean up associated resources (volumes, network)
- Update database to reflect deletion
- Handle force deletion for stuck instances
- Implement deletion confirmation/safety checks

---

## Node Agent Service

### 12. Get actual CPU count

**Location:** `internal/node-agent/instances.go:171` and `internal/node-agent/service.go:167`

```go
"vcpu": 4, // TODO: Get actual CPU count
```

**What needs to be done:**

- Implement system resource detection using runtime.NumCPU()
- Account for CPU limits set by cgroups/containers
- Support CPU overcommit configurations

### 13. Get actual memory

**Location:** `internal/node-agent/instances.go:172` and `internal/node-agent/service.go:168`

```go
"memory": 8192, // TODO: Get actual memory
```

**What needs to be done:**

- Read system memory from /proc/meminfo or syscalls
- Account for memory reservations and limits
- Convert between different units (MB, MiB, GB)
- Cache memory information appropriately
- Handle memory hotplug scenarios

### 14. Download and prepare images

**Location:** `internal/node-agent/instances.go:200`

```go
// TODO: Download and prepare the image
```

**What needs to be done:**

- Implement image pull from registry
- Add image caching to avoid redundant downloads
- Verify image checksums and signatures
- Extract and prepare rootfs for Firecracker
- Implement progress reporting during download

### 15. Create Firecracker configuration

**Location:** `internal/node-agent/instances.go:201`

```go
// TODO: Create Firecracker configuration
```

**What needs to be done:**

- Generate Firecracker VM configuration JSON
- Configure boot source (kernel, initrd)
- Set up drive configurations for rootfs
- Configure network interfaces
- Set resource limits (CPU, memory)
- Add metadata service configuration

### 16. Start Firecracker process

**Location:** `internal/node-agent/instances.go:202`

```go
// TODO: Start Firecracker process
```

**What needs to be done:**

- Spawn Firecracker process with proper arguments
- Set up Unix domain socket for API communication
- Configure logging and metrics collection
- Implement process monitoring and restart
- Handle startup failures and errors
- Set up cgroups for resource isolation

### 17. Get actual Firecracker PID

**Location:** `internal/node-agent/instances.go:207`

```go
PID: 12345, // TODO: Get actual PID
```

**What needs to be done:**

- Capture process ID when starting Firecracker
- Store PID for process management
- Implement PID file handling
- Add process existence validation
- Handle PID recycling edge cases

### 18. Send shutdown command to Firecracker

**Location:** `internal/node-agent/instances.go:220`

```go
// TODO: Send shutdown command to Firecracker
```

**What needs to be done:**

- Implement Firecracker API client for shutdown
- Send InstanceActionInfo with action_type: "SendCtrlAltDel"
- Handle shutdown confirmation
- Implement shutdown status monitoring
- Add logging for shutdown events

### 19. Wait for graceful shutdown

**Location:** `internal/node-agent/instances.go:221`

```go
// TODO: Wait for graceful shutdown
```

**What needs to be done:**

- Implement configurable shutdown timeout
- Monitor Firecracker process status
- Check for clean VM termination
- Handle partial shutdown scenarios
- Collect shutdown logs and metrics

### 20. Force kill if necessary

**Location:** `internal/node-agent/instances.go:222`

```go
// TODO: Force kill if necessary
```

**What needs to be done:**

- Implement force termination after timeout
- Use SIGKILL to terminate unresponsive processes
- Clean up orphaned resources
- Log force termination events
- Handle zombie processes

### 21. Calculate available vCPU resources

**Location:** `internal/node-agent/service.go:221`

```go
"vcpu_available": 4, // TODO: Calculate available resources
```

**What needs to be done:**

- Track vCPU allocation across all instances
- Implement resource accounting system
- Handle CPU overcommit ratios
- Update availability in real-time
- Consider CPU shares and limits

### 22. Calculate available memory

**Location:** `internal/node-agent/service.go:222`

```go
"memory_available": 4096, // TODO: Calculate available memory
```

**What needs to be done:**

- Track memory usage across all instances
- Account for system overhead and buffers
- Implement memory overcommit policies
- Update availability metrics regularly
- Handle memory pressure scenarios

### 23. Implement actual instance stopping

**Location:** `internal/node-agent/service.go:282`

```go
// TODO: Implement actual instance stopping
```

**What needs to be done:**

- Send stop command to Firecracker API
- Implement graceful shutdown sequence
- Clean up instance resources
- Update instance state in database
- Handle stop failures and retries
- Notify operator of status changes

### 24. Implement actual IP address detection

**Location:** `internal/node-agent/service.go:289`

```go
// TODO: Implement actual IP address detection
```

**What needs to be done:**

- Detect primary network interface
- Get IP address from network configuration
- Handle multiple network interfaces
- Support both IPv4 and IPv6
- Cache IP address information
- Handle dynamic IP changes

---

## Archive

### 25. Calculate vCPU availability based on running instances

**Location:** `_archive_/cmd/node_manager.go:199`

```go
node.Resources.VCPUAvailable = cpuCount // TODO: Calculate based on running instances
```

**What needs to be done:**

- Query all running instances on the node
- Sum allocated vCPUs across instances
- Subtract from total to get available
- Account for system reserved resources
- Update calculation on instance changes

### 26. Calculate memory availability based on running instances

**Location:** `_archive_/cmd/node_manager.go:209`

```go
node.Resources.MemoryMiBAvailable = memMiB // TODO: Calculate based on running instances
```

**What needs to be done:**

- Query memory allocation for all instances
- Calculate total memory usage
- Account for memory overhead per instance
- Update availability metrics
- Handle memory fragmentation

---

## Priority Classification

### Critical (P0)

- IP address allocation (#9)
- Instance creation/deletion communication (#10, #11)
- Firecracker configuration and startup (#15, #16)
- Resource detection and calculation (#12, #13, #21, #22)

### High (P1)

- Image management endpoints (#4, #5, #6)
- Deployment management endpoints (#7, #8)
- Instance lifecycle management (#14, #18, #19, #20, #23)
- Node registration (#3)

### Medium (P2)

- Metrics endpoint (#2)
- IP address detection (#24)
- GitHub build integration (#1)

### Low (P3)

- Archive items (#25, #26)

---

## Next Steps

1. **Resource Management Foundation**: Implement actual resource detection and tracking (CPU, memory) as this is fundamental to the platform
2. **Communication Layer**: Establish reliable operator-to-node-agent communication for instance management
3. **Firecracker Integration**: Complete the Firecracker VM lifecycle management
4. **API Completeness**: Implement remaining API endpoints for images and deployments
5. **Monitoring & Observability**: Add metrics and health check endpoints

## Notes

- Many TODOs are interconnected and should be addressed in logical groups
- Resource management TODOs should be tackled first as they're foundational
- Consider implementing a proper task queue system for async operations
- Add comprehensive error handling and logging alongside TODO implementations
