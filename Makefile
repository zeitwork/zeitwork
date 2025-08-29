.PHONY: all build clean test install uninstall run-operator run-node-agent run-load-balancer run-edge-proxy

# Build variables
BUILD_DIR := build
BINARIES := zeitwork-operator zeitwork-node-agent zeitwork-load-balancer zeitwork-edge-proxy
GO_BUILD_FLAGS := -a -installsuffix cgo
LDFLAGS := -s -w

# Default target
all: build

# Build all services
build:
	@echo "Building Zeitwork services..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/zeitwork-operator ./cmd/operator
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/zeitwork-node-agent ./cmd/node-agent
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/zeitwork-edge-proxy ./cmd/edge-proxy
	@echo "Build complete!"

# Build for local development (current OS/arch)
build-local:
	@echo "Building for local development..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/zeitwork-operator ./cmd/operator
	go build -o $(BUILD_DIR)/zeitwork-node-agent ./cmd/node-agent
	go build -o $(BUILD_DIR)/zeitwork-edge-proxy ./cmd/edge-proxy
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
	@sudo systemctl stop zeitwork-operator zeitwork-node-agent zeitwork-edge-proxy 2>/dev/null || true
	@sudo systemctl disable zeitwork-operator zeitwork-node-agent zeitwork-edge-proxy 2>/dev/null || true
	@sudo rm -f /usr/local/bin/zeitwork-*
	@sudo rm -f /etc/systemd/system/zeitwork-*.service
	@sudo systemctl daemon-reload
	@echo "Uninstall complete!"

# Development run targets (run locally with test config)
run-operator:
	@echo "Running operator locally..."
	DATABASE_URL=postgres://localhost/zeitwork_dev PORT=8080 LOG_LEVEL=debug ENVIRONMENT=development go run ./cmd/operator

run-node-agent:
	@echo "Running node-agent locally..."
	OPERATOR_URL=http://localhost:8080 PORT=8081 LOG_LEVEL=debug ENVIRONMENT=development go run ./cmd/node-agent

run-edge-proxy:
	@echo "Running edge-proxy locally..."
	OPERATOR_URL=http://localhost:8080 PORT=8083 LOG_LEVEL=debug ENVIRONMENT=development go run ./cmd/edge-proxy

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
	@echo "  make build          - Build all services for Linux AMD64"
	@echo "  make build-local    - Build all services for current OS/arch"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run tests"
	@echo "  make install        - Install services (requires sudo)"
	@echo "  make uninstall      - Uninstall services (requires sudo)"
	@echo "  make run-operator   - Run operator locally for development"
	@echo "  make run-node-agent - Run node-agent locally for development"
	@echo "  make run-edge-proxy - Run edge-proxy locally for development"
	@echo "  make sqlc           - Generate SQL code with sqlc"
	@echo "  make fmt            - Format Go code"
	@echo "  make lint           - Lint Go code"
	@echo "  make builder-vm     - Build Firecracker Builder VM (requires sudo)"
	@echo "  make test-builder-vm - Test Firecracker Builder VM"
	@echo "  make help           - Show this help message"
