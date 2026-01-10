# Design: Ephemeral Time Entries

> **Status**: Active
> **Issue**: #64
> **Supersedes**: Reactive time entry creation in `prd-time-entry-enhancements.md`

## Overview

Time entries are derived from classified calendar events. Rather than materializing entries reactively when events are classified, entries are computed on demand and only persisted when the user interacts with them or they are invoiced.

This document describes how time entries work, not how we got here.

## Core Concepts

### Ephemeral vs Materialized

A time entry exists in one of two states:

| State | Stored in DB | Has user input | Behavior |
|-------|--------------|----------------|----------|
| **Ephemeral** | No | No | Computed fresh on every request |
| **Materialized** | Yes | Yes | User values preserved, computed values tracked for staleness |

**Ephemeral entries** are computed from classified events when the API requests time entries for a date range. They exist only in the API response.

**Materialized entries** are created when:
- User sets hours (including setting to 0 via suppression)
- User edits title or description
- Entry is included in an invoice

### The Staleness Formula

A materialized entry can become stale when the underlying events change. Staleness is defined as:

```
is_stale = (hours IS NOT NULL)                              -- materialized
       AND (hours != computed_hours)                        -- user differs from computed
       AND (computed_hours != snapshot_computed_hours)      -- computed has drifted since materialization
```

This three-way comparison distinguishes:
- User intentionally set different hours (not stale)
- Computed values changed after user set hours (stale)

### Snapshot at Materialization

When a time entry is materialized, we capture `snapshot_computed_hours`—the computed hours at the moment the user made their edit. This anchors the staleness check.

If `computed_hours` later changes (events added, removed, reclassified), we compare against the snapshot to determine if the user's decision was based on different data.

## Data Model

### time_entries table

```sql
-- User-facing values (set when materialized)
hours                    DECIMAL(5,2)    -- User's hours (NULL if ephemeral)
title                    TEXT            -- User's title
description              TEXT            -- User's description

-- Computed values (always fresh)
computed_hours           DECIMAL(5,2)    -- Calculated from events
computed_title           TEXT            -- Generated from events
computed_description     TEXT            -- Generated from events

-- Materialization tracking
snapshot_computed_hours  DECIMAL(5,2)    -- Computed hours at materialization time
has_user_edits           BOOLEAN         -- TRUE when user has set any value
is_suppressed            BOOLEAN         -- User explicitly suppressed this entry

-- Protection
is_pinned                BOOLEAN         -- User edited (implicit protection)
is_locked                BOOLEAN         -- Day/week locked (explicit protection)
invoice_id               UUID            -- Invoice reference (immutable when set)

-- Audit
calculation_details      JSONB           -- Breakdown of computation
```

### time_entry_events junction table

For materialized entries only, tracks which events contributed at materialization time:

```sql
CREATE TABLE time_entry_events (
    time_entry_id     UUID REFERENCES time_entries(id) ON DELETE CASCADE,
    calendar_event_id UUID REFERENCES calendar_events(id) ON DELETE CASCADE,
    PRIMARY KEY (time_entry_id, calendar_event_id)
);
```

This provides an audit trail. If contributing events are later deleted, the entry becomes stale (computed_hours drops, but snapshot preserves what user approved).

## API Behavior

### Listing Time Entries

`GET /api/time-entries?start_date=X&end_date=Y`

1. **Compute** ephemeral entries from classified events in the date range
2. **Fetch** materialized entries from the database
3. **Merge**: materialized entries take precedence; ephemeral entries fill gaps
4. **Return** unified list with both `hours` (user) and `computed_hours` (fresh)

For ephemeral entries in the response:
- `hours` = `computed_hours` (no user override)
- `snapshot_computed_hours` = NULL
- `has_user_edits` = false

For materialized entries:
- `hours` = user's value
- `computed_hours` = freshly computed
- `snapshot_computed_hours` = value at materialization
- `is_stale` = computed per formula above

### Creating/Updating Time Entries

`POST /api/time-entries` or `PUT /api/time-entries/{id}`

When user sets hours:
1. Compute current `computed_hours` from events
2. Set `snapshot_computed_hours` = `computed_hours`
3. Set `hours` = user's value
4. Set `has_user_edits` = true
5. Populate `time_entry_events` with contributing event IDs

