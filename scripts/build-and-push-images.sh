#!/bin/bash

# Build and push zeitwork services to GitHub Container Registry
# Usage: ./scripts/build-and-push-images.sh [--push] [service_name]
# 
# Examples:
#   ./scripts/build-and-push-images.sh                    # Build all services locally
#   ./scripts/build-and-push-images.sh --push            # Build and push all services
#   ./scripts/build-and-push-images.sh --push builder    # Build and push only builder service

set -e

REGISTRY="ghcr.io/zeitwork"
PUSH_IMAGES=false
SPECIFIC_SERVICE=""

# Define services
SERVICES=("builder" "certmanager" "listener" "manager" "web")

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --push)
      PUSH_IMAGES=true
      shift
      ;;
    *)
      # Check if it's a valid service name
      if [[ " ${SERVICES[*]} " =~ " $1 " ]]; then
        SPECIFIC_SERVICE="$1"
      else
        echo "Error: Unknown service '$1'"
        echo "Available services: ${SERVICES[*]}"
        exit 1
      fi
      shift
      ;;
  esac
done

# Get current git commit hash for tagging
GIT_COMMIT=$(git rev-parse --short HEAD)
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

# Determine which services to build
if [[ -n "$SPECIFIC_SERVICE" ]]; then
  SERVICES_TO_BUILD=("$SPECIFIC_SERVICE")
else
  SERVICES_TO_BUILD=("${SERVICES[@]}")
fi

echo "Building zeitwork services..."
echo "Registry: $REGISTRY"
echo "Git commit: $GIT_COMMIT"
echo "Push images: $PUSH_IMAGES"
echo "Services to build: ${SERVICES_TO_BUILD[*]}"
echo ""

# Function to build and optionally push an image
build_and_push() {
  local service=$1
  local image_name="$REGISTRY/$service"
  
  echo "Building $service..."
  
  # Build the image with multiple tags for amd64 architecture
  docker build \
    --platform linux/amd64 \
    -t "$image_name:latest" \
    -t "$image_name:$GIT_COMMIT" \
    -t "$image_name:$TIMESTAMP" \
    -f "docker/$service/Dockerfile" \
    .
  
  if [[ "$PUSH_IMAGES" == "true" ]]; then
    echo "Pushing $service to registry..."
    docker push "$image_name:latest"
    docker push "$image_name:$GIT_COMMIT"
    docker push "$image_name:$TIMESTAMP"
    echo "✅ Pushed $service successfully"
  else
    echo "✅ Built $service successfully (local only)"
  fi
  
  echo ""
}

# Check if we're authenticated to GitHub Container Registry if pushing
if [[ "$PUSH_IMAGES" == "true" ]]; then
  echo "Checking GitHub Container Registry authentication..."
  if ! docker info | grep -q "ghcr.io"; then
    echo "⚠️  Make sure you're logged in to GitHub Container Registry:"
    echo "   echo \$GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin"
    echo ""
  fi
fi

# Build and push each service
for service in "${SERVICES_TO_BUILD[@]}"; do
  build_and_push "$service"
done

echo "🎉 All done!"

if [[ "$PUSH_IMAGES" == "true" ]]; then
  echo ""
  echo "Images pushed to GitHub Container Registry:"
  for service in "${SERVICES_TO_BUILD[@]}"; do
    echo "  - $REGISTRY/$service:latest"
    echo "  - $REGISTRY/$service:$GIT_COMMIT"
    echo "  - $REGISTRY/$service:$TIMESTAMP"
  done
else
  echo ""
  echo "To push images to registry, run:"
  if [[ -n "$SPECIFIC_SERVICE" ]]; then
    echo "  $0 --push $SPECIFIC_SERVICE"
  else
    echo "  $0 --push"
  fi
fi
