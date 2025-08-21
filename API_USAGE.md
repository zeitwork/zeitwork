# Firecracker Manager API

This is a REST API server for managing Firecracker VMs across multiple nodes.

## Running the Server

```bash
./firecracker-manager
```

The server will start on port 8080 by default.

## API Endpoints

### Nodes Management

#### List all nodes

```bash
GET /nodes
```

Returns a list of all registered nodes (remote machines).

#### Add a new node

```bash
POST /nodes/add
Content-Type: application/json

{
  "name": "node-1",
  "host": "192.168.1.100",
  "port": 22,
  "ssh_key_path": "/path/to/ssh/key"  # Optional, will generate if not provided
}
```

### Images Management

#### List all images

```bash
GET /images
```

Returns a list of all available VM images.

#### Build an image from GitHub repository

```bash
POST /images/build
Content-Type: application/json

{
  "github_repo": "owner/repository",
  "tag": "main",  # Optional, defaults to main branch
  "name": "my-app"  # Optional, defaults to repo name
}
```

This endpoint will:

1. Clone the specified GitHub repository
2. Look for Dockerfile, Containerfile, go.mod, or package.json
3. Build the appropriate container/application image
4. Store it for use by Firecracker VMs

The build happens asynchronously. Check the image status in the response or by listing images.

### Instances Management

#### List all instances

```bash
GET /instances
```

Returns a list of all VM instances across all nodes.

#### Create a new VM instance

```bash
POST /instances
Content-Type: application/json

{
  "node_id": "node-123456",
  "image_id": "img-789012",
  "vcpu_count": 2,      # Optional, defaults to 1
  "memory_mib": 256,    # Optional, defaults to 128
  "default_port": 3000  # Optional, the port the application listens on (used for proxy setup)
}
```

Creates a new Firecracker VM instance with the specified image on the specified node.
The `default_port` parameter specifies which port the application inside the container will be listening on.
This port will be used as the default remote port when setting up SSH proxy connections to the instance.

### Instance Logs

#### Get instance logs

```bash
GET /instances/{instance_id}/logs
```

Returns all Firecracker logs for the specified instance, including boot logs and runtime messages.

### Instance Proxy Management

#### Setup SSH proxy for an instance

```bash
POST /instances/{instance_id}/proxy
Content-Type: application/json

{
  "remote_port": 8080,  # Optional, overrides instance's default_port
  "local_port": 9001    # Optional, specifies local port for tunnel
}
```

Sets up an SSH tunnel to forward traffic from a local port to the application running in the instance.
If `remote_port` is not specified, it will use the instance's `default_port` (if set) or default to 8080.
If `local_port` is not specified, it will be automatically allocated starting from 9000.

#### Get proxy information

```bash
GET /instances/{instance_id}/proxy
```

Returns the current proxy configuration for the instance.

#### Delete proxy

```bash
DELETE /instances/{instance_id}/proxy
```

Tears down the SSH tunnel for the instance.

### Health Check

```bash
GET /health
```

Returns the health status of the API server.

## Example Workflow

1. **Add a node** to manage:

```bash
curl -X POST http://localhost:8080/nodes/add \
  -H "Content-Type: application/json" \
  -d '{"name": "my-server", "host": "192.168.1.100"}'
```

2. **Build an image** from a GitHub repository:

```bash
curl -X POST http://localhost:8080/images/build \
  -H "Content-Type: application/json" \
  -d '{"github_repo": "golang/example", "name": "go-example"}'
```

3. **List images** to get the image ID:

```bash
curl http://localhost:8080/images
```

4. **Create a VM instance** with the image:

```bash
curl -X POST http://localhost:8080/instances \
  -H "Content-Type: application/json" \
  -d '{"node_id": "node-xxx", "image_id": "img-yyy", "vcpu_count": 2, "memory_mib": 512}'
```

5. **List instances** to see running VMs:

```bash
curl http://localhost:8080/instances
```

## Architecture

The system consists of:

- **API Server**: HTTP REST API for managing the infrastructure
- **Nodes**: Remote machines that run Firecracker VMs
- **Images**: Container/application images built from GitHub repositories
- **Instances**: Firecracker VMs running on nodes with specific images

The server uses SSH to communicate with nodes and manage VMs remotely.

## Requirements

On the API server:

- Go 1.23+ (for building)
- SSH key for accessing nodes

On each node:

- Linux with KVM support
- Firecracker
- Containerd (for container operations)
- Root SSH access

The system will attempt to install missing dependencies automatically when adding nodes.
