# Service-level Makefile for Go development
#
# For Docker commands, use the root Makefile (make up, make down, etc.)

.PHONY: generate generate-api generate-mcp build run test clean deps

# Generate all code from OpenAPI spec
generate: generate-api generate-mcp

# Generate API code (types and HTTP handlers)
generate-api:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest \
		-config oapi-codegen.yaml ../docs/v2/api-spec.yaml

# Generate MCP tool definitions
generate-mcp:
	go run ./cmd/mcp-codegen ../docs/v2/api-spec.yaml

# Build the server binary
build:
	go build -o bin/server ./cmd/server

# Run the server locally (expects PostgreSQL at localhost:5432)
run:
	go run ./cmd/server

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/

# Download/tidy dependencies
deps:
	go mod tidy
	go mod download
