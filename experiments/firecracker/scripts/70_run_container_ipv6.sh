#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "${SCRIPT_DIR}/common.sh"

NS=${NS:-fc6}
NAME=${1:-svc-$(hexdump -vn6 -e '6/1 "%02x"' /dev/urandom)}
IMAGE=${2:-docker.io/local/hello3000:latest}
LOG="/tmp/${NAME}.log"

log "Ensuring namespace ${NS} with defaults"
run "sudo /usr/local/bin/firecracker-ctr --address /run/firecracker-containerd/containerd.sock namespaces create ${NS} || true"
run "sudo /usr/local/bin/firecracker-ctr --address /run/firecracker-containerd/containerd.sock namespaces label ${NS} containerd.io/defaults/runtime=aws.firecracker containerd.io/defaults/snapshotter=devmapper"

log "Images in namespace before run"
run "sudo /usr/local/bin/firecracker-ctr --address /run/firecracker-containerd/containerd.sock -n ${NS} images list"

log "Running VM-backed container ${NAME} from ${IMAGE}"
set +e
sudo /usr/local/bin/firecracker-ctr --address /run/firecracker-containerd/containerd.sock -n ${NS} run -d --net-host --cap-add CAP_NET_ADMIN --snapshotter devmapper --runtime aws.firecracker ${IMAGE} ${NAME} >>"${LOG}" 2>&1
rc=$?
set -e
if [[ $rc -ne 0 ]]; then
  err "ctr run failed with code $rc; last log lines:"
  tail -n 60 "${LOG}" || true
  exit $rc
fi

echo "${NAME}" > /tmp/last_svc_name

log "Waiting for task to appear"
for i in {1..60}; do
  if sudo /usr/local/bin/firecracker-ctr --address /run/firecracker-containerd/containerd.sock -n ${NS} tasks list | grep -qw "${NAME}"; then
    break
  fi
  sleep 1
 done

log "Waiting a bit for VM boot and service"
sleep 5

log "Querying IPv6 address"
IP6=$(sudo /usr/local/bin/firecracker-ctr --address /run/firecracker-containerd/containerd.sock -n ${NS} tasks exec --exec-id ip6-${RANDOM} ${NAME} sh -lc "ip -6 -o addr show dev eth0 scope global | awk '{print \$4}' | head -n1" | head -n1 | tr -d '\r')
IP6=${IP6%%/*}
if [[ -z "${IP6}" ]]; then
  err "Failed to discover IPv6"
  exit 1
fi

echo "${IP6}" > "/tmp/${NAME}.ipv6"
log "Container IPv6: ${IP6}"


