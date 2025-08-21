#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
API_URL="http://localhost:8080"
TIMEOUT_SECONDS=300  # 5 minutes timeout for builds
POLL_INTERVAL=2      # Poll every 2 seconds

# Function to print colored messages
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Function to check if jq is installed
check_dependencies() {
    if ! command -v jq &> /dev/null; then
        print_error "jq is not installed. Please install it first:"
        echo "  - macOS: brew install jq"
        echo "  - Ubuntu/Debian: sudo apt-get install jq"
        echo "  - RHEL/CentOS: sudo yum install jq"
        exit 1
    fi
}

# Function to check if server is running
check_server() {
    print_info "Checking if server is running..."
    if ! curl -s -f "${API_URL}/health" > /dev/null 2>&1; then
        print_error "Server is not running at ${API_URL}"
        print_info "Please start the server with: ./firecracker-manager"
        exit 1
    fi
    print_info "Server is healthy"
}

# Main script
main() {
    print_info "Starting Firecracker Manager Test Run"
    echo "======================================"
    
    # Check dependencies
    check_dependencies
    
    # Check if server is running
    check_server
    
    # Step 1: Add a node
    print_info "Adding node..."
    NODE_RESPONSE=$(curl -s -X POST \
        --url "${API_URL}/nodes/add" \
        --header 'Content-Type: application/json' \
        --data '{
            "name": "node-1",
            "host": "192.168.178.33",
            "port": 22
        }')
    
    if [ $? -ne 0 ]; then
        print_error "Failed to add node"
        exit 1
    fi
    
    NODE_ID=$(echo "$NODE_RESPONSE" | jq -r '.id')
    NODE_STATUS=$(echo "$NODE_RESPONSE" | jq -r '.status')
    
    if [ "$NODE_ID" = "null" ] || [ -z "$NODE_ID" ]; then
        print_error "Failed to extract node ID from response"
        echo "Response: $NODE_RESPONSE"
        exit 1
    fi
    
    print_info "Node added successfully"
    echo "  - Node ID: $NODE_ID"
    echo "  - Status: $NODE_STATUS"
    echo
    
    # Step 2: Build an image
    print_info "Building image from GitHub repository..."
    IMAGE_RESPONSE=$(curl -s -X POST \
        --url "${API_URL}/images/build" \
        --header 'Content-Type: application/json' \
        --data '{
            "github_repo": "dokedu/nuxt-demo",
            "tag": "main",
            "name": "dokedu-nuxt-demo"
        }')
    
    if [ $? -ne 0 ]; then
        print_error "Failed to start image build"
        exit 1
    fi
    
    IMAGE_ID=$(echo "$IMAGE_RESPONSE" | jq -r '.id')
    IMAGE_STATUS=$(echo "$IMAGE_RESPONSE" | jq -r '.status')
    
    if [ "$IMAGE_ID" = "null" ] || [ -z "$IMAGE_ID" ]; then
        print_error "Failed to extract image ID from response"
        echo "Response: $IMAGE_RESPONSE"
        exit 1
    fi
    
    print_info "Image build started"
    echo "  - Image ID: $IMAGE_ID"
    echo "  - Initial Status: $IMAGE_STATUS"
    echo
    
    # Step 3: List all images (optional verification)
    print_info "Listing all images..."
    curl -s -X GET --url "${API_URL}/images" | jq '.'
    echo
    
    # Step 4: Poll image status until ready or timeout
    print_info "Waiting for image to be ready (timeout: ${TIMEOUT_SECONDS}s)..."
    START_TIME=$(date +%s)
    IMAGE_READY=false
    
    while true; do
        CURRENT_TIME=$(date +%s)
        ELAPSED=$((CURRENT_TIME - START_TIME))
        
        if [ $ELAPSED -ge $TIMEOUT_SECONDS ]; then
            print_error "Image build timed out after ${TIMEOUT_SECONDS} seconds"
            exit 1
        fi
        
        # Since there's no /images/{id} endpoint, we'll check the list and filter
        IMAGE_STATUS=$(curl -s -X GET --url "${API_URL}/images" | jq -r ".[] | select(.id == \"$IMAGE_ID\") | .status")
        
        if [ "$IMAGE_STATUS" = "ready" ]; then
            IMAGE_READY=true
            print_info "Image is ready!"
            break
        elif [ "$IMAGE_STATUS" = "failed" ]; then
            print_error "Image build failed"
            
            # Try to extract build log
            BUILD_LOG=$(curl -s -X GET --url "${API_URL}/images" | jq -r ".[] | select(.id == \"$IMAGE_ID\") | .build_log")
            if [ -n "$BUILD_LOG" ] && [ "$BUILD_LOG" != "null" ]; then
                echo "Build log:"
                echo "$BUILD_LOG"
            fi
            exit 1
        else
            echo -n "  Status: $IMAGE_STATUS (elapsed: ${ELAPSED}s)"
            echo -ne "\r"
        fi
        
        sleep $POLL_INTERVAL
    done
    echo
    
    # Step 5: Create an instance
    print_info "Creating VM instance..."
    # Note: default_port is the port the application inside the container will be listening on
    # This will be used as the default remote port when setting up proxy connections
    
    INSTANCE_RESPONSE=$(curl -s -X POST \
        --url "${API_URL}/instances" \
        --header 'Content-Type: application/json' \
        --data "{
            \"node_id\": \"$NODE_ID\",
            \"image_id\": \"$IMAGE_ID\",
            \"vcpu_count\": 2,
            \"memory_mib\": 1024,
            \"default_port\": 3000
        }")
    
    if [ $? -ne 0 ]; then
        print_error "Failed to create instance"
        exit 1
    fi
    
    # Check for error in response
    if echo "$INSTANCE_RESPONSE" | grep -q "error\|Error\|failed\|Failed"; then
        print_error "Failed to create instance"
        echo "Response: $INSTANCE_RESPONSE"
        exit 1
    fi
    
    INSTANCE_ID=$(echo "$INSTANCE_RESPONSE" | jq -r '.id')
    INSTANCE_STATUS=$(echo "$INSTANCE_RESPONSE" | jq -r '.status')
    
    if [ "$INSTANCE_ID" = "null" ] || [ -z "$INSTANCE_ID" ]; then
        print_error "Failed to extract instance ID from response"
        echo "Response: $INSTANCE_RESPONSE"
        exit 1
    fi
    
    print_info "Instance creation started"
    echo "  - Instance ID: $INSTANCE_ID"
    echo "  - Initial Status: $INSTANCE_STATUS"
    echo
    
    # Step 6: Poll instance status until running
    print_info "Waiting for instance to be running..."
    START_TIME=$(date +%s)
    INSTANCE_RUNNING=false
    
    while true; do
        CURRENT_TIME=$(date +%s)
        ELAPSED=$((CURRENT_TIME - START_TIME))
        
        if [ $ELAPSED -ge 60 ]; then  # 1 minute timeout for instance creation
            print_error "Instance creation timed out after 60 seconds"
            exit 1
        fi
        
        # Get instance details from the list
        INSTANCE_DATA=$(curl -s -X GET --url "${API_URL}/instances" | jq ".[] | select(.id == \"$INSTANCE_ID\")")
        INSTANCE_STATUS=$(echo "$INSTANCE_DATA" | jq -r '.status')
        INSTANCE_IP=$(echo "$INSTANCE_DATA" | jq -r '.ip_address // empty')
        
        if [ "$INSTANCE_STATUS" = "running" ]; then
            INSTANCE_RUNNING=true
            print_info "Instance is running!"
            if [ -n "$INSTANCE_IP" ]; then
                echo "  - IP Address: $INSTANCE_IP"
            fi
            break
        elif [ "$INSTANCE_STATUS" = "error" ] || [ "$INSTANCE_STATUS" = "failed" ]; then
            print_error "Instance creation failed"
            echo "Instance details:"
            echo "$INSTANCE_DATA" | jq '.'
            exit 1
        else
            echo -n "  Status: $INSTANCE_STATUS (elapsed: ${ELAPSED}s)"
            echo -ne "\r"
        fi
        
        sleep $POLL_INTERVAL
    done
    echo
    
    # Step 7: Final summary
    echo
    print_info "Test run completed successfully!"
    echo "======================================"
    echo "Summary:"
    echo "  - Node ID: $NODE_ID"
    echo "  - Image ID: $IMAGE_ID"
    echo "  - Instance ID: $INSTANCE_ID"
    echo "  - Default Port: 3000"
    if [ -n "$INSTANCE_IP" ]; then
        echo "  - Instance IP: $INSTANCE_IP"
    fi
    echo
    print_info "You can verify the deployment with:"
    echo "  curl ${API_URL}/instances | jq '.'"
    echo
    
    # Optional: List all instances for verification
    print_info "Current instances:"
    curl -s "${API_URL}/instances" | jq '.'
}

# Run the main function
main "$@"