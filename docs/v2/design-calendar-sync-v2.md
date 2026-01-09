# Design Doc: Calendar Sync Architecture v2

**Status:** Draft (Rev 3)
**Author:** Claude (with Michael)
**Date:** 2026-01-09
**PRD:** `prd-calendar-sync-v2.md`

## Overview

This document describes the technical architecture for the calendar sync system, implementing water mark-based sync with incremental updates via Google Calendar sync tokens.

## Architecture

### Data Model Changes

```sql
-- Add water mark and sync tracking columns to calendars table
ALTER TABLE calendars ADD COLUMN low_water_mark DATE;
ALTER TABLE calendars ADD COLUMN high_water_mark DATE;
ALTER TABLE calendars ADD COLUMN last_synced_at TIMESTAMPTZ;
ALTER TABLE calendars ADD COLUMN sync_failure_count INT DEFAULT 0;
ALTER TABLE calendars ADD COLUMN needs_reauth BOOLEAN DEFAULT FALSE;

-- Index for efficient water mark queries
CREATE INDEX idx_calendars_water_marks ON calendars (connection_id, low_water_mark, high_water_mark);

-- Index for background sync job (find stale calendars)
CREATE INDEX idx_calendars_last_synced ON calendars (last_synced_at) WHERE needs_reauth = FALSE;
```

**Water mark semantics:**
- `low_water_mark`: Monday of earliest synced week (NULL = never synced)
- `high_water_mark`: Sunday of latest synced week (NULL = never synced)
- Both NULL → calendar needs initial sync
- Both set → range is inclusive, all weeks between marks are synced

**Staleness tracking:**
- `last_synced_at`: Timestamp of last successful sync (for 24h staleness check)
- `sync_failure_count`: Consecutive failures (stop retrying after 3)
- `needs_reauth`: True if OAuth token refresh failed (user must reconnect)

### Background Sync Job Queue

Water mark expansion is handled by a database-backed job queue, ensuring atomicity and enabling job coalescing.

```sql
CREATE TABLE calendar_sync_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    calendar_id UUID NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
    job_type TEXT NOT NULL,  -- 'expand_watermarks', 'fill_gap'
    target_min_date DATE NOT NULL,
    target_max_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, running, completed, failed
    priority INT DEFAULT 0,  -- higher = more urgent
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT
);

CREATE INDEX idx_sync_jobs_pending ON calendar_sync_jobs (calendar_id, status)
    WHERE status = 'pending';
```

### On-Demand Fetch Flow

When a user navigates to a date range outside the current water marks:

```
User navigates to June 2025 (outside current marks Dec 2025 - Jan 2026)
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│ GET /api/calendar-events?start_date=2025-06-30&end_date=... │
└─────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│ Server checks: is June 30 week within water marks?          │
│   → No, it's outside [Dec 11, 2025 - Jan 15, 2026]          │
└─────────────────────────────────────────────────────────────┘
    │
    ├──────────────────────────────────────┐
    ▼                                      ▼
┌──────────────────────┐    ┌─────────────────────────────────┐
│ SYNCHRONOUS:         │    │ ASYNC: Insert job into          │
│ Fetch June 30 week   │    │ calendar_sync_jobs table        │
│ from Google, store   │    │                                 │
│ events, return to    │    │ job_type: 'expand_watermarks'   │
│ client immediately   │    │ target_min: 2025-06-30          │
└──────────────────────┘    │ target_max: current_min - 1 day │
                            └─────────────────────────────────┘
                                           │
                                           ▼
                            ┌─────────────────────────────────┐
                            │ Background worker picks up job  │
                            │                                 │
                            │ 1. Coalesce with other pending  │
                            │    jobs for same calendar       │
                            │ 2. Fetch events for gap         │
                            │ 3. BEGIN TRANSACTION            │
                            │    - Upsert events              │
                            │    - Update water marks         │
                            │    - Update last_synced_at      │
                            │ 4. COMMIT                       │
                            └─────────────────────────────────┘
```

**Key behaviors:**

1. **Immediate fetch is synchronous**: The user gets events for their requested week immediately. This ensures responsive UX.

2. **Gap-filling is asynchronous**: The background job fills the gap between the "island" (June 2025) and the main water mark range. This happens without blocking the user.

3. **Job coalescing**: When inserting a new job, merge with overlapping/adjacent pending jobs:

