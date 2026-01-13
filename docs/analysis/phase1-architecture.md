# Architecture Analysis: Timesheet App v2

## 1. Executive Summary

The Timesheet App is a **Go + SvelteKit monorepo** application for automatic timesheet creation from Google Calendar. It follows an **API-first design** with a strongly-typed contract defined in OpenAPI, from which both server stubs and client types are generated.

**Key Architectural Characteristics:**
- **Monorepo structure** with clear separation: `service/` (Go backend), `web/` (SvelteKit frontend)
- **API-first development** using OpenAPI spec as the source of truth (`docs/v2/api-spec.yaml`)
- **Code generation** for API handlers (oapi-codegen) and MCP tools
- **PostgreSQL database** with in-Go migrations (not separate SQL files)
- **Multi-layered backend**: Handlers -> Services -> Stores -> Database
- **Component-based frontend** with Svelte 5 runes and centralized state management
- **Domain-driven design** with rich domain glossary and architectural decision records (ADRs)
- **Multi-client support**: Web UI, MCP Server for AI agents

The application evolved from an earlier Python/FastAPI prototype (documented in `design.md`) to a more robust Go + Svelte stack (v2).

---

## 2. System Architecture

```
                           +----------------------------------+
                           |         Environment              |
                           | (Docker, OAuth, Secrets, Config) |
                           +----------------+-----------------+
                                            |
                           +----------------v-----------------+
                           |           Service (Go)           |
                           | +-----------------------------+  |
                           | |        API Layer            |  |
                           | | (Chi Router + oapi-codegen) |  |
                           | +-------------+---------------+  |
                           |               |                  |
                           | +-------------v---------------+  |
                           | |       Service Layer         |  |
                           | | - ClassificationService     |  |
                           | | - TimeEntryService          |  |
                           | | - SyncService (Background)  |  |
                           | +-------------+---------------+  |
                           |               |                  |
                           | +-------------v---------------+  |
                           | |        Store Layer          |  |
                           | | (PostgreSQL repositories)   |  |
                           | +-------------+---------------+  |
                           |               |                  |
                           | +-------------v---------------+  |
                           | |        Database Layer       |  |
                           | | (pgx + in-code migrations)  |  |
                           | +-----------------------------+  |
                           +----------------+-----------------+
                                            |
                        +-------------------+-------------------+
                        |                   |                   |
             +----------v-------+  +--------v--------+  +-------v-------+
             |   Web Client     |  |   MCP Server    |  |  Future CLI   |
             |   (SvelteKit)    |  |   (AI Agents)   |  |   etc.        |
             +------------------+  +-----------------+  +---------------+

External Dependencies:
  - Google Calendar API (OAuth 2.0 + REST)
  - Google Sheets API (for invoice export)
  - PostgreSQL 16
```

**Deployment Architecture (docker-compose.prod.yaml):**
```
+-------------------+       +----------------------+
|   timesheet-api   | <---> |   postgres:16-alpine |
|   (Go + Static)   |       +----------------------+
+-------------------+
        |
        | Port 8080 (internal) -> 8000 (external)
        v
   [Reverse Proxy / Direct Access]
```

---

## 3. Backend Architecture

### 3.1 Directory Structure

