# Zeitwork CLI

A production-grade CLI tool for building Docker images and deploying Zeitwork services to Hetzner infrastructure.

## Features

- **Multi-service deployment**: Deploy builder, edgeproxy, and reconciler services
- **Docker integration**: Build images locally, push to registry, and deploy to remote servers
- **SSH-based deployment**: Secure remote deployment using SSH keys
- **Environment file management**: Securely manage service-specific environment variables
- **Hetzner Cloud integration**: Automatic server discovery using Hetzner Cloud API
- **Zero-downtime deployment**: Gracefully stop old containers and start new ones

## Prerequisites

- Go 1.25 or higher
- Docker installed and running locally
- SSH access to target servers
- Hetzner Cloud API token
- Docker registry credentials
- `.env.prod` file with required configuration

## Installation

Build the CLI from the project root:

```bash
go build -o zeitwork-cli ./cmd/cli
```

## Configuration

### Environment File (.env.prod)

Create a `.env.prod` file in the project root with the following variables:

```bash
# Docker Registry
DOCKER_REGISTRY_URL=ghcr.io/your-org
DOCKER_REGISTRY_USERNAME=your-username
DOCKER_REGISTRY_PASSWORD=your-token

# SSH Keys
SSH_PUBLIC_KEY="ssh-rsa AAAA..."
SSH_PRIVATE_KEY="-----BEGIN OPENSSH PRIVATE KEY-----
...
-----END OPENSSH PRIVATE KEY-----"

# Hetzner Cloud
HETZNER_TOKEN=your-hetzner-token

# Edgeproxy Service
EDGEPROXY_DATABASE_URL=postgresql://...
EDGEPROXY_REGION_ID=nbg1

# Reconciler Service
RECONCILER_DATABASE_URL=postgresql://...
RECONCILER_HETZNER_TOKEN=your-token
RECONCILER_DOCKER_REGISTRY_URL=ghcr.io/your-org
RECONCILER_DOCKER_REGISTRY_USERNAME=your-username
RECONCILER_DOCKER_REGISTRY_PASSWORD=your-token
RECONCILER_SSH_PUBLIC_KEY="ssh-rsa AAAA..."
RECONCILER_SSH_PRIVATE_KEY="-----BEGIN OPENSSH PRIVATE KEY-----..."

# Builder Service
BUILDER_DATABASE_URL=postgresql://...
BUILDER_GITHUB_APP_ID=123456
BUILDER_GITHUB_APP_KEY="-----BEGIN RSA PRIVATE KEY-----..."
BUILDER_REGISTRY_URL=ghcr.io/your-org
BUILDER_REGISTRY_USERNAME=your-username
BUILDER_REGISTRY_PASSWORD=your-token
BUILDER_HETZNER_TOKEN=your-token
BUILDER_SSH_PUBLIC_KEY="ssh-rsa AAAA..."
BUILDER_SSH_PRIVATE_KEY="-----BEGIN OPENSSH PRIVATE KEY-----..."
```

### Deployment Configuration (config/deploy.yaml)

The CLI reads server configurations from `config/deploy.yaml`:

```yaml
regions:
  nbg1:
    no: 1
    lb: 4806437
    proxies:
      - 111089432
      - 111089433
    manager: 111089624
```

## Usage

### Deploy All Services

Deploy all services (builder, edgeproxy, reconciler) to their respective servers:

```bash
./zeitwork-cli deploy
```

### Deploy Specific Services

Deploy only specific services:

```bash
./zeitwork-cli deploy --services builder,edgeproxy
./zeitwork-cli deploy --services reconciler
```

### Custom Configuration Files

Specify custom environment or deployment configuration files:

```bash
./zeitwork-cli deploy --env-file .env.staging --config config/deploy-staging.yaml
```

## How It Works

### 1. Image Building

The CLI builds Docker images for each service using their respective Dockerfiles:

- `docker/builder/Dockerfile`
- `docker/edgeproxy/Dockerfile`
- `docker/reconciler/Dockerfile`

Each image is tagged with:

- Timestamp: `registry/service:20060102-150405`
- Latest: `registry/service:latest`

### 2. Registry Push

Images are authenticated and pushed to the configured Docker registry using the provided credentials.

### 3. Server Discovery

The CLI uses the Hetzner Cloud API to discover server IP addresses based on the server IDs in the deployment configuration:

- **edgeproxy**: Deployed to proxy servers
- **reconciler**: Deployed to manager server
- **builder**: Deployed to manager server

### 4. Remote Deployment

For each service and target server:

1. **SSH Connection**: Establish secure SSH connection using private key
2. **Docker Check**: Verify Docker daemon is running
3. **Environment Setup**: Create service-specific `.env` file on remote server
4. **Registry Login**: Authenticate with Docker registry on remote server
5. **Container Stop**: Gracefully stop and remove existing container
6. **Image Pull**: Pull the newly built image
7. **Container Start**: Start new container with:
   - Environment variables from `.env` file
   - Appropriate port mappings
   - Docker socket mounting (for builder service)
   - Restart policy: `unless-stopped`
8. **Cleanup**: Remove unused Docker images

## Service-Specific Configurations

### Edgeproxy

- **Ports**: 8080:8080, 8443:8443
- **Target Servers**: Proxy servers
- **Environment**: Database URL, Region ID

### Reconciler

- **Ports**: None (internal service)
- **Target Servers**: Manager server
- **Environment**: Database URL, Hetzner credentials, Docker registry, SSH keys

### Builder

- **Ports**: 8080:8080
- **Target Servers**: Manager server
- **Special**: Mounts Docker socket for building images
- **Environment**: Database URL, GitHub App credentials, registry credentials, SSH keys

## Troubleshooting

### Docker Build Fails

- Ensure you're running the command from the project root
- Check that all source files are present
- Verify Go modules are downloaded

### SSH Connection Fails

- Verify SSH private key format (should include header/footer)
- Check server IP addresses in Hetzner
- Ensure SSH keys are added to servers

### Container Won't Start

- Check container logs: `docker logs <service-name>`
- Verify environment variables are set correctly
- Ensure ports are not already in use

### Registry Authentication Fails

- Verify registry credentials are correct
- For GitHub Container Registry (ghcr.io), use a Personal Access Token with `write:packages` scope
- Check registry URL format (no `https://` prefix)

## Security Considerations

- Environment files (`.env.prod`) contain sensitive data and should never be committed to version control
- SSH private keys are stored securely and transmitted only over encrypted SSH connections
- Remote environment files are created with restricted permissions
- Docker registry credentials are cleared from command history after use

## Development

To modify the CLI:

1. Edit `cmd/cli/cli.go`
2. Rebuild: `go build -o zeitwork-cli ./cmd/cli`
3. Test with: `./zeitwork-cli deploy --help`

## License

See the LICENSE file in the project root.
