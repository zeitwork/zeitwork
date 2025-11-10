# Reconciler Docker Save/Load Workflow

## Overview

The reconciler uses a **docker save/load** workflow to deploy containers to VMs without requiring registry credentials on each VM. This provides better security and simplifies VM configuration.

## Architecture

```
┌─────────────┐
│ Reconciler  │ ← Has registry credentials
│   Service   │
└──────┬──────┘
       │
       │ 1. docker pull (with auth)
       │ 2. docker save → tar
       │ 3. SCP tar via IPv6
       ↓
┌─────────────┐
│  VM Server  │ ← No registry credentials needed!
│  (Hetzner)  │
└─────────────┘
       │
       │ 4. docker load < tar
       │ 5. docker run
       ↓
   Container running
```

## Workflow Steps

### 1. Pull Image Locally (Reconciler)

The reconciler pulls the image from the registry with credentials:

```go
func (s *Service) pullImageLocally(ctx context.Context, imageName string) error {
    // Authenticates to registry with username/password
    // Pulls image to reconciler's local Docker
    // Returns when pull is complete
}
```

**Example**: `ghcr.io/yourorg/app:v1.2.3`

### 2. Save Image to Tar (Reconciler)

Save the Docker image to a tar file:

```go
func (s *Service) saveImageToTar(ctx context.Context, imageName, tarPath string) error {
    // Uses Docker API to export image
    // Writes to /tmp/zeitwork-image-{vm-id}.tar
    // Logs file size for monitoring
}
```

**Tar location**: `/tmp/zeitwork-image-abc12345.tar`  
**Typical size**: 50-500MB depending on image

### 3. Transfer Tar to VM (SCP over IPv6)

Transfer the tar file to the VM via SSH:

```go
func (s *Service) transferFileToVM(ctx context.Context, vm *database.Vm, localPath, remotePath string) error {
    // Connects to VM via IPv6 SSH
    // Uses stdin pipe to stream file
    // Remote: cat > /tmp/image.tar
}
```

**Transfer method**: SCP-like using stdin pipe  
**Connection**: IPv6 only (no public IPv4)  
**Speed**: ~10-20 seconds for 100MB over Hetzner network

### 4. Load Image on VM

Load the tar into VM's local Docker registry:

```go
func (s *Service) loadImageOnVM(ctx context.Context, vm *database.Vm, remoteTarPath string) error {
    // SSH to VM
    // docker load -i /tmp/image.tar
    // rm /tmp/image.tar (cleanup)
}
```

**Result**: Image available in VM's local Docker without registry pull

### 5. Run Container

Run the container from locally loaded image:

```go
func (s *Service) runContainerOnVM(ctx context.Context, vm *database.Vm, imageName, containerName string) error {
    // docker run -d -p {port}:8080 --name {name} --restart unless-stopped {image}
    // No registry credentials needed!
}
```

## Security Benefits

### Before (Registry Pull on Each VM)

❌ Registry credentials on every VM  
❌ Credentials in environment variables  
❌ Risk if VM compromised  
❌ Credential rotation requires updating all VMs

### After (Docker Save/Load)

✅ Credentials only on reconciler  
✅ VMs never touch registry  
✅ Compromised VM can't access registry  
✅ Credential rotation in one place

## Performance Characteristics

### First Deployment (Cold)

1. Pull image on reconciler: **5-30s** (depends on image size, network)
2. Save to tar: **1-5s** (local disk I/O)
3. Transfer to VM: **10-60s** (depends on size, network)
4. Load on VM: **5-15s** (local disk I/O)
5. Run container: **1-2s**

**Total**: ~25-120 seconds for 100-500MB image

### Subsequent Deployments to Same VM

1. Pull (cached): **1-2s** (Docker layer cache)
2. Save to tar: **1-5s**
3. Transfer: **10-60s**
4. Load: **5-15s**
5. Run: **1-2s**

**Total**: ~20-90 seconds

### Concurrent Deployments

- Multiple VMs can be provisioned in parallel
- Each gets its own tar file copy
- No registry rate limiting concerns
- Limited only by reconciler's network bandwidth

## Disk Usage

### Reconciler

- Temporary tar files in `/tmp/`
- Cleaned up immediately after transfer
- One tar per concurrent deployment
- **Max usage**: ~500MB per concurrent deployment

### VMs

- Tar file removed after `docker load`
- Image stored in Docker's local registry
- **Max usage**: Image size in Docker storage

## Error Handling

### Pull Failures

- Logged as error
- Deployment marked as failed
- VM returned to pool
- Retried on next reconciliation cycle

### Transfer Failures

- SSH connection retried automatically
- Transfer failures logged
- Partial files not loaded
- VM cleaned up and returned to pool

### Load Failures

- Container deployment fails
- VM status reverted
- Deployment marked as failed

## Cleanup

### Local (Reconciler)

```go
defer os.Remove(tarPath) // Runs even on error
```

### Remote (VM)

```go
docker load -i /tmp/image.tar && rm /tmp/image.tar  // Single command
```

## Monitoring

Key metrics to track:

- Image pull duration
- Tar file sizes
- Transfer duration
- SSH connection failures
- Disk space on reconciler

## Alternative Approaches Considered

### Option A: Private Registry Mirror

- **Pros**: Standard Docker workflow, caching
- **Cons**: Extra infrastructure, maintenance overhead
- **When to use**: When you have 100+ VMs and many repeated deployments

### Option B: Image Baking

- **Pros**: Fastest deployment
- **Cons**: Less flexible, snapshot management
- **When to use**: Standardized deployments with minimal variation

### Option C: Direct Registry Pull (Current Avoided)

- **Pros**: Simplest implementation
- **Cons**: Security risk, credential management overhead
- **Why avoided**: Security is priority

## Future Optimizations

1. **Image Caching**: Keep recent images on reconciler to avoid re-pulling
2. **Compression**: Use `gzip` on tar before transfer (50% size reduction)
3. **Parallel Transfers**: Deploy to multiple VMs simultaneously
4. **Delta Transfers**: Only transfer changed layers (more complex)
5. **Registry Mirror**: Add later when scale demands it

## Example Flow

```bash
# Reconciler pulls with credentials
[RECONCILER] docker pull ghcr.io/myorg/app:v1.2.3
[RECONCILER] docker save ghcr.io/myorg/app:v1.2.3 > /tmp/image-abc.tar
[RECONCILER] scp /tmp/image-abc.tar [2001:db8::1]:/tmp/image.tar
[RECONCILER] rm /tmp/image-abc.tar

# VM loads and runs (no registry credentials!)
[VM] docker load -i /tmp/image.tar
[VM] rm /tmp/image.tar
[VM] docker run -d -p 3000:8080 --name app ghcr.io/myorg/app:v1.2.3
```

## Configuration

The workflow uses existing registry credentials from reconciler config:

```bash
RECONCILER_DOCKER_REGISTRY_URL="ghcr.io"
RECONCILER_DOCKER_REGISTRY_USERNAME="myorg"
RECONCILER_DOCKER_REGISTRY_PASSWORD="ghp_xxxxxxxxxxxx"
```

VMs require **zero** registry configuration! 🎉
