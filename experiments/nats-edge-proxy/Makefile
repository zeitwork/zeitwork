.PHONY: generate help

help:
	@echo "Available targets:"
	@echo "  generate   Run code generation commands (e.g., protoc)"
	@echo "  help       Show this help message"

generate:
	@echo "Running code generation..."
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative proto/*.proto

