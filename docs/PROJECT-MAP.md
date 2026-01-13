# Timesheet Application - Project Map

*A quick reference for understanding what code does what.*

---

## Overview Diagram

```
                         ┌─────────────────────────────────────┐
                         │      SvelteKit Frontend (web/)      │
                         │  Calendar View | Projects | Rules    │
                         └────────────┬──────────────────────────┘
                                      │ HTTP/REST API
                         ┌────────────▼──────────────────────────┐
                         │      Go Backend (service/)            │
                         │  cmd/server/main.go (entrypoint)      │
                         └────────────┬──────────────────────────┘
                                      │
        ┌─────────────────────────────┼─────────────────────────────┐
        │                             │                             │
   ┌────▼──────┐          ┌──────────▼──────────┐      ┌───────────▼────────┐
   │ PostgreSQL │         │  Internal Packages   │      │   Google APIs      │
   │  Database  │         │                      │      │   - Calendar       │
   │            │         │  handler/ → HTTP     │      │   - Sheets         │
   │ - users    │         │  store/   → SQL      │      │   - OAuth          │
   │ - projects │         │  classification/     │      │                    │
   │ - events   │         │  analyzer/           │      └────────────────────┘
   │ - entries  │         │  sync/               │
   │ - invoices │         │  google/             │
   └────────────┘         └──────────────────────┘
```

---

## Backend Components (`service/internal/`)

| Package | Purpose | Key Files |
|---------|---------|-----------|
| **handler/** | HTTP request handlers (thin layer) | `server.go`, `auth.go`, `calendars.go`, `projects.go`, `rules.go`, `invoices.go`, `time_entries.go` |
| **store/** | Database operations (SQL queries) | One file per entity: `users.go`, `projects.go`, `calendar_events.go`, `time_entries.go`, etc. |
| **classification/** | Event classification engine | `classifier.go` (pure logic), `evaluator.go` (rule evaluation), `parser.go` (query parsing), `service.go` (orchestration) |
| **analyzer/** | Time entry computation | `analyzer.go` - merges events, rounds hours, generates audit trail |
| **sync/** | Calendar synchronization | `background.go` (scheduled), `job_worker.go` (on-demand), `week.go` (helpers) |
| **google/** | Google API integration | `calendar.go`, `sheets.go` |
| **database/** | Migrations and connection | `database.go` - all migrations defined here |
| **crypto/** | Encryption for OAuth tokens | `crypto.go` - AES-256-GCM |
| **timeentry/** | Time entry service | `service.go` - recalculation triggers |
| **api/** | Generated OpenAPI types | `api.gen.go` (DO NOT EDIT) |

---

## Frontend Components (`web/src/`)

### Routes

| Route | Purpose | Key Component |
|-------|---------|---------------|
| `/` | Main calendar view, classification, time entries | `routes/+page.svelte` (primary UI) |
| `/projects` | Project management CRUD | `routes/projects/` |
| `/rules` | Classification rule management | `routes/rules/` |
| `/invoices` | Billing and invoice generation | `routes/invoices/` |
| `/settings` | Calendar connections, API keys | `routes/settings/` |
| `/login`, `/signup` | Authentication | `routes/login/`, `routes/signup/` |

### Library Modules (`lib/`)

| Module | Purpose | Key Exports |
|--------|---------|-------------|
| **api/** | Backend communication | `ApiClient` class, TypeScript types |
| **stores/** | Global state | `auth`, `theme` stores |
| **components/widgets/** | Feature components | `CalendarEventCard`, `TimeEntryCard`, `EventPopup`, `ProjectChip`, etc. |
| **components/primitives/** | Base UI elements | `Button`, `Modal`, `Toast` |
| **styles/** | Styling logic | `getClassificationStyles()` |
| **utils/** | Helpers | `eventLayout.ts`, `debounce.ts`, `colors.ts` |

---

## Domain Concept → Code Map

| Domain Concept | Backend Code | Frontend Code |
|----------------|--------------|---------------|
| **Calendar Event** | `store/calendar_events.go`, `classification/` | `CalendarEventCard`, `EventPopup` |
| **Classification** | `classification/classifier.go`, `classification/evaluator.go` | `ExplainClassificationModal`, classification styles |
| **Time Entry** | `store/time_entries.go`, `analyzer/`, `timeentry/` | `TimeEntryCard`, `TimeEntryPopup`, `TimeEntryBarChart` |
| **Project** | `store/projects.go`, `handler/projects.go` | `/projects` route, `ProjectChip` |
| **Rule** | `store/classification_rules.go`, `classification/parser.go` | `/rules` route, `RuleCard` |
| **Invoice** | `store/invoices.go`, `handler/invoices.go` | `/invoices` route |
| **Calendar Sync** | `sync/`, `google/calendar.go` | `/settings` route, sync status |

---

## Data Flow

### Classification Flow
```
Google Calendar → sync/job_worker.go → google/calendar.go
    → store/calendar_events.go → classification/service.go
    → timeentry/service.go → store/time_entries.go
```

### API Request Flow
```
Svelte Component → lib/api/client.ts → handler/*.go
    → store/*.go → PostgreSQL → response
```

---

## Key Architectural Patterns

1. **Handler/Store/Service Layering** - Handlers are thin, stores do SQL, services have business logic
2. **Pure Classification** - `classifier.go` has no I/O, `service.go` orchestrates
3. **Generated API Types** - OpenAPI spec generates Go handlers and TS types
4. **ID-Based State (Svelte)** - Store IDs in `$state`, derive objects with `$derived`
5. **In-Code Migrations** - All in `database/database.go`, no separate SQL files

---

## File Naming Conventions

| Location | Convention | Example |
|----------|------------|---------|
| Go handlers | `{feature}.go` | `projects.go`, `calendars.go` |
| Go stores | `{entity}.go` | `time_entries.go` |
| Go tests | `{name}_test.go` | `classifier_test.go` |
| Generated Go | `*.gen.go` | `api.gen.go` |
| Svelte routes | `+page.svelte` | `routes/projects/+page.svelte` |
| Svelte components | `PascalCase.svelte` | `TimeEntryCard.svelte` |
| TypeScript utils | `camelCase.ts` | `eventLayout.ts` |

---

## Quick Reference: Where to Find Things

| Looking for... | Look in... |
|----------------|------------|
| API endpoint implementation | `service/internal/handler/` |
| Database queries | `service/internal/store/` |
| Classification logic | `service/internal/classification/` |
| Time calculation | `service/internal/analyzer/` |
| Google Calendar integration | `service/internal/google/calendar.go` |
| Main UI | `web/src/routes/+page.svelte` |
| Reusable components | `web/src/lib/components/widgets/` |
| API client | `web/src/lib/api/client.ts` |
| OpenAPI spec | `docs/v2/api-spec.yaml` |
| Design docs | `docs/v2/` |
