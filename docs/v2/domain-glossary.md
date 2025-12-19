# Domain Glossary - Timesheet App v2

This document defines the ubiquitous language for the Timesheet application. All code, APIs, tests, and documentation should use these terms consistently.

---

## Core Entities

### User
The authenticated person using the application. Owns all other entities.

- Has many **Projects**
- Has many **CalendarConnections** (supports multiple calendars)
- Has many **TimeEntries**

*Future: User would have many TeamMemberships*

### Project
A billable or trackable unit of work. Projects group time entries for reporting and invoicing.

**Attributes:**
- `name` - Display name
- `short_code` - Brief identifier (e.g., "ACM") used in chips and reports
- `color` - Hex color for visual identification
- `is_billable` - Whether this project generates invoices
- `is_archived` - Soft delete; archived projects don't appear in active lists
- `is_hidden_by_default` - Excluded from default views (noise projects)
- `does_not_accumulate_hours` - Time tracked but not counted in totals

**Relationships:**
- Has many **BillingPeriods** (if billable)
- Has many **ClassificationRules**
- Has many **TimeEntries**

*Future: Project might belong to a Team*

### BillingPeriod
A date range with a specific bill rate for a Project. Enables rate changes over time and defines invoiceable windows.

**Attributes:**
- `project_id` - Parent project
- `starts_on` - First day this rate applies
- `ends_on` - Last day (null = ongoing)
- `hourly_rate` - Rate in dollars

**Default behavior:** When a billable project is created, a BillingPeriod is auto-created with `starts_on = creation date`, `ends_on = null`, and the user-entered rate. Users only interact with BillingPeriods when changing rates.

**Invariants:**
- Periods for a project must not overlap
- A billable project must have at least one period

### CalendarConnection
A link to an external calendar source (e.g., Google Calendar). Users may have multiple connections.

**Attributes:**
- `provider` - e.g., "google"
- `credentials` - OAuth tokens (encrypted)
- `last_synced_at` - Timestamp of last successful sync

### CalendarEvent
A meeting or event imported from a connected calendar. CalendarEvents are **inputs** to the system, not the source of truth for time tracking.

**Attributes:**
- `external_id` - ID from the calendar provider
- `title` - Event summary
- `description` - Event body/notes
- `start_time` / `end_time` - When the event occurs
- `attendees` - List of email addresses
- `is_recurring` - Whether this is part of a series
- `response_status` - User's RSVP (accepted, declined, tentative)
- `transparency` - Busy/free status
- `is_orphaned` - Event was deleted from source calendar after sync
- `is_suppressed` - User explicitly chose to ignore this event

**Derived (computed at query/classification time):**
- `duration` - Computed from start/end
- `attendee_domains` - Extracted for classification
- `day_of_week` - For rules like `day-of-week:mon`
- `time_of_day` - For rules like `time:>17:00`

**States:**
- **Pending** - Not yet classified
- **Classified** - Matched to a TimeEntry
  - `classification_source`: `rule`, `fingerprint`, `manual`, `llm`
- **Skipped** - Marked as "did not attend" or matched a skip rule
- **Suppressed** - User deleted the resulting TimeEntry; don't recreate
- **Orphaned** - Deleted from source calendar; may affect linked TimeEntry

### TimeEntry
The core unit of tracked time. Represents work done on a Project for a specific day.

**Key Design Decision:** One TimeEntry per Project per Day. Multiple calendar events for the same project on the same day accumulate into a single entry. This is enforced at the data level.

**Attributes:**
- `project_id` - Which project this time is for
- `date` - The calendar day (not datetime)
- `hours` - Total hours worked (final, possibly rounded)
- `description` - User-editable notes (may accumulate from multiple events)
- `source` - How this entry was created: `calendar`, `manual`, `import`
- `invoice_id` - If invoiced, reference to the invoice (null = uninvoiced)
- `has_user_edits` - Whether user has modified hours, description, or project
- `contributing_events` - List of CalendarEvent IDs that fed into this entry
- `calculation_details` - JSON blob: original durations, overlap handling, rounding applied

