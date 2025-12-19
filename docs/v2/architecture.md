# Architecture Guide - Timesheet App v2

This document defines the macro-architecture and establishes conventions for reasoning about components at each layer. It provides durable guidance for consistent design and implementation decisions.

---

## Architectural Layers

```
┌─────────────────────────────────────────────────────────────┐
│                      Environment                             │
│  (Docker, Auth providers, URLs, secrets, infrastructure)    │
└─────────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────┐
│                        Service                               │
│  (API spec, business logic, data layer, internal services)  │
└─────────────────────────────────────────────────────────────┘
        │
        ▼ API Contract (OpenAPI)
        │
┌───────┴───────┬─────────────────┬──────────────────────────┐
│  Web Client   │   MCP Server    │   Future Clients...      │
│  (Browser)    │   (AI Agent)    │   (Mobile, CLI, etc.)    │
└───────────────┴─────────────────┴──────────────────────────┘
```

### Layer Definitions

#### Environment
The runtime context in which the system operates.

**Concerns:**
- Container orchestration (Docker, docker-compose)
- Authentication providers (Google OAuth)
- Secrets management
- URLs and redirect configurations
- Database connections
- Deployment targets (local, staging, production)

**Not concerned with:** Business logic, UI, data models

#### Service
The backend application exposing a RESTful API. Stateless with respect to clients.

**Concerns:**
- API contract (OpenAPI spec)
- Business logic and domain rules
- Data persistence and schema
- Internal services (Classifier, Summarizer, Sync)
- Authentication/authorization enforcement

**Contract:** Clients interact via the API spec. The Service makes no assumptions about client implementation.

#### Clients
Applications that consume the Service API. Each client has its own internal architecture.

**Current clients:**
- **Web Client** - Browser-based SPA/MPA
- **MCP Server** - Exposes Service capabilities to AI agents

**Each client has:**
- Its own component architecture
- Its own state management
- Its own presentation logic

---

## Naming Conventions

To avoid ambiguity, use **layer prefixes** when discussing components:

### Domain Concepts (Layer-Agnostic)
Use plain names when discussing the abstract concept:
- "A TimeEntry represents work done on a Project for a day"
- "When events overlap, TimeEntry uses union of time"

These are defined in **domain-glossary.md**.

### Service Layer
Prefix with `Service.` or use context-specific terms:

| Term | Meaning |
|------|---------|
| `Service.TimeEntry` | The TimeEntry as implemented in the Service (schema, API) |
| `TimeEntryRepository` | Data access component for TimeEntries |
| `TimeEntryService` | Business logic for TimeEntry operations |
| `POST /api/time-entries` | API endpoint |
| `Classifier` | Internal service for event classification |

### Web Client
Prefix with `WebClient.` or use UI-specific terms:

| Term | Meaning |
|------|---------|
| `WebClient.TimeEntry` | TimeEntry as represented in the web client |
| `TimeEntryCard` | UI widget displaying a single entry |
| `TimeEntryEditor` | Form for editing an entry |
| `TimeEntryList` | Container showing multiple entries |
| `useTimeEntry()` | Hook/state management for entry data |

### MCP Server
Prefix with `MCP.` when discussing MCP-specific concerns:

| Term | Meaning |
|------|---------|
| `MCP.classify_events` | Tool exposed to AI agents |
| `MCP.get_time_entries` | Resource query for agents |

---

## Reasoning About Changes

When analyzing an issue or feature, determine which layer(s) are affected:

### Example: "User can't find time entries after event cancelled by someone else"

**Analysis:**
1. What happened? Calendar event deleted externally → Event orphaned → TimeEntry may be affected
2. Is the Service handling this? Check if `is_orphaned` is set, if TimeEntry is preserved correctly
3. Is the API exposing this? Check if orphan status is in the response
4. Is the Web Client showing this? Check if UI highlights orphaned entries

