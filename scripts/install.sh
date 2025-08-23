#!/bin/bash
set -e

# Installation script for Zeitwork platform services
# Installs binaries and systemd service files

if [ "$EUID" -ne 0 ]; then 
    echo "Please run as root (use sudo)"
    exit 1
fi

# Configuration
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/zeitwork"
DATA_DIR="/var/lib/zeitwork"
LOG_DIR="/var/log/zeitwork"
SYSTEMD_DIR="/etc/systemd/system"

# Create directories
echo "Creating directories..."
mkdir -p $CONFIG_DIR
mkdir -p $DATA_DIR
mkdir -p $LOG_DIR

# Create zeitwork user (for services that don't need root)
if ! id -u zeitwork >/dev/null 2>&1; then
    echo "Creating zeitwork user..."
    useradd -r -s /bin/false -d $DATA_DIR zeitwork
fi

# Install binaries
echo "Installing binaries..."
cp build/zeitwork-operator $INSTALL_DIR/
cp build/zeitwork-node-agent $INSTALL_DIR/
cp build/zeitwork-load-balancer $INSTALL_DIR/
cp build/zeitwork-edge-proxy $INSTALL_DIR/

chmod +x $INSTALL_DIR/zeitwork-*

# Install systemd service files
echo "Installing systemd services..."
cp deployments/systemd/*.service $SYSTEMD_DIR/

# Set ownership
chown -R zeitwork:zeitwork $DATA_DIR
chown -R zeitwork:zeitwork $LOG_DIR

# Install configuration templates
echo "Installing configuration templates..."
cp deployments/config/*.env $CONFIG_DIR/

echo "Installation complete!"
echo ""
echo "Next steps:"
echo "1. Edit configuration files in $CONFIG_DIR"
echo "2. Set up PostgreSQL database and update DATABASE_URL in $CONFIG_DIR/operator.env"
echo "3. Enable and start services:"
echo "   sudo systemctl daemon-reload"
echo "   sudo systemctl enable zeitwork-operator"
echo "   sudo systemctl start zeitwork-operator"
echo "   (Start other services as needed)"
echo ""
echo "To check service status:"
echo "   sudo systemctl status zeitwork-operator"
echo "   sudo journalctl -u zeitwork-operator -f"
