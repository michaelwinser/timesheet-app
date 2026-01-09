# PRD: Calendar Sync Architecture v2

**Status:** Draft (Rev 5)
**Author:** Claude (with Michael)
**Date:** 2026-01-08
**Replaces:** Portions of `prd-sync-behaviour.md`

## Problem Statement

Current calendar sync has poor UX:
- Manual sync takes ~10 seconds and reports "5700 events updated" even for incremental refreshes
- Users don't know if their data is current
- Navigating to historical dates may show no data even though events exist
- No distinction between "week has no events" and "week not yet synced"

## Goals

1. **Fast incremental sync** - Manual "Sync" button completes in <1 second for typical use
2. **Responsive navigation** - User can navigate to any week and see data within 2-3 seconds
3. **Transparent sync state** - User always knows if data is loading or current
4. **Background efficiency** - Historical data loads without blocking user interaction

## Non-Goals

- Per-calendar sync progress indicators
- Manual control over sync date ranges (Settings page, future)
- Real-time push updates from Google Calendar

## User Stories

### Daily Review (Primary)
> As a consultant, I open the app Monday morning to review last week's events and fill out timesheets. I expect my calendar data to be current within seconds.

### Historical Lookup
> As a consultant, I need to check my calendar from 3 months ago to verify a timesheet entry. I expect the data to load quickly even though I haven't viewed that week before.

### New Calendar Setup
> As a new user, I connect my Google Calendar and expect to see my recent events immediately, not wait minutes for a full historical sync.

### Returning After Absence
> As a user who hasn't opened the app in a week, I log in and view this week. Because the server synced my calendar daily in the background, my data is already fresh - I see current events immediately without waiting for a sync.

### Interrupted Navigation
> As a user, I navigate to a historical week. While it's loading, I change my mind and navigate to a different week. I expect the app to cancel the first request and load my new destination quickly.

### Multi-Tab Usage
> As a user with the app open in two browser tabs, I classify an event in Tab A. When I switch to Tab B, the app automatically refreshes and I see the updated classification within a few seconds.

### Offline Graceful Degradation
> As a user on flaky wifi, when I lose connection I expect to see my cached calendar data with a clear "Offline" indicator, not a broken loading state.

## Functional Requirements

### 1. Water Mark System

The system maintains per-calendar "water marks" - the range of weeks for which we have fetched events.

| Property | Value | Notes |
|----------|-------|-------|
| Low water mark | Earliest Monday we've synced | Week-aligned |
| High water mark | Latest Sunday we've synced | Week-aligned |
| Initial window | -4 weeks to +1 week | Fast startup |
| Background target | -52 weeks to +5 weeks | Covers typical use |
| Staleness threshold | 24 hours | Data older than this triggers refresh |

**Week definition:** Monday 00:00:00 UTC to Sunday 23:59:59 UTC

**Timezone handling:** Events are stored with their original timezone. Week boundaries for water marks use UTC. An event at "Sunday 11pm PST" belongs to the week containing that UTC timestamp.

### 2. API Behavior: Get Events for Week

When client requests events for a specific week:

**Case A: Week is within water marks AND fresh (synced within 24h)**
1. Return cached events immediately
2. Trigger async incremental sync (sync token)
3. If sync finds changes to visible events, notify client

**Case A': Week is within water marks BUT stale (synced >24h ago)**
1. Return cached events immediately with "stale" indicator
2. Trigger synchronous incremental sync (sync token)
3. Return fresh data when sync completes
4. UI shows subtle "Refreshing..." indicator (not blocking overlay)

**Case B: Week is outside water marks**
1. Synchronously fetch that week from Google Calendar
2. Store events and return to client
3. Trigger async water mark expansion to include that week

**Case C: Manual sync (force flag)**
1. Re-fetch events for the **currently viewed week only**
2. Overwrite existing records
3. Apply incremental sync (sync token) for any other changes
4. Return fresh events to client

### 3. Automatic Sync Triggers

| Trigger | Action |
|---------|--------|
| User navigates to different week | Fetch/sync that week per Case A/A'/B above |
| User clicks "Sync" button | Case C (force re-fetch current week + sync token for changes) |
| App becomes visible (focus) | Incremental sync (sync token only) |
| App active for 5+ minutes | Incremental sync (sync token only) |
| New calendar connected | Background sync to current water marks |

**Periodic sync (client):** While the app is active, trigger incremental sync every 5 minutes. This ensures data stays fresh even if user doesn't navigate.

### 3a. Server-Side Background Sync

The server runs scheduled background syncs to keep calendar data fresh even when the user isn't actively using the app. This eliminates the "stale data on return" problem.

