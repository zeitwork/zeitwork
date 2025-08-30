#!/usr/bin/env bash

set -euo pipefail

# Load environment variables from .env file if it exists
if [ -f ".env" ]; then
    print_status() { echo -e "\033[0;34m[STATUS]\033[0m $1"; }
    print_status "Loading environment variables from .env file..."
    set -a  # automatically export all variables
    source .env
    set +a  # disable automatic export
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print colored output
print_status() {
    echo -e "${BLUE}[STATUS]${NC} $1"
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

# Check if a command exists
check_command() {
    if ! command -v "$1" &> /dev/null; then
        print_error "$1 is not installed"
        return 1
    fi
    return 0
}

# Check prerequisites
check_prerequisites() {
    print_status "Checking prerequisites..."
    
    local missing_prereqs=0
    
    # Check for required commands
    local commands=("docker" "docker-compose" "go" "bun" "make")
    for cmd in "${commands[@]}"; do
        if ! check_command "$cmd"; then
            missing_prereqs=1
        else
            print_success "$cmd is installed"
        fi
    done
    
    # Check if Docker is running
    if command -v docker &> /dev/null; then
        if ! docker info &> /dev/null; then
            print_error "Docker is not running"
            missing_prereqs=1
        else
            print_success "Docker is running"
        fi
    fi
    
    # Check for required environment variables
    if [ -z "${DATABASE_URL:-}" ]; then
        print_error "DATABASE_URL environment variable is not set"
        print_error "Please set DATABASE_URL (e.g., export DATABASE_URL='postgres://postgres:postgres@localhost:5432/postgres')"
        missing_prereqs=1
    fi
    
    if [ -z "${NATS_URL:-}" ]; then
        print_error "NATS_URL environment variable is not set"
        print_error "Please set NATS_URL (e.g., export NATS_URL='nats://localhost:4222')"
        missing_prereqs=1
    fi
    
    return $missing_prereqs
}

# Start docker compose services
start_docker_services() {
    print_status "Starting Docker services (PostgreSQL and NATS)..."
    
    # Check if services are already running
    if docker-compose ps | grep -q "Up"; then
        print_warning "Some Docker services are already running"
        docker-compose ps
    else
        docker-compose up -d
        
        # Wait for PostgreSQL to be ready
        print_status "Waiting for PostgreSQL to be ready..."
        local retries=30
        while [ $retries -gt 0 ]; do
            if docker-compose exec -T postgres pg_isready -U postgres &> /dev/null; then
                print_success "PostgreSQL is ready"
                break
            fi
            retries=$((retries - 1))
            sleep 1
        done
        
        if [ $retries -eq 0 ]; then
            print_error "PostgreSQL failed to start in time"
            return 1
        fi
        
        # NATS usually starts quickly, but let's give it a moment
        sleep 2
        print_success "Docker services started"
    fi
}



# Run database migrations
run_migrations() {
    print_status "Running database migrations with Drizzle..."
    
    # Navigate to the database package directory
    cd packages/database
    
    # Install dependencies if needed
    if [ ! -d "node_modules" ]; then
        print_status "Installing database package dependencies..."
        bun install
    fi
    
    # Run migrations
    bun run drizzle-kit push
    
    # Return to root directory
    cd ../..
    
    print_success "Database migrations completed"
}

# Export service-specific environment variables
export_service_env() {
    local service_prefix=$(echo "$1" | tr '[:lower:]' '[:upper:]')
    
    # Export service-specific variables if they exist
    for var in LOG_LEVEL ENVIRONMENT DATABASE_URL NATS_URLS NATS_MAX_RECONNECTS NATS_RECONNECT_WAIT_MS NATS_TIMEOUT_MS; do
        local service_var="${service_prefix}_${var}"
        if [ -n "${!service_var:-}" ]; then
            export $var="${!service_var}"
        fi
    done
    
    # Export service-specific variables that don't have global equivalents
    case "$service_prefix" in
        "BUILDER")
            for var in TYPE WORK_DIR BUILD_POLL_INTERVAL_MS BUILD_TIMEOUT_MS MAX_CONCURRENT_BUILDS CONTAINER_REGISTRY PORT; do
                local service_var="${service_prefix}_${var}"
                if [ -n "${!service_var:-}" ]; then
                    export $var="${!service_var}"
                fi
            done
            ;;
        "NODEAGENT")
            for var in NODE_ID PORT; do
                local service_var="${service_prefix}_${var}"
                if [ -n "${!service_var:-}" ]; then
                    export $var="${!service_var}"
                fi
            done
            ;;
        "EDGEPROXY"|"CERTS"|"LISTENER")
            local service_var="${service_prefix}_PORT"
            if [ -n "${!service_var:-}" ]; then
                export PORT="${!service_var}"
            fi
            ;;
    esac
}

