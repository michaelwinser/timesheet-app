# Optimized Frontend Updates (Superseded)

> [!WARNING]
> This document has been superseded by [Design: Composite View API & Smart Updates](design-composite-view-api.md). The concepts here (Smart Responses) have been integrated into the broader Composite View architecture.

**Status**: Superseded
**Date**: 2026-01-11
**Area**: Frontend, Performance

## Current Reactivity Model

The application currently follows a **"Single Source of Truth"** pattern with one-way data flow, leveraging Svelte 5's runes (`$state`, `$derived`).

### 1. State Topology
*   **Root Store**: `+page.svelte` holds the authoritative state arrays:
    *   `entries`: `TimeEntry[]`
    *   `calendarEvents`: `CalendarEvent[]`
*   **Derivation**: Child components (Charts, Popups, Lists) receive these arrays as props and use `$derived` to compute their local views. They do **not** maintain internal copies of the data.
*   **Consistency**: This ensures that "split-brain" states (where a chart shows 5h but the list shows 4h) are impossible.

### 2. The "Save" Cycle (High Performance)
When updating a single entity (Time Entry), we achieve near-instant UI updates without a full reload.
1.  **API Call**: `PUT /time-entries/{id}` returns the updated entity.
2.  **State Patch**: We replace *only* that object in the `entries` array.
    ```typescript
    entries = entries.map(e => e.id === updated.id ? updated : e);
    ```
3.  **Reactivity**: Svelte's fine-grained reactivity automatically updates all `$derived` totals, charts, and sidebars instantly.

## Optimization Opportunities

### The "Classify" Bottleneck
Currently, classifying a calendar event triggers a "heavy" update cycle because the action has side effects on the `entries` domain (creating/modifying ephemeral or materialized entries).

**Current Flow**:
1.  Optimistically update `calendarEvents` (Status -> 'classified').
2.  **Blocking**: Await `api.listTimeEntries({ start, end })`.
3.  Replace entire `entries` array.

**Drawbacks**:
- Requires a network round-trip fetching potentially 100+ entries.
- UI may feel sluggish on slow connections.

### Proposed Optimization: "Smart Responses"

We can eliminate the full reload by enriching the API response.

#### 1. Backend Change
Update `POST /calendar/events/{id}/classify` to return `ClassifyResponse`:
```go
type ClassifyResponse struct {
    Event      store.CalendarEvent `json:"event"`       // The updated event
    TimeEntry  *store.TimeEntry    `json:"time_entry"`  // The created/affected time entry
}
```

#### 2. Frontend Change
Update `handleClassify` to patch both arrays locally:

```typescript
const { event, time_entry } = await api.classifyEvent(...);

// 1. Update Event
calendarEvents = calendarEvents.map(e => e.id === event.id ? event : e);

// 2. Update Time Entry (Upsert Logic)
if (entries.find(e => e.id === time_entry.id)) {
    entries = entries.map(e => e.id === time_entry.id ? time_entry : e);
} else {
    entries = [...entries, time_entry];
}
```

#### 3. Handling Complex Side Effects
Note: If `Classify` causes a "Split" or affects multiple entries (rare, but possible with complex rules), the backend should return a list `TimeEntries: []*TimeEntry`. If the side effects are too complex to capture (e.g., cascading rule re-evaluations), the client can fall back to the current full-reload strategy.

## Guidelines for Future Features
1.  **Prefer Returning Full Objects**: Mutation endpoints should always return the fully database-synced object.
2.  **Patch, Don't Reload**: Whenever possible, splice the returned object into the local state array rather than re-fetching the list.
3.  **Derived Over State**: Never copy data into a component's local state unless it is a form draft (edit buffer). Use `$derived` for everything else.
