# Service-level Makefile for Go development
#
# For Docker commands, use the root Makefile (make up, make down, etc.)

.PHONY: generate build run test clean deps

# Generate API code from OpenAPI spec
generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest \
		-config oapi-codegen.yaml ../docs/v2/api-spec.yaml

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
