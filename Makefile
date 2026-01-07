# Makefile for Timesheet App v2
#
# All commands use Docker for consistency.

.PHONY: help up down logs ps build clean db-reset db-clear-entries db-clear-time-data psql generate test

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
	@echo "  make db-clear-time-data  Clear events, time entries, invoices (keeps projects/rules)"
	@echo "  make db-clear-entries  Delete all time entries"
	@echo "  make db-clear-classifications  Reset all events to pending"
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
		DELETE FROM calendar_events;"
	@echo "Done! All events, time entries, and invoices deleted."

psql:
	docker compose exec postgres psql -U timesheet -d timesheet_v2

# =============================================================================
# Development
# =============================================================================

generate:
	cd service && make generate

test:
	cd service && go test -v ./...