```sql
-- Before inserting new job for calendar X, range [A, B]:
-- Find pending jobs that overlap or are adjacent (within 7 days)
UPDATE calendar_sync_jobs
SET target_min_date = LEAST(target_min_date, $new_min),
    target_max_date = GREATEST(target_max_date, $new_max)
WHERE calendar_id = $calendar_id
  AND status = 'pending'
  AND (target_min_date <= $new_max + 7 AND target_max_date >= $new_min - 7);
-- If no rows updated, insert new job
```

4. **Atomic updates**: The background job updates events and water marks in a single transaction. Either both succeed or neither does.

5. **Events endpoint checks DB first**: Since "islands" exist temporarily (events stored but water marks not yet updated), the endpoint queries the database for events before deciding to fetch from Google.

### API Changes

#### GET /api/events

**Current:** Returns events for date range from database only.

**New behavior:**

```
GET /api/events?start_date=2025-10-06&end_date=2025-10-12&force=false

Response:
{
  "events": [...],
  "sync_status": "fresh" | "syncing" | "stale",
  "synced_at": "2025-10-08T10:30:00Z"
}
```

**Server logic:**

```go
const StalenessThreshold = 24 * time.Hour

func (h *Handler) GetEvents(ctx context.Context, req GetEventsRequest) (*GetEventsResponse, error) {
    weekStart := normalizeToWeekStart(req.StartDate)  // Monday
    weekEnd := normalizeToWeekEnd(req.EndDate)        // Sunday

    calendars := h.store.GetSelectedCalendars(ctx, req.UserID)

    // Check if all calendars have this week within water marks
    allWithinMarks := true
    isStale := false
    for _, cal := range calendars {
        if !cal.HasWeekSynced(weekStart, weekEnd) {
            allWithinMarks = false
            break
        }
        // Check staleness (any calendar >24h old makes the data stale)
        if cal.LastSyncedAt.Before(time.Now().Add(-StalenessThreshold)) {
            isStale = true
        }
    }

    syncStatus := "fresh"

    if !allWithinMarks || req.Force {
        // Case B or C: Synchronous fetch for this week
        h.syncWeekForAllCalendars(ctx, req.UserID, weekStart, weekEnd)

        // Async: expand water marks if needed
        if !allWithinMarks {
            go h.expandWaterMarks(ctx, req.UserID, weekStart, weekEnd)
        }
    } else if isStale {
        // Case A': Within marks but stale - synchronous incremental sync
        syncStatus = "refreshing"
        h.incrementalSync(ctx, req.UserID)  // Synchronous, not async
    } else {
        // Case A: Fresh - async incremental sync
        go h.incrementalSync(ctx, req.UserID)
    }

    // Return events from database
    events := h.store.GetEvents(ctx, req.UserID, weekStart, weekEnd)
    return &GetEventsResponse{
        Events:     events,
        SyncStatus: syncStatus,
        SyncedAt:   h.store.GetLatestSyncTime(ctx, req.UserID),
    }, nil
}
```

#### POST /api/calendars/sync

**Current:** Triggers full sync with date range.

**New behavior:**

```
POST /api/calendars/sync
{
  "week_start": "2025-10-06",  // Optional: specific week to force-refresh
  "force": true                 // Force re-fetch even if within water marks
}
```

**Server logic:**
- If `week_start` provided and `force=true`: re-fetch that specific week
- If `week_start` provided and `force=false`: fetch if outside marks, else sync token
- If no `week_start`: incremental sync only (sync token)

### Sync Token Usage

**Current problem:** Sync tokens exist but are bypassed when date params are provided.

**New behavior:**

```go
func (h *Handler) incrementalSync(ctx context.Context, userID uuid.UUID) error {
    calendars := h.store.GetSelectedCalendars(ctx, userID)

    var wg sync.WaitGroup
    errChan := make(chan error, len(calendars))

    for _, cal := range calendars {
        wg.Add(1)
        go func(cal Calendar) {
            defer wg.Done()

            if cal.SyncToken == "" {
                // No sync token = need full sync (shouldn't happen after initial setup)
                errChan <- h.fullSyncCalendar(ctx, cal)
                return
            }

            // Use sync token for incremental update
            changes, newToken, err := h.google.FetchChanges(ctx, cal.ExternalID, cal.SyncToken)
            if err != nil {
                if isTokenExpired(err) {
                    // Token expired (410 Gone) - fall back to full sync
                    errChan <- h.fullSyncCalendar(ctx, cal)
                    return
                }
                errChan <- err
                return
            }

            // Apply changes to database
            h.applyChanges(ctx, cal.ID, changes)

            // Update sync token
            h.store.UpdateSyncToken(ctx, cal.ID, newToken)

            errChan <- nil
        }(cal)
    }

    wg.Wait()
    close(errChan)

    // Collect errors
    for err := range errChan {
        if err != nil {
            return err
        }
    }
    return nil
}
```