**Trigger:** Server-side scheduler runs daily (configurable).

**Behavior:**
1. Query all calendars that haven't synced in the last 24 hours
2. For each calendar, perform incremental sync using stored sync token
3. Update `last_synced_at` timestamp on success
4. On token expiry (Google 410), attempt token refresh; if refresh fails, mark calendar as needing re-auth

**Token refresh handling:**
- Google OAuth refresh tokens are long-lived
- On 401/403 errors, attempt to refresh the access token using the stored refresh token
- If refresh succeeds, retry the sync
- If refresh fails (token revoked, account deleted), mark the calendar connection as "needs_reauth"
- User sees "Calendar disconnected - please reconnect" on next login

**Implementation:** Single goroutine with `time.Ticker` running in the Go server. Suitable for self-hosted/singleton deployments.

**Error handling:**
- Log failures but continue with other calendars
- After 3 consecutive failures for a calendar, stop retrying until user manually triggers sync
- Send notification (future: email) if calendar stays broken for >7 days

### 4. Loading States

**Week outside water marks (loading):**
```
┌──────────────────────────────────────────────────┐
│  Week of October 6, 2025                         │
│                                                  │
│  [backdrop blur overlay]                         │
│            ⟳ Loading week...                    │
│                                                  │
└──────────────────────────────────────────────────┘
```

**Week within water marks, fresh (background syncing):**
- Header shows "Syncing..." with spinner
- Events remain visible and interactive
- No overlay, no blocking

**Week within water marks, stale (refreshing):**
- Thin progress bar at top of calendar area (non-blocking)
- Events remain visible and interactive
- Header shows "Refreshing..."

**Sync complete with visible changes:**
- Toast: "3 events updated" (only if changes affect currently visible week)
- Events update in place

**Sync complete, no changes:**
- Header shows "X min ago" timestamp
- No toast (avoid notification fatigue)

**Sync failed:**
- Toast: "Failed to sync calendar. [Retry]"
- Cached data remains visible
- Header shows stale timestamp with warning icon

**Offline:**
- Detect via `navigator.onLine`
- Skip sync attempts
- Show indicator: "Offline - showing cached data"
- Resume sync when connection restored

### 5. Navigation Debounce

When user navigates rapidly (pressing j/k repeatedly):
- Navigation is instant (week changes immediately)
- API call is debounced by **250ms**
- No loading state shown during debounce
- Only final destination week triggers fetch
- If previous fetch is in-flight, cancel it

### 6. Multi-Calendar Handling

**Sync behavior:**
- All calendars sync in parallel
- Wait for all calendars to complete before updating UI
- User sees all events or no events for a week (no partial updates)

**Per-calendar failure handling:**
- If one calendar fails, show events from successful calendars
- Show error indicator for failed calendar: "Work calendar failed to sync [Retry]"
- Don't block entire UI for single calendar failure

**Progress display:**
- Single "Syncing..." indicator (not per-calendar)
- Calendars sync internally in parallel

### 7. New Calendar Connection

**First calendar (onboarding):**
1. Blocking foreground sync of initial window (-4w to +1w)
2. Show progress: "Syncing calendar..."
3. User sees events when complete
4. Background job expands to full range (-52w to +5w)

**Additional calendars:**
1. Background sync to current water marks
2. Toast when complete: "Calendar synced"
3. Events appear in current view automatically

### 8. Data Preservation During Sync

**Classification persistence:** When syncing updates from Google Calendar:
- Preserve local classification (project assignment)
- Preserve "Did Not Attend" flag
- Preserve any user-edited fields

**Exception:** If an event is deleted in Google Calendar (sync returns deletion), the local event and its classification are removed.

**Conflict resolution:** Google Calendar is the source of truth for:
- Event existence (created/deleted)
- Event details (title, time, attendees)

Local state is authoritative for:
- Classification (project assignment)
- Did Not Attend flag
- Time entry linkage

### 9. Multi-Tab Synchronization

**Approach:** Refresh on focus (simple, covers all entity types uniformly).

**When tab gains focus:**
1. Check if last refresh was >30 seconds ago
2. If stale, trigger incremental calendar sync
3. Re-fetch current week's time entries
4. Projects and rules change rarely - refresh only if >5 minutes stale

**Example scenario:**
1. User classifies event in Tab A
2. User switches to Tab B
3. Tab B gains focus → triggers refresh → shows updated classification

