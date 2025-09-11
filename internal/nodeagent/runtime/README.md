# Firecracker Runtime Implementation Plan

The Firecracker runtime is responsible for managing VM lifecycle on worker nodes, providing secure isolation for containerized applications using Firecracker microVMs.

## Architecture Overview

Based on the platform architecture, each VM instance runs:

- **Firecracker microVM**: Provides isolation and resource limits (2 vCPU, 2GB RAM)
- **Jailer**: Security sandbox for the Firecracker process
- **Container Runtime**: Runs the application container inside the VM
- **VM Network Agent**: Handles networking and health reporting

## Core Components

### 1. VM Lifecycle Manager

- **Create**: Initialize new Firecracker VM instances
- **Start**: Boot VMs and start application containers
- **Stop**: Gracefully shutdown VMs
- **Destroy**: Clean up VM resources
- **Health Monitoring**: Continuous health checks and reporting

### 2. Network Configuration

- **IPv6 Assignment**: Each VM gets isolated IPv6 address from database
- **IP Management**: Read from database `instance.ip_address` field, generate unique IP if null
- **IP Uniqueness**: Check database for existing IPs before assignment
- **Network Isolation**: VMs cannot communicate with each other
- **Host Network Bridge**: Connect VMs to external network
- **Traffic Routing**: Enable inbound/outbound connectivity

### 3. Image Management

- **OCI Image Pulling**: Download container images from distribution registry

### 4. Resource Management

- **CPU/Memory Limits**: Enforce 2 vCPU, 2GB RAM per VM via Firecracker
- **Storage Allocation**: Manage VM disk space and container volumes
- **Resource Monitoring**: Track usage and report metrics DEFER

### 5. Security & Isolation

- **Jailer Integration**: Run all VMs in secure sandboxes
- **Network Segmentation**: Prevent inter-VM communication
- **Host Protection**: Isolate VMs from host system

## Implementation Phases

### Phase 1: Core VM Lifecycle

**Goal**: Basic VM create/start/stop/destroy functionality

**Components**:

- Firecracker API client
- VM configuration management
- Basic jailer integration
- Simple health checking

**Deliverables**:

- Can create and start a basic VM
- Can stop and destroy VMs
- Basic error handling and logging

### Phase 2: Networking Foundation

**Goal**: Isolated networking per VM with external connectivity

**Components**:

- Database-driven IPv6 address allocation
- IP uniqueness validation against database
- TAP device management
- Bridge networking setup
- Network namespace isolation

**Deliverables**:

- VMs get unique IPv6 addresses
- VMs can reach external services
- VMs are isolated from each other

### Phase 3: Container Runtime Integration

**Goal**: Run application containers inside VMs

**Components**:

- OCI image fetching from S3
- Container runtime (likely containerd or Docker)
- Image caching and management
- Container lifecycle within VM

**Deliverables**:

- Can pull and cache container images
- Can start application containers in VMs
- Basic container health monitoring

### Phase 4: Advanced Features

**Goal**: Production-ready reliability and monitoring

**Components**:

- Comprehensive health monitoring
- Metrics collection and reporting
- Resource usage tracking
- Graceful shutdown handling
- Error recovery mechanisms

**Deliverables**:

- Full health reporting to Management API
- Detailed metrics and logging
- Robust error handling
- Production-ready reliability

## Key Design Decisions

### VM Base Image

- **Minimal Linux**: Use Alpine or similar minimal distribution
- **Container Runtime**: Pre-installed containerd or Docker
- **System Services**: Minimal set for container operation
- **Network Agent**: Custom agent for health reporting and container management

### Networking Stack

- **Host Bridge**: Single bridge for all VMs on node
- **IPv6 Only**: Simplifies networking, provides unique addresses
- **Database-Driven IPs**: IP addresses managed in database for consistency
- **IP Generation**: Generate unique IPs when instance.ip_address is null
- **No Inter-VM Communication**: Strict network isolation
- **External Access**: VMs can reach internet and platform services

### Resource Management

- **Firecracker Limits**: Hard limits enforced by hypervisor
- **No Swap**: Apps crash when out of memory (by design)
- **Fixed Resources**: All VMs get same resource allocation
- **Efficient Packing**: Multiple VMs per physical node

### Security Model

- **Jailer Mandatory**: All VMs run in jailer sandbox
- **Host Isolation**: VMs cannot access host filesystem
- **Network Isolation**: Each VM in separate network namespace
- **Image Verification**: Verify image signatures (future enhancement)

## File Structure

```
internal/nodeagent/runtime/firecracker/
├── client.go              # Firecracker API client
├── lifecycle.go           # VM lifecycle management
├── network.go             # Networking setup and management
├── image.go               # OCI image handling
├── health.go              # Health monitoring
├── jailer.go              # Jailer integration
├── resources.go           # Resource management
└── vm_config.go           # VM configuration templates
```

## Dependencies

- **Firecracker Binary**: Core hypervisor
- **Jailer Binary**: Security sandbox
- **Firecracker Go SDK**: https://github.com/firecracker-microvm/firecracker-go-sdk
- **Container Runtime**: For running app containers
- **Network Tools**: ip, iptables for networking setup
- **Image Tools**: For OCI image manipulation

## Success Criteria

- [x] Can create/start/stop/destroy VMs reliably
- [x] VMs are properly isolated with IPv6 networking
- [x] Jailer security sandbox working (UID/GID isolation, chroot jail)
- [ ] Can run application containers inside VMs
- [ ] Health monitoring reports to Management API
- [x] Resource limits are enforced
- [x] Production-ready error handling and logging
