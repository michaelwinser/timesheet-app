# Design: Composite View API & Smart Updates

**Status**: Proposed
**Date**: 2026-01-11
**Area**: API Architecture, Performance
**Supersedes**: `optimized-updates.md`

## Problem Statement

The application's primary view (Day/Week/Month dashboard) currently suffers from:
1.  **Multiple Round Trips**: Loading the dashboard requires separate calls for `calendar-events` and `time-entries`.
2.  **Consistency Gaps**: Since calls are independent, it's possible (though rare) to fetch an ephemeral time entry whose source calendar event has changed in the interim, leading to a "split-brain" UI.
3.  **Chatty Updates**: Actions like "Classify Event" require a full reload of the time entry list to ensure computed totals are correct, causing UI sluggishness.

## Solution: Composite Range Resource

We introduce a unified **View Endpoint** that returns a consistent snapshot of all data needed to render the dashboard for a given time range.

### 1. `GET /sync/view`

**Parameters**:
- `start_date` (required): YYYY-MM-DD
- `end_date` (required): YYYY-MM-DD
- `project_id` (optional): Filter by project

**Response**:
```json
{
  "meta": {
    "sync_token": "abc-123",       // For future delta sync
    "generated_at": "2026-01-11T12:00:00Z"
  },
  "events": [
    // List of CalendarEvent objects
  ],
  "entries": [
    // List of TimeEntry objects (Materialized + Ephemeral)
    // Computed based on the exact events returned in "events"
  ],
  "connections": [
    // Status of calendar connections (e.g., "synced", "error")
  ],
  "totals": {
    // Optional: Pre-calculated totals for the view
    "total_hours": 42.5
  }
}
```

**Benefits**:
- **Atomic Consistency**: The backend computes ephemeral entries from *exactly* the set of events it is returning.
- **Performance**: Single HTTP request. Ideal for mobile or high-latency networks.
- **Simplicity**: Frontend `loadData()` becomes a single call.

### 2. Smart Modification Responses

To obsolete the need for "Refetch after Action", all state-changing endpoints must return a **Composite Patch**.

**Example**: `POST /calendar/events/{id}/classify`

**Old Response**:
Returns `CalendarEvent`. Client must refetch `time-entries`.

**New Response**:
```json
{
  "event": { ... },       // The updated CalendarEvent (status='classified')
  "entry": { ... },       // The resulting TimeEntry (Upsert this ID)
  "meta": {
    "affected_entries": [] // In rare complex cases, other entries might change
  }
}
```

**Client Logic**:
```typescript
// Optimistic-like update
const { event, entry } = await api.classify(...);
store.updateEvent(event);
store.upsertEntry(entry); // No reload needed!
```

## Migration Plan

### Backend
1.  Implement `ViewHandler.GetCompositeView(ctx, start, end)`:
    - Parallel fetch of Events and Connections.
    - Compute Ephemeral Entries using the fetched Events.
    - Return combined JSON.
2.  Update `ClassificationHandler` to return the `TimeEntry` along with the `Event`.

### Frontend
1.  Update `loadData()` in `+page.svelte` to use `api.getCompositeView()`.
2.  Update `handleClassify` to consume the composite response.

## FAQ

**Q: Why not a "Day" resource?**
A: A strictly partitioned "Day" resource forces N requests for an N-day view. The `start/end` range query allows the client to fetch *exactly* what the viewport needs (Day, Week, or Month) in a single optimized call.

**Q: How does this relate to Ephemeral Invoices?**
A: This pattern (Composite View) complements Ephemeral Invoices. Both move complex coordination/calculation to the backend, ensuring the frontend is a "dumb" renderer of authoritative sources.
