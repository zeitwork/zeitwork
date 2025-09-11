#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "${SCRIPT_DIR}/common.sh"

NAME_FILE=/tmp/last_svc_name
if [[ -f "$NAME_FILE" ]]; then
  NAME=$(cat "$NAME_FILE")
fi
IP6_FILE=/tmp/${NAME:-svc}.ipv6

IP6=${1:-}
PORT=${2:-3000}

if [[ -z "$IP6" ]]; then
  if [[ -f "$IP6_FILE" ]]; then
    IP6=$(cat "$IP6_FILE")
  else
    err "Usage: $0 <ipv6> [port]"
    exit 1
  fi
fi

log "Curling http://[${IP6}]:${PORT}"
set +e
RESP=$(curl -g -6 --connect-timeout 5 --max-time 10 "http://[${IP6}]:${PORT}" 2>/dev/null)
rc=$?
set -e
if [[ $rc -ne 0 ]]; then
  err "curl failed with code $rc"
  exit $rc
fi
echo "$RESP"
if ! echo "$RESP" | grep -qi "hello world"; then
  err "Unexpected response"
  exit 1
fi
log "IPv6 curl verified"


