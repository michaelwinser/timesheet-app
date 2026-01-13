# Design Doc: Calendar Disconnection and Time Entry Preservation

## Problem Statement

When a user unchecks a calendar or disconnects from Google Calendar, time entries derived from events in those calendars must be handled correctly. Currently, the CASCADE delete on `time_entry_events` breaks links to invoiced time entries, causing them to display as 0h.

## Background

### Time Entry States

| State | Description | Persisted | `has_user_edits` | `invoice_id` |
|-------|-------------|-----------|------------------|--------------|
| Ephemeral | Computed from events, not in DB | No | N/A | N/A |
| Materialized | Persisted because invoiced or edited | Yes | `true` | `NULL` or set |

**Key insight:** An entry only becomes materialized when the user edits it OR when it's invoiced. There is no "materialized but not edited" state - if it's in the DB, `has_user_edits` should be `true` (or it's invoiced).

### Current Data Model

```
calendar_connections
    ↓ ON DELETE CASCADE
calendars
    ↓ ON DELETE CASCADE
calendar_events
    ↓ ON DELETE CASCADE
time_entry_events (join table)

time_entries
    ← invoice_line_items (ON DELETE RESTRICT - protects invoiced entries)
```

### The Problem

When `calendar_events` are deleted:
1. `time_entry_events` rows are CASCADE deleted (breaks the link)
2. Invoiced `time_entries` survive (protected by RESTRICT)
3. Display logic shows `ComputedHours = 0` because no events exist
4. User sees 0h for invoiced entries that should show stored hours

## Scenarios

### User Actions

| # | Action | Description |
|---|--------|-------------|
| A | Uncheck calendar | User deselects a calendar in settings |
| B | Disconnect provider | User disconnects entire Google Calendar connection |
| C | Lose access | Calendar becomes inaccessible (permissions revoked, deleted) |

### Scenario Matrix: Expected Behavior

| Action | Entry State | Expected Result |
|--------|-------------|-----------------|
| A/B (Uncheck/Disconnect) | Ephemeral | Entry disappears (not persisted, events gone) |
| A/B (Uncheck/Disconnect) | Materialized | Entry preserved with stored hours, event link removed |
| C (Lose access) | Any | Entry preserved, sync stops (out of scope) |

**Key Insight:** Materialized entries always survive. Their stored values are authoritative. The event link (`time_entry_events`) can be safely removed.

## Proposed Solution

### 1. Handle event deletion in application code

Before deleting events (via uncheck or disconnect):
1. Find all `time_entries` linked to affected events via `time_entry_events`
2. These entries are materialized (by definition - they're in the DB)
3. The `time_entry_events` link will be CASCADE deleted - this is fine
4. The `time_entries` themselves survive with their stored values

No special handling needed because:
- Materialized entries have `has_user_edits = true` OR `invoice_id IS NOT NULL`
- Their stored `hours` field is authoritative
- Losing the event link doesn't change the stored data

### 2. Display logic: stored hours always win

Current (broken):
```
if events_exist:
    display computed_hours
else:
    display 0  // ← BUG: ignores stored hours
```

Proposed:
```
display stored_hours  // Always. This is the authoritative value.
// computed_hours is informational only, shown separately if needed
```

### 3. Unified handling for uncheck/disconnect

Both actions follow the same flow:
1. Mark affected calendars as `is_selected = false` (uncheck) or delete connection (disconnect)
2. Events become invisible (filtered by `is_selected`) or are CASCADE deleted
3. `time_entry_events` links are CASCADE deleted (or orphaned)
4. Materialized `time_entries` survive with their stored values
5. Ephemeral entries simply cease to exist (they were never persisted)

## Implementation Plan

### Phase 1: Fix display logic (stored hours always win)
- [ ] Audit timeentry/service.go - remove ComputedHours = 0 fallback
- [ ] Audit Svelte components that display hours - use stored hours field
- [ ] Audit totals/summary calculations - use stored hours field
- [ ] Audit export logic - use stored hours field

### Phase 2: Fix calendar deselection (issue #93)
- [ ] Keep the query-time filter fix (c.is_selected = true)
- [ ] Verify no additional changes needed (CASCADE handles cleanup)

### Phase 3: Add integration tests
- [ ] Test: Uncheck calendar - ephemeral entries disappear
- [ ] Test: Uncheck calendar - materialized entries survive with stored hours
- [ ] Test: Disconnect - materialized entries survive with stored hours
- [ ] Test: Disconnect then reconnect - no duplicate entries
- [ ] Test: Display shows stored hours when events are missing

## Design Decisions

1. **Materialized entries are always preserved.** A time entry only becomes materialized because it was invoiced or user-edited. These represent real tracked time and must not be deleted.

2. **No re-linking after disconnect/reconnect.** When a calendar is re-added, we don't attempt to match old entries to new events. Instead, show an indicator that entries are "out of sync" with calendar (future enhancement - not in scope for this work).

3. **Stored hours always win.** This is a fundamental principle:
   - `hours` field in DB is authoritative for display, totals, invoicing, export
   - `computed_hours` is informational only (shows what events would calculate)
   - User edits to hours/title/description are NEVER overwritten by sync
   - Sync only updates `computed_*` fields, never stored fields

## Data Model Principles

### Hours Priority (Baked Into Model)

```
Stored hours (time_entries.hours)
    ↑ Written by: User edits only
    ↑ Read by: Display, totals, invoicing, export

Computed hours (calculated from events)
    ↑ Written by: Sync cycle only
    ↑ Read by: Informational display (e.g., "computed: 1.5h")
```

### Field Ownership

| Field | Written By | Overwrite Allowed |
|-------|------------|-------------------|
| `hours` | User (UI) | Never by sync |
| `title` | User (UI) | Never by sync |
| `description` | User (UI) | Never by sync |
| `computed_hours` | Sync | Always by sync |
| `computed_title` | Sync | Always by sync |
| `computed_description` | Sync | Always by sync |

### Scenario C (lose access)

Out of scope for this work. Sync errors are handled separately - events become stale but remain valid.

## Database Changes

None required - using application logic to promote entries before delete.

## Migration

For existing orphaned entries (from past disconnects):
```sql
-- Find time entries with no linked events but have stored hours
UPDATE time_entries
SET has_user_edits = true
WHERE id NOT IN (SELECT time_entry_id FROM time_entry_events)
  AND hours > 0
  AND has_user_edits = false;
```