```
service/
+-- cmd/
|   +-- server/main.go         # Application entry point
|   +-- mcp-codegen/           # MCP tool generation utility
+-- internal/
|   +-- api/api.gen.go         # Generated from OpenAPI (oapi-codegen)
|   +-- handler/               # HTTP handlers (thin, delegate to services)
|   |   +-- server.go          # Aggregates all handlers
|   |   +-- auth.go            # Authentication endpoints
|   |   +-- projects.go        # Project CRUD
|   |   +-- time_entries.go    # Time entry operations
|   |   +-- calendars.go       # Calendar sync operations
|   |   +-- rules.go           # Classification rules
|   |   +-- invoices.go        # Invoice management
|   |   +-- mcp.go             # MCP protocol handler
|   |   +-- middleware.go      # JWT auth middleware
|   +-- store/                 # Data access layer (repositories)
|   |   +-- users.go
|   |   +-- projects.go
|   |   +-- time_entries.go
|   |   +-- calendar_events.go
|   |   +-- classification_rules.go
|   |   +-- invoices.go
|   +-- database/database.go   # DB connection + migrations (in-code)
|   +-- classification/        # Classification business logic
|   |   +-- service.go         # Orchestration with I/O
|   |   +-- classifier.go      # Pure classification logic (no I/O)
|   |   +-- parser.go          # Query language parser
|   |   +-- evaluator.go       # Query evaluation
|   +-- timeentry/service.go   # Time entry computation logic
|   +-- sync/                  # Calendar sync services
|   |   +-- background.go      # Periodic incremental sync
|   |   +-- job_worker.go      # On-demand sync job queue
|   +-- google/                # Google API integrations
|   |   +-- calendar.go
|   |   +-- sheets.go
|   +-- crypto/encryption.go   # OAuth token encryption
|   +-- mcp/tools.gen.go       # Generated MCP tool definitions
+-- Dockerfile
+-- Makefile
+-- go.mod, go.sum
+-- oapi-codegen.yaml          # Code generation config
```

### 3.2 Architectural Patterns

**Handler Pattern (Thin Controllers)**
Handlers validate input, extract user context, call services, and return responses. No business logic.

```go
// server.go - Composition of all handlers
type Server struct {
    *AuthHandler
    *ProjectHandler
    *TimeEntryHandler
    *CalendarHandler
    *RulesHandler
    // ...
}
var _ api.StrictServerInterface = (*Server)(nil)
```

**Service Layer Pattern**
Services contain business logic and orchestration. They coordinate between stores and external services.

```go
// classification/service.go
type Service struct {
    pool             *pgxpool.Pool
    ruleStore        *store.ClassificationRuleStore
    eventStore       *store.CalendarEventStore
    timeEntryStore   *store.TimeEntryStore
    timeEntryService *timeentry.Service
}
```

**Repository/Store Pattern**
Stores encapsulate all database operations. No raw SQL in handlers or services.

```go
// store/time_entries.go
type TimeEntryStore struct {
    pool *pgxpool.Pool
}

func (s *TimeEntryStore) GetByID(ctx context.Context, userID, entryID uuid.UUID) (*TimeEntry, error)
func (s *TimeEntryStore) List(ctx context.Context, userID uuid.UUID, ...) ([]*TimeEntry, error)
func (s *TimeEntryStore) Create(ctx context.Context, ...) (*TimeEntry, error)
```

**Pure Functions for Business Logic**
The classification library is designed with pure functions that have no I/O:

```go
// classifier.go - Pure function, no database access
func Classify(rules []Rule, targets []Target, items []Item, config Config) []Result

// service.go - Orchestrates I/O around pure functions
func (s *Service) ClassifyEvent(ctx context.Context, ...) (*ClassificationResult, error)
```

### 3.3 Database Design

**Technology:** PostgreSQL 16 with pgx driver

**Migration Approach:** In-code migrations in `database/database.go`. Each migration is a versioned SQL string that runs on startup. Migrations are idempotent using `IF NOT EXISTS` patterns.

```go
var migrations = []migration{
    {version: 1, sql: `CREATE TABLE users (...)`},
    {version: 2, sql: `ALTER TABLE calendars ADD COLUMN sync_failure_count INT ...`},
    // ...
}
```

**Key Tables:**
- `users` - Authentication and user data
- `projects` - Billable work units with fingerprint fields for auto-classification
- `time_entries` - One per (user, project, date) with computed and user-edited values
- `calendar_events` - Cached events from Google Calendar
- `classification_rules` - User-defined rules with query DSL
- `billing_periods` - Rate management for invoicing
- `invoices`, `invoice_line_items` - Billing records

