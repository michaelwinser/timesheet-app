# Makefile for timesheet-app Docker operations
#
# Usage:
#   make build       - Build Docker image
#   make push        - Push Docker image to DockerHub
#   make build-push  - Build and push in one command
#   make login       - Login to DockerHub

# Docker image configuration
IMAGE_NAME = michaelwinser/timesheet-app
VERSION ?= latest

# Platform selection
# For local development on Apple Silicon: linux/arm64
# For TrueNAS deployment (Intel/AMD): linux/amd64
# For both: use build-multiarch target
PLATFORM ?= linux/amd64

.PHONY: help
help:
	@echo "Timesheet App Docker Build Commands"
	@echo ""
	@echo "  make build              Build for specific platform (default: amd64)"
	@echo "  make build-local        Build for current machine architecture"
	@echo "  make build-multiarch    Build for both amd64 and arm64 (RECOMMENDED for publishing)"
	@echo "  make buildx-setup       Set up Docker buildx (run once, or auto-runs with build-multiarch)"
	@echo "  make push               Push Docker image to DockerHub"
	@echo "  make login              Login to DockerHub"
	@echo "  make tag                Tag image with custom version"
	@echo "  make run                Start container with docker-compose"
	@echo "  make test               Run health check on container"
	@echo ""
	@echo "Options:"
	@echo "  VERSION=<tag>           Specify version tag (default: latest)"
	@echo "  PLATFORM=<arch>         Specify platform (default: linux/amd64)"
	@echo ""
	@echo "Publishing Workflow (Apple Silicon → TrueNAS Intel/AMD):"
	@echo "  1. make login                              # Login to DockerHub"
	@echo "  2. make build-multiarch VERSION=v1.0.0     # Build for both architectures"
	@echo "     (This builds and pushes automatically)"
	@echo ""
	@echo "Local Development on Apple Silicon:"
	@echo "  make build-local        # Build for arm64 (faster, for local testing)"
	@echo "  make run                # Run locally"
	@echo ""
	@echo "Other Examples:"
	@echo "  make build VERSION=v1.0.0 PLATFORM=linux/amd64"
	@echo "  make tag TAG=v1.0.0"

.PHONY: build
build:
	@echo "Building Docker image for $(PLATFORM): $(IMAGE_NAME):$(VERSION)"
	docker build --platform $(PLATFORM) -t $(IMAGE_NAME):$(VERSION) .
	@echo "✓ Build complete: $(IMAGE_NAME):$(VERSION)"
	@echo ""
	@echo "Note: Built for $(PLATFORM)"
	@echo "      To build for multiple platforms, use: make build-multiarch"

.PHONY: build-local
build-local:
	@echo "Building Docker image for current architecture: $(IMAGE_NAME):$(VERSION)"
	docker build -t $(IMAGE_NAME):$(VERSION) .
	@echo "✓ Build complete: $(IMAGE_NAME):$(VERSION)"
	@echo ""
	@echo "Note: Built for native architecture (arm64 on Apple Silicon)"
	@echo "      This is faster but won't work on Intel/AMD servers"

.PHONY: buildx-setup
buildx-setup:
	@echo "Checking Docker buildx setup..."
	@if docker buildx ls | grep -q "linux/amd64.*linux/arm64\|linux/arm64.*linux/amd64"; then \
		echo "✓ Multi-platform build support detected"; \
	else \
		echo "⚠ Warning: Multi-platform support may not be available"; \
		echo "  Creating custom builder..."; \
		docker buildx create --name multiarch-builder --use 2>/dev/null || true; \
	fi
	@echo ""

.PHONY: build-multiarch
build-multiarch: buildx-setup
	@echo "Building multi-architecture Docker image: $(IMAGE_NAME):$(VERSION)"
	@echo "Platforms: linux/amd64,linux/arm64"
	@echo ""
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(IMAGE_NAME):$(VERSION) \
		--push \
		.
	@echo ""
	@echo "✓ Multi-arch build complete and pushed: $(IMAGE_NAME):$(VERSION)"
	@echo "  Image works on both Intel/AMD (amd64) and Apple Silicon (arm64)"
	@echo ""
	@echo "Verify on DockerHub:"
	@echo "  https://hub.docker.com/r/$(IMAGE_NAME)/tags"

.PHONY: login
login:
	@echo "Logging in to DockerHub..."
	docker login
	@echo "✓ Login successful"

.PHONY: push
push:
	@echo "Pushing Docker image: $(IMAGE_NAME):$(VERSION)"
	docker push $(IMAGE_NAME):$(VERSION)
	@echo "✓ Push complete: $(IMAGE_NAME):$(VERSION)"

.PHONY: build-push
build-push: build push
	@echo "✓ Build and push complete: $(IMAGE_NAME):$(VERSION)"

.PHONY: tag
tag:
	@if [ -z "$(TAG)" ]; then \
		echo "Error: TAG is required. Usage: make tag TAG=v1.0.0"; \
		exit 1; \
	fi
	@echo "Tagging $(IMAGE_NAME):$(VERSION) as $(IMAGE_NAME):$(TAG)"
	docker tag $(IMAGE_NAME):$(VERSION) $(IMAGE_NAME):$(TAG)
	docker push $(IMAGE_NAME):$(TAG)
	@echo "✓ Tagged and pushed: $(IMAGE_NAME):$(TAG)"

.PHONY: pull
pull:
	@echo "Pulling Docker image: $(IMAGE_NAME):$(VERSION)"
	docker pull $(IMAGE_NAME):$(VERSION)
	@echo "✓ Pull complete: $(IMAGE_NAME):$(VERSION)"

.PHONY: run
run:
	@echo "Running Docker container from $(IMAGE_NAME):$(VERSION)"
	docker-compose up -d
	@echo "✓ Container started. Check status: docker-compose ps"

.PHONY: stop
stop:
	@echo "Stopping Docker container..."
	docker-compose down
	@echo "✓ Container stopped"

.PHONY: logs
logs:
	docker-compose logs -f timesheet-app

.PHONY: shell
shell:
	docker-compose exec timesheet-app /bin/sh

.PHONY: test
test:
	@echo "Running health check on container..."
	@curl -f http://localhost:8000/health || (echo "✗ Health check failed" && exit 1)
	@echo "✓ Health check passed"

.PHONY: clean
clean:
	@echo "Cleaning up Docker resources..."
	docker-compose down -v
	@echo "✓ Cleanup complete"

# Development helpers
.PHONY: rebuild
rebuild:
	@echo "Rebuilding and restarting container..."
	docker-compose down
	docker-compose build --no-cache
	docker-compose up -d
	@echo "✓ Rebuild complete"

.PHONY: inspect
inspect:
	@echo "Image details for $(IMAGE_NAME):$(VERSION):"
	@docker inspect $(IMAGE_NAME):$(VERSION) | grep -A 5 "Architecture"
	@docker images $(IMAGE_NAME)
