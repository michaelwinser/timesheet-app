# Makefile for Timesheet App v2
#
# All commands use Docker for consistency.

.PHONY: help up down logs ps build clean clean-all db-up db-reset db-clear-entries \
	db-clear-classifications db-clear-time-data psql generate test \
	login image image-local build-multiarch push build-push tag pull

# =============================================================================
# Docker Hub publishing
# =============================================================================

# Image consumed by docker-compose.prod.yaml on TrueNAS
IMAGE_NAME ?= michaelwinser/timesheet-app
VERSION ?= latest

# TrueNAS runs on Intel/AMD. Publish multi-arch so one tag serves both the
# server and an Apple Silicon workstation.
PLATFORMS ?= linux/amd64,linux/arm64
PLATFORM ?= linux/amd64

# A versioned release also moves :latest, which is what prod pulls
LATEST_TAG := $(if $(filter-out latest,$(VERSION)),-t $(IMAGE_NAME):latest,)

# Default target
help:
	@echo "Timesheet App v2"
	@echo ""
	@echo "Docker Commands:"
	@echo "  make up          Start all services (postgres + api)"
	@echo "  make down        Stop all services"
	@echo "  make logs        Follow logs"
	@echo "  make ps          Show container status"
	@echo "  make build       Rebuild Docker image"
	@echo "  make clean       Stop and remove containers (keeps data)"
	@echo "  make clean-all   Stop and remove containers AND volumes"
	@echo ""
	@echo "Database:"
	@echo "  make db-up       Start PostgreSQL only"
	@echo "  make db-reset    Reset database (WARNING: deletes data)"
	@echo "  make db-clear-time-data  Clear events, time entries, invoices and trigger full re-sync"
	@echo "  make db-clear-entries  Delete all time entries"
	@echo "  make db-clear-classifications  Reset all events to pending"
	@echo "  make psql        Connect to PostgreSQL shell"
	@echo ""
	@echo "Development:"
	@echo "  make generate    Regenerate API code from OpenAPI spec"
	@echo "  make test        Run tests"
	@echo ""
	@echo "Docker Hub (image: $(IMAGE_NAME)):"
	@echo "  make login             Log in to Docker Hub"
	@echo "  make build-multiarch   Build for amd64+arm64 and push (also moves :latest)"
	@echo "  make image             Build a tagged image for one platform ($(PLATFORM))"
	@echo "  make image-local       Build a tagged image for this machine only"
	@echo "  make push              Push an already built tagged image"
	@echo "  make build-push        make image, then make push"
	@echo "  make tag TAG=1.0.5     Retag VERSION as TAG registry-side (no rebuild)"
	@echo "  make pull              Pull the published image"
	@echo ""
	@echo "  Options: VERSION=<tag> (default: $(VERSION))  PLATFORM=<arch> (default: $(PLATFORM))"
	@echo ""
	@echo "Access:"
	@echo "  http://localhost:8080  - Web UI + API"

# =============================================================================
# Docker Commands
# =============================================================================

up:
	@echo "Starting services..."
	docker compose up -d
	@echo ""
	@echo "App running at http://localhost:8080"

down:
	docker compose down

logs:
	docker compose logs -f api

ps:
	docker compose ps

build:
	docker compose build

clean:
	docker compose down

clean-all:
	@echo "WARNING: This will delete all data!"
	@echo "Press Ctrl+C to cancel, or wait 3 seconds..."
	@sleep 3
	docker compose down -v

# =============================================================================
# Docker Hub
# =============================================================================
#
# `build` above builds the local compose stack. These targets build and publish
# the tagged image that docker-compose.prod.yaml pulls on TrueNAS.

login:
	docker login

image:
	@echo "Building $(IMAGE_NAME):$(VERSION) for $(PLATFORM)..."
	docker build --platform $(PLATFORM) -f service/Dockerfile -t $(IMAGE_NAME):$(VERSION) .
	@echo "Built $(IMAGE_NAME):$(VERSION) ($(PLATFORM))"
	@echo "Note: single-platform. TrueNAS needs linux/amd64 - use build-multiarch to cover both."