**Key Design Decisions:**
- `UNIQUE(user_id, project_id, date)` constraint ensures one TimeEntry per project per day
- Soft-delete patterns (`is_archived`, `is_suppressed`)
- Junction table `time_entry_events` tracks which events contribute to each entry
- Computed columns (`computed_hours`, `computed_title`) alongside user-editable columns

### 3.4 API Design

**OpenAPI-First:**
The API contract lives in `docs/v2/api-spec.yaml`. This is the source of truth for:
- Go server stubs (oapi-codegen)
- TypeScript client types
- MCP tool definitions

**Code Generation:**
```bash
# Generate Go handlers
oapi-codegen -config oapi-codegen.yaml docs/v2/api-spec.yaml

# Generate MCP tools
go run cmd/mcp-codegen/main.go
```

**API Structure:**
```
/api/auth/*           - Authentication (login, signup, OAuth)
/api/projects         - Project CRUD
/api/time-entries     - Time entry operations
/api/calendars        - Calendar connections and sync
/api/calendar-events  - Event classification
/api/rules            - Classification rules
/api/invoices         - Invoice management
/api/billing-periods  - Rate configuration
/health               - Health check
/mcp/*                - MCP protocol endpoints
```

**Authentication:** JWT tokens with middleware-based validation. MCP uses OAuth 2.1 with PKCE.

---

## 4. Frontend Architecture

### 4.1 Directory Structure

```
web/src/
+-- routes/                    # SvelteKit file-based routing
|   +-- +layout.svelte         # Root layout with auth guard
|   +-- +page.svelte           # Main calendar/timesheet view (~1900 lines)
|   +-- login/+page.svelte
|   +-- signup/+page.svelte
|   +-- projects/+page.svelte
|   +-- projects/[id]/+page.svelte
|   +-- rules/+page.svelte
|   +-- settings/+page.svelte
|   +-- invoices/+page.svelte
+-- lib/
|   +-- api/
|   |   +-- client.ts          # API client singleton
|   |   +-- types.ts           # TypeScript types (from OpenAPI)
|   +-- components/
|   |   +-- primitives/        # Generic UI (Button, Modal, Input, Toast)
|   |   +-- widgets/           # Domain-specific (TimeEntryCard, EventPopup, etc.)
|   |   +-- AppShell.svelte    # Layout wrapper
|   +-- stores/
|   |   +-- auth.ts            # Authentication state
|   |   +-- theme.ts           # Dark mode
|   +-- styles/
|   |   +-- classification.ts  # Style computation for event states
|   +-- utils/
|       +-- eventLayout.ts     # Calendar event positioning
|       +-- colors.ts          # Contrast calculation
|       +-- debounce.ts
+-- app.css                    # Tailwind CSS + custom properties
+-- app.html
```

### 4.2 Architectural Patterns

**Svelte 5 Runes:**
The application uses Svelte 5's runes API (`$state`, `$derived`, `$effect`, `$props`).

```svelte
<script lang="ts">
  let { event, projects }: Props = $props();
  let hoveredEventId = $state<string | null>(null);
  const hoveredEvent = $derived(
    hoveredEventId ? calendarEvents.find(e => e.id === hoveredEventId) ?? null : null
  );
</script>
```

**State Synchronization Pattern:**
Critical pattern documented in `ui-coding-guidelines.md`: Store IDs, derive objects.

```svelte
<!-- Anti-pattern: Storing object copies -->
let hoveredEvent = $state<CalendarEvent | null>(null); // STALE!

<!-- Correct pattern: Store ID, derive object -->
let hoveredEventId = $state<string | null>(null);
const hoveredEvent = $derived(events.find(e => e.id === hoveredEventId) ?? null);
```

**Component Hierarchy:**
1. **Primitives** - Generic, reusable (Button, Modal, Input, Toast)
2. **Widgets** - Domain-bound (TimeEntryCard, CalendarEventCard, ProjectChip)
3. **Pages** - Route-level composition

**Widget Naming:** `{Entity}{Presentation}` (e.g., `ProjectChip`, `TimeEntryCard`, `EventPopup`)