### Suppressing Time Entries

`DELETE /api/time-entries/{id}` (on ephemeral) or explicit suppression

When user wants to hide a computed entry:
1. Materialize the entry
2. Set `is_suppressed` = true
3. Set `hours` = 0 (or NULL, TBD)
4. Capture `snapshot_computed_hours`

The entry won't appear in normal listings but is tracked to prevent re-computation from recreating it.

### Resolving Staleness

User has two options for stale entries:

**Accept computed values:**
```
hours = computed_hours
snapshot_computed_hours = computed_hours
```

**Keep override (acknowledge drift):**
```
snapshot_computed_hours = computed_hours
-- hours unchanged
```

Both clear staleness by updating `snapshot_computed_hours` to match current `computed_hours`.

## Invoicing Integration

### Invoice Creation

When creating an invoice for a date range:

1. Compute time entries for the range (ephemeral + materialized)
2. For each entry to be invoiced:
   - If ephemeral: materialize it (capture `snapshot_computed_hours`)
   - If already materialized: no change needed
3. Create invoice line items referencing time entries
4. Do NOT set `invoice_id` yet (draft invoice)

### Invoice Finalization (Draft → Sent)

When invoice status changes to "sent":
1. Set `time_entries.invoice_id` for all referenced entries
2. Entries are now locked from edits

### Invoice Line Items

Line items are lightweight references:

```sql
invoice_line_items:
    time_entry_id   UUID NOT NULL   -- Reference to time entry
    hourly_rate     DECIMAL(10,2)   -- Snapshot from billing period
    amount          DECIMAL(10,2)   -- hours × rate at invoice time
```

Hours, title, and description are read from the time entry. The time entry is the source of truth.

### Stale Invoices

An invoice is stale if any of its referenced time entries are stale.

- **Draft invoice**: Show warning, allow regeneration
- **Sent invoice**: Show warning (informational—hours have drifted since sent)

## Event Changes and Staleness

### Event Classification Changes

When an event is reclassified:
- No reactive time entry creation
- Next `ListTimeEntries` call computes fresh values
- Materialized entries see updated `computed_hours`, may become stale

### Event Deletion

When contributing events are deleted:
- `computed_hours` drops (fewer events)
- Materialized entries become stale if `snapshot_computed_hours` differs
- Special case: all events deleted → `computed_hours` = 0

### Calendar Removal

When a calendar is removed:
- All its events are deleted (cascade)
- Affected materialized entries become stale
- Ephemeral entries for that calendar simply don't appear

## Computation Details

### Time Entry Analyzer

The analyzer is a pure function:

```
Input:  date, classified events for that date
Output: computed time entries (one per project)
```

For each project:
1. Filter events assigned to that project
2. Merge overlapping time ranges (union)
3. Apply rounding (15-minute increments)
4. Generate title and description
5. Return computed entry with `calculation_details`

### Calculation Details

```json
{
    "events": [
        {"id": "...", "title": "Weekly Sync", "start": "09:00", "end": "10:00", "raw_minutes": 60},
        {"id": "...", "title": "Code Review", "start": "09:45", "end": "11:00", "raw_minutes": 75}
    ],
    "time_ranges": [
        {"start": "09:00", "end": "11:00", "minutes": 120}
    ],
    "union_minutes": 120,
    "rounding_applied": "none",
    "final_minutes": 120
}
```

## Migration from Reactive Model

The previous model created time entries reactively when events were classified. Migration:

1. Existing entries with `has_user_edits = true` are already materialized
2. Existing entries with `has_user_edits = false` can be treated as ephemeral (will be recomputed)
3. Backfill `snapshot_computed_hours` for materialized entries (set to current `computed_hours`)
4. Remove reactive creation code paths

## Summary

| Aspect | Behavior |
|--------|----------|
| Default state | Ephemeral (computed on demand) |
| Materialization trigger | User edit, suppression, or invoicing |
| Staleness detection | Three-way: user vs computed vs snapshot |
| Resolution | Update snapshot to clear staleness |
| Invoice source of truth | Time entry (not line item snapshot) |
| Event changes | Reflected in computed values; may trigger staleness |