This approach is simpler than real-time cross-tab sync and applies uniformly to all entity types (events, time entries, projects, rules).

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| User navigates during active sync | Cancel current sync, start new for destination week |
| Sync token expires (Google 410) | Fall back to full fetch for affected week, get new token |
| User navigates far into past (>52w) | Fetch that week on-demand, expand water marks |
| Calendar removed | Orphan events with warning indicator |
| Google API rate limited | Retry with exponential backoff, show error after 3 attempts |
| Network timeout | Retry once, then show error toast with retry button |
| User offline | Show cached data with "Offline" indicator, skip sync |
| Multiple rapid navigations | Debounce 250ms, cancel in-flight requests |
| Background sync running, user navigates outside marks | Pause background, prioritize on-demand fetch, resume background |
| Returning user (data >24h stale) | Case A' applies: show cached + synchronous refresh |
| OAuth token revoked/expired | Mark calendar as "needs_reauth", prompt user to reconnect |
| Server-side sync fails 3 times | Stop retrying until user manually triggers sync |

## Success Metrics

| Metric | Target |
|--------|--------|
| Manual sync time (within water marks) | <1 second |
| On-demand week fetch (outside water marks) | <3 seconds |
| Initial calendar setup (first 5 weeks) | <5 seconds |
| Background sync completion | <60 seconds |
| Staleness refresh (Case A') | <2 seconds |
| Returning user load time (with background sync) | <1 second |
| Calendar data freshness on return | <24 hours old |

**Qualitative success criteria:**
- User can answer "Is my calendar up to date?" by glancing at header
- User understands when a sync is in progress vs. complete
- User knows how to force a refresh if something looks wrong

## Phase 2 Enhancements

The following are planned enhancements after the core sync architecture is stable:

### Background Sync Improvements

| Enhancement | Description | Priority |
|-------------|-------------|----------|
| **Weekly sync cadence** | Reduce server-side sync from daily to weekly for lower API usage | Medium |
| **Rate limiting** | Spread syncs over time to avoid Google API quota issues at scale | Medium |
| **Dormant account archiving** | Stop syncing accounts inactive for 1 year; archive data | Low |
| **Multi-instance coordination** | Database job queue (see below) | Low |

**Multi-instance coordination approach:**

When horizontal scaling is needed, replace the single-goroutine scheduler with a database-backed job queue:

1. External cron job inserts a single "daily_sync" row into `sync_jobs` table
2. Server instances compete for the job using `SELECT ... FOR UPDATE SKIP LOCKED`
3. Only one instance wins the lock and processes the job
4. Standard PostgreSQL transaction semantics - no distributed coordination needed

```sql
CREATE TABLE sync_jobs (
    id SERIAL PRIMARY KEY,
    job_type TEXT NOT NULL,
    status TEXT DEFAULT 'pending',  -- pending, running, completed, failed
    created_at TIMESTAMPTZ DEFAULT NOW(),
    claimed_at TIMESTAMPTZ,
    claimed_by TEXT  -- instance identifier for debugging
);
```

This is the preferred approach because it uses existing PostgreSQL infrastructure with no additional dependencies.

### User-Facing Features

| Enhancement | Description | Priority |
|-------------|-------------|----------|
| **Real-time multi-tab sync** | Use BroadcastChannel API for instant cross-tab updates | Medium |
| **Settings for sync range** | Let power users configure water mark targets | Low |
| **Bulk re-sync** | "Re-sync all data" button in settings | Low |
| **Selective calendar sync** | Sync only specific calendars on demand | Low |

## Out of Scope

1. **Full offline mode** - Complete offline-first architecture with service workers
2. **Real-time updates** - Push notifications from Google Calendar

## Decisions

| Question | Decision | Rationale |
|----------|----------|-----------|
| Water marks per-calendar or per-user? | Per-calendar | Resilience: one failing calendar doesn't block others |
| Maximum historical range? | No limit | On-demand fetch works for any date; storage is cheap |
| Sync button scope? | Current week only | Fast, focused; background handles the rest |
| Debounce timing? | 250ms | Responsive for 2-3 week jumps; prevents API spam |
| Staleness threshold? | 24 hours | Balance between freshness and API efficiency |
| Server-side sync frequency? | Daily | Ensures data is never more than 24h stale on login |
| Server-side sync implementation? | Single goroutine | Simple; self-hosted/singleton is primary use case |

## Appendix: Glossary

| Term | Definition |
|------|------------|
| Water marks | The date range (low/high) for which we have fetched events |
| Sync token | Google Calendar API token for incremental updates |
| Incremental sync | Using sync token to fetch only changed events |
| Full fetch | Fetching all events in a date range (ignoring sync token) |
| On-demand fetch | Synchronous fetch triggered by user navigation |
| Stale | Data synced more than 24 hours ago |
| Fresh | Data synced within the last 24 hours |