**Style System:**
Centralized style computation in `lib/styles/` for complex conditional styling based on domain state (classification status, project colors, etc.).

```typescript
export function getClassificationStyles(state: ClassificationState): ClassificationStyles
```

### 4.3 API Client

Singleton pattern with token management:

```typescript
// client.ts
class ApiClient {
  private token: string | null = null;
  setToken(token: string | null) { this.token = token; }
  private async request<T>(method: string, path: string, body?: unknown): Promise<T>

  // Domain methods
  async listProjects(): Promise<Project[]>
  async classifyCalendarEvent(id: string, data: ClassifyEventRequest): Promise<ClassifyEventResponse>
  // ...
}

export const api = new ApiClient();
```

---

## 5. API Design

### 5.1 Contract-First Development

The OpenAPI specification (`docs/v2/api-spec.yaml`) defines:
- All endpoints with request/response schemas
- Authentication requirements
- MCP metadata for AI agent instructions

**Generation Flow:**
```
api-spec.yaml
    |
    +---> oapi-codegen ---> api.gen.go (Go server interface)
    |
    +---> TypeScript types ---> web/src/lib/api/types.ts
    |
    +---> MCP tool generation ---> mcp/tools.gen.go
```

### 5.2 Key API Patterns

**Resource Operations:**
```
GET    /api/projects           - List
POST   /api/projects           - Create
GET    /api/projects/{id}      - Read
PUT    /api/projects/{id}      - Update
DELETE /api/projects/{id}      - Delete
```

**Specialized Operations:**
```
PUT    /api/calendar-events/{id}/classify   - Classify event to project
POST   /api/rules/preview                   - Preview rule matches (dry run)
POST   /api/rules/apply                     - Apply rules to events
POST   /api/calendar-events/bulk-classify   - Bulk classification
```

**Composite Views:**
```
GET /api/time-entries?start_date=...&end_date=...
    -> Returns entries with computed values from classified events
```

### 5.3 Error Handling

Consistent error response format:
```json
{
  "code": "not_found",
  "message": "Time entry not found"
}
```

Standard HTTP status codes: 400 (validation), 401 (auth), 403 (forbidden), 404 (not found), 500 (server error)

---

## 6. Data Architecture

### 6.1 Domain Model

From `domain-glossary.md`:

```
User
  +-- has many Projects
  +-- has many CalendarConnections
  +-- has many TimeEntries

Project
  +-- has many BillingPeriods
  +-- has many ClassificationRules
  +-- has many TimeEntries

CalendarConnection
  +-- has many Calendars
  +-- has many CalendarEvents

TimeEntry (one per User+Project+Date)
  +-- belongs to Project
  +-- has many contributing CalendarEvents (junction table)
  +-- may belong to Invoice

Invoice
  +-- has many line items (TimeEntries)
```

### 6.2 Key Data Patterns

**Ephemeral vs Materialized TimeEntries:**
TimeEntries are computed on-demand from classified CalendarEvents. When displayed, the system calculates hours from contributing events (handling overlaps via union, not sum). User edits "materialize" the entry, preserving their changes.

**Protection Model:**
- `has_user_edits` - User modified; preserve during recalculation
- `invoice_id` - Locked when invoiced
- `is_stale` - Computed values changed since materialization
- `snapshot_computed_hours` - Captured at edit time for staleness detection

**Classification State Machine:**
```
CalendarEvent: Pending -> Classified (rule/fingerprint/manual/llm)
                      -> Skipped (did not attend)
                      -> Suppressed (user deleted entry)
                      -> Orphaned (deleted from source)
```

**Overlap Handling:**
Multiple events for the same project on the same day use **union of time** (not sum of durations). Example: 9:00-9:30 + 9:15-10:00 = 1.0 hour.

---

## 7. Key Architectural Decisions

### 7.1 Documented (ADRs)

