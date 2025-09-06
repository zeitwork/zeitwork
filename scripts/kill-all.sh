#!/bin/bash

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