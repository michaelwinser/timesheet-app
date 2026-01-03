# Makefile for Timesheet App v2
#
# All commands use Docker for consistency.

.PHONY: help up down logs ps build clean db-reset psql generate test

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
	@echo "  make psql        Connect to PostgreSQL shell"
	@echo ""
	@echo "Development:"
	@echo "  make generate    Regenerate API code from OpenAPI spec"
	@echo "  make test        Run tests"
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
	docker compose exec postgres psql -U timesheet -c "DROP DATABASE IF EXISTS timesheet_v2;"
	docker compose exec postgres psql -U timesheet -c "CREATE DATABASE timesheet_v2;"
	@echo "Database reset. Restart the API to run migrations."

psql:
	docker compose exec postgres psql -U timesheet -d timesheet_v2

# =============================================================================
# Development
# =============================================================================

generate:
	cd service && make generate

test:
	cd service && go test -v ./...
