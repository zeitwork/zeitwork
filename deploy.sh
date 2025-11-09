#!/bin/bash

set -euo pipefail

set -a
source .env.prod
set +a

# create builder
if docker buildx inspect zeitwork > /dev/null 2>&1; then
    echo "Builder already exists"
else
    echo "Creating builder..."
    docker buildx create --name zeitwork --use
fi

# build images
docker buildx build --platform linux/amd64 -t ghcr.io/zeitwork/reconciler:latest --file docker/reconciler/Dockerfile --push --builder zeitwork --progress=plain --output=type=registry .
docker buildx build --platform linux/amd64 -t ghcr.io/zeitwork/builder:latest --file docker/builder/Dockerfile --push --builder zeitwork --progress=plain --output=type=registry .
docker buildx build --platform linux/amd64 -t ghcr.io/zeitwork/edgeproxy:latest --file docker/edgeproxy/Dockerfile --push --builder zeitwork --progress=plain --output=type=registry .

# run on remote servers
EDGE_PROXY_1="91.98.67.157"
EDGE_PROXY_2="91.98.200.17"
MANAGER="49.12.204.67"

# install docker if not already installed
for SERVER in $EDGE_PROXY_1 $EDGE_PROXY_2 $MANAGER; do
    echo "Checking Docker installation on $SERVER..."
    ssh -o StrictHostKeyChecking=no root@$SERVER "if ! command -v docker &> /dev/null; then echo 'Docker not found, installing...'; curl -fsSL https://get.docker.com -o get-docker.sh; sh get-docker.sh; systemctl enable docker; systemctl start docker; usermod -aG docker root; echo 'Docker installed successfully'; else echo 'Docker is already installed'; fi"
done

# log into the registry on the remote servers
ssh -o StrictHostKeyChecking=no root@$EDGE_PROXY_1 "docker login ghcr.io -u $DOCKER_REGISTRY_USERNAME -p $DOCKER_REGISTRY_PASSWORD"
ssh -o StrictHostKeyChecking=no root@$EDGE_PROXY_2 "docker login ghcr.io -u $DOCKER_REGISTRY_USERNAME -p $DOCKER_REGISTRY_PASSWORD"
ssh -o StrictHostKeyChecking=no root@$MANAGER "docker login ghcr.io -u $DOCKER_REGISTRY_USERNAME -p $DOCKER_REGISTRY_PASSWORD"

# proxies
ssh -o StrictHostKeyChecking=no root@$EDGE_PROXY_1 "docker pull ghcr.io/zeitwork/edgeproxy:latest"
ssh -o StrictHostKeyChecking=no root@$EDGE_PROXY_2 "docker pull ghcr.io/zeitwork/edgeproxy:latest"

# manager
ssh -o StrictHostKeyChecking=no root@$MANAGER "docker pull ghcr.io/zeitwork/reconciler:latest"
ssh -o StrictHostKeyChecking=no root@$MANAGER "docker pull ghcr.io/zeitwork/builder:latest"

# Create env files directly on remote servers
echo "Creating environment files on remote servers..."

# Edgeproxy 1 env file
ssh -o StrictHostKeyChecking=no root@$EDGE_PROXY_1 "cat > /root/edgeproxy.env << 'EOF'
EDGEPROXY_DATABASE_URL=${EDGEPROXY_DATABASE_URL}
EDGEPROXY_REGION_ID=${EDGEPROXY_REGION_ID}
EDGEPROXY_ACME_EMAIL=admin@zeitwork.com
EDGEPROXY_ACME_STAGING=false
EOF
chmod 600 /root/edgeproxy.env"

# Edgeproxy 2 env file
ssh -o StrictHostKeyChecking=no root@$EDGE_PROXY_2 "cat > /root/edgeproxy.env << 'EOF'
EDGEPROXY_DATABASE_URL=${EDGEPROXY_DATABASE_URL}
EDGEPROXY_REGION_ID=${EDGEPROXY_REGION_ID}
EDGEPROXY_ACME_EMAIL=admin@zeitwork.com
EDGEPROXY_ACME_STAGING=false
EOF
chmod 600 /root/edgeproxy.env"

# Reconciler env file
# Note: RECONCILER_SSH_PRIVATE_KEY should be base64 encoded
# To encode: cat your-key.pem | base64 | tr -d '\n'
ssh -o StrictHostKeyChecking=no root@$MANAGER "cat > /root/reconciler.env << 'EOF'
RECONCILER_DATABASE_URL=${RECONCILER_DATABASE_URL}
RECONCILER_HETZNER_TOKEN=${RECONCILER_HETZNER_TOKEN}
RECONCILER_DOCKER_REGISTRY_URL=${RECONCILER_DOCKER_REGISTRY_URL}
RECONCILER_DOCKER_REGISTRY_USERNAME=${RECONCILER_DOCKER_REGISTRY_USERNAME}
RECONCILER_DOCKER_REGISTRY_PASSWORD=${RECONCILER_DOCKER_REGISTRY_PASSWORD}
RECONCILER_SSH_PUBLIC_KEY=${RECONCILER_SSH_PUBLIC_KEY}
RECONCILER_SSH_PRIVATE_KEY=${RECONCILER_SSH_PRIVATE_KEY}
EOF
chmod 600 /root/reconciler.env"

# Builder env file
# Note: BUILDER_GITHUB_APP_KEY and BUILDER_SSH_PRIVATE_KEY should be base64 encoded
# To encode: cat your-key.pem | base64 | tr -d '\n'
ssh -o StrictHostKeyChecking=no root@$MANAGER "cat > /root/builder.env << 'EOF'
BUILDER_DATABASE_URL=${BUILDER_DATABASE_URL}
BUILDER_GITHUB_APP_ID=${BUILDER_GITHUB_APP_ID}
BUILDER_GITHUB_APP_KEY=${BUILDER_GITHUB_APP_KEY}
BUILDER_REGISTRY_URL=${BUILDER_REGISTRY_URL}
BUILDER_REGISTRY_USERNAME=${BUILDER_REGISTRY_USERNAME}
BUILDER_REGISTRY_PASSWORD=${BUILDER_REGISTRY_PASSWORD}
BUILDER_HETZNER_TOKEN=${BUILDER_HETZNER_TOKEN}
BUILDER_SSH_PUBLIC_KEY=${BUILDER_SSH_PUBLIC_KEY}
BUILDER_SSH_PRIVATE_KEY=${BUILDER_SSH_PRIVATE_KEY}
EOF
chmod 600 /root/builder.env"

# Stop and remove existing containers
echo "Stopping existing containers..."
ssh -o StrictHostKeyChecking=no root@$EDGE_PROXY_1 "docker stop edgeproxy 2>/dev/null || true && docker rm edgeproxy 2>/dev/null || true"
ssh -o StrictHostKeyChecking=no root@$EDGE_PROXY_2 "docker stop edgeproxy 2>/dev/null || true && docker rm edgeproxy 2>/dev/null || true"
ssh -o StrictHostKeyChecking=no root@$MANAGER "docker stop reconciler 2>/dev/null || true && docker rm reconciler 2>/dev/null || true"
ssh -o StrictHostKeyChecking=no root@$MANAGER "docker stop builder 2>/dev/null || true && docker rm builder 2>/dev/null || true"

# Run containers with env files
echo "Starting containers..."

# Edge proxy 1
ssh -o StrictHostKeyChecking=no root@$EDGE_PROXY_1 "docker run -d \
    --name edgeproxy \
    --restart unless-stopped \
    --env-file /root/edgeproxy.env \
    -p 8080:8080 \
    -p 8443:8443 \
    ghcr.io/zeitwork/edgeproxy:latest"

# Edge proxy 2
ssh -o StrictHostKeyChecking=no root@$EDGE_PROXY_2 "docker run -d \
    --name edgeproxy \
    --restart unless-stopped \
    --env-file /root/edgeproxy.env \
    -p 8080:8080 \
    -p 8443:8443 \
    ghcr.io/zeitwork/edgeproxy:latest"

# Reconciler
ssh -o StrictHostKeyChecking=no root@$MANAGER "docker run -d \
    --name reconciler \
    --restart unless-stopped \
    --env-file /root/reconciler.env \
    --user root \
    -v /var/run/docker.sock:/var/run/docker.sock \
    ghcr.io/zeitwork/reconciler:latest"

# Builder
ssh -o StrictHostKeyChecking=no root@$MANAGER "docker run -d \
    --name builder \
    --restart unless-stopped \
    --env-file /root/builder.env \
    -v /var/run/docker.sock:/var/run/docker.sock \
    ghcr.io/zeitwork/builder:latest"

echo "Deployment complete!"
echo "Edge Proxy 1: $EDGE_PROXY_1:8080 (HTTP), $EDGE_PROXY_1:8443 (HTTPS)"
echo "Edge Proxy 2: $EDGE_PROXY_2:8080 (HTTP), $EDGE_PROXY_2:8443 (HTTPS)"
echo "Manager: $MANAGER (reconciler + builder)"