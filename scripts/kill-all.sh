#!/bin/bash

# Kill processes tracked by PID files
for pidfile in logs/*.pid; do
	if [ -f "$pidfile" ]; then
		pid=$(cat "$pidfile")
		if kill -0 "$pid" 2>/dev/null; then
			kill "$pid"
			echo "Killed process $pid"
		else
			echo "Process $pid not running"
		fi
		rm "$pidfile"
	fi
done

# Also kill any orphan/still-running services by name
# Built binaries (installed/locally built)
services=(
	"zeitwork-nodeagent"
	"zeitwork-edgeproxy"
	"zeitwork-builder"
	"zeitwork-certmanager"
	"zeitwork-listener"
	"zeitwork-manager"
)

for svc in "${services[@]}"; do
	if pkill -x "$svc" 2>/dev/null; then
		echo "Killed orphan service: $svc"
	fi
done

# Dev-run processes started via `go run ./cmd/<service>`
patterns=(
	"/cmd/nodeagent"
	"/cmd/edgeproxy"
	"/cmd/builder"
	"/cmd/certmanager"
	"/cmd/listener"
	"/cmd/manager"
)

for pat in "${patterns[@]}"; do
	if pkill -f "$pat" 2>/dev/null; then
		echo "Killed processes matching pattern: $pat"
	fi
done

echo "All known Zeitwork services stopped."