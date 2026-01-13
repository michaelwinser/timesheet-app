# Project Lexicon

*Standardized terminology for the Timesheet application. Use these terms consistently in code, documentation, and conversation.*

---

## Core Domain Terms

### Calendar Event
**Definition:** A scheduled item from a connected Google Calendar.
**Not:** "Meeting", "appointment", "calendar item"
**Code locations:** `CalendarEvent` type, `calendar_events` table, `CalendarEventCard` component
**States:** `pending` | `classified` | `skipped` | `needs_review`

### Classification
**Definition:** The act of assigning a calendar event to a project for time tracking.
**Not:** "Categorization", "tagging", "assignment"
**Code locations:** `classification/` package, `ClassificationStatus` enum
**Types:** Rule-based (via classification rules) or Manual (user-initiated)

### Classification Rule
**Definition:** A user-defined pattern that automatically classifies matching events.
**Not:** "Filter", "matcher", "pattern"
**Code locations:** `classification_rules` table, `ClassificationRule` type, `/rules` route
**Format:** Gmail-style query syntax (e.g., `text:acme AND from:client@example.com`)

### Confidence
**Definition:** A score (0.0-1.0) indicating how certain the classifier is about a classification.
**Not:** "Score", "certainty", "probability"
**Thresholds:**
- `< 0.4` (floor): Don't classify
- `0.4 - 0.65`: Classify but flag as `needs_review`
- `> 0.65` (ceiling): Auto-classify without review

### Fingerprint
**Definition:** A project's characteristic pattern used for implicit matching (attendee domains, email addresses).
**Not:** "Signature", "pattern", "identifier"
**Code locations:** `fingerprint` column on projects, `classification/evaluator.go`

### Time Entry
**Definition:** A record of hours worked on a project for a specific date.
**Not:** "Log", "record", "timesheet entry"
**Code locations:** `time_entries` table, `TimeEntry` type, `TimeEntryCard` component
**Constraint:** One time entry per project per day (enforced by unique constraint)

### Ephemeral Time Entry
**Definition:** A computed time entry that hasn't been persisted to the database.
**Not:** "Temporary", "draft", "unsaved"
**Code locations:** `analyzer/` package, frontend computation
**Purpose:** Show real-time calculations before materialization

### Materialized Time Entry
**Definition:** A time entry that has been persisted to the database with computed values.
**Not:** "Saved", "committed", "final"
**Code locations:** `time_entries` table with `is_materialized = true`

---

## Project & Billing Terms

### Project
**Definition:** A billable work category that time entries are assigned to.
**Not:** "Client", "account", "category"
**Code locations:** `projects` table, `Project` type, `/projects` route
**Properties:** `name`, `short_code`, `color`, `hourly_rate`, `is_billable`

### Short Code
**Definition:** A unique abbreviated identifier for a project (e.g., "ACME", "INTERNAL").
**Not:** "Code", "abbreviation", "key"
**Constraint:** Must be unique per user

### Billing Period
**Definition:** A date range within which time entries are grouped for invoicing.
**Not:** "Invoice period", "billing cycle", "date range"
**Code locations:** `billing_periods` table, `BillingPeriod` type

### Invoice
**Definition:** A generated record of billable hours for a billing period.
**Not:** "Bill", "statement"
**Code locations:** `invoices` table, `Invoice` type, `/invoices` route
**States:** `draft` | `sent` | `paid`

---

## Calendar & Sync Terms

### Calendar Connection
**Definition:** The OAuth-authenticated link between a user and their Google Calendar.
**Not:** "Integration", "link", "auth"
**Code locations:** `calendar_connections` table, encrypted credentials

### Calendar
**Definition:** An individual calendar within a Google Calendar account (primary, secondary, or shared).
**Not:** "Calendar connection", "source"
**Code locations:** `calendars` table, `Calendar` type