| ADR | Decision | Rationale |
|-----|----------|-----------|
| 001 | One TimeEntry per Project per Day | Simplifies aggregation, matches billing reality |
| 002 | Billing Periods for Rate Management | Enables rate changes over time without breaking history |
| 003 | Scoring-Based Classification | Rules compete via weighted scores; highest confidence wins |

### 7.2 Inferred from Code

| Decision | Evidence | Rationale |
|----------|----------|-----------|
| API-first development | OpenAPI spec + code generation | Strong typing, documentation, multi-client support |
| Monorepo structure | `service/` + `web/` in same repo | Simpler deployment, atomic changes |
| In-code migrations | `database/database.go` | No migration files to manage; version controlled with code |
| Pure classification library | `classifier.go` (no I/O) | Testable, reusable logic; I/O handled by service layer |
| JWT + API keys + MCP OAuth | Multiple auth schemes | Different client needs (browser, scripts, AI agents) |
| Computed + user-edited columns | `computed_hours` vs `hours` | Preserve user intent while tracking source data |
| Background + on-demand sync | `background.go` + `job_worker.go` | Keep data fresh; expand watermarks as needed |

### 7.3 Evolution

The app evolved from Python/FastAPI (v1 in `design.md`) to Go/SvelteKit (v2). Key changes:
- SQLite -> PostgreSQL
- Server-rendered HTML -> SPA with API
- Single-user -> Multi-user with proper auth
- Manual time entries -> Computed from calendar events

---

## 8. Recommendations

### 8.1 Principles Worth Codifying

1. **API-First Always**
   - The OpenAPI spec is the contract. Changes flow from spec to code.
   - Generate rather than hand-write API types and handlers.

2. **Pure Business Logic**
   - Separate pure functions (no I/O) from orchestration services.
   - The classification library demonstrates this well.

3. **State Ownership**
   - One source of truth for each piece of data.
   - UI derives from source arrays; never stores object copies.

4. **Protection Over Deletion**
   - Soft-delete, archive, and suppression patterns.
   - User edits are preserved across recalculations.

5. **In-Code Migrations**
   - Migrations live in `database.go`, not separate SQL files.
   - Idempotent SQL (`IF NOT EXISTS`) for safety.

6. **Component Hierarchy**
   - Primitives -> Widgets -> Pages
   - Naming: `{Entity}{Presentation}`

### 8.2 Documentation Gaps

Consider documenting:
1. **Testing strategy** - No test plan visible for backend or frontend
2. **Error handling conventions** - Consistent error codes and recovery
3. **Performance considerations** - Query optimization, pagination
4. **Deployment runbook** - Step-by-step production deployment

### 8.3 Potential Improvements

1. **Extract shared types package** - Types used by both Go and TS could be generated from a shared schema
2. **Add integration tests** - API contract testing between frontend and backend
3. **Formalize service boundaries** - Some handlers do more than thin routing
4. **Add observability** - Structured logging, metrics, tracing

---

## Appendix: File Reference

| Path | Purpose |
|------|---------|
| `/service/cmd/server/main.go` | Application entry point |
| `/service/internal/api/api.gen.go` | Generated API handlers |
| `/service/internal/database/database.go` | DB connection + migrations |
| `/service/internal/handler/*.go` | HTTP handlers |
| `/service/internal/store/*.go` | Repository layer |
| `/service/internal/classification/` | Classification business logic |
| `/web/src/routes/+page.svelte` | Main calendar view |
| `/web/src/lib/api/client.ts` | API client |
| `/web/src/lib/components/` | UI components |
| `/docs/v2/api-spec.yaml` | OpenAPI specification |
| `/docs/v2/architecture.md` | Existing architecture guide |
| `/docs/v2/domain-glossary.md` | Domain vocabulary |
| `/docs/v2/decisions/` | ADRs |
| `/docs/ui-coding-guidelines.md` | Frontend patterns |
| `/.claude/CLAUDE.md` | AI collaboration guidelines |
| `/docker-compose.prod.yaml` | Production deployment |
