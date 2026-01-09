# Calendar Sync v2 Testing Strategy

## Overview

This document outlines the testing strategy for the calendar sync v2 implementation. The goal is to achieve high confidence in the sync logic while maintaining a sustainable test suite.

## Current Test Coverage

### Unit Tests (Implemented)

**Location:** `service/internal/sync/`

- `week_test.go` - Week normalization, water mark logic, sync decision algorithm
- `edge_cases_test.go` - Boundary conditions, error scenarios, concurrency logic
- `service/internal/google/mock_test.go` - MockCalendarClient verification

**Coverage includes:**
- Sync decision logic (when to sync, what's missing)
- Week boundary calculations (year boundaries, DST, leap years)
- Staleness detection (24-hour threshold)
- Job coalescing logic
- Failure threshold behavior

### What's Not Yet Tested

1. **Database integration** - SyncJobStore with real PostgreSQL
2. **End-to-end flows** - Full user journey through the UI

---

## Proposed: Integration Tests (Docker + PostgreSQL)

### Approach

Use **testcontainers-go** to spin up PostgreSQL containers for integration tests.

```go
// Example structure
func TestSyncJobStore_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    ctx := context.Background()
    container, connString := startPostgresContainer(t, ctx)
    defer container.Terminate(ctx)

    pool := connectAndMigrate(t, connString)
    store := store.NewSyncJobStore(pool)

    // Test cases...
}
```

### What to Test

1. **SyncJobStore operations**
   - Create jobs
   - Claim with `FOR UPDATE SKIP LOCKED` (concurrent safety)
   - Coalesce pending jobs for same calendar
   - Mark completed/failed
   - Delete old jobs

2. **Atomic transactions**
   - Events and water marks updated together
   - Rollback on partial failure

3. **Job worker with real queue**
   - Worker claims and processes jobs
   - Multiple workers don't conflict
   - Failed jobs are marked correctly

4. **Water mark expansion**
   - `ExpandSyncedWindow` correctly expands range
   - Island sync creates proper gaps

### Implementation Notes

```yaml
# Required dependencies
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
```

**Test file organization:**
```
service/internal/store/
  sync_jobs.go
  sync_jobs_integration_test.go  # New

service/internal/sync/
  job_worker.go
  job_worker_integration_test.go  # New
```

**CI considerations:**
- GitHub Actions supports Docker natively
- Use `testing.Short()` to skip in quick local runs
- Container startup ~2-3 seconds per test file

### Estimated Effort

- Setup testcontainers infrastructure: 2-3 hours
- SyncJobStore integration tests: 3-4 hours
- Job worker integration tests: 2-3 hours
- CI configuration: 1 hour

---

## Proposed: End-to-End Tests (Docker + Playwright)

### Approach

Docker Compose stack with PostgreSQL, Go backend, and Svelte frontend. Playwright tests interact with the browser.

### Architecture

```yaml
# docker-compose.e2e.yml
services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: timesheet_test
      POSTGRES_PASSWORD: test

  backend:
    build:
      context: ./service
      dockerfile: Dockerfile
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://postgres:test@postgres:5432/timesheet_test
      # No Google Calendar - E2E tests don't call external APIs

  frontend:
    build:
      context: ./web
      dockerfile: Dockerfile
    depends_on:
      - backend
    ports:
      - "3000:3000"
```

### What to Test

**Critical paths only (keep minimal):**

1. **Authentication flow** - Login, session persistence
2. **Date navigation** - Navigate between weeks, go to specific date
3. **Event classification** - Classify event to project, verify time entry created
4. **Time entry management** - Add, edit, delete entries
5. **Project visibility** - Toggle project filters

**Explicitly NOT testing in E2E:**
- Google Calendar API integration (requires mocking, high maintenance)
- Background sync job processing (tested in integration tests)
- Water mark expansion logic (tested in unit/integration tests)

### Implementation Notes

```
e2e/
  playwright.config.ts
  docker-compose.e2e.yml
  tests/
    auth.spec.ts
    navigation.spec.ts
    classification.spec.ts
    time-entries.spec.ts
  fixtures/
    test-data.sql  # Seed data for tests
```

**Test data strategy:**
- Seed database with known test data before each test run
- Use unique user accounts per test to allow parallelism
- Reset database between test suites (not individual tests)

### Concerns and Mitigations

| Concern | Mitigation |
|---------|------------|
| Flakiness | Use Playwright's auto-waiting, avoid arbitrary sleeps |
| Slow CI | Run E2E only on merge to main, not every PR |
| Maintenance | Keep to 5-10 critical tests, no edge cases |
| Google API | Don't test it in E2E - that's what mocks are for |

### Estimated Effort

- Docker Compose setup: 3-4 hours
- Playwright configuration: 2 hours
- 5-10 E2E tests: 4-6 hours
- CI integration: 2 hours
- Seed data management: 2 hours

---

## Recommendation

### Priority Order

1. **Integration tests (Docker + PostgreSQL)** - High value, moderate effort
   - This is where the calendar sync v2 complexity lives
   - Real database catches issues mocks miss
   - Testcontainers is well-supported in Go ecosystem

2. **E2E tests (Docker + Playwright)** - Defer or keep minimal
   - High maintenance burden
   - Most sync logic is backend (covered by integration tests)
   - Consider API-level tests as lighter alternative

### Alternative: API Integration Tests

Instead of full browser-based E2E, consider HTTP-level integration tests:

```go
func TestListCalendarEvents_TriggersSync(t *testing.T) {
    // Start real server against test database
    server := startTestServer(t)

    // Make HTTP request
    resp, _ := http.Get(server.URL + "/api/calendar-events?start_date=2025-03-01&end_date=2025-03-07")

    // Verify response and that sync job was created
    // ...
}
```

**Benefits:**
- 80% of E2E confidence, 20% of maintenance
- No browser flakiness
- Much faster execution
- Can test sync triggering without mocking Google

---

## Decision Log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2025-01-09 | Defer integration/E2E tests | Focus on other features, develop sustainable strategy |
| 2025-01-09 | Unit tests for sync logic | Implemented - covers decision logic, edge cases |
| 2025-01-09 | MockCalendarClient created | Enables future integration tests without Google API |
