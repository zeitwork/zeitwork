#!/bin/bash

# Test script for the GET /instances/{id}/logs endpoint

set -e

# Configuration
API_URL="http://localhost:8080"
INSTANCE_ID="${1:-}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🔍 Instance Logs Test Script"
echo "============================"
echo

# Check if instance ID is provided
if [ -z "$INSTANCE_ID" ]; then
    echo -e "${YELLOW}Usage: $0 <instance_id>${NC}"
    echo
    echo "Available instances:"
    curl -s "$API_URL/instances" | jq -r '.[] | "\(.id) - Node: \(.node_id), Status: \(.status)"' 2>/dev/null || {
        echo -e "${RED}Failed to fetch instances. Is the server running?${NC}"
        exit 1
    }
    echo
    echo "Please provide an instance ID as argument"
    exit 1
fi

# Function to get instance logs
get_instance_logs() {
    local instance_id=$1
    
    echo -e "${GREEN}Fetching logs for instance: $instance_id${NC}"
    echo "----------------------------------------"
    
    # Make the API call
    response=$(curl -s -w "\n%{http_code}" "$API_URL/instances/$instance_id/logs")
    http_code=$(echo "$response" | tail -n 1)
    # Use sed instead of head -n -1 for macOS compatibility
    body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        # Parse and display the logs
        echo "$body" | jq -r '.logs' 2>/dev/null || echo "$body"
        echo
        echo -e "${GREEN}✓ Logs retrieved successfully${NC}"
    else
        echo -e "${RED}✗ Failed to get logs (HTTP $http_code)${NC}"
        echo "$body" | jq . 2>/dev/null || echo "$body"
        exit 1
    fi
}

# Main execution
echo "Getting logs for instance: $INSTANCE_ID"
echo

get_instance_logs "$INSTANCE_ID"

echo
echo "----------------------------------------"
echo "You can also use curl directly:"
echo -e "${YELLOW}curl $API_URL/instances/$INSTANCE_ID/logs | jq .${NC}"
