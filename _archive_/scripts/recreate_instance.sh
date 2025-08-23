#!/bin/bash

# Script to create a new instance with the fixed networking

set -e

# Configuration
API_URL="http://localhost:8080"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🔄 Recreating Instance with Fixed Networking"
echo "============================================"
echo

# Step 1: Get the current instance details
echo -e "${YELLOW}Step 1: Getting instance details...${NC}"
INSTANCE_ID="${1:-instance-1755788268543774000}"
INSTANCE_JSON=$(curl -s "$API_URL/instances" | jq ".[] | select(.id == \"$INSTANCE_ID\")")

if [ -z "$INSTANCE_JSON" ]; then
    echo -e "${RED}Instance $INSTANCE_ID not found${NC}"
    exit 1
fi

NODE_ID=$(echo "$INSTANCE_JSON" | jq -r '.node_id')
IMAGE_ID=$(echo "$INSTANCE_JSON" | jq -r '.image_id')
VCPU_COUNT=$(echo "$INSTANCE_JSON" | jq -r '.vcpu_count')
MEMORY_MIB=$(echo "$INSTANCE_JSON" | jq -r '.memory_mib')
DEFAULT_PORT=$(echo "$INSTANCE_JSON" | jq -r '.default_port // 3000')

echo "Current instance configuration:"
echo "  Node: $NODE_ID"
echo "  Image: $IMAGE_ID"
echo "  vCPUs: $VCPU_COUNT"
echo "  Memory: ${MEMORY_MIB}MiB"
echo "  Default Port: $DEFAULT_PORT"
echo

# Step 2: Delete the old proxy if it exists
echo -e "${YELLOW}Step 2: Cleaning up old proxy...${NC}"
curl -X DELETE "$API_URL/instances/$INSTANCE_ID/proxy" 2>/dev/null || true
echo "Old proxy removed (if existed)"
echo

# Step 3: Create a new instance with the same configuration
echo -e "${YELLOW}Step 3: Creating new instance with fixed networking...${NC}"
NEW_INSTANCE_RESPONSE=$(curl -s -X POST "$API_URL/instances" \
  -H "Content-Type: application/json" \
  -d "{
    \"node_id\": \"$NODE_ID\",
    \"image_id\": \"$IMAGE_ID\",
    \"vcpu_count\": $VCPU_COUNT,
    \"memory_mib\": $MEMORY_MIB,
    \"default_port\": $DEFAULT_PORT
  }")

NEW_INSTANCE_ID=$(echo "$NEW_INSTANCE_RESPONSE" | jq -r '.id')

if [ -z "$NEW_INSTANCE_ID" ] || [ "$NEW_INSTANCE_ID" = "null" ]; then
    echo -e "${RED}Failed to create new instance${NC}"
    echo "$NEW_INSTANCE_RESPONSE"
    exit 1
fi

echo -e "${GREEN}New instance created: $NEW_INSTANCE_ID${NC}"
echo

# Step 4: Wait for the instance to be ready
echo -e "${YELLOW}Step 4: Waiting for instance to be ready...${NC}"
for i in {1..30}; do
    STATUS=$(curl -s "$API_URL/instances" | jq -r ".[] | select(.id == \"$NEW_INSTANCE_ID\") | .status")
    if [ "$STATUS" = "running" ]; then
        echo -e "${GREEN}Instance is running!${NC}"
        break
    elif [ "$STATUS" = "error" ]; then
        echo -e "${RED}Instance failed to start${NC}"
        exit 1
    fi
    echo -n "."
    sleep 2
done
echo

# Step 5: Wait a bit more for the application to start
echo -e "${YELLOW}Step 5: Waiting for application to start...${NC}"
sleep 10

# Step 6: Check the logs
echo -e "${YELLOW}Step 6: Checking instance logs...${NC}"
curl -s "$API_URL/instances/$NEW_INSTANCE_ID/logs" | jq -r '.logs' | tail -20
echo

# Step 7: Setup proxy for the new instance
echo -e "${YELLOW}Step 7: Setting up proxy...${NC}"
PROXY_RESPONSE=$(curl -s -X POST "$API_URL/instances/$NEW_INSTANCE_ID/proxy" \
  -H "Content-Type: application/json" \
  -d "{
    \"remote_port\": $DEFAULT_PORT
  }")

LOCAL_PORT=$(echo "$PROXY_RESPONSE" | jq -r '.local_port')
ACCESS_URL=$(echo "$PROXY_RESPONSE" | jq -r '.access_url')

if [ -z "$LOCAL_PORT" ] || [ "$LOCAL_PORT" = "null" ]; then
    echo -e "${RED}Failed to setup proxy${NC}"
    echo "$PROXY_RESPONSE"
    exit 1
fi

echo -e "${GREEN}Proxy setup successful!${NC}"
echo "$PROXY_RESPONSE" | jq .
echo

# Step 8: Test the connection
echo -e "${YELLOW}Step 8: Testing connection...${NC}"
sleep 3  # Give proxy time to establish

echo "Testing $ACCESS_URL..."
if curl -s -o /dev/null -w "%{http_code}" --max-time 5 "$ACCESS_URL" | grep -q "200\|301\|302"; then
    echo -e "${GREEN}✓ Application is responding!${NC}"
    echo
    echo -e "${GREEN}Success! Your application is now accessible at:${NC}"
    echo -e "${GREEN}$ACCESS_URL${NC}"
else
    echo -e "${YELLOW}⚠ Application may still be starting up${NC}"
    echo "Try accessing $ACCESS_URL in a few seconds"
    echo
    echo "You can check the logs with:"
    echo "curl $API_URL/instances/$NEW_INSTANCE_ID/logs | jq -r '.logs'"
fi

echo
echo "----------------------------------------"
echo "Old instance: $INSTANCE_ID"
echo "New instance: $NEW_INSTANCE_ID"
echo "Access URL: $ACCESS_URL"
echo
echo "To delete the old instance, run:"
echo -e "${YELLOW}curl -X DELETE $API_URL/instances/$INSTANCE_ID${NC}"
