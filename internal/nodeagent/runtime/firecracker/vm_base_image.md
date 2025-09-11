# VM Base Image Configuration for Zeitwork Platform

## Overview

This document describes the base VM image configuration that enables automatic IP configuration via Firecracker's MMDS (Micro Metadata Service).

## Base Image Requirements

The base VM image should include:

### 1. Network Configuration Script

Create `/etc/zeitwork/configure-network.sh`:

```bash
#!/bin/bash
set -euo pipefail

# Configure networking using metadata from Firecracker MMDS
MMDS_URL="http://169.254.169.254"

# Get network configuration from MMDS
if curl -s --max-time 5 "$MMDS_URL/network" > /tmp/network-config.json; then
    IPV6_ADDR=$(jq -r '.ipv6_address' /tmp/network-config.json)
    INTERFACE=$(jq -r '.interface' /tmp/network-config.json)
    PREFIX_LEN=$(jq -r '.prefix_len' /tmp/network-config.json)
    GATEWAY=$(jq -r '.gateway' /tmp/network-config.json)

    if [ "$IPV6_ADDR" != "null" ] && [ "$IPV6_ADDR" != "" ]; then
        echo "Configuring IPv6: $IPV6_ADDR/$PREFIX_LEN on $INTERFACE"

        # Bring interface up
        ip link set "$INTERFACE" up

        # Add IPv6 address
        ip -6 addr add "$IPV6_ADDR/$PREFIX_LEN" dev "$INTERFACE"

        # Add default route if gateway provided
        if [ "$GATEWAY" != "null" ] && [ "$GATEWAY" != "" ]; then
            ip -6 route add default via "$GATEWAY" dev "$INTERFACE"
        fi

        echo "Network configuration complete: $IPV6_ADDR"
    else
        echo "No IPv6 configuration found in metadata"
    fi
else
    echo "Failed to fetch network configuration from MMDS"
fi
```

### 2. Systemd Service

Create `/etc/systemd/system/zeitwork-network.service`:

```ini
[Unit]
Description=Zeitwork Network Configuration
After=network.target
Before=multi-user.target

[Service]
Type=oneshot
ExecStart=/etc/zeitwork/configure-network.sh
RemainAfterExit=yes
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

### 3. Enable Service in Base Image

```bash
# During base image build:
systemctl enable zeitwork-network.service
```

## Usage

When the nodeagent starts a VM:

1. **Database IP**: Read `instance.ip_address` from database
2. **MMDS Metadata**: Set IP via `client.SetMetadata()` before starting VM
3. **VM Boot**: VM automatically configures networking via systemd service
4. **Ready**: VM boots with IP pre-configured and reachable

## Benefits

- ✅ **No Runtime IP Management**: IP configured during boot automatically
- ✅ **Database-Driven**: IP comes from database as per architecture
- ✅ **Consistent**: Same process for all VMs
- ✅ **Fast**: No post-boot configuration delays
- ✅ **Reliable**: Networking ready when VM reaches "running" state

## Implementation in Nodeagent

The Firecracker client now supports this via:

```go
instance := &firecracker.VMInstance{
    ID:        "vm-123",
    IPAddress: net.ParseIP("fd00:fc::100"), // From database
    // ... other fields
}

client, _ := firecracker.NewClient(instance)
// MMDS automatically configured with IP metadata
client.Start(ctx) // VM boots with IP pre-configured
```

## Testing

Use the new `TestFirecrackerNetworkConnectivity` test to verify:

- TAP device creation
- MMDS metadata configuration
- VM network interface setup
- Ready for HTTP server connectivity testing