### Water Mark Expansion

```go
func (h *Handler) expandWaterMarks(ctx context.Context, userID uuid.UUID, targetStart, targetEnd time.Time) error {
    calendars := h.store.GetSelectedCalendars(ctx, userID)

    for _, cal := range calendars {
        // Calculate what weeks need fetching
        weeksToFetch := calculateMissingWeeks(cal.LowWaterMark, cal.HighWaterMark, targetStart, targetEnd)

        for _, week := range weeksToFetch {
            // Fetch events for this week
            events, err := h.google.FetchEvents(ctx, cal.ExternalID, week.Start, week.End)
            if err != nil {
                return err
            }

            // Upsert events
            h.store.UpsertEvents(ctx, cal.ID, events)
        }

        // Update water marks atomically
        newLow := minDate(cal.LowWaterMark, targetStart)
        newHigh := maxDate(cal.HighWaterMark, targetEnd)
        h.store.UpdateWaterMarks(ctx, cal.ID, newLow, newHigh)
    }

    return nil
}
```

### Server-Side Background Sync

The server runs a daily background job to keep calendars fresh even when users aren't active.

```go
// BackgroundSyncJob runs daily via time.Ticker in a goroutine
func (s *SyncScheduler) RunBackgroundSync(ctx context.Context) error {
    // Find calendars that haven't synced in 24h and aren't broken
    calendars, err := s.store.GetCalendarsNeedingSync(ctx, StalenessThreshold)
    if err != nil {
        return err
    }

    for _, cal := range calendars {
        // Skip if too many consecutive failures
        if cal.SyncFailureCount >= 3 {
            continue
        }

        err := s.syncCalendarWithTokenRefresh(ctx, cal)
        if err != nil {
            s.store.IncrementSyncFailureCount(ctx, cal.ID)
            log.Error("background sync failed", "calendar_id", cal.ID, "error", err)
            continue
        }

        // Reset failure count on success
        s.store.ResetSyncFailureCount(ctx, cal.ID)
        s.store.UpdateLastSyncedAt(ctx, cal.ID, time.Now())
    }

    return nil
}

// syncCalendarWithTokenRefresh handles OAuth token refresh
func (s *SyncScheduler) syncCalendarWithTokenRefresh(ctx context.Context, cal Calendar) error {
    changes, newToken, err := s.google.FetchChanges(ctx, cal.ExternalID, cal.SyncToken)

    if err != nil {
        if isAuthError(err) {
            // Try to refresh the OAuth token
            newAccessToken, refreshErr := s.google.RefreshToken(ctx, cal.Connection.RefreshToken)
            if refreshErr != nil {
                // Refresh failed - mark calendar as needing re-auth
                s.store.MarkNeedsReauth(ctx, cal.ID)
                return fmt.Errorf("token refresh failed: %w", refreshErr)
            }

            // Update stored access token and retry
            s.store.UpdateAccessToken(ctx, cal.ConnectionID, newAccessToken)
            changes, newToken, err = s.google.FetchChanges(ctx, cal.ExternalID, cal.SyncToken)
            if err != nil {
                return err
            }
        } else if isTokenExpired(err) {
            // Sync token expired (410 Gone) - need full re-sync
            return s.fullSyncCalendar(ctx, cal)
        } else {
            return err
        }
    }

    // Apply changes
    s.applyChanges(ctx, cal.ID, changes)
    s.store.UpdateSyncToken(ctx, cal.ID, newToken)

    return nil
}

// Start the background scheduler (called at server startup)
func (s *SyncScheduler) Start(ctx context.Context) {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()

    // Run immediately on startup, then daily
    s.RunBackgroundSync(ctx)

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.RunBackgroundSync(ctx)
        }
    }
}
```

