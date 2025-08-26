#!/bin/bash
#
# Zeitwork Platform Bootstrap Script
# This script initializes a new Zeitwork platform or recovers an existing installation
#
set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
CONFIG_FILE="${CONFIG_FILE:-$HOME/.zeitwork/bootstrap.conf}"

# Default values
DEFAULT_REGION_1="eu-central-1"
DEFAULT_REGION_2="us-east-1"
DEFAULT_REGION_3="asia-southeast-1"
DEFAULT_S3_ENDPOINT=""
DEFAULT_S3_BUCKET="zeitwork-images"
DEFAULT_DB_HOST=""
DEFAULT_DB_NAME="zeitwork_production"

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    local prereqs_met=true
    
    # Check for required commands
    for cmd in psql ssh-keygen openssl docker go make bun; do
        if ! command -v $cmd &> /dev/null; then
            log_error "$cmd is not installed"
            prereqs_met=false
        fi
    done
    
    # Check Go version
    if command -v go &> /dev/null; then
        GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
        REQUIRED_GO_VERSION="1.21"
        if [ "$(printf '%s\n' "$REQUIRED_GO_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED_GO_VERSION" ]; then
            log_error "Go version $REQUIRED_GO_VERSION or higher required (found $GO_VERSION)"
            prereqs_met=false
        fi
    fi
    
    if [ "$prereqs_met" = false ]; then
        log_error "Prerequisites check failed. Please install missing dependencies."
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Load or create configuration
load_config() {
    if [ -f "$CONFIG_FILE" ]; then
        log_info "Loading existing configuration from $CONFIG_FILE"
        source "$CONFIG_FILE"
    else
        log_info "No configuration found. Creating new configuration..."
        mkdir -p "$(dirname "$CONFIG_FILE")"
        create_config
    fi
}

# Create new configuration
create_config() {
    echo "=== Zeitwork Platform Configuration ==="
    
    # Database configuration
    read -p "PlanetScale Database URL (postgresql://...): " DATABASE_URL
    
    # S3 configuration
    read -p "S3 Endpoint (leave empty for AWS S3): " S3_ENDPOINT
    read -p "S3 Bucket name [$DEFAULT_S3_BUCKET]: " S3_BUCKET
    S3_BUCKET=${S3_BUCKET:-$DEFAULT_S3_BUCKET}
    read -p "S3 Access Key ID: " S3_ACCESS_KEY_ID
    read -s -p "S3 Secret Access Key: " S3_SECRET_ACCESS_KEY
    echo
    read -p "S3 Region [us-east-1]: " S3_REGION
    S3_REGION=${S3_REGION:-us-east-1}
    
    # Domain configuration
    read -p "Primary domain (e.g., zeitwork.com): " PRIMARY_DOMAIN
    
    # GitHub configuration
    read -p "GitHub App Client ID: " GITHUB_CLIENT_ID
    read -s -p "GitHub App Client Secret: " GITHUB_CLIENT_SECRET
    echo
    
    # Regions
    read -p "Region 1 [$DEFAULT_REGION_1]: " REGION_1
    REGION_1=${REGION_1:-$DEFAULT_REGION_1}
    read -p "Region 2 [$DEFAULT_REGION_2]: " REGION_2
    REGION_2=${REGION_2:-$DEFAULT_REGION_2}
    read -p "Region 3 [$DEFAULT_REGION_3]: " REGION_3
    REGION_3=${REGION_3:-$DEFAULT_REGION_3}
    
    # Node configuration
    echo "Enter IP addresses for operator nodes (3 per region):"
    for i in 1 2 3; do
        for j in 1 2 3; do
            read -p "Region $i Operator Node $j IP: " "OPERATOR_${i}_${j}_IP"
        done
    done
    
    echo "Enter IP addresses for worker nodes (6 per region):"
    for i in 1 2 3; do
        for j in 1 2 3 4 5 6; do
            read -p "Region $i Worker Node $j IP: " "WORKER_${i}_${j}_IP"
        done
    done
    
    # Save configuration
    cat > "$CONFIG_FILE" <<EOF
# Zeitwork Platform Configuration
# Generated on $(date)

DATABASE_URL="$DATABASE_URL"

S3_ENDPOINT="$S3_ENDPOINT"
S3_BUCKET="$S3_BUCKET"
S3_ACCESS_KEY_ID="$S3_ACCESS_KEY_ID"
S3_SECRET_ACCESS_KEY="$S3_SECRET_ACCESS_KEY"
S3_REGION="$S3_REGION"

PRIMARY_DOMAIN="$PRIMARY_DOMAIN"
GITHUB_CLIENT_ID="$GITHUB_CLIENT_ID"
GITHUB_CLIENT_SECRET="$GITHUB_CLIENT_SECRET"

REGION_1="$REGION_1"
REGION_2="$REGION_2"
REGION_3="$REGION_3"

$(for i in 1 2 3; do
    for j in 1 2 3; do
        var="OPERATOR_${i}_${j}_IP"
        echo "$var=\"${!var}\""
    done
done)

$(for i in 1 2 3; do
    for j in 1 2 3 4 5 6; do
        var="WORKER_${i}_${j}_IP"
        echo "$var=\"${!var}\""
    done
done)
EOF
    
    log_success "Configuration saved to $CONFIG_FILE"
}

# Generate or use existing SSH key
setup_ssh_key() {
    log_info "Setting up SSH key..."
    
    SSH_KEY_PATH="$HOME/.ssh/zeitwork_deploy"
    
    if [ -f "$SSH_KEY_PATH" ]; then
        log_info "Using existing SSH key at $SSH_KEY_PATH"
    else
        log_info "Generating new SSH key..."
        ssh-keygen -t ed25519 -f "$SSH_KEY_PATH" -N "" -C "zeitwork-deploy"
        log_success "SSH key generated at $SSH_KEY_PATH"
    fi
}

# Setup database
setup_database() {
    log_info "Setting up database..."
    
    # Check database connection
    if psql "$DATABASE_URL" -c "SELECT 1" &> /dev/null; then
        log_success "Database connection successful"
    else
        log_error "Failed to connect to database"
        exit 1
    fi
    
    # Check if database is already initialized
    if psql "$DATABASE_URL" -c "SELECT 1 FROM regions LIMIT 1" &> /dev/null 2>&1; then
        log_warning "Database already initialized. Checking for recovery..."
        recover_from_database
    else
        log_info "Initializing new database..."
        initialize_database
    fi
}

# Initialize new database
initialize_database() {
    log_info "Running database migrations..."
    
    cd "$PROJECT_ROOT/packages/database"
    export DATABASE_URL
    bun install
    bun run db:migrate
    
    log_success "Database migrations completed"
    
    # Insert initial regions
    log_info "Inserting initial regions..."
    psql "$DATABASE_URL" <<EOF
INSERT INTO regions (name, code, country) VALUES
    ('Europe Central', '$REGION_1', 'Germany'),
    ('US East', '$REGION_2', 'United States'),
    ('Asia Southeast', '$REGION_3', 'Singapore');
EOF
    
    log_success "Regions initialized"
}

# Recover from existing database
recover_from_database() {
    log_info "Recovering from existing database state..."
    
    # Get existing regions
    EXISTING_REGIONS=$(psql -t -A "$DATABASE_URL" -c "SELECT code FROM regions ORDER BY created_at")
    
    if [ -n "$EXISTING_REGIONS" ]; then
        log_info "Found existing regions: $EXISTING_REGIONS"
    fi
    
    # Get existing nodes
    EXISTING_NODES=$(psql -t -A "$DATABASE_URL" -c "SELECT COUNT(*) FROM nodes WHERE state != 'terminated'")
    
    if [ "$EXISTING_NODES" -gt 0 ]; then
        log_info "Found $EXISTING_NODES existing nodes"
    fi
    
    log_success "Recovery information collected"
}

# Generate SSL certificates
generate_ssl_certificates() {
    log_info "Generating SSL certificates..."
    
    SSL_DIR="$HOME/.zeitwork/ssl"
    mkdir -p "$SSL_DIR"
    
    # Generate self-signed certificates for development
    # In production, use Let's Encrypt or commercial certificates
    
    for domain in "$PRIMARY_DOMAIN" "*.${PRIMARY_DOMAIN}" "*.zeitwork.app" "*.zeitwork-dns.com"; do
        cert_name=$(echo "$domain" | sed 's/\*\./wildcard./g')
        
        if [ -f "$SSL_DIR/${cert_name}.crt" ]; then
            log_info "Certificate for $domain already exists"
        else
            log_info "Generating certificate for $domain..."
            
            openssl req -x509 -newkey rsa:4096 -sha256 -days 365 \
                -nodes -keyout "$SSL_DIR/${cert_name}.key" \
                -out "$SSL_DIR/${cert_name}.crt" \
                -subj "/CN=$domain" \
                -addext "subjectAltName=DNS:$domain" 2>/dev/null
            
            log_success "Certificate generated for $domain"
        fi
    done
}

# Setup S3 bucket
setup_s3_bucket() {
    log_info "Setting up S3 bucket..."
    
    # Create S3 configuration file
    cat > "$HOME/.zeitwork/s3.conf" <<EOF
S3_ENDPOINT=$S3_ENDPOINT
S3_BUCKET=$S3_BUCKET
S3_ACCESS_KEY_ID=$S3_ACCESS_KEY_ID
S3_SECRET_ACCESS_KEY=$S3_SECRET_ACCESS_KEY
S3_REGION=$S3_REGION
EOF
    
    log_success "S3 configuration saved"
    
    # TODO: Create bucket if using MinIO
    if [ -n "$S3_ENDPOINT" ]; then
        log_info "Using custom S3 endpoint: $S3_ENDPOINT"
        # Add MinIO bucket creation logic here
    else
        log_info "Using AWS S3"
        # Bucket should be pre-created in AWS
    fi
}

# Build binaries
build_binaries() {
    log_info "Building Zeitwork binaries..."
    
    cd "$PROJECT_ROOT"
    
    # Build all services
    make build
    
    # Build API
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo \
        -ldflags "-s -w" -o build/zeitwork-api ./cmd/api
    
    # Build CLI
    go build -o build/zeitwork-cli ./cmd/cli
    
    log_success "Binaries built successfully"
}

# Deploy to nodes
deploy_to_nodes() {
    log_info "Deploying to nodes..."
    
    # Create deployment package
    DEPLOY_DIR="/tmp/zeitwork-deploy-$(date +%s)"
    mkdir -p "$DEPLOY_DIR"
    
    cp -r "$PROJECT_ROOT/build/"* "$DEPLOY_DIR/"
    cp -r "$PROJECT_ROOT/deployments/config/"* "$DEPLOY_DIR/"
    cp -r "$PROJECT_ROOT/deployments/systemd/"* "$DEPLOY_DIR/"
    
    # Add configuration
    cat > "$DEPLOY_DIR/bootstrap.env" <<EOF
DATABASE_URL=$DATABASE_URL
S3_ENDPOINT=$S3_ENDPOINT
S3_BUCKET=$S3_BUCKET
S3_ACCESS_KEY_ID=$S3_ACCESS_KEY_ID
S3_SECRET_ACCESS_KEY=$S3_SECRET_ACCESS_KEY
S3_REGION=$S3_REGION
GITHUB_CLIENT_ID=$GITHUB_CLIENT_ID
GITHUB_CLIENT_SECRET=$GITHUB_CLIENT_SECRET
EOF
    
    # Deploy to each region
    for region_num in 1 2 3; do
        region_var="REGION_${region_num}"
        region="${!region_var}"
        
        log_info "Deploying to region: $region"
        
        # Deploy to operators
        for node_num in 1 2 3; do
            ip_var="OPERATOR_${region_num}_${node_num}_IP"
            node_ip="${!ip_var}"
            
            if [ -n "$node_ip" ]; then
                log_info "Deploying operator to $node_ip..."
                deploy_operator_node "$node_ip" "$region" "$DEPLOY_DIR"
            fi
        done
        
        # Deploy to workers
        for node_num in 1 2 3 4 5 6; do
            ip_var="WORKER_${region_num}_${node_num}_IP"
            node_ip="${!ip_var}"
            
            if [ -n "$node_ip" ]; then
                log_info "Deploying worker to $node_ip..."
                deploy_worker_node "$node_ip" "$region" "$DEPLOY_DIR"
            fi
        done
    done
    
    rm -rf "$DEPLOY_DIR"
    log_success "Deployment completed"
}

# Deploy operator node
deploy_operator_node() {
    local node_ip=$1
    local region=$2
    local deploy_dir=$3
    
    # Copy files
    scp -i "$SSH_KEY_PATH" -r "$deploy_dir"/* "root@$node_ip:/tmp/" 2>/dev/null || true
    
    # Install and configure
    ssh -i "$SSH_KEY_PATH" "root@$node_ip" <<EOF
# Create directories
mkdir -p /usr/local/bin /etc/zeitwork /etc/systemd/system

# Copy binaries
cp /tmp/zeitwork-operator /usr/local/bin/
cp /tmp/zeitwork-load-balancer /usr/local/bin/
cp /tmp/zeitwork-edge-proxy /usr/local/bin/
chmod +x /usr/local/bin/zeitwork-*

# Copy configuration
cp /tmp/*.env /etc/zeitwork/
cp /tmp/*.service /etc/systemd/system/

# Add region-specific configuration
echo "REGION=$region" >> /etc/zeitwork/operator.env
echo "NODE_TYPE=operator" >> /etc/zeitwork/operator.env

# Start services
systemctl daemon-reload
systemctl enable zeitwork-operator zeitwork-load-balancer zeitwork-edge-proxy
systemctl restart zeitwork-operator
systemctl restart zeitwork-load-balancer
systemctl restart zeitwork-edge-proxy
EOF
    
    log_success "Operator deployed to $node_ip"
}

# Deploy worker node
deploy_worker_node() {
    local node_ip=$1
    local region=$2
    local deploy_dir=$3
    
    # Copy files
    scp -i "$SSH_KEY_PATH" -r "$deploy_dir"/* "root@$node_ip:/tmp/" 2>/dev/null || true
    
    # Install and configure
    ssh -i "$SSH_KEY_PATH" "root@$node_ip" <<EOF
# Create directories
mkdir -p /usr/local/bin /etc/zeitwork /etc/systemd/system
mkdir -p /var/lib/zeitwork/vms /var/lib/zeitwork/images
mkdir -p /var/lib/firecracker/kernels

# Copy binaries
cp /tmp/zeitwork-node-agent /usr/local/bin/
chmod +x /usr/local/bin/zeitwork-node-agent

# Copy configuration
cp /tmp/*.env /etc/zeitwork/
cp /tmp/*.service /etc/systemd/system/

# Add region-specific configuration
echo "REGION=$region" >> /etc/zeitwork/node-agent.env
echo "NODE_TYPE=worker" >> /etc/zeitwork/node-agent.env

# Download Firecracker
FC_VERSION="v1.12.1"
ARCH=\$(uname -m)
cd /tmp
wget https://github.com/firecracker-microvm/firecracker/releases/download/\${FC_VERSION}/firecracker-\${FC_VERSION}-\${ARCH}.tgz
tar -xzf firecracker-\${FC_VERSION}-\${ARCH}.tgz
cp release-\${FC_VERSION}-\${ARCH}/firecracker-\${FC_VERSION}-\${ARCH} /usr/bin/firecracker
chmod +x /usr/bin/firecracker

# Download kernel
cd /var/lib/firecracker/kernels
wget https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/\${ARCH}/kernels/vmlinux.bin

# Start services
systemctl daemon-reload
systemctl enable zeitwork-node-agent
systemctl restart zeitwork-node-agent
EOF
    
    log_success "Worker deployed to $node_ip"
}

# Register nodes in database
register_nodes() {
    log_info "Registering nodes in database..."
    
    # Get region IDs
    REGION_1_ID=$(psql -t -A "$DATABASE_URL" -c "SELECT id FROM regions WHERE code = '$REGION_1'")
    REGION_2_ID=$(psql -t -A "$DATABASE_URL" -c "SELECT id FROM regions WHERE code = '$REGION_2'")
    REGION_3_ID=$(psql -t -A "$DATABASE_URL" -c "SELECT id FROM regions WHERE code = '$REGION_3'")
    
    # Register operators
    for region_num in 1 2 3; do
        region_id_var="REGION_${region_num}_ID"
        region_id="${!region_id_var}"
        
        for node_num in 1 2 3; do
            ip_var="OPERATOR_${region_num}_${node_num}_IP"
            node_ip="${!ip_var}"
            
            if [ -n "$node_ip" ]; then
                psql "$DATABASE_URL" <<EOF
INSERT INTO nodes (region_id, hostname, ip_address, state, resources)
VALUES ('$region_id', 'operator-${region_num}-${node_num}', '$node_ip', 'ready', 
        '{"vcpu": 4, "memory": 8192, "type": "operator"}'::jsonb)
ON CONFLICT (hostname) DO UPDATE
SET ip_address = EXCLUDED.ip_address,
    state = EXCLUDED.state,
    updated_at = NOW();
EOF
            fi
        done
    done
    
    # Register workers
    for region_num in 1 2 3; do
        region_id_var="REGION_${region_num}_ID"
        region_id="${!region_id_var}"
        
        for node_num in 1 2 3 4 5 6; do
            ip_var="WORKER_${region_num}_${node_num}_IP"
            node_ip="${!ip_var}"
            
            if [ -n "$node_ip" ]; then
                psql "$DATABASE_URL" <<EOF
INSERT INTO nodes (region_id, hostname, ip_address, state, resources)
VALUES ('$region_id', 'worker-${region_num}-${node_num}', '$node_ip', 'ready', 
        '{"vcpu": 8, "memory": 16384, "type": "worker"}'::jsonb)
ON CONFLICT (hostname) DO UPDATE
SET ip_address = EXCLUDED.ip_address,
    state = EXCLUDED.state,
    updated_at = NOW();
EOF
            fi
        done
    done
    
    log_success "Nodes registered in database"
}

# Verify deployment
verify_deployment() {
    log_info "Verifying deployment..."
    
    local all_good=true
    
    # Check operator services
    for region_num in 1 2 3; do
        for node_num in 1 2 3; do
            ip_var="OPERATOR_${region_num}_${node_num}_IP"
            node_ip="${!ip_var}"
            
            if [ -n "$node_ip" ]; then
                if curl -s "http://$node_ip:8080/health" | grep -q "healthy"; then
                    log_success "Operator at $node_ip is healthy"
                else
                    log_error "Operator at $node_ip is not responding"
                    all_good=false
                fi
            fi
        done
    done
    
    # Check worker services
    for region_num in 1 2 3; do
        for node_num in 1 2 3 4 5 6; do
            ip_var="WORKER_${region_num}_${node_num}_IP"
            node_ip="${!ip_var}"
            
            if [ -n "$node_ip" ]; then
                if curl -s "http://$node_ip:8081/health" | grep -q "healthy"; then
                    log_success "Worker at $node_ip is healthy"
                else
                    log_error "Worker at $node_ip is not responding"
                    all_good=false
                fi
            fi
        done
    done
    
    if [ "$all_good" = true ]; then
        log_success "All nodes are healthy"
    else
        log_warning "Some nodes are not healthy. Please check the logs."
    fi
}

# Main execution
main() {
    echo "==================================="
    echo "  Zeitwork Platform Bootstrap"
    echo "==================================="
    echo
    
    check_prerequisites
    load_config
    setup_ssh_key
    setup_database
    generate_ssl_certificates
    setup_s3_bucket
    build_binaries
    deploy_to_nodes
    register_nodes
    verify_deployment
    
    echo
    log_success "Zeitwork platform bootstrap completed!"
    echo
    echo "Next steps:"
    echo "1. Configure DNS to point *.${PRIMARY_DOMAIN} to your edge proxies"
    echo "2. Create a GitHub App and update the configuration"
    echo "3. Access the platform at https://app.${PRIMARY_DOMAIN}"
    echo
    echo "Configuration saved at: $CONFIG_FILE"
    echo "SSL certificates at: $HOME/.zeitwork/ssl/"
    echo "S3 configuration at: $HOME/.zeitwork/s3.conf"
}

# Run main function
main "$@"