### Sync
**Definition:** The process of fetching calendar events from Google and updating local state.
**Not:** "Refresh", "update", "fetch"
**Types:**
- **Initial Sync:** First fetch (-4 to +1 weeks)
- **Incremental Sync:** Delta fetch using sync tokens
- **Full Sync:** Complete re-fetch (fallback)
- **Background Sync:** Scheduled expansion of sync window
- **On-Demand Sync:** User-triggered immediate sync

### Sync Token
**Definition:** A Google-provided marker for incremental synchronization.
**Not:** "Token", "marker", "cursor"
**Code locations:** `sync_token` column on `calendars` table

### Water Mark
**Definition:** The earliest and latest dates that have been synced for a calendar.
**Not:** "Boundary", "range", "limit"
**Code locations:** `oldest_synced_date`, `newest_synced_date` on `calendars`

---

## UI & State Terms

### Popup
**Definition:** A floating UI element that appears on hover or click to show details.
**Not:** "Modal", "tooltip", "overlay"
**Examples:** `EventPopup`, `TimeEntryPopup`

### Modal
**Definition:** A dialog that blocks interaction with the rest of the UI until dismissed.
**Not:** "Popup", "dialog", "overlay"
**Examples:** `ExplainClassificationModal`, `ReclassifyWeekModal`

### Scope
**Definition:** The time range currently displayed in the calendar view.
**Not:** "View", "range", "period"
**Values:** `day` | `week` | `full-week`

### Display Mode
**Definition:** How calendar events are rendered.
**Not:** "View type", "layout"
**Values:** `calendar` (grid) | `list`

### Needs Review
**Definition:** A classification state where the system is uncertain and wants user confirmation.
**Not:** "Pending review", "flagged", "uncertain"
**Code locations:** `needs_review` boolean on events, distinct from `pending`

### Skipped
**Definition:** An event marked as "did not attend" or "not work time."
**Not:** "Ignored", "excluded", "declined"
**Code locations:** `is_skipped` boolean, dashed border styling

---

## Architecture Terms

### Handler
**Definition:** HTTP request handler in the Go backend (thin layer, no business logic).
**Not:** "Controller", "endpoint", "route"
**Code locations:** `service/internal/handler/`

### Store
**Definition:** Database access layer in the Go backend (SQL queries only).
**Not:** "Repository", "DAO", "model"
**Code locations:** `service/internal/store/`

### Service
**Definition:** Business logic orchestration in the Go backend.
**Not:** "Manager", "helper"
**Code locations:** `service.go` files in feature packages

### Pure Function
**Definition:** A function with no side effects (no I/O, database, or external calls).
**Examples:** `classifier.go` functions, `analyzer.go` computation

### Derived State
**Definition:** Svelte state computed from other state using `$derived`.
**Not:** "Computed", "calculated"
**Pattern:** `const x = $derived(source.find(...))`

---

## Abbreviations & Acronyms

| Abbreviation | Meaning |
|--------------|---------|
| **PRD** | Product Requirements Document |
| **ADR** | Architecture Decision Record |
| **MCP** | Model Context Protocol (AI integration) |
| **JWT** | JSON Web Token (authentication) |
| **OAuth** | Open Authorization (Google auth) |

---

## Anti-Terms (Don't Use These)

| Avoid | Use Instead | Reason |
|-------|-------------|--------|
| "Task" | "Calendar Event" or "Time Entry" | Ambiguous in this domain |
| "Category" | "Project" | We use project consistently |
| "Tag" | "Classification" | Tags imply multiple; we do single classification |
| "Refresh" | "Sync" | Sync is the established term |
| "Save" (for time entries) | "Materialize" | Distinguishes from edit saves |
| "Auto-classify" | "Rule-based classification" | More precise |

---

## Usage Examples

**Good:** "The calendar event was classified to the Acme project via a keyword rule."

**Bad:** "The meeting was tagged to the Acme category automatically."

**Good:** "After sync, the time entries are recalculated and materialized."

**Bad:** "After refresh, the logs are updated and saved."

**Good:** "The event needs review because confidence was below the ceiling."

**Bad:** "The event is flagged because the score was too low."