**Token refresh flow:**
1. Attempt sync with stored access token
2. On 401/403, refresh using stored refresh token
3. If refresh succeeds, update stored token and retry sync
4. If refresh fails (token revoked), set `needs_reauth = true`
5. User sees "Calendar disconnected" on next login

### Week Normalization

All date operations normalize to week boundaries:

```go
// normalizeToWeekStart returns the Monday of the week containing the given date
func normalizeToWeekStart(d time.Time) time.Time {
    d = d.UTC().Truncate(24 * time.Hour)
    weekday := int(d.Weekday())
    if weekday == 0 {
        weekday = 7  // Sunday = 7
    }
    return d.AddDate(0, 0, -(weekday - 1))  // Back to Monday
}

// normalizeToWeekEnd returns the Sunday of the week containing the given date
func normalizeToWeekEnd(d time.Time) time.Time {
    monday := normalizeToWeekStart(d)
    return monday.AddDate(0, 0, 6)  // Forward to Sunday
}
```

## Frontend Changes

### State Management

```typescript
// New state for sync tracking
let syncingWeek = $state<string | null>(null);  // ISO week string, e.g., "2025-W41"
let lastSyncedAt = $state<Date | null>(null);

// Debounced navigation
let navigationDebounce: ReturnType<typeof setTimeout> | null = null;
const DEBOUNCE_MS = 250;  // Short enough to feel responsive, long enough to skip rapid clicks

// Track last refresh for multi-tab focus handling
let lastRefreshAt = $state<Date>(new Date());

function navigateToWeek(weekStart: Date) {
    // Update UI immediately
    currentDate = weekStart;

    // Cancel pending debounce
    if (navigationDebounce) {
        clearTimeout(navigationDebounce);
    }

    // Debounce API call
    navigationDebounce = setTimeout(() => {
        fetchWeekEvents(weekStart);
    }, DEBOUNCE_MS);
}
```

### Loading State Logic

```typescript
async function fetchWeekEvents(weekStart: Date, force = false) {
    const weekKey = formatISOWeek(weekStart);

    // Show loading overlay if we don't have events for this week
    const hasEvents = eventsByWeek.has(weekKey);
    if (!hasEvents) {
        syncingWeek = weekKey;
    }

    try {
        const response = await api.getEvents({
            start_date: weekStart,
            end_date: addDays(weekStart, 6),
            force
        });

        // Update events
        eventsByWeek.set(weekKey, response.events);
        lastSyncedAt = new Date(response.synced_at);

    } catch (error) {
        showToast('Failed to load calendar events', 'error', { retry: () => fetchWeekEvents(weekStart, force) });
    } finally {
        syncingWeek = null;
        lastRefreshAt = new Date();
    }
}
```

### Multi-Tab Focus Handling

Instead of real-time cross-tab sync via BroadcastChannel, we use a simpler "refresh on focus" approach that works for all entity types.

```typescript
const FOCUS_REFRESH_THRESHOLD_MS = 30_000;  // 30 seconds for events/entries
const FOCUS_REFRESH_THRESHOLD_SLOW_MS = 300_000;  // 5 minutes for projects/rules

// Set up visibility change listener
$effect(() => {
    function handleVisibilityChange() {
        if (document.visibilityState === 'visible') {
            const timeSinceRefresh = Date.now() - lastRefreshAt.getTime();

            if (timeSinceRefresh > FOCUS_REFRESH_THRESHOLD_MS) {
                // Refresh calendar events and time entries
                fetchWeekEvents(currentWeekStart);
                fetchTimeEntries(currentWeekStart);
            }

            if (timeSinceRefresh > FOCUS_REFRESH_THRESHOLD_SLOW_MS) {
                // Also refresh projects and rules (change rarely)
                fetchProjects();
                fetchRules();
            }
        }
    }

    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => document.removeEventListener('visibilitychange', handleVisibilityChange);
});
```

This approach:
- Handles multi-tab sync without complex BroadcastChannel logic
- Applies uniformly to all entity types (events, time entries, projects, rules)
- Uses appropriate refresh thresholds per entity type
- Automatically catches changes made in other tabs

### Loading Overlay Component

