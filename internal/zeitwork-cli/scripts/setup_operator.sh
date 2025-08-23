#!/bin/bash
set -e

# Accept DATABASE_URL as first argument
DATABASE_URL="$1"
if [ -z "$DATABASE_URL" ]; then
    echo "Error: DATABASE_URL is required as first argument"
    exit 1
fi

# Stop existing services if they're running (ignore errors if they don't exist)
echo "Stopping existing services..."
sudo systemctl stop zeitwork-operator 2>/dev/null || true
sudo systemctl stop zeitwork-load-balancer 2>/dev/null || true
sudo systemctl stop zeitwork-edge-proxy 2>/dev/null || true

# Give services time to fully stop
sleep 2

# Extract binaries (suppress extended header warnings from macOS tar)
cd /tmp
tar -xzf zeitwork-binaries.tar.gz 2>&1 | grep -v "Ignoring unknown extended header" || true

# Install binaries (use -f to force overwrite)
sudo cp -f build/zeitwork-operator /usr/local/bin/
sudo cp -f build/zeitwork-load-balancer /usr/local/bin/
sudo cp -f build/zeitwork-edge-proxy /usr/local/bin/
sudo chmod +x /usr/local/bin/zeitwork-*

# Create directories
sudo mkdir -p /etc/zeitwork /var/lib/zeitwork /var/log/zeitwork

# Create service user
sudo useradd -r -s /bin/false zeitwork 2>/dev/null || true
sudo chown -R zeitwork:zeitwork /var/lib/zeitwork /var/log/zeitwork

# Create configurations
cat << EOF | sudo tee /etc/zeitwork/operator.env
SERVICE_NAME=zeitwork-operator
PORT=8080
DATABASE_URL=${DATABASE_URL}
NODE_AGENT_PORT=8081
LOG_LEVEL=info
ENVIRONMENT=production
EOF

cat << EOF | sudo tee /etc/zeitwork/load-balancer.env
SERVICE_NAME=zeitwork-load-balancer
PORT=8082
OPERATOR_URL=http://localhost:8080
HEALTH_CHECK_INTERVAL=10s
ALGORITHM=round-robin
LOG_LEVEL=info
ENVIRONMENT=production
EOF

cat << EOF | sudo tee /etc/zeitwork/edge-proxy.env
SERVICE_NAME=zeitwork-edge-proxy
PORT=443
HTTP_PORT=8083
OPERATOR_URL=http://localhost:8080
RATE_LIMIT_PER_IP=100
RATE_LIMIT_WINDOW=1m
SSL_CERT_PATH=/etc/zeitwork/certs/server.crt
SSL_KEY_PATH=/etc/zeitwork/certs/server.key
LOG_LEVEL=info
ENVIRONMENT=production
EOF

# Generate self-signed certificate
sudo mkdir -p /etc/zeitwork/certs
sudo openssl req -x509 -newkey rsa:4096 -keyout /etc/zeitwork/certs/server.key \
    -out /etc/zeitwork/certs/server.crt -days 365 -nodes \
    -subj "/C=US/ST=State/L=City/O=Zeitwork/CN=*.zeitwork.com" 2>/dev/null
sudo chmod 644 /etc/zeitwork/certs/server.crt
sudo chmod 600 /etc/zeitwork/certs/server.key
sudo chown zeitwork:zeitwork /etc/zeitwork/certs/*

# Create systemd services
sudo tee /etc/systemd/system/zeitwork-operator.service << SERVICE
[Unit]
Description=Zeitwork Operator Service
After=network.target

[Service]
Type=simple
User=zeitwork
EnvironmentFile=/etc/zeitwork/operator.env
ExecStart=/usr/local/bin/zeitwork-operator
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
SERVICE

sudo tee /etc/systemd/system/zeitwork-load-balancer.service << SERVICE
[Unit]
Description=Zeitwork Load Balancer Service
After=network.target zeitwork-operator.service

[Service]
Type=simple
User=zeitwork
EnvironmentFile=/etc/zeitwork/load-balancer.env
ExecStart=/usr/local/bin/zeitwork-load-balancer
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
SERVICE

sudo tee /etc/systemd/system/zeitwork-edge-proxy.service << SERVICE
[Unit]
Description=Zeitwork Edge Proxy Service
After=network.target zeitwork-operator.service

[Service]
Type=simple
User=zeitwork
EnvironmentFile=/etc/zeitwork/edge-proxy.env
ExecStart=/usr/local/bin/zeitwork-edge-proxy
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
SERVICE

# Start services
sudo systemctl daemon-reload
sudo systemctl enable zeitwork-operator zeitwork-load-balancer zeitwork-edge-proxy

# Start services with error handling
echo "Starting services..."
if sudo systemctl start zeitwork-operator; then
    echo "Operator service started successfully"
else
    echo "Failed to start operator service, checking logs..."
    sudo journalctl -u zeitwork-operator --no-pager -n 20
    exit 1
fi

sleep 5

if sudo systemctl start zeitwork-load-balancer; then
    echo "Load balancer service started successfully"
else
    echo "Failed to start load balancer service, checking logs..."
    sudo journalctl -u zeitwork-load-balancer --no-pager -n 20
    exit 1
fi

sleep 2

if sudo systemctl start zeitwork-edge-proxy; then
    echo "Edge proxy service started successfully"
else
    echo "Failed to start edge proxy service, checking logs..."
    sudo journalctl -u zeitwork-edge-proxy --no-pager -n 20
    exit 1
fi

echo "Operator setup complete"
