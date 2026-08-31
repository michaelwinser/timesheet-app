# Makefile for Timesheet App v2
#
# All commands use Docker for consistency.

.PHONY: help up down logs ps build clean clean-all db-up db-reset db-clear-entries \
	db-clear-classifications db-clear-time-data psql generate test \
	login image image-local build-multiarch push build-push tag pull check-clean \
	db-backup db-restore db-verify-backup

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

# Stamped into the image so a published tag can always be traced to a commit
GIT_REV := $(shell git rev-parse --short HEAD 2>/dev/null)

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
	@echo "Backup:"
	@echo "  make db-backup                 Dump the database to $(BACKUP_DIR)/"
	@echo "  make db-verify-backup FILE=f   Restore f into a throwaway container and count rows"
	@echo "  make db-restore FILE=f         Replace the database with f (WARNING: destructive)"
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
	@echo "  build-multiarch refuses a dirty tree; override with ALLOW_DIRTY=1"
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

# docker build reads the working tree, not HEAD, so publishing from a dirty
# tree produces an image that corresponds to no commit and cannot be rebuilt.
check-clean:
	@if [ -z "$(ALLOW_DIRTY)" ] && [ -n "$$(git status --porcelain)" ]; then \
		echo "Refusing to publish from a dirty working tree:"; \
		echo ""; \
		git status --short; \
		echo ""; \
		echo "Commit or stash first, or override with ALLOW_DIRTY=1"; \
		exit 1; \
	fi

build-multiarch: check-clean
	@docker buildx inspect multiarch-builder >/dev/null 2>&1 || \
		docker buildx create --name multiarch-builder --use
	@echo "Building and pushing $(IMAGE_NAME):$(VERSION) for $(PLATFORMS)..."
	docker buildx build --builder multiarch-builder --platform $(PLATFORMS) \
		-f service/Dockerfile \
		--label org.opencontainers.image.revision=$(GIT_REV) \
		--label org.opencontainers.image.version=$(VERSION) \
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
# Backup and restore
# =============================================================================
#
# pg_dump runs against the live database in a consistent snapshot, so no
# downtime is needed to take a backup. Restoring does require stopping the API.
#
# The dump contains API key hashes, MCP tokens, and Google refresh tokens
# encrypted with ENCRYPTION_KEY. Treat it as a credential, and back up
# ENCRYPTION_KEY separately - without it the restored tokens are unusable.

BACKUP_DIR ?= ./backups

db-backup:
	@mkdir -p $(BACKUP_DIR)
	@f="$(BACKUP_DIR)/timesheet-$$(date +%Y%m%d-%H%M%S).dump"; \
	docker compose exec -T postgres pg_dump -U timesheet -Fc timesheet_v2 > "$$f" || \
		{ echo "Dump failed"; rm -f "$$f"; exit 1; }; \
	if [ ! -s "$$f" ]; then echo "Dump is empty - is postgres running?"; rm -f "$$f"; exit 1; fi; \
	echo "Wrote $$f ($$(du -h "$$f" | cut -f1))"; \
	echo "Verify it before trusting it:  make db-verify-backup FILE=$$f"

# Restores into a scratch container so the dump is proved readable without
# touching the real database. An unrestored backup is only a hypothesis.
db-verify-backup:
	@if [ -z "$(FILE)" ]; then echo "Usage: make db-verify-backup FILE=<dump>"; exit 1; fi
	@if [ ! -f "$(FILE)" ]; then echo "No such file: $(FILE)"; exit 1; fi
	@set -e; \
	name=timesheet-verify-$$$$; \
	trap 'docker rm -f $$name >/dev/null 2>&1 || true' EXIT; \
	echo "Starting scratch postgres..."; \
	docker run -d --name $$name -e POSTGRES_PASSWORD=verify \
		-e POSTGRES_USER=timesheet -e POSTGRES_DB=timesheet_v2 \
		postgres:16-alpine >/dev/null; \
	until docker exec $$name pg_isready -U timesheet -d timesheet_v2 >/dev/null 2>&1; do sleep 1; done; \
	echo "Restoring $(FILE)..."; \
	docker exec -i $$name pg_restore -U timesheet -d timesheet_v2 --no-owner < "$(FILE)"; \
	echo ""; \
	docker exec $$name psql -U timesheet -d timesheet_v2 -c \
		"select relname as table, n_live_tup as rows from pg_stat_user_tables where n_live_tup > 0 order by relname;"; \
	echo "Backup restores cleanly."

db-restore:
	@if [ -z "$(FILE)" ]; then echo "Usage: make db-restore FILE=<dump>"; exit 1; fi
	@if [ ! -f "$(FILE)" ]; then echo "No such file: $(FILE)"; exit 1; fi
	@echo "WARNING: This REPLACES everything in timesheet_v2 with $(FILE)"
	@echo "Press Ctrl+C to cancel, or wait 5 seconds..."
	@sleep 5
	@echo "Stopping API to release database connections..."
	-docker compose stop api
	docker compose exec -T postgres pg_restore -U timesheet -d timesheet_v2 \
		--clean --if-exists --no-owner < $(FILE)
	@echo "Starting API..."
	docker compose start api
	@echo "Restored from $(FILE)"

# =============================================================================
# Development
# =============================================================================

generate:
	cd service && make generate

test:
	cd service && go test -v ./...
