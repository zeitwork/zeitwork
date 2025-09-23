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
    local commands=("docker" "go" "bun" "make")
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
    
    return $missing_prereqs
}

# Start docker compose services
start_docker_services() {
    print_status "Starting Docker services (PostgreSQL and NATS)..."
    
    # Check if services are already running
    if docker compose ps | grep -q "Up"; then
        print_warning "Some Docker services are already running"
        docker compose ps
    else
        docker compose up -d
        
        # Wait for PostgreSQL to be ready
        print_status "Waiting for PostgreSQL to be ready..."
        local retries=30
        while [ $retries -gt 0 ]; do
            if docker compose exec -T postgres pg_isready -U postgres &> /dev/null; then
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

# Seed database with initial data
seed_database() {
    print_status "Seeding database with initial data..."
    
    local db_url="postgresql://postgres:postgres@localhost:5432/postgres?sslmode=disable"
    
    # Upsert eu-central-1 region
    print_status "Upserting eu-central-1 region..."
    docker compose exec -T postgres psql "$db_url" -c "
        INSERT INTO regions (id, name, code, country, created_at, updated_at) 
        VALUES (
            '01994e90-2c67-72c7-be2f-ec9089969c2f',
            'eu-central-1',
            'eu-central-1',
            'DE',
            NOW(),
            NOW()
        )
        ON CONFLICT (code) 
        DO UPDATE SET 
            name = EXCLUDED.name,
            country = EXCLUDED.country,
            updated_at = NOW()
        WHERE regions.deleted_at IS NULL;
    " > /dev/null
    
    print_success "Database seeding completed"
}

# All environment variables are already exported from .env file at script startup

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
    
    # Environment variables are already exported from .env file
    
    # Start the service
    if [ "$service_name" = "nodeagent" ]; then
        # Nodeagent requires root privileges for firecracker-containerd
        # nohup sudo -E bash -c "$service_cmd" > "$log_file" 2>&1 &
        nohup bash -c "$service_cmd" > "$log_file" 2>&1 &
    else
        nohup bash -c "$service_cmd" > "$log_file" 2>&1 &
    fi
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
    
    local services=("nodeagent" "edgeproxy" "builder" "certmanager" "listener" "manager")
    
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
    
    # Seed database with initial data
    if ! seed_database; then
        print_error "Failed to seed database"
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
    
    # Start certmanager
    start_service "certmanager" "go run ./cmd/certmanager/certmanager.go"
    
    # Start listener
    start_service "listener" "go run ./cmd/listener/listener.go"
    
    # Start manager (should be last as it orchestrates other services)
    start_service "manager" "go run ./cmd/manager/manager.go"
    
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
        docker compose down
        print_success "Development environment stopped"
        ;;
    *)
        main "${1:-}"
        ;;
esac
