# PRD: Time Entry Enhancements (Phase 2)

This document defines requirements for Phase 2 of the v2 roadmap: Time Entry Enhancements.

---

## Problem Statement

The current time entry generation is reactive and error-prone:
- Entries are updated incrementally as events are classified, leading to incorrect hour totals
- Overlapping events (especially busy blocks synced from other calendars) cause inflated hours
- No visibility into how hours were calculated
- No clear handling of rounding for events with unusual start/end times
- Users editing entries have no way to know if source events have changed

**Goal**: Time entries should be derived from classified events using clear, auditable logic, with simple controls for freezing entries when the user is satisfied.

---

## Core Architectural Change

**From**: Reactive/additive (classify event → increment entry hours)
**To**: Derived/calculated (entry hours = computed from contributing events)

Time entries become a **computed view** over classified events. They update automatically until the user protects them via editing, pinning, or locking.

---

## Protection Model

A simple three-state model for both events and time entries:

| State | How it happens | Behavior |
|-------|----------------|----------|
| **Unlocked** | Default | System updates freely |
| **Pinned/Locked** | User edits, or explicit lock | Protected from auto-update |
| **Invoiced** | Added to invoice | Immutable |

### Pinned vs Locked

Both mean "system won't auto-update" - the difference is how you got there:

- **Pinned**: Implicit, via user action on the item
  - Time entry: User edits hours or description
  - Event: User manually classifies
- **Locked**: Explicit, via bulk action
  - User clicks "Lock Day" or "Lock Week"
  - Locks all events AND time entries in that period

### Stale Indicator

Protected items (pinned or locked) can become stale when:
- Underlying events change (added, removed, times modified)
- Classification rules change (for locked events)

When stale:
- Visual indicator on the lock/pin icon (color change or refresh badge)
- User can "refresh" to accept computed value (stays protected)
- User can "unpin/unlock" to return to auto-update mode

### Invoiced Items

Invoiced entries are immutable:
- Cannot refresh, cannot unlock
- If stale, shown as informational only ("computed value would be X")
- Different visual treatment (checkmark or invoice icon)

---

## Components

### 1. Time Entry Analyzer

A **pure function** that computes time entries from classified events.

**Input**:
- Date
- User's classified events for that date
- Project settings

**Output**:
- One computed time entry per project for that day
- Each entry includes:
  - `hours`: Calculated duration (after union and rounding)
  - `title`: Short summary (first/primary event title)
  - `description`: Detailed summary (all events, attendees, etc.)
  - `contributing_event_ids`: List of event IDs
  - `calculation_details`: JSON audit trail

**Responsibilities**:
- Group events by project
- Handle same-project overlaps via time union
- Apply rounding rules
- Generate title and description from events

The description generation is part of the Analyzer, not a separate component. Initial implementation is algorithmic; architecture supports future LLM enhancement.

---

## Overlap Handling

### Same-Project Overlaps

Multiple events for the same project on the same day are **unioned** by time range.

**Example**:
```
Event A: 9:00 - 9:30
Event B: 9:15 - 10:00
Union:   9:00 - 10:00 = 1.0 hours (not 1.25 hours from sum)
```

The union is calculated from raw times, then rounding is applied to the result.

### Cross-Project Overlaps

Events in different projects are calculated **independently**. The system does not automatically adjust.

**Behavior**:
- Detect overlaps > 15 minutes
- Surface to user with visual indicator
- Offer **one-click fixes**:
  - "Reduce Project A by 30m"
  - "Ignore this overlap"

The goal is to inform, not enforce. Users decide what's appropriate.

---

## Rounding

Events often have unusual durations due to Google's "End meetings early" feature, external scheduling, etc.

### Rounding Rules

**Granularity**: 15 minutes (configurable per project in future)

**Logic** (applied to remainder after dividing by 15):
- 0-6 minutes remainder → round down
- 7-14 minutes remainder → round up

**Implementation**: Rounding is a separate, parameterized function that can be enhanced later without touching other code.

### Application Order

1. Calculate time union for same-project overlaps (raw times)
2. Apply rounding to the union result
3. Store both raw and rounded values in `calculation_details`

### Transparency

Users can see:
- Original event durations
- Union calculation (if applicable)
- Rounding applied
- Final hours

This is captured in the `calculation_details` JSON and displayed in the UI.

---

## All-Day Events

All-day events have no meaningful duration. Rather than per-project settings, handle in-situ:

- All-day events contribute to a time entry but with 0 hours by default
- UI shows "Remove all-day event" action on affected time entries
- User can manually adjust hours if needed

This optimizes for quick corrections over upfront configuration.

---

## Time Entry Fields

### Current (User) Values

| Field | Description |
|-------|-------------|
| `hours` | Displayed hours |
| `title` | Short description (user-facing) |
| `description` | Detailed description |

### Computed Values

| Field | Description |
|-------|-------------|
| `computed_hours` | Latest calculated hours |
| `computed_title` | Generated short title |
| `computed_description` | Generated detailed description |

### State Fields

| Field | Description |
|-------|-------------|
| `is_pinned` | User has edited this entry |
| `is_locked` | Part of a locked day/week |
| `is_stale` | Computed differs from current |
| `invoice_id` | If invoiced, reference to invoice |

### Audit Fields

| Field | Description |
|-------|-------------|
| `contributing_event_ids` | Events that feed into this entry |
| `calculation_details` | JSON: raw durations, overlaps, rounding |

---

## Update Behavior

### When Updates Run

The Analyzer runs and updates unlocked entries:
- After calendar sync completes
- After event classification changes
- After bulk rule application

### Update Logic

```
For each (date, project) with classified events:

    computed = Analyzer.compute(date, project, events)
    existing = find_existing_entry(date, project)

    if existing is null:
        create_entry(computed)

    else if existing.is_invoiced:
        update_computed_fields_only(existing, computed)
        set_stale_if_differs(existing)

    else if existing.is_pinned or existing.is_locked:
        update_computed_fields_only(existing, computed)
        set_stale_if_differs(existing)

    else:  # unlocked
        update_all_fields(existing, computed)
        clear_stale(existing)
```

### Real-time Feedback

When user classifies an event:
1. Affected time entry updates immediately (if unlocked)
2. Entry flashes briefly to show the change
3. No modal confirmation - just visual feedback

---

## Locking

### Lock Day / Lock Week

Bulk action that locks all events and time entries in the period.

**Behavior**:
- Sets `is_locked = true` on all time entries in range
- Sets `is_locked = true` on all classified events in range
- Prevents auto-updates to those items

**Why lock events too?** An unlocked event that gets reclassified would logically affect its time entry. If the entry is locked but the event isn't, this creates confusion. Lock the whole stack.

### Unlock

Reverse operation:
- Sets `is_locked = false` on items in range
- Items return to auto-update mode
- Next sync/update will refresh them

### Lock + Refresh

If a locked item is stale, user can:
1. **Refresh**: Accept computed values, stay locked
2. **Unlock**: Return to auto-update mode

"Click to refresh, click to relock" is the mental model for stale locked items.

---

## Orphaned Events

When a calendar event is deleted from Google Calendar after being classified:

1. Sync marks the event as `is_orphaned = true`
2. Affected time entries are recalculated (if unlocked)
3. Decision tree for entries where all events are orphaned:

```
All contributing events orphaned?
  ├─ is_invoiced? → Keep (immutable), show warning
  ├─ is_pinned or is_locked? → Keep, show warning
  └─ Otherwise → Delete entry
```

### Search Support

Add query term `orphaned:yes` / `orphaned:no` for finding orphaned events.

---

## Title and Description Generation

Time entries have both a **title** (short, user-facing) and **description** (detailed).

### Title Generation

- Use the primary/first event's title
- Or synthesize from multiple: "Weekly Sync +2 more"
- Keep short (< 50 chars)

### Description Generation

- List all contributing event titles
- Include attendee information
- Include event descriptions (truncated)
- Deduplicate repeated titles

### User Edit Preservation

If user edits title or description:
- Entry becomes pinned
- `computed_title` and `computed_description` still updated
- User values preserved until explicit refresh

---

## API Changes

### Time Entry Response

```json
{
  "id": "...",
  "project_id": "...",
  "date": "2024-01-15",
  "hours": 4.5,
  "title": "Client meetings",
  "description": "Weekly Sync, Code Review, Design Discussion",
  "is_pinned": false,
  "is_locked": true,
  "is_stale": true,
  "invoice_id": null,
  "computed_hours": 4.75,
  "computed_title": "Client meetings",
  "computed_description": "Weekly Sync, Code Review, Design Discussion, Planning",
  "contributing_events": ["evt_1", "evt_2", "evt_3"],
  "calculation_details": {
    "events": [
      {"id": "evt_1", "title": "Weekly Sync", "start": "09:00", "end": "10:00", "raw_minutes": 60},
      {"id": "evt_2", "title": "Code Review", "start": "10:00", "end": "11:30", "raw_minutes": 90},
      {"id": "evt_3", "title": "Design Discussion", "start": "11:15", "end": "12:00", "raw_minutes": 45}
    ],
    "overlap_minutes": 15,
    "union_minutes": 180,
    "rounding_applied": "+5m",
    "final_minutes": 285
  }
}
```

### New Endpoints

| Endpoint | Purpose |
|----------|---------|
| `POST /api/time-entries/{id}/refresh` | Accept computed values (stays protected) |
| `POST /api/time-entries/{id}/pin` | Explicitly pin an entry |
| `POST /api/time-entries/{id}/unpin` | Remove pin, return to auto-update |
| `POST /api/days/{date}/lock` | Lock all entries and events for a day |
| `POST /api/days/{date}/unlock` | Unlock all entries and events for a day |
| `POST /api/weeks/{date}/lock` | Lock all entries and events for a week |
| `POST /api/weeks/{date}/unlock` | Unlock all entries and events for a week |
| `GET /api/days/{date}/overlaps` | Get cross-project overlap report |

### MCP Tools

Update MCP server to expose:
- Computed vs current values in time entry queries
- Overlap detection: `get_overlaps(date_range)` returns cross-project overlaps for AI assistant review
- Lock/unlock operations
- Bulk refresh operations

---

## UI Indicators

### Time Entry List

| Indicator | Meaning |
|-----------|---------|
| 📌 (pin icon) | Entry is pinned (user edited) |
| 🔒 (lock icon) | Entry is locked (via lock day/week) |
| ✓ (check icon) | Entry is invoiced |
| Yellow/orange tint on icon | Protected item is stale |
| ↻ (refresh badge) | Click to refresh stale item |
| ⚠️ (overlap icon) | Cross-project overlap detected |

### Time Entry Detail

- Show contributing events list
- Show calculation breakdown (expandable)
- "Refresh" button (if stale and protected)
- "Unpin" / "Unlock" button

### Week Header

- "Lock Week" button
- Indicator showing locked status: "Week locked" or "3 of 5 days locked"

---

## Database Schema Changes

### time_entries table

Add columns:

```sql
ALTER TABLE time_entries ADD COLUMN title TEXT;
ALTER TABLE time_entries ADD COLUMN is_pinned BOOLEAN DEFAULT FALSE;
ALTER TABLE time_entries ADD COLUMN is_locked BOOLEAN DEFAULT FALSE;
ALTER TABLE time_entries ADD COLUMN is_stale BOOLEAN DEFAULT FALSE;
ALTER TABLE time_entries ADD COLUMN computed_hours DECIMAL(5,2);
ALTER TABLE time_entries ADD COLUMN computed_title TEXT;
ALTER TABLE time_entries ADD COLUMN computed_description TEXT;
ALTER TABLE time_entries ADD COLUMN calculation_details JSONB;
```

### calendar_events table

Add columns:

```sql
ALTER TABLE calendar_events ADD COLUMN is_locked BOOLEAN DEFAULT FALSE;
```

### time_entry_events junction table

Track contributing events:

```sql
CREATE TABLE time_entry_events (
  time_entry_id UUID REFERENCES time_entries(id) ON DELETE CASCADE,
  calendar_event_id UUID REFERENCES calendar_events(id) ON DELETE CASCADE,
  PRIMARY KEY (time_entry_id, calendar_event_id)
);
```

---

## Implementation Phases

### Phase 2.1: Analyzer Foundation
- Implement Time Entry Analyzer as pure function
- Implement overlap union logic
- Implement rounding logic (separate function)
- Add `calculation_details` storage
- Unit tests for calculation edge cases

### Phase 2.2: Contributing Events
- Add `time_entry_events` junction table
- Track which events feed into each entry
- API returns contributing events
- Basic UI showing contributing events

### Phase 2.3: Protection Model
- Add is_pinned, is_locked, is_stale fields
- Implement pin on edit behavior
- Implement lock day/week endpoints
- Implement refresh endpoint

### Phase 2.4: Live Updates
- Wire Analyzer into sync flow
- Wire Analyzer into classification flow
- Implement flash feedback on UI
- Remove old reactive time entry code

### Phase 2.5: UI Polish
- Stale indicators with refresh action
- Calculation breakdown display
- Lock/unlock UI in week header
- Contributing events detail view

### Phase 2.6: Cross-Project Overlaps
- Overlap detection (>15m threshold)
- Overlap report endpoint
- One-click fix suggestions
- UI for overlap warnings

### Phase 2.7: Title/Description Generation
- Implement title generation logic
- Implement description generation logic
- Add computed_title, computed_description fields
- Respect user edits (pinning)

---

## Open Questions

1. **Orphaned event search**: Confirmed - add `orphaned:yes/no` query term.

2. **Rounding configurability**: Start with 15m/7m default. Make it a project setting later if needed. Keep rounding as separate function for easy enhancement.

3. **Overlap tolerance**: Confirmed - >15m threshold for surfacing cross-project overlaps.

4. **Batch operations**: Defer "pin all" / "refresh all" for now. Lock day/week covers the main use case.

5. **Calendar sync frequency**: Calendar sync should be automatic (periodic). Define interval based on usage patterns.

---

## Success Criteria

- Time entries accurately reflect classified events with clear audit trail
- Users can see how hours were calculated (contributing events, overlaps, rounding)
- Simple protection model: unlocked items update freely, protected items stay frozen
- Lock day/week provides easy "sign off" workflow
- Stale protected items clearly indicated with one-click refresh
- Cross-project overlaps surfaced (>15m) with resolution options
- Flash feedback on classification keeps UI responsive without modal interruptions
- Weekly workflow: classify → review totals → lock week → invoice