```svelte
<!-- In TimeGrid or week view container -->
{#if syncingWeek === currentWeekKey}
    <div
        class="absolute inset-0 z-10 flex flex-col items-center justify-center
               bg-white/80 backdrop-blur-[1px] dark:bg-zinc-900/80"
        role="status"
        aria-live="polite"
    >
        <svg class="h-5 w-5 animate-spin text-primary-500" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
        <span class="mt-2 text-sm text-gray-600 dark:text-gray-400">Loading week...</span>
        <span class="sr-only">Loading calendar events for this week</span>
    </div>
{/if}
```

## Sequence Diagrams

### Case A: Week Within Water Marks

```
User          Frontend        Backend         Google
 │               │               │               │
 │──navigate──►  │               │               │
 │               │──GET /events──►               │
 │               │               │──[check marks]│
 │               │               │  (within)     │
 │               │◄──events──────│               │
 │◄──render──────│               │               │
 │               │               │──async sync───►
 │               │               │◄──changes─────│
 │               │◄──[optional notification]─────│
```

### Case B: Week Outside Water Marks

```
User          Frontend        Backend         Google
 │               │               │               │
 │──navigate──►  │               │               │
 │               │──GET /events──►               │
 │               │               │──[check marks]│
 │               │               │  (outside)    │
 │               │               │──fetch week───►
 │               │               │◄──events──────│
 │               │◄──events──────│               │
 │◄──render──────│               │               │
 │               │               │──async expand─►
 │               │               │  water marks  │
```

### Case A': Week Within Marks But Stale

```
User          Frontend        Backend         Google
 │               │               │               │
 │──navigate──►  │               │               │
 │               │──GET /events──►               │
 │               │               │──[check marks]│
 │               │               │  (within but  │
 │               │               │   >24h stale) │
 │               │◄──events+─────│               │
 │               │  "refreshing" │               │
 │◄──render with │               │               │
 │  progress bar │               │──sync token───►
 │               │               │◄──changes─────│
 │               │◄──fresh data──│               │
 │◄──update──────│               │               │
```

### Case C: Manual Sync (Force)

```
User          Frontend        Backend         Google
 │               │               │               │
 │──click sync──►│               │               │
 │               │──POST /sync───►               │
 │               │  (force=true) │               │
 │               │               │──fetch week───►
 │               │               │◄──events──────│
 │               │               │──sync token───►
 │               │               │◄──changes─────│
 │               │◄──complete────│               │
 │◄──update──────│               │               │
```

### Server-Side Background Sync

```
                    Server          Google
                      │               │
   [daily ticker]────►│               │
                      │──query stale──┤
                      │  calendars    │
                      │               │
                      │──for each:────┤
                      │  sync token───►
                      │◄──changes─────│
                      │──update DB────┤
                      │               │
                      │──[on auth     │
                      │   error]:─────┤
                      │  refresh──────►
                      │◄──new token───│
                      │  retry sync───►
                      │◄──changes─────│
                      │               │
```

## Migration Plan

### Phase 1: Database Changes
1. Add `low_water_mark` and `high_water_mark` columns to `calendars` table
2. Add `last_synced_at`, `sync_failure_count`, `needs_reauth` columns
3. Backfill existing calendars: set marks based on existing synced events
4. Add indexes for water mark and background sync queries

