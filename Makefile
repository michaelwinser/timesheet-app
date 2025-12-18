# Makefile for timesheet-app
#
# Usage:
#   make dev         - Run locally (no Docker)
#   make run         - Start with Docker Compose
#   make build       - Build Docker image
#   make push        - Push Docker image to DockerHub

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
	@echo "Timesheet App Commands"
	@echo ""
	@echo "Local Development:"
	@echo "  make install            Install Python dependencies"
	@echo "  make dev                Run app locally with uvicorn (requires PostgreSQL)"
	@echo "  make db-up              Start PostgreSQL container only"
	@echo "  make psql               Connect to PostgreSQL shell"
	@echo "  make db-reset           Reset database (WARNING: deletes all data)"
	@echo ""
	@echo "Docker Compose:"
	@echo "  make run                Start all services (app + postgres)"
	@echo "  make stop               Stop all services"
	@echo "  make ps                 Show container status"
	@echo "  make logs               Follow app logs"
	@echo "  make logs-all           Follow all service logs"
	@echo "  make shell              Shell into app container"
	@echo "  make restart            Restart services"
	@echo "  make rebuild            Rebuild and restart (no cache)"
	@echo "  make clean              Stop and remove containers (keeps data)"
	@echo "  make clean-all          Stop and remove containers AND volumes (DELETES DATA)"
	@echo ""
	@echo "Docker Build:"
	@echo "  make build              Build for specific platform (default: amd64)"
	@echo "  make build-local        Build for current machine architecture"
	@echo "  make build-multiarch    Build for amd64 and arm64 (RECOMMENDED for publishing)"
	@echo "  make push               Push Docker image to DockerHub"
	@echo "  make login              Login to DockerHub"
	@echo "  make tag TAG=v1.0.0     Tag and push with custom version"
	@echo ""
	@echo "Production:"
	@echo "  make run-prod           Start with docker-compose.prod.yaml"
	@echo "  make stop-prod          Stop production services"
	@echo "  make logs-prod          Follow production logs"
	@echo ""
	@echo "Testing:"
	@echo "  make test               Run health check"
	@echo ""
	@echo "Options:"
	@echo "  VERSION=<tag>           Specify version tag (default: latest)"
	@echo "  PLATFORM=<arch>         Specify platform (default: linux/amd64)"

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
	@echo "Stopping and removing containers (keeping volumes)..."
	docker-compose down
	@echo "✓ Cleanup complete (data preserved)"

.PHONY: clean-all
clean-all:
	@echo "WARNING: This will delete all data including the PostgreSQL database!"
	@echo "Press Ctrl+C to cancel, or wait 5 seconds to continue..."
	@sleep 5
	docker-compose down -v
	@echo "✓ Full cleanup complete (volumes removed)"

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

# =============================================================================
# Local Development (no Docker for app)
# =============================================================================

.PHONY: install
install:
	pip install -r requirements.txt
	@echo "✓ Dependencies installed"

.PHONY: dev
dev:
	@echo "Starting app locally (requires PostgreSQL at localhost:5432)..."
	@echo "Tip: Run 'make db-up' first to start PostgreSQL in Docker"
	@echo ""
	cd src && uvicorn main:app --reload --host 0.0.0.0 --port 8000

# =============================================================================
# Database Operations
# =============================================================================

.PHONY: db-up
db-up:
	@echo "Starting PostgreSQL container..."
	docker-compose up -d postgres
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 3
	@docker-compose exec postgres pg_isready -U timesheet || (echo "✗ PostgreSQL not ready" && exit 1)
	@echo "✓ PostgreSQL is ready at localhost:5432"

.PHONY: psql
psql:
	docker-compose exec postgres psql -U timesheet -d timesheet

.PHONY: db-reset
db-reset:
	@echo "WARNING: This will delete all database data!"
	@echo "Press Ctrl+C to cancel, or wait 5 seconds to continue..."
	@sleep 5
	docker-compose down -v postgres
	docker-compose up -d postgres
	@sleep 3
	@echo "✓ Database reset complete"

# =============================================================================
# Additional Docker Compose Commands
# =============================================================================

.PHONY: ps
ps:
	docker-compose ps

.PHONY: logs-all
logs-all:
	docker-compose logs -f

.PHONY: restart
restart:
	docker-compose restart
	@echo "✓ Services restarted"

# =============================================================================
# Production Deployment
# =============================================================================

.PHONY: run-prod
run-prod:
	@echo "Starting production services..."
	docker-compose -f docker-compose.prod.yaml up -d
	@echo "✓ Production services started"

.PHONY: stop-prod
stop-prod:
	docker-compose -f docker-compose.prod.yaml down
	@echo "✓ Production services stopped"

.PHONY: logs-prod
logs-prod:
	docker-compose -f docker-compose.prod.yaml logs -f
