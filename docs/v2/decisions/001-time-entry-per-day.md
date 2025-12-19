# ADR 001: One TimeEntry Per Project Per Day

## Status

Proposed

## Context

In v1, time entries are tightly coupled to calendar events (1:1 relationship). This creates problems:

1. **Overlapping events** - Two meetings for the same project at the same time result in duplicate or confusing entries
2. **User edits** - Editing a time entry description can conflict with calendar sync
3. **Manual entries** - Non-calendar time tracking requires workarounds
4. **Aggregation** - Reporting requires summing across many small entries

## Decision

In v2, we will consolidate to **one TimeEntry per Project per Day per User**.

- Calendar events are *inputs* that feed into time entries
- Multiple events for the same (project, date) **accumulate** into a single entry
- Users can edit the consolidated entry freely
- Manual time entry adds to the same daily bucket

## Consequences

### Positive

- **Overlapping events** become trivial - just add to the daily total
- **User edits persist** - the entry is the source of truth, not the events
- **Simpler data model** - fewer entries, clearer relationships
- **Natural invoicing** - daily entries roll up cleanly
- **Manual entry** - just create/update the daily entry

### Negative

- **Less granularity** - can't see per-meeting breakdown (mitigated by description accumulation)
- **Migration complexity** - need to consolidate existing entries
- **Event-to-entry tracing** - harder to see which events contributed (could keep references)

### Neutral

- Calendar events remain in the database for audit/reference
- Classification rules continue to work (they target projects, not entries)

## Implementation Notes

- `TimeEntry` gets a unique constraint on `(user_id, project_id, date)`
- Accumulation of descriptions: store as structured list or formatted text
- Consider `TimeEntry.sources` - array of contributing event IDs for traceability
