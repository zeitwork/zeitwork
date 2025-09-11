#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "${SCRIPT_DIR}/common.sh"

NS=${NS:-fc6}
NAME=$(cat /tmp/last_svc_name)

log "Discovering VM ID from recent logs"
VMID=$(tac /tmp/firecracker-containerd.log | grep -m1 -oE 'vmID=[a-f0-9\-]+' | head -n1 | cut -d= -f2 || true)
if [[ -z "$VMID" ]]; then
  err "Could not determine VMID"
  exit 1
fi
log "VMID: ${VMID}"

log "Finding assigned IPv6 from CNI lease directory"
LEASE_DIR=/var/lib/cni/networks/fcnet6
IP6=""
for f in $(sudo ls -1 ${LEASE_DIR}/fd00:fc::* 2>/dev/null || true); do
  if grep -q "$VMID" "$f" 2>/dev/null; then
    IP6=$(basename "$f")
    break
  fi
done
if [[ -z "$IP6" ]]; then
  err "No IPv6 lease found for VM ${VMID}"
  exit 1
fi
log "Assigning IPv6 ${IP6} inside VM"

run "sudo /usr/local/bin/firecracker-ctr --address /run/firecracker-containerd/containerd.sock -n ${NS} tasks exec --exec-id ifup-$RANDOM ${NAME} sh -lc 'ip link set eth0 up'"
run "sudo /usr/local/bin/firecracker-ctr --address /run/firecracker-containerd/containerd.sock -n ${NS} tasks exec --exec-id ip-$RANDOM ${NAME} sh -lc 'ip -6 addr add ${IP6}/64 dev eth0 || true'"
run "sudo /usr/local/bin/firecracker-ctr --address /run/firecracker-containerd/containerd.sock -n ${NS} tasks exec --exec-id rt-$RANDOM ${NAME} sh -lc 'ip -6 route add default via fd00:fc::1 dev eth0 || true'"

echo "${IP6}" > "/tmp/${NAME}.ipv6"
log "Configured IPv6 ${IP6}"


