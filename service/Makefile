# Service-level Makefile for Go development
#
# For Docker commands, use the root Makefile (make up, make down, etc.)

.PHONY: generate generate-api generate-mcp build run test clean deps

# Generate all code from OpenAPI spec
generate: generate-api generate-mcp

# Generate API code (types and HTTP handlers)
# Pinned, not @latest: the generator emits code against a specific
# oapi-codegen/runtime API, and v2.8.0 requires a newer runtime than the one in
# go.mod. @latest meant `make generate` could break without anything changing
# in this repository.
OAPI_CODEGEN_VERSION := v2.5.1

generate-api:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) \
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