### Phase 2: Backend Changes
1. Implement week normalization functions
2. Update `GET /api/events` to check water marks and staleness (Case A, A', B)
3. Update `POST /api/calendars/sync` to support force flag
4. Implement water mark expansion logic
5. Ensure sync token is used for incremental updates
6. Add token refresh handling with `needs_reauth` flow
7. Implement background sync scheduler (goroutine with daily ticker)

### Phase 3: Frontend Changes
1. Add debounced navigation (250ms)
2. Add loading overlay for unsynced weeks
3. Add progress bar for stale data refresh (Case A')
4. Update sync button to pass force flag
5. Update header sync status display
6. Add visibility change listener for multi-tab refresh
7. Add offline detection and indicator

### Phase 4: Testing
1. Unit tests for week normalization
2. Unit tests for staleness detection
3. Integration tests for water mark expansion
4. Integration tests for token refresh flow
5. Integration tests for background sync
6. E2E tests for navigation and sync flows
7. E2E tests for multi-tab focus refresh

## Performance Considerations

| Operation | Target | Notes |
|-----------|--------|-------|
| Incremental sync (sync token) | <500ms | No date range, just changes |
| On-demand week fetch | <2s | Single week, ~50 events max |
| Water mark expansion | <10s per week | Background, non-blocking |
| Initial sync (-4w to +1w) | <5s | 5 weeks, ~250 events |
| Background sync (-52w to +5w) | <60s | Can run async |

## Error Handling

| Error | Recovery |
|-------|----------|
| Sync token expired (410) | Full re-sync for affected calendar, get new token |
| Google API rate limit (429) | Exponential backoff, max 3 retries |
| Network timeout | Retry once, then show error toast |
| Database error | Log, show generic error, don't corrupt water marks |
| OAuth access token expired (401) | Refresh using stored refresh token, retry sync |
| OAuth refresh token revoked | Set `needs_reauth = true`, show "reconnect calendar" prompt |
| 3 consecutive sync failures | Stop background sync for calendar until manual retry |
| User offline | Skip sync, show "Offline - cached data" indicator |
| Navigation during sync | Cancel in-flight request via AbortController |

## Monitoring

Track these metrics:
- `sync_duration_ms` by type (incremental, on_demand, expansion, background)
- `sync_events_count` per sync operation
- `water_mark_expansion_triggered` count
- `sync_token_expired` count
- `sync_errors` by type
- `background_sync_calendars_processed` per run
- `oauth_token_refresh_count` (success/failure)
- `calendars_needing_reauth` count
- `stale_data_served` count (Case A' triggered)

## Security Considerations

- Water marks are per-calendar, per-user - no cross-user data access
- Force flag doesn't bypass auth, just skips water mark check
- Sync token is stored encrypted (existing behavior)

## Key Architectural Principles

### 1. Server Owns Sync Complexity

The client requests events for a date range. It is the **server's responsibility** to ensure those events are available, whether from cache or by fetching from Google. The client should not:
- Know about water marks
- Decide when to trigger sync
- Track which date ranges have been synced

The client simply calls `GET /api/calendar-events?start_date=...&end_date=...` and receives events.

### 2. Water Marks Are Internal Server State

Water marks are an optimization mechanism for the server to efficiently sync with Google Calendar. They are never exposed to the client. The client cannot distinguish between:
- "Events fetched from cache (within water marks)"
- "Events fetched on-demand (outside water marks)"

Both cases return the same response structure.

### 3. Current vs Desired Water Marks

The server maintains two conceptual states:

| State | Description | Stored In |
|-------|-------------|-----------|
| **Current water marks** | Range of weeks where events are actually synced | `min_synced_date`, `max_synced_date` |
| **Desired water marks** | Range we want to eventually sync to | Computed from current + user navigation |

When a user navigates outside current water marks:
1. **Immediately**: Fetch the requested week, update current marks to include it (creates "island")
2. **Async**: Background job fills the gap between old marks and new island
3. **Eventually**: Current marks become contiguous

### 4. Atomic Water Mark Updates

Water mark updates must be atomic with event storage:
- Either: events stored AND water marks updated
- Or: neither (rollback on failure)

This ensures water marks accurately reflect what's in the database.

## Open Questions

1. **Concurrent water mark expansion:** If user rapidly navigates to multiple out-of-range weeks, multiple expansion jobs may run. Accept this for now, or queue them?
   - **Recommendation:** Accept for now. On-demand fetch ensures UI is responsive. Expansion jobs are idempotent.

2. **Water mark gaps:** If user jumps from week 1 to week 52, do we fill the gap?
   - **Recommendation:** Yes, expand marks to cover the full range. Background job fills weeks 2-51.

3. **Maximum historical range:** Should we cap how far back we sync?
   - **Recommendation:** No cap. On-demand works for any date. Storage is cheap.

## Appendix: SQL Queries

### Check if week is within water marks

```sql
SELECT id, low_water_mark, high_water_mark
FROM calendars
WHERE connection_id IN (
    SELECT id FROM calendar_connections WHERE user_id = $1 AND is_selected = true
)
AND (low_water_mark IS NULL
     OR high_water_mark IS NULL
     OR $2 < low_water_mark
     OR $3 > high_water_mark);
-- Returns calendars that need syncing for the given week
```

### Update water marks atomically

```sql
UPDATE calendars
SET
    low_water_mark = LEAST(COALESCE(low_water_mark, $2), $2),
    high_water_mark = GREATEST(COALESCE(high_water_mark, $3), $3),
    updated_at = NOW()
WHERE id = $1;
```
