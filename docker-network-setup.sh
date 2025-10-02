#!/bin/bash

# docker network setup

# if it exists, delete it
if docker network ls | grep -q zeitwork-network; then
    docker network rm zeitwork-network
fi

# with a custom large subnet and enable IPv6
docker network create zeitwork-network \
    --subnet 10.0.0.0/8 \
    --gateway 10.0.0.1 \
    --ipv6 \
    --subnet fdb6:cd2f:4329:2::/64 \
    --gateway fdb6:cd2f:4329:2::1 \
    --opt com.docker.network.bridge.enable_ip_masquerade=true

# get the bridge interface name
BRIDGE_INTERFACE=$(docker network inspect zeitwork-network -f '{{.Options.com.docker.network.bridge.name}}' 2>/dev/null)
if [ -z "$BRIDGE_INTERFACE" ]; then
    # fallback: get the bridge name from the network ID
    BRIDGE_INTERFACE="br-$(docker network inspect zeitwork-network -f '{{.Id}}' | cut -c1-12)"
fi

echo "Bridge interface: $BRIDGE_INTERFACE"


# print the network details
docker network inspect zeitwork-network


# run docker mac nat connect
sudo /opt/homebrew/opt/docker-mac-net-connect/bin/docker-mac-net-connect