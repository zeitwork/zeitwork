#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "${SCRIPT_DIR}/common.sh"

log "Writing IPv6-only CNI conflist fcnet6"
run "sudo mkdir -p /etc/cni/net.d /etc/cni/conf.d"
sudo tee /etc/cni/net.d/fcnet6.conflist >/dev/null <<'EOF'
{
  "cniVersion": "0.4.0",
  "name": "fcnet6",
  "plugins": [
    {
      "type": "ptp",
      "ipMasq": false,
      "ipam": {
        "type": "host-local",
        "ranges": [[ {"subnet": "fd00:fc::/64"} ]],
        "routes": [ {"dst": "::/0"} ]
      }
    },
    {
      "type": "tc-redirect-tap"
    }
  ]
}
EOF

# Also copy to /etc/cni/conf.d for runtimes expecting that path
run "sudo cp /etc/cni/net.d/fcnet6.conflist /etc/cni/conf.d/fcnet6.conflist"

log "IPv6 CNI config written"


