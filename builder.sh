#!/bin/bash

set -e

export BUILDER_ID="0199a567-d0ba-76f0-a7fd-acc48f68fa69"
export BUILDER_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/postgres"
export BUILDER_RUNTIME_MODE="docker"
export BUILDER_WORK_DIR="/tmp/zeitwork-builder"

go run ./cmd/builder