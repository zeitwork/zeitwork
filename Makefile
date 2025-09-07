.PHONY: all build clean test install uninstall dev run-nodeagent run-edgeproxy run-builder run-certmanager run-listener run-manager

# Build variables
BUILD_DIR := build
BINARIES := zeitwork-nodeagent zeitwork-edgeproxy zeitwork-builder zeitwork-certmanager zeitwork-listener zeitwork-manager
GO_BUILD_FLAGS := -a -installsuffix cgo
LDFLAGS := -s -w

# Default target
all: build

# Build all services
build:
	@echo "Building Zeitwork services..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/zeitwork-nodeagent ./cmd/nodeagent
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/zeitwork-edgeproxy ./cmd/edgeproxy
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/zeitwork-builder ./cmd/builder
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/zeitwork-certmanager ./cmd/certmanager
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/zeitwork-listener ./cmd/listener
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/zeitwork-manager ./cmd/manager
	@echo "Build complete!"

# Build for local development (current OS/arch)
build-local:
	@echo "Building for local development..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/zeitwork-nodeagent ./cmd/nodeagent
	go build -o $(BUILD_DIR)/zeitwork-edgeproxy ./cmd/edgeproxy
	go build -o $(BUILD_DIR)/zeitwork-builder ./cmd/builder
	go build -o $(BUILD_DIR)/zeitwork-certmanager ./cmd/certmanager
	go build -o $(BUILD_DIR)/zeitwork-listener ./cmd/listener
	go build -o $(BUILD_DIR)/zeitwork-manager ./cmd/manager
	@echo "Local build complete!"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete!"

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Install services (requires sudo)
install: build
	@echo "Installing services (requires sudo)..."
	@sudo bash scripts/install.sh

# Uninstall services (requires sudo)
uninstall:
	@echo "Uninstalling services (requires sudo)..."
	@sudo systemctl stop zeitwork-nodeagent zeitwork-edgeproxy zeitwork-builder zeitwork-certmanager zeitwork-listener zeitwork-manager 2>/dev/null || true
	@sudo systemctl disable zeitwork-nodeagent zeitwork-edgeproxy zeitwork-builder zeitwork-certmanager zeitwork-listener zeitwork-manager 2>/dev/null || true
	@sudo rm -f /usr/local/bin/zeitwork-*
	@sudo rm -f /etc/systemd/system/zeitwork-*.service
	@sudo systemctl daemon-reload
	@echo "Uninstall complete!"

# Start development environment
dev:
	@echo "Starting development environment..."
	@chmod +x scripts/dev-setup.sh
	@scripts/dev-setup.sh

# Stop development environment
dev-stop:
	@echo "Stopping development environment..."
	@chmod +x scripts/dev-setup.sh
	@scripts/dev-setup.sh stop

# Development run targets (run locally with test config)
run-nodeagent:
	@echo "Running nodeagent locally..."
	DATABASE_URL=postgres://postgres:postgres@localhost:5432/zeitwork_dev NATS_URL=nats://localhost:4222 LOG_LEVEL=debug ENVIRONMENT=development NODEAGENT_NODE_ID=00000001-0000-0000-0000-000000000001 NODEAGENT_REGION_ID=00000001-0000-0000-0000-000000000000 go run ./cmd/nodeagent/nodeagent.go

run-edgeproxy:
	@echo "Running edgeproxy locally..."
	DATABASE_URL=postgres://postgres:postgres@localhost:5432/zeitwork_dev NATS_URL=nats://localhost:4222 LOG_LEVEL=debug ENVIRONMENT=development go run ./cmd/edgeproxy/edgeproxy.go

run-builder:
	@echo "Running builder locally..."
	DATABASE_URL=postgres://postgres:postgres@localhost:5432/zeitwork_dev NATS_URL=nats://localhost:4222 LOG_LEVEL=debug ENVIRONMENT=development go run ./cmd/builder/builder.go

run-certmanager:
	@echo "Running certmanager locally..."
	DATABASE_URL=postgres://postgres:postgres@localhost:5432/zeitwork_dev NATS_URL=nats://localhost:4222 LOG_LEVEL=debug ENVIRONMENT=development go run ./cmd/certmanager/certmanager.go

run-listener:
	@echo "Running listener locally..."
	DATABASE_URL=postgres://postgres:postgres@localhost:5432/zeitwork_dev NATS_URL=nats://localhost:4222 LOG_LEVEL=debug ENVIRONMENT=development go run ./cmd/listener/listener.go

run-manager:
	@echo "Running manager locally..."
	DATABASE_URL=postgres://postgres:postgres@localhost:5432/zeitwork_dev NATS_URL=nats://localhost:4222 LOG_LEVEL=debug ENVIRONMENT=development go run ./cmd/manager/manager.go

# Generate SQL code
sqlc:
	@echo "Generating SQL code..."
	sqlc generate

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run

# Build Firecracker Builder VM
builder-vm:
	@echo "Building Firecracker Builder VM..."
	@chmod +x scripts/build-builder-vm.sh
	@sudo scripts/build-builder-vm.sh
	@echo "Builder VM created successfully!"

# Test Firecracker Builder VM
test-builder-vm:
	@echo "Testing Firecracker Builder VM..."
	@chmod +x scripts/test-builder-vm.sh
	@scripts/test-builder-vm.sh

# Help target
help:
	@echo "Zeitwork Platform - Build and Deployment"
	@echo ""
	@echo "Available targets:"
	@echo "  make dev            - Start complete development environment"
	@echo "  make dev-stop       - Stop development environment"
	@echo "  make build          - Build all services for Linux AMD64"
	@echo "  make build-local    - Build all services for current OS/arch"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run tests"
	@echo "  make install        - Install services (requires sudo)"
	@echo "  make uninstall      - Uninstall services (requires sudo)"
	@echo "  make run-nodeagent  - Run nodeagent locally for development"
	@echo "  make run-edgeproxy  - Run edgeproxy locally for development"
	@echo "  make run-builder    - Run builder locally for development"
	@echo "  make run-certmanager - Run certmanager locally for development"
	@echo "  make run-listener   - Run listener locally for development"
	@echo "  make run-manager    - Run manager locally for development"
	@echo "  make sqlc           - Generate SQL code with sqlc"
	@echo "  make fmt            - Format Go code"
	@echo "  make lint           - Lint Go code"
	@echo "  make builder-vm     - Build Firecracker Builder VM (requires sudo)"
	@echo "  make test-builder-vm - Test Firecracker Builder VM"
	@echo "  make generate       - Run code generation (protoc: go+gRPC)"
	@echo "  make help           - Show this help message"


.PHONY: generate proto

PROTO_DIR := proto

proto generate:
	@echo "Running code generation..."
	@PATH=$(HOME)/go/bin:$$PATH protoc --go_out=. --go_opt=paths=source_relative $(PROTO_DIR)/*.proto
	@sh -c 'PATH=$(HOME)/go/bin:$$PATH; command -v protoc-gen-go-grpc >/dev/null 2>&1 && \
	  protoc --go-grpc_out=. --go-grpc_opt=paths=source_relative $(PROTO_DIR)/*.proto || \
	  echo "protoc-gen-go-grpc not found; skipping gRPC codegen"'
