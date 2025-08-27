# Firecracker Builder VM

The Firecracker Builder VM is a specialized virtual machine image that runs inside Firecracker to securely build Docker containers from customer code. This provides strong isolation during the build process while still allowing Docker-based workflows.

## Architecture

```
┌───────────────────────────────────┐
│         Node Agent Host           │
├───────────────────────────────────┤
│                                   │
│  ┌─────────────────────────────┐  │
│  │   Firecracker Builder VM    │  │
│  ├─────────────────────────────┤  │
│  │  • Ubuntu 22.04 Base        │  │
│  │  • Docker Daemon            │  │
│  │  • Git                      │  │
│  │  • Build Tools              │  │
│  │  • AWS CLI                  │  │
│  │  • Auto-build Script        │  │
│  └─────────────────────────────┘  │
│                                   │
└───────────────────────────────────┘
```

## Features

- **Secure Isolation**: Runs in a Firecracker microVM with hardware-enforced isolation
- **Docker Support**: Full Docker daemon for building container images
- **Build Tools**: Includes common build tools (gcc, make, npm, python, etc.)
- **S3 Integration**: AWS CLI for uploading built images to S3
- **Auto-build**: Automatically builds on boot based on kernel parameters
- **Auto-shutdown**: Shuts down after build completion to free resources

## Building the Builder VM

### Prerequisites

- Linux host with sudo access
- `debootstrap` installed (`apt-get install debootstrap`)
- At least 8GB free disk space
- Firecracker installed (for testing)

### Build Process

```bash
# Build the Builder VM image
make builder-vm

# This creates:
#   build/vms/builder-rootfs.ext4  - Root filesystem (4GB)
#   build/vms/vmlinux              - Linux kernel
#   build/vms/builder-vm.json      - VM metadata
```

### Testing

```bash
# Test with a sample repository
make test-builder-vm

# Or test with a specific repository
scripts/test-builder-vm.sh https://github.com/yourorg/yourapp.git main Dockerfile
```

## Build Workflow

1. **Node Agent receives build request** from operator
2. **Creates Firecracker VM** with builder rootfs
3. **Passes build parameters** via kernel command line:

   - `repo_url` - Git repository URL
   - `commit_sha` - Specific commit to build
   - `dockerfile` - Dockerfile path (default: Dockerfile)
   - `s3_bucket` - S3 bucket for output
   - `s3_key` - S3 key for rootfs
   - `notify_url` - Webhook for completion

4. **VM boots and runs build script**:

   - Clones repository
   - Checks out specified commit
   - Builds Docker image
   - Converts to Firecracker rootfs
   - Uploads to S3
   - Notifies completion
   - Shuts down

5. **Node Agent receives notification** and updates database

## Configuration

### VM Resources

Default configuration (adjustable):

- **vCPUs**: 4
- **Memory**: 2048 MB
- **Disk**: 4 GB
- **Network**: TAP interface with NAT

### Build Timeout

Default: 10 minutes (configurable in node-agent)

### Supported Languages

The builder VM includes runtimes for:

- Node.js / JavaScript
- Python 3
- Go
- Java (via apt)
- Ruby (via apt)
- Any language installable via apt or buildable from source

## Security Considerations

1. **Network Isolation**: Builder VMs run in isolated network namespace
2. **Resource Limits**: CPU and memory limits enforced by Firecracker
3. **Ephemeral**: VM and all data destroyed after build
4. **No Persistent Storage**: Each build starts fresh
5. **Audit Logging**: All builds logged with repository and commit info

## Customization

To add additional tools or languages:

1. Edit `scripts/build-builder-vm.sh`
2. Add packages in the `CHROOT_CMDS` section
3. Rebuild: `make builder-vm`

Example adding Ruby:

```bash
apt-get install -y ruby ruby-dev
```

## Troubleshooting

### Build Fails

Check logs:

- Firecracker logs: `/tmp/builder-test-*/firecracker.log`
- Build logs: Inside VM at `/var/log/build.log`

### Network Issues

Ensure TAP interface and NAT are configured:

```bash
sudo ip tuntap add tap0 mode tap
sudo ip addr add 172.16.0.1/24 dev tap0
sudo ip link set tap0 up
sudo sysctl -w net.ipv4.ip_forward=1
sudo iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
```

### VM Won't Start

Verify:

- KVM is available: `ls /dev/kvm`
- User has permissions: `sudo usermod -aG kvm $USER`
- Sufficient memory available

## Performance Tuning

### Cache Docker Layers

To speed up builds, you can:

1. Pre-pull common base images into the builder image
2. Use a Docker registry mirror
3. Mount a shared cache directory (reduces isolation)

### Parallel Builds

Run multiple builder VMs on different CPUs:

```bash
taskset -c 0-3 firecracker ... # VM 1 on cores 0-3
taskset -c 4-7 firecracker ... # VM 2 on cores 4-7
```

## Monitoring

The builder VM exports metrics via:

- Build status in S3 metadata
- Completion webhooks with timing info
- Firecracker metrics API
- Node agent health checks
