#!/bin/bash

set -e

# node agent

export NODE_ID="0199a567-d0ba-76f0-a7fd-acc48f68fa69" 
export NODE_REGION_ID="0199a4b5-09cf-79ee-b554-5db20c31a468" 
export NODE_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/postgres" 
export NODE_RUNTIME_MODE="docker" 
export NODE_IP_ADDRESS="10.0.0.2" 

go run ./cmd/nodeagent
