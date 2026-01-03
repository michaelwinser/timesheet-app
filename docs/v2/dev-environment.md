# Development Environment - Timesheet App v2

This document describes how to set up and run the v2 development environment.

---

## Prerequisites

- **Go 1.22+** - [Install Go](https://go.dev/doc/install)
- **Docker & Docker Compose** - For PostgreSQL
- **curl** or **httpie** - For API testing (optional)

---

## Quick Start

```bash
# 1. Start PostgreSQL
docker-compose up -d postgres

# 2. Run the service
cd service
make run
```

The API is now available at http://localhost:8080

---

## Components

### PostgreSQL

The database runs via Docker Compose. Two databases exist:

| Database | Purpose |
|----------|---------|
| `timesheet` | v1 Python app (production data) |
| `timesheet_v2` | v2 Go service (development) |

```bash
# Start PostgreSQL
docker-compose up -d postgres

# Check status
docker-compose ps

# View logs
docker-compose logs -f postgres

# Stop
docker-compose down
```

**Connection details:**
- Host: `localhost`
- Port: `5432`
- User: `timesheet`
- Password: `changeMe123!`
- Database: `timesheet_v2`

```bash
# Connect with psql
docker exec -it timesheet-postgres psql -U timesheet -d timesheet_v2

# Or use the full connection string
psql "postgresql://timesheet:changeMe123!@localhost:5432/timesheet_v2"
```

### Go Service

The v2 API service lives in the `service/` directory.

```bash
cd service

# Run directly
make run

# Or build and run
make build
./bin/server

# Run with custom port
PORT=9000 make run
```

**Makefile targets:**

| Target | Description |
|--------|-------------|
| `make run` | Run the server |
| `make build` | Build binary to `bin/server` |
| `make generate` | Regenerate code from OpenAPI spec |
| `make test` | Run tests |
| `make deps` | Download/tidy dependencies |
| `make clean` | Remove build artifacts |

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `DATABASE_URL` | `postgresql://timesheet:changeMe123!@localhost:5432/timesheet_v2` | PostgreSQL connection |
| `JWT_SECRET` | `development-secret-change-in-production` | JWT signing key |

Example:
```bash
export DATABASE_URL="postgresql://user:pass@host:5432/dbname"
export JWT_SECRET="your-secure-secret"
export PORT="8080"
```

---

## API Access

### OpenAPI Spec

The API specification is served at runtime:
- **Spec:** http://localhost:8080/api/openapi.yaml
- **Source:** `docs/v2/api-spec.yaml`

### Swagger UI

Use the hosted Swagger UI to explore the API:

https://petstore.swagger.io/?url=http://localhost:8080/api/openapi.yaml

### Health Check

```bash
curl http://localhost:8080/health
# OK
```

### Example Requests

```bash
# Sign up
curl -X POST http://localhost:8080/api/auth/signup \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123","name":"Test User"}'

# Login (save token)
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}' | jq -r '.token')

# List projects
curl http://localhost:8080/api/projects \
  -H "Authorization: Bearer $TOKEN"

# Create project
curl -X POST http://localhost:8080/api/projects \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"Acme Corp","short_code":"ACM","color":"#3B82F6"}'

# Create time entry
curl -X POST http://localhost:8080/api/time-entries \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"project_id":"<project-uuid>","date":"2024-12-20","hours":2.5}'

# List time entries
curl "http://localhost:8080/api/time-entries?start_date=2024-12-01&end_date=2024-12-31" \
  -H "Authorization: Bearer $TOKEN"
```

---

## Code Generation

The API types and server stubs are generated from the OpenAPI spec using [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen).

```bash
cd service

# Regenerate after modifying docs/v2/api-spec.yaml
make generate

# This updates: internal/api/api.gen.go
```

**Config:** `service/oapi-codegen.yaml`

---

## Database Migrations

Migrations run automatically on server startup. They're defined in `service/internal/database/database.go`.

To view current schema:
```bash
docker exec timesheet-postgres psql -U timesheet -d timesheet_v2 -c "\dt"
docker exec timesheet-postgres psql -U timesheet -d timesheet_v2 -c "\d users"
docker exec timesheet-postgres psql -U timesheet -d timesheet_v2 -c "\d projects"
docker exec timesheet-postgres psql -U timesheet -d timesheet_v2 -c "\d time_entries"
```

To reset the database:
```bash
docker exec timesheet-postgres psql -U timesheet -c "DROP DATABASE timesheet_v2;"
docker exec timesheet-postgres psql -U timesheet -c "CREATE DATABASE timesheet_v2;"
# Restart the service to re-run migrations
```

---

## Project Structure

```
timesheet-app/
├── docs/v2/
│   ├── api-spec.yaml        # OpenAPI specification (source of truth)
│   ├── architecture.md      # System architecture
│   ├── components.md        # UI component catalog
│   ├── domain-glossary.md   # Domain model definitions
│   ├── dev-environment.md   # This file
│   └── decisions/           # Architecture Decision Records
│
├── service/                  # Go API service
│   ├── cmd/server/          # Entry point
│   ├── internal/
│   │   ├── api/             # Generated OpenAPI code
│   │   ├── database/        # DB connection & migrations
│   │   ├── handler/         # HTTP handlers
│   │   └── store/           # Data access layer
│   ├── go.mod
│   ├── Makefile
│   └── oapi-codegen.yaml
│
├── docker-compose.yaml       # PostgreSQL for dev
└── README.md                 # v1 app docs (legacy)
```

---

## Troubleshooting

### Port already in use

```bash
# Find and kill process on port 8080
lsof -ti:8080 | xargs kill -9
```

### Database connection refused

```bash
# Check if PostgreSQL is running
docker-compose ps

# Start it
docker-compose up -d postgres

# Wait for it to be ready
docker-compose logs -f postgres
# Look for: "database system is ready to accept connections"
```

### Regenerate after API spec changes

```bash
cd service
make generate
make build
```

---

## v1 vs v2

| Aspect | v1 | v2 |
|--------|----|----|
| Language | Python/FastAPI | Go/Chi |
| Database | `timesheet` | `timesheet_v2` |
| Auth | Google OAuth | Email/password + JWT |
| Port | 8000 | 8080 |
| API Spec | Swagger auto-gen | OpenAPI-first |

Both can run simultaneously since they use different databases and ports.
