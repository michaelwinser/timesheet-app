.PHONY: generate build run test clean

# Generate API code from OpenAPI spec
generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest \
		-config oapi-codegen.yaml ../docs/v2/api-spec.yaml

# Build the server
build:
	go build -o bin/server ./cmd/server

# Run the server
run:
	go run ./cmd/server

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/

# Download dependencies
deps:
	go mod tidy
	go mod download

# All: generate, deps, build
all: generate deps build