**Possible fixes:**
- **Service only:** Logic bug in orphan handling → fix `TimeEntryService`
- **API + Client:** Orphan state not exposed → add to API response + update `TimeEntryCard`
- **Client only:** Data is there, UI doesn't show it → update `TimeEntryCard` styling

### Decision Framework

```
Issue/Feature
    │
    ├─ Does it change what data exists or how it's structured?
    │   └─ Yes → Service (schema, repository)
    │
    ├─ Does it change business rules or operations?
    │   └─ Yes → Service (services, domain logic)
    │
    ├─ Does it change what the API exposes or accepts?
    │   └─ Yes → Service (API spec, controllers)
    │
    ├─ Does it change how data is displayed?
    │   └─ Yes → Client (widgets, views)
    │
    ├─ Does it change client-side behavior/interaction?
    │   └─ Yes → Client (state, event handlers)
    │
    └─ Does it change deployment/runtime configuration?
        └─ Yes → Environment (docker, config)
```

---

## Service Architecture

The Service has internal structure:

```
┌─────────────────────────────────────────────────────────┐
│                      API Layer                           │
│  (Routes, Controllers, Request/Response validation)     │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                   Service Layer                          │
│  (Business logic, orchestration, domain services)       │
│                                                          │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐       │
│  │ Classifier  │ │ Summarizer  │ │ Invoicer    │       │
│  └─────────────┘ └─────────────┘ └─────────────┘       │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                  Repository Layer                        │
│  (Data access, queries, persistence abstraction)        │
│                                                          │
│  ┌─────────────────┐ ┌─────────────────┐               │
│  │ TimeEntryRepo   │ │ CalendarEventRepo│  ...         │
│  └─────────────────┘ └─────────────────┘               │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                   Data Layer                             │
│  (Database, ORM, migrations)                            │
│  PostgreSQL / Firestore (abstracted by Repository)      │
└─────────────────────────────────────────────────────────┘
```

### Service Components

| Component | Responsibility |
|-----------|----------------|
| `TimeEntryService` | CRUD operations, overlap calculation, accumulation |
| `ClassificationService` | Matches events to projects using Classifier |
| `SyncService` | Fetches events from calendar providers |
| `InvoiceService` | Creates invoices, locks entries |
| `Classifier` | Rule/fingerprint/LLM matching engine |
| `Summarizer` | Generates descriptions from events |

### Repositories

| Repository | Entity |
|------------|--------|
| `UserRepository` | User |
| `ProjectRepository` | Project, BillingPeriod |
| `CalendarEventRepository` | CalendarEvent, CalendarConnection |
| `TimeEntryRepository` | TimeEntry |
| `ClassificationRuleRepository` | ClassificationRule |
| `InvoiceRepository` | Invoice |

---

## Web Client Architecture

The Web Client has its own component model:

```
┌─────────────────────────────────────────────────────────┐
│                      Pages                               │
│  (Route-level components, layout, navigation)           │
│                                                          │
│  WeekPage, ProjectsPage, RulesPage, InvoicesPage        │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                    Containers                            │
│  (Composite components, data fetching, state)           │
│                                                          │
│  ProjectSummary, TimeEntryList, ClassificationPanel     │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                     Widgets                              │
│  (Entity-bound display/edit components)                 │
│                                                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Project: Chip, ListItem, Editor, Tooltip        │   │
│  │ TimeEntry: Card, Row, Editor                    │   │
│  │ CalendarEvent: Card, Popover                    │   │
│  │ Invoice: Card, LineItem                         │   │
│  │ Rule: Card, Editor, MatchExplanation            │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                    Primitives                            │
│  (Generic, reusable UI elements)                        │
│                                                          │
│  Button, Input, TagInput, QueryInput, DatePicker,       │
│  Modal, Toast, Tooltip, Dropdown                        │
└─────────────────────────────────────────────────────────┘
```

### Widget Naming Convention

For entity-bound widgets: `{Entity}{Presentation}`

