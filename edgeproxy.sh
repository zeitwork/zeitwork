#!/bin/bash

set -e

# edge proxy

export EDGEPROXY_ID="0199a567-d0ba-76f0-a7fd-acc48f68fa69" 
export EDGEPROXY_REGION_ID="0199a4b5-09cf-79ee-b554-5db20c31a468" 
export EDGEPROXY_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/postgres" 

go run ./cmd/edgeproxy
