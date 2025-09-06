#!/usr/bin/env bash

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print colored output
print_status() {
    echo -e "${BLUE}[IPv6 SETUP]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Check if running on macOS
check_macos() {
    if [[ "$OSTYPE" != "darwin"* ]]; then
        print_error "This script is designed for macOS. For Linux, you'll need different commands."
        exit 1
    fi
    print_success "Running on macOS"
}

# Check if Docker is running
check_docker() {
    if ! docker info &> /dev/null; then
        print_error "Docker is not running. Please start Docker first."
        exit 1
    fi
    print_success "Docker is running"
}

# Enable IPv6 forwarding (if needed)
enable_ipv6_forwarding() {
    print_status "Checking IPv6 forwarding..."
    
    # On macOS, IPv6 forwarding is usually enabled by default
    if sysctl net.inet6.ip6.forwarding | grep -q "1"; then
        print_success "IPv6 forwarding is already enabled"
    else
        print_warning "IPv6 forwarding is disabled. This might be needed for proper routing."
        print_status "You may need to enable it with: sudo sysctl -w net.inet6.ip6.forwarding=1"
    fi
}

# Setup Docker network with IPv6
setup_docker_network() {
    print_status "Setting up zeitwork Docker network with IPv6 support..."
    print_status "This network is used by nodeagent to create VM instances with IPv6 addresses"
    
    # Check if zeitwork network already exists
    if docker network ls | grep -q zeitwork; then
        print_warning "zeitwork network already exists, removing it first..."
        docker network rm zeitwork || true
    fi
    
    # Create the network with IPv6 support for VM instances
    # Infrastructure services (NATS, PostgreSQL, etc.) use the default network
    docker network create \
        --driver=bridge \
        --subnet=172.20.0.0/16 \
        --ipv6 \
        --subnet=fd00::/16 \
        zeitwork
    
    print_success "Created zeitwork network with IPv6 support for VM instances"
}

    # Setup routing for the ULA range
setup_ipv6_routing() {
    print_status "Setting up IPv6 routing for ULA range fd12::/16..."
    
    # Get the Docker network gateway
    DOCKER_IPV6_GATEWAY=$(docker network inspect zeitwork | jq -r '.[0].IPAM.Config[] | select(.Subnet | startswith("fd12:")) | .Gateway')
    
    if [ "$DOCKER_IPV6_GATEWAY" = "null" ] || [ -z "$DOCKER_IPV6_GATEWAY" ]; then
        print_warning "Could not determine Docker IPv6 gateway automatically"
        print_status "You may need to add routing manually after containers are running"
        return
    fi
    
    print_status "Docker IPv6 gateway: $DOCKER_IPV6_GATEWAY"
    
    # Check if route already exists
    if route -n get -inet6 fd00:: 2>/dev/null | grep -q "gateway"; then
        print_warning "IPv6 route for fd00:: already exists"
    else
        print_status "Adding IPv6 route for fd00::/16 via Docker gateway..."
        print_warning "This requires sudo access:"
        sudo route -n add -inet6 fd00:: -prefixlen 16 $DOCKER_IPV6_GATEWAY || {
            print_error "Failed to add IPv6 route. You may need to add it manually later."
        }
        print_success "Added IPv6 route"
    fi
}

# Verify setup
verify_setup() {
    print_status "Verifying IPv6 setup..."
    
    # Check network exists
    if docker network ls | grep -q zeitwork; then
        print_success "zeitwork network exists"
    else
        print_error "zeitwork network not found"
        return 1
    fi
    
    # Check IPv6 is enabled on network
    if docker network inspect zeitwork | grep -q '"EnableIPv6": true'; then
        print_success "IPv6 is enabled on zeitwork network"
    else
        print_error "IPv6 is not enabled on zeitwork network"
        return 1
    fi
    
    print_success "IPv6 setup verification complete"
}

# Clean up function
cleanup() {
    print_status "Cleaning up IPv6 setup..."
    
    # Remove route if it exists
    sudo route -n delete -inet6 fd00:: 2>/dev/null || true
    
    # Remove Docker network if it exists
    docker network rm zeitwork 2>/dev/null || true
    
    print_success "Cleanup complete"
}

# Main function
main() {
    print_status "Setting up IPv6 networking for Zeitwork development..."
    
    case "${1:-setup}" in
        "setup")
            check_macos
            check_docker
            enable_ipv6_forwarding
            setup_docker_network
            setup_ipv6_routing
            verify_setup
            print_success "IPv6 development setup complete!"
            print_status "You can now start your development environment with docker-compose up -d"
            ;;
        "cleanup")
            cleanup
            ;;
        "verify")
            verify_setup
            ;;
        *)
            echo "Usage: $0 [setup|cleanup|verify]"
            echo "  setup   - Set up IPv6 networking (default)"
            echo "  cleanup - Remove IPv6 setup"
            echo "  verify  - Verify IPv6 setup"
            exit 1
            ;;
    esac
}

# Check if jq is available
if ! command -v jq &> /dev/null; then
    print_error "jq is required but not installed. Please install it with: brew install jq"
    exit 1
fi

main "$@"
