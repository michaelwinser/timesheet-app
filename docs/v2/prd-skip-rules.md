# PRD: Skip Rules

## Problem Statement

Users need to exclude certain calendar events from time tracking without changing their actual Google Calendar response (which sends email notifications). Examples:
- Meetings the user declined but still appear on their calendar
- Canceled meetings
- All-day events that don't represent actual work time
- "Busy" blocks synced from other calendars

Currently, the only way to exclude events is manual deletion or classifying to a "Noise" project, both of which have drawbacks.

## Terminology

| Term | Definition |
|------|------------|
| `response_status` | User's actual Google Calendar response: `accepted`, `declined`, `tentative`, `needsAction` |
| `is_skipped` | Boolean flag: should this event contribute to time entries? |
| `classification_status` | Event state: `pending` or `classified` |
| `classification_source` | How classification happened: `rule`, `fingerprint`, `manual`, `llm` |

**Deprecated**: The term "DNA" (Did Not Attend) from v1 is replaced by "skipped".

## Design

### Principle: Orthogonal Concerns

Skip and classification are independent:
- **Skip**: "Should this event contribute to time entries?" (yes/no)
- **Classification**: "What project is this event for?" (project assignment)

An event can be both skipped AND classified to a project. This allows users to:
1. Unskip an event later and have it already classified
2. See skipped events organized by project for review

### Response Status Determination

The user's response status comes from Google Calendar's attendee data:

1. During sync, find the attendee with `Self=true` (Google sets this based on calendar context)
2. Extract their `ResponseStatus` field
3. If no self attendee found, default to `accepted`

Google correctly handles multiple calendars - `Self=true` is set based on the calendar's email address, not the authenticated user.

### Skip Rules

Skip rules are classification rules that set `is_skipped=true` instead of assigning a project.

**Database representation**:
- `project_id = NULL`
- `attended = false` (repurposed field, indicates "skip rule")

**Example skip rules**:
```
response:declined          → Skip declined meetings
title:Canceled             → Skip canceled meetings
transparency:transparent   → Skip "free" time blocks
is-all-day:yes            → Skip all-day events
```

### Rule Evaluation Order

ApplyRules performs two passes on each event:

```
Pass 1 - Skip Rules:
  Evaluate all rules where attended=false
  If ANY match → set is_skipped=true

Pass 2 - Project Rules:
  Evaluate all rules where project_id IS NOT NULL
  Classify to winning project (scoring model)
```

Both passes always run. A skipped event still gets classified to a project.

### Schema Changes

**calendar_events table**:
```sql
-- Remove 'skipped' from classification_status enum
-- (requires recreating the enum since Postgres doesn't support DROP VALUE)
ALTER TYPE classification_status RENAME TO classification_status_old;
CREATE TYPE classification_status AS ENUM ('pending', 'classified');

-- Add is_skipped column
ALTER TABLE calendar_events ADD COLUMN is_skipped BOOLEAN NOT NULL DEFAULT false;

-- Update column type
ALTER TABLE calendar_events
  ALTER COLUMN classification_status TYPE classification_status
  USING classification_status::text::classification_status;

DROP TYPE classification_status_old;

-- Index for finding skipped events
CREATE INDEX idx_calendar_events_is_skipped ON calendar_events(is_skipped) WHERE is_skipped = true;
```

**classification_rules table**: No changes needed. Existing constraint supports skip rules:
```sql
CHECK (project_id IS NOT NULL AND attended IS NULL
    OR project_id IS NULL AND attended IS NOT NULL)
```

Rules with `attended=false` are skip rules.

### MCP Changes

**create_rule**: Add `skip` parameter as alternative to `project_id`:
```json
{
  "query": "response:declined",
  "skip": true
}
```

**list_rules**: Already shows "skip" for attendance rules (no change needed).

**apply_rules**: Update to run both skip and project passes.

**explain_classification**: Show skip rule evaluation in output.

### Time Entry Calculation

Events with `is_skipped=true` are excluded from time entry calculation:
```sql
SELECT ... FROM calendar_events
WHERE classification_status = 'classified'
  AND is_skipped = false
  AND project_id IS NOT NULL
```

### UI Presentation

Skipped events use Google Calendar's visual style for declined meetings:

**Visual Treatment:**
- **Strikethrough title**: The event title has a line through it (`text-decoration: line-through`)
- **Muted colors**: Gray text and borders instead of project colors
- **Reduced opacity**: Overall card/block appears dimmed
- **Skip indicator**: Small "✕" icon in a dashed border box

**Card/Block Styling:**
```css
/* Skipped event styling */
.event-skipped {
  background-color: transparent;
  border: 1px solid #9CA3AF; /* gray-400 */
}

.event-skipped .title {
  text-decoration: line-through;
  color: #9CA3AF; /* gray-400 */
}

.event-skipped .metadata {
  color: #9CA3AF; /* gray-400 */
}
```

**Interaction:**
- Clicking a skipped event shows reclassification options (unskip + assign to project)
- Skip indicator button shows "Skipped - click to reclassify" tooltip
- Unskipping an event restores its classification (if it was classified) or returns to pending state

**Filters:**
- "Show skipped" toggle to include/exclude skipped events from views
- Skipped events hidden by default in time entry views (since they don't contribute)
- Visible by default in classification/event list views for review

## Implementation Plan

1. Schema migration (add `is_skipped`, simplify `classification_status`)
2. Update `ApplyRules` to run skip pass before project pass
3. Update `create_rule` MCP tool to accept `skip` parameter
4. Update `explain_classification` to show skip rules
5. Update time entry calculation to exclude skipped events
6. Update event queries to handle `is_skipped` filter

## Success Criteria

- `response:declined` rule correctly skips matching events
- Skipped events are excluded from time entries
- Skipped events can still be classified to projects
- Users can manually unskip events
- `explain_classification` shows why an event was skipped