# Start a service in the background
start_service() {
    local service_name=$1
    local service_cmd=$2
    local log_file="logs/${service_name}.log"
    
    # Create logs directory if it doesn't exist
    mkdir -p logs
    
    print_status "Starting $service_name..."
    
    # Check if service is already running
    if pgrep -f "$service_cmd" > /dev/null; then
        print_warning "$service_name appears to be already running"
        return 0
    fi
    
    # Export service-specific environment variables
    export_service_env "$service_name"
    
    # Start the service
    nohup bash -c "$service_cmd" > "$log_file" 2>&1 &
    local pid=$!
    
    # Save PID for later cleanup
    echo $pid > "logs/${service_name}.pid"
    
    # Give it a moment to start
    sleep 2
    
    # Check if it's still running
    if kill -0 $pid 2>/dev/null; then
        print_success "$service_name started (PID: $pid, log: $log_file)"
    else
        print_error "$service_name failed to start. Check $log_file for details"
        tail -20 "$log_file"
        return 1
    fi
}

# Stop all services
stop_services() {
    print_status "Stopping all services..."
    
    local services=("nodeagent" "edgeproxy" "builder" "certs" "listener")
    
    for service in "${services[@]}"; do
        if [ -f "logs/${service}.pid" ]; then
            local pid=$(cat "logs/${service}.pid")
            if kill -0 $pid 2>/dev/null; then
                print_status "Stopping $service (PID: $pid)"
                kill $pid
                rm -f "logs/${service}.pid"
            fi
        fi
    done
    
    print_success "All services stopped"
}

# Main function
main() {
    print_status "Starting Zeitwork development environment setup..."
    
    # Check prerequisites
    if ! check_prerequisites; then
        print_error "Prerequisites check failed. Please install missing dependencies."
        exit 1
    fi
    
    # Trap to ensure cleanup on exit
    trap 'print_status "Cleaning up..."; stop_services' EXIT INT TERM
    
    # Start Docker services
    if ! start_docker_services; then
        print_error "Failed to start Docker services"
        exit 1
    fi
    
    # Run migrations
    if ! run_migrations; then
        print_error "Failed to run migrations"
        exit 1
    fi
    
    # Build services if needed
    if [ ! -d "build" ] || [ "$1" == "--rebuild" ]; then
        print_status "Building services..."
        make build-local
    fi
    
    # Start services in order
    print_status "Starting services in order..."
    
    # Start nodeagent
    start_service "nodeagent" "go run ./cmd/nodeagent/nodeagent.go"
    
    # Start edgeproxy
    start_service "edgeproxy" "go run ./cmd/edgeproxy/edgeproxy.go"
    
    # Start builder
    start_service "builder" "go run ./cmd/builder/builder.go"
    
    # Start certs
    start_service "certs" "go run ./cmd/certs/certs.go"
    
    # Start listener
    start_service "listener" "go run ./cmd/listener/listener.go"
    
    print_success "Development environment is up and running!"
    print_status "Service logs are available in the logs/ directory"
    print_status "To stop all services, press Ctrl+C"
    
    # Keep script running
    print_status "Press Ctrl+C to stop all services..."
    while true; do
        sleep 1
    done
}

# Handle command line arguments
case "${1:-}" in
    "stop")
        stop_services
        docker-compose down
        print_success "Development environment stopped"
        ;;
    *)
        main "${1:-}"
        ;;
esac