image-local:
	@echo "Building $(IMAGE_NAME):$(VERSION) for this machine..."
	docker build -f service/Dockerfile -t $(IMAGE_NAME):$(VERSION) .
	@echo "Built $(IMAGE_NAME):$(VERSION) (native architecture, for local testing only)"

build-multiarch:
	@docker buildx inspect multiarch-builder >/dev/null 2>&1 || \
		docker buildx create --name multiarch-builder --use
	@echo "Building and pushing $(IMAGE_NAME):$(VERSION) for $(PLATFORMS)..."
	docker buildx build --builder multiarch-builder --platform $(PLATFORMS) \
		-f service/Dockerfile \
		-t $(IMAGE_NAME):$(VERSION) $(LATEST_TAG) \
		--push .
	@echo ""
	@echo "Published $(IMAGE_NAME):$(VERSION). Platforms now on the tag:"
	@docker buildx imagetools inspect $(IMAGE_NAME):$(VERSION) | grep -i platform

push:
	docker push $(IMAGE_NAME):$(VERSION)

build-push: image push

# Retag an already published image without rebuilding. imagetools copies the
# manifest list registry-side, so the multi-arch tag stays multi-arch; plain
# `docker tag` would flatten it to whichever single platform the local daemon
# happens to hold.
tag:
	@if [ -z "$(TAG)" ]; then echo "Usage: make tag TAG=1.0.5 [VERSION=<source tag>]"; exit 1; fi
	docker buildx imagetools create -t $(IMAGE_NAME):$(TAG) $(IMAGE_NAME):$(VERSION)
	@echo "Platforms on $(IMAGE_NAME):$(TAG):"
	@docker buildx imagetools inspect $(IMAGE_NAME):$(TAG) | grep -i platform

pull:
	docker pull $(IMAGE_NAME):$(VERSION)

# =============================================================================
# Database
# =============================================================================

db-up:
	@echo "Starting PostgreSQL..."
	docker compose up -d postgres
	@echo "Waiting for PostgreSQL..."
	@sleep 2
	@docker compose exec postgres pg_isready -U timesheet -d timesheet_v2 || (echo "Not ready yet, waiting..." && sleep 3)
	@echo "PostgreSQL ready at localhost:5432"

db-reset:
	@echo "WARNING: This will delete all database data!"
	@echo "Press Ctrl+C to cancel, or wait 3 seconds..."
	@sleep 3
	@echo "Stopping API to release database connections..."
	-docker compose stop api
	docker compose exec postgres psql -U timesheet -c "DROP DATABASE IF EXISTS timesheet_v2;"
	docker compose exec postgres psql -U timesheet -c "CREATE DATABASE timesheet_v2;"
	@echo "Database reset. Starting API to run migrations..."
	docker compose start api
	@echo "Done! Database has been reset."

db-clear-entries:
	@echo "Deleting all time entries..."
	docker compose exec postgres psql -U timesheet -d timesheet_v2 -c "DELETE FROM time_entries;"
	@echo "Done! All time entries deleted."

db-clear-classifications:
	@echo "Clearing all event classifications..."
	docker compose exec postgres psql -U timesheet -d timesheet_v2 -c "\
		UPDATE calendar_events SET \
			classification_status = 'pending', \
			classification_source = NULL, \
			classification_confidence = NULL, \
			needs_review = false, \
			project_id = NULL, \
			updated_at = NOW();"
	@echo "Done! All events reset to pending."

db-clear-time-data:
	@echo "Clearing all time data (keeping projects and rules)..."
	docker compose exec postgres psql -U timesheet -d timesheet_v2 -c "\
		DELETE FROM invoices; \
		DELETE FROM time_entries; \
		DELETE FROM calendar_events; \
		UPDATE calendars SET \
			sync_token = NULL, \
			last_synced_at = NULL, \
			min_synced_date = NULL, \
			max_synced_date = NULL; \
		UPDATE calendar_connections SET \
			last_synced_at = NULL, \
			sync_token = NULL;"
	@echo "Done! All events, time entries, and invoices deleted. Calendars will re-sync."

psql:
	docker compose exec postgres psql -U timesheet -d timesheet_v2

# =============================================================================
# Development
# =============================================================================

generate:
	cd service && make generate

test:
	cd service && go test -v ./...
