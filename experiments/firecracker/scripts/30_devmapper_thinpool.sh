#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "${SCRIPT_DIR}/common.sh"

DIR=/var/lib/firecracker-containerd/snapshotter/devmapper
POOL=fc-dev-thinpool

log "Setting up devmapper thinpool at ${DIR} (loopback)"
run "sudo mkdir -p ${DIR}"

if [[ ! -f "${DIR}/data" ]]; then
  run "sudo touch ${DIR}/data"
  run "sudo truncate -s 100G ${DIR}/data"
fi
if [[ ! -f "${DIR}/metadata" ]]; then
  run "sudo touch ${DIR}/metadata"
  run "sudo truncate -s 2G ${DIR}/metadata"
fi

DATADEV=$(sudo losetup --output NAME --noheadings --associated ${DIR}/data || true)
if [[ -z "${DATADEV}" ]]; then
  DATADEV=$(sudo losetup --find --show ${DIR}/data)
fi

METADEV=$(sudo losetup --output NAME --noheadings --associated ${DIR}/metadata || true)
if [[ -z "${METADEV}" ]]; then
  METADEV=$(sudo losetup --find --show ${DIR}/metadata)
fi

SECTORSIZE=512
DATASIZE=$(sudo blockdev --getsize64 -q ${DATADEV})
LENGTH_SECTORS=$(bash -lc "bc <<< \"${DATASIZE}/${SECTORSIZE}\"")
DATA_BLOCK_SIZE=128
LOW_WATER_MARK=32768
THINP_TABLE="0 ${LENGTH_SECTORS} thin-pool ${METADEV} ${DATADEV} ${DATA_BLOCK_SIZE} ${LOW_WATER_MARK} 1 skip_block_zeroing"

if ! sudo dmsetup reload "${POOL}" --table "${THINP_TABLE}" 2>/dev/null; then
  run "sudo dmsetup create ${POOL} --table '${THINP_TABLE}'"
fi

log "Thinpool ready: ${POOL}"