**Overlapping Events:** When multiple events for the same project overlap in time, we take the **union** of time covered (not the sum). Example: 9:00-9:30 + 9:15-10:00 = 1.0 hrs. This is ethically correct and easy to explain. We do NOT rationalize hours across different projects - that's on the user.

**States:**
- **Draft** - Can be edited freely
- **Invoiced** - Associated with an Invoice; editability depends on invoice status

**Invariants:**
- Only one entry per (user, project, date) tuple
- Hours must be >= 0
- Cannot be deleted if invoiced

### ClassificationRule
A rule that matches CalendarEvents to Projects. Rules use a query language to express matching criteria.

**Attributes:**
- `project_id` - Target project (or null for special actions)
- `query` - Query string (e.g., `domain:acme.com title:"standup"`)
- `priority` - Higher priority rules match first
- `target_type` - `project` or `did_not_attend`
- `is_enabled` - Can be disabled without deletion

**Rule Sources:**
- **Explicit** - User-created query rules
- **Fingerprint** - Auto-generated from project's matching patterns (domains, emails, keywords)

### Invoice
A collection of TimeEntries for billing purposes.

**Attributes:**
- `project_id` - Which project this invoice is for
- `billing_period_id` - Which rate period applies
- `period_start` / `period_end` - Date range covered
- `status` - `draft`, `sent`, `paid`
- `total_hours` - Sum of entry hours
- `total_amount` - Calculated from hours × rate

**Status semantics:**
- **Draft** - TimeEntries assigned; can still be edited; can't be assigned elsewhere
- **Sent** - TimeEntries locked (no edits allowed)
- **Paid** - Invoice can be archived

**Invariants:**
- TimeEntries cannot be deleted once associated with any Invoice
- Sent/Paid invoices are immutable

---

## Services

### Classifier
Manages classification of CalendarEvents to Projects. Abstracts over multiple classification implementations.

**Implementations:**
- **Rule-based** - Matches events using ClassificationRules and query syntax
- **Fingerprint-based** - Uses project matching patterns (domains, emails, keywords)
- **LLM-based** - Uses AI to suggest classifications (may require MCP interaction)

**Behavior:**
- Skips suppressed events
- Tracks classification source on each event
- Respects priority ordering of rules

### Summarizer
Generates human-readable TimeEntry descriptions from contributing CalendarEvents.

**Behavior:**
- Collects event titles and relevant metadata
- Produces formatted description (template-based or LLM-powered)
- May leverage Contact information if available for richer context

---

## Key Operations

### Sync
Fetches new and updated CalendarEvents from connected calendars. This is a **one-way sync** - the calendar is the source; our CalendarEvent table is a cache.

**Trigger:** Manual or scheduled
**Effect:**
- Creates/updates CalendarEvents
- Marks events as `is_orphaned` if deleted from source
- Does not automatically classify

**Edge cases:**
- Event metadata changes after classification → may need re-classification
- Event deleted after TimeEntry invoiced → TimeEntry preserved (see Orphaned Events)

### Classify
Matches pending CalendarEvents to Projects using the Classifier.

**Trigger:** After sync, or manual "Apply Rules"
**Effect:**
- Creates or updates TimeEntries
- Multiple events for same (project, date) accumulate into one entry
- Marks events as Classified or Skipped
- Records `classification_source` on each event

### Reclassify
Re-runs classification on already-classified events. Used when rules change.

**Default behavior:**
- Reclassify: events that are uninvoiced AND (unclassified OR auto-classified)
- Warn: events that were manually classified or are invoiced that *would* change
- Summarize: events that changed to a different project

**Consideration:** Respects user edits to TimeEntry descriptions

### Create TimeEntry (Manual)
User manually creates a TimeEntry without a backing CalendarEvent.

**Use case:** Work not on calendar, adjustments
**Effect:** Creates entry with `source = manual`, `has_user_edits = true`

### Edit TimeEntry
User modifies hours or description of an existing entry.

**Effect:** Sets `has_user_edits = true`
**Constraint:** Cannot edit if Invoice status is Sent or Paid

### Delete TimeEntry
User removes a TimeEntry.

**Constraints:**
- Cannot delete if associated with any Invoice
- If deleted, contributing CalendarEvents are marked `is_suppressed = true` to prevent recreation

### Create Invoice
Generates an invoice for uninvoiced TimeEntries in a date range.

**Effect:**
- Collects matching entries
- Calculates totals using applicable BillingPeriod rate
- Associates entries with Invoice (status = Draft)

---

## Handling Orphaned Events

When a CalendarEvent is deleted from the source calendar:

1. Sync marks the event as `is_orphaned = true`
2. Any TimeEntry referencing only orphaned events is evaluated:

```
TimeEntry with all events orphaned:
  └─ Is invoiced? → Keep (immutable)
  └─ Has other non-orphaned events? → Keep (still has valid sources)
  └─ Has user edits? → Keep (user expressed intent; treat as manual)
  └─ Otherwise → Delete (auto-generated from vanished data)
```

Orphaned entries that are kept but have issues should be surfaced to the user for review. The specific UX is not defined at the model level.

**Reversion:** User may want to revert a TimeEntry back to its auto-computed state. This clears `has_user_edits` and re-runs Summarizer on contributing events.

---

## UI Components

*Components with distinct UI representation that benefit from a named abstraction.*

### ProjectChip
Colored label showing project short_code (or name if no code). Auto-selects text color for contrast.

### TimeEntryCard
Displays a single TimeEntry with project, hours, description. Supports inline editing.

### ProjectSummary
Sidebar showing hours by project for the current view.

### ClassificationRuleEditor
Query input with live preview of matching events.

---

## States & Transitions

### CalendarEvent Lifecycle
```
[New from Sync] → Pending
Pending → Classified (via Classifier, records source)
Pending → Skipped (via DNA or rule)
Classified → Suppressed (if user deletes TimeEntry)
Any → Orphaned (if deleted from source calendar)
```

### TimeEntry Lifecycle
```
[Created] → Draft
Draft → Draft (edits set has_user_edits = true)
Draft → Invoiced/Draft (via Invoice creation)
Invoiced/Draft → Invoiced/Sent (Invoice sent)
Invoiced/Sent → Invoiced/Paid (Invoice paid)
```

### Invoice Lifecycle
```
[Created] → Draft (entries editable, locked to this invoice)
Draft → Sent (entries locked)
Sent → Paid (archivable)
```

---

## Glossary of Terms

| Term | Definition |
|------|------------|
| Accumulate | When multiple events merge into one TimeEntry, their descriptions accumulate |
| Chip | A small colored label displaying project code |
| Classify | Match an event to a project |
| DNA | "Did Not Attend" - event happened but user didn't participate |
| Fingerprint | Project's matching patterns (domains, emails, keywords) |
| Orphaned | A CalendarEvent deleted from source, or a TimeEntry whose events are all orphaned |
| Query | The DSL for classification rules (e.g., `domain:x.com`) |
| Suppressed | An event the user explicitly chose to ignore |
| Sync | Pull events from external calendar (one-way) |
| Union | For overlapping events, take the time span covered (not sum of durations) |

---

## Future Considerations (Teams)

If Teams are added:
- `Team` entity with `name`, `members`
- `Project.team_id` - projects belong to teams
- `TimeEntry` remains per-user (individuals track their time)
- Invoicing could be per-team or per-project
- Auth layer would need team-based permissions

The current design accommodates this by keeping `user_id` on all user-owned entities.

---

## Open Questions

1. **Contacts integration:** Would syncing user Contacts improve the Summarizer? Adds complexity but could produce richer descriptions.

2. **LLM classification via MCP:** How do we handle the UX when classification requires user to invoke from their chat client? May need a "pending LLM review" state.

3. **Reversion UX:** How does user revert a modified TimeEntry back to auto-computed state? Button? Confirmation?