| Entity | Presentations |
|--------|---------------|
| Project | `ProjectChip`, `ProjectListItem`, `ProjectEditor`, `ProjectTooltip` |
| TimeEntry | `TimeEntryCard`, `TimeEntryRow`, `TimeEntryEditor` |
| CalendarEvent | `CalendarEventCard`, `CalendarEventPopover` |
| Invoice | `InvoiceCard`, `InvoiceLineItem`, `InvoiceEditor` |
| Rule | `RuleCard`, `RuleEditor`, `RuleMatchExplanation` |

### When to Create a Widget

Create a named widget when:
- It represents a domain entity in a specific context
- It's reused in multiple places
- It has meaningful props/events contract
- It benefits from isolated testing

Don't create a widget for:
- One-off layouts
- Pure styling wrappers
- Things that are just a primitive with props

---

## MCP Server Architecture

The MCP Server exposes Service capabilities to AI agents:

```
┌─────────────────────────────────────────────────────────┐
│                    MCP Tools                             │
│  (Actions the agent can take)                           │
│                                                          │
│  classify_events, create_time_entry, get_projects, etc. │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                   MCP Resources                          │
│  (Data the agent can query)                             │
│                                                          │
│  time_entries, projects, unclassified_events, etc.      │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                   Service API                            │
│  (Same API used by Web Client)                          │
└─────────────────────────────────────────────────────────┘
```

MCP is a thin adapter over the Service API, not a separate implementation.

---

## Cross-Cutting Concerns

### Authentication
- **Environment:** OAuth provider configuration, redirect URLs
- **Service:** Token validation, user context extraction
- **Clients:** Login flow, token storage, authenticated requests

### Error Handling
- **Service:** Domain errors, validation errors, HTTP status codes
- **Clients:** Error display, retry logic, user feedback

### State Synchronization
- **Service:** Source of truth for data
- **Clients:** May cache, must handle stale data, optimistic updates

---

## Document Map

| Document | Layer | Purpose |
|----------|-------|---------|
| `domain-glossary.md` | Domain (all layers) | Entity definitions, operations, vocabulary |
| `architecture.md` | All | Layer definitions, naming, reasoning guidance |
| `components.md` | Web Client | Widget catalog, props/events contracts |
| `api-spec.yaml` | Service | OpenAPI contract |
| `decisions/*.md` | All | Architectural decision records |

---

## Summary: How to Talk About Components

1. **Domain concept:** "TimeEntry" - the abstract entity
2. **Service component:** "TimeEntryService" or "Service.TimeEntry" - backend implementation
3. **API surface:** "POST /api/time-entries" - the contract
4. **Web widget:** "TimeEntryCard" or "WebClient.TimeEntryCard" - UI component
5. **MCP tool:** "MCP.create_time_entry" - agent capability

When in doubt, ask: "Which layer are we talking about?"

This precision enables:
- Accurate issue triage
- Correct change scoping
- Clear communication
- Testable contracts at each boundary

---

## Pragmatic Usage

You don't need formal prefixes in every sentence. The goal is clarity about **what kind of change** is being discussed, not rigid terminology.

**Use domain language for conceptual changes:**
> "I'd like to change how orphaned TimeEntries work - they should be deletable if not invoiced."

This signals a domain/Service change. The rules are changing.

**Use "UI" as the abstract layer over all clients:**
> "The UI should dim orphaned TimeEntries."

This signals a presentation change across clients.

**Specify the client when it matters:**
> "The CLI should hide orphaned entries unless `--show-orphaned` is passed."
> "The web UI should show a warning banner for orphaned entries."

**Context usually makes it obvious.** If we're looking at `week.html` and you say "dim orphaned entries" - that's clearly Web UI. No prefix needed.

**The architecture doc is for ambiguous moments:**
- "Should this logic go in the API or the client?"
- "Is this a schema change or just a display change?"
- "Which components need updating for this feature?"

That's when having the layers defined pays off. The rest of the time, just talk naturally.
