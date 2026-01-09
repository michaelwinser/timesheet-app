# Test Plan: Calendar Sync v2

This document outlines the unit and integration tests for the calendar sync system.

## Prerequisites: Mock Infrastructure

### GoogleCalendarClient Interface

Extract an interface from `CalendarService` to enable mocking:

```go
// CalendarClient defines the interface for Google Calendar API operations
type CalendarClient interface {
    ListCalendars(ctx context.Context, creds *store.OAuthCredentials) ([]*CalendarInfo, error)
    FetchEvents(ctx context.Context, creds *store.OAuthCredentials, calendarID string, minTime, maxTime time.Time) (*SyncResult, error)
    FetchEventsIncremental(ctx context.Context, creds *store.OAuthCredentials, calendarID string, syncToken string) (*SyncResult, error)
    RefreshToken(ctx context.Context, creds *store.OAuthCredentials) (*store.OAuthCredentials, error)
}
```

### MockCalendarClient

```go
type MockCalendarClient struct {
    // Configurable responses
    EventsByRange     map[string][]*calendar.Event  // key: "calendarID:startDate:endDate"
    EventsByToken     map[string][]*calendar.Event  // key: "calendarID:syncToken"
    NextSyncTokens    map[string]string             // key: "calendarID" -> next sync token

    // Error injection
    FetchError        error
    IncrementalError  error
    RefreshError      error

    // Call tracking
    FetchCalls        []FetchCall
    IncrementalCalls  []IncrementalCall
    RefreshCalls      int
}

type FetchCall struct {
    CalendarID string
    MinTime    time.Time
    MaxTime    time.Time
}

type IncrementalCall struct {
    CalendarID string
    SyncToken  string
}
```

---

## Unit Tests: Sync Decision Logic

**File:** `service/internal/sync/week_test.go` (extend existing)

### Test: DecideSync cases

| Test Case | Input | Expected |
|-----------|-------|----------|
| Case A: Fresh data within window | minSynced=Jan6, maxSynced=Jan26, lastSync=1h ago, target=Jan13 | NeedsSync=false, Reason="fresh_data" |
| Case A': Stale data within window | minSynced=Jan6, maxSynced=Jan26, lastSync=25h ago, target=Jan13 | NeedsSync=true, Reason="stale_data", IsStaleRefresh=true |
| Case B: Week before window | minSynced=Jan13, maxSynced=Jan26, target=Jan6 | NeedsSync=true, Reason="outside_window", MissingWeeks=[Jan6] |
| Case C: Week after window | minSynced=Jan6, maxSynced=Jan19, target=Jan26 | NeedsSync=true, Reason="outside_window", MissingWeeks=[Jan26] |
| No synced range | minSynced=nil, maxSynced=nil, target=Jan6-Jan20 | NeedsSync=true, Reason="no_synced_range", MissingWeeks=[Jan6,Jan13,Jan20] |
| Far historical (6 months ago) | minSynced=Dec2025, maxSynced=Jan2026, target=Jun2025 | NeedsSync=true, MissingWeeks includes Jun2025 week only (on-demand) |

### Test: Week normalization

Already covered in existing tests.

---

## Unit Tests: Job Queue Logic

**File:** `service/internal/sync/jobs_test.go` (new)

### Test: Job coalescing

| Test Case | Pending Jobs | New Job | Expected Result |
|-----------|--------------|---------|-----------------|
| No existing jobs | [] | expand to Jun2025 | Single job: Jun2025 |
| Adjacent weeks (merge) | [expand to Jun30-Jul6] | expand to Jul7-Jul13 | Single job: Jun30-Jul13 |
| Overlapping ranges | [expand to Jun1-Jun30] | expand to Jun15-Jul15 | Single job: Jun1-Jul15 |
| Non-adjacent (no merge) | [expand to Jun2025] | expand to Sep2025 | Two jobs |
| Multiple pending (chain merge) | [Jun1-Jun7, Jun14-Jun21] | Jun7-Jun14 | Single job: Jun1-Jun21 |

### Test: Job priority

| Test Case | Jobs | Expected Execution Order |
|-----------|------|--------------------------|
| Same priority | [Job A created 10s ago, Job B created 5s ago] | Job A first (FIFO) |
| Different priority | [Job A priority=0, Job B priority=10] | Job B first |

### Test: Job claiming (concurrency)

| Test Case | Scenario | Expected |
|-----------|----------|----------|
| Single worker | One pending job | Job claimed and executed |
| Multiple workers | One pending job | Exactly one worker claims it |
| Job already running | Worker tries to claim | Gets different job or waits |

---

## Integration Tests: Event Fetch Flow

**File:** `service/internal/handler/calendars_test.go` (new or extend)

### Test: GET /api/calendar-events behavior

| Test Case | Water Marks | Request | Mock Setup | Expected |
|-----------|-------------|---------|------------|----------|
| Case A: Within marks, fresh | min=Jan6, max=Jan26, lastSync=1h ago | Jan13-Jan19 | DB has 5 events | Return 5 events, no Google API call |
| Case A': Within marks, stale | min=Jan6, max=Jan26, lastSync=25h ago | Jan13-Jan19 | Mock returns 2 changed events | Return merged events, incremental sync called |
| Case B: Outside marks (before) | min=Jan13, max=Jan26 | Jan6-Jan12 | Mock returns 3 events | Return 3 events, Google FetchEvents called, job queued |
| Case C: Outside marks (after) | min=Jan6, max=Jan19 | Jan26-Feb1 | Mock returns 2 events | Return 2 events, Google FetchEvents called, job queued |
| Island fetch (far historical) | min=Dec2025, max=Jan2026 | Jun2025 | Mock returns 4 events | Return 4 events, only Jun week fetched, job queued for gap |

### Test: Sync token handling

| Test Case | Scenario | Expected |
|-----------|----------|----------|
| Valid sync token | Incremental sync succeeds | Events updated, new token stored |
| Expired sync token (410) | Incremental returns 410 Gone | Fall back to full fetch, new token obtained |
| Missing sync token | Calendar has no sync_token | Full fetch for date range |

### Test: OAuth token handling

| Test Case | Scenario | Expected |
|-----------|----------|----------|
| Valid access token | API call succeeds | Normal operation |
| Expired access token, valid refresh | API returns 401, refresh succeeds | Retry with new token |
| Expired access token, invalid refresh | API returns 401, refresh fails | Mark needs_reauth=true |
| Revoked token | Refresh returns invalid_grant | Mark needs_reauth=true |

---

## Integration Tests: Background Job Worker

**File:** `service/internal/sync/worker_test.go` (new)

### Test: Job execution

| Test Case | Job | Mock Setup | Expected |
|-----------|-----|------------|----------|
| Successful expansion | expand Jun-Aug | Mock returns events for each week | Events stored, water marks updated, job completed |
| Partial failure | expand Jun-Aug, Aug fetch fails | Jun/Jul succeed, Aug fails | Jun/Jul stored, job marked failed with error |
| Empty result | expand Jun2020 | No events in range | Water marks still updated, job completed |

### Test: Atomicity

| Test Case | Scenario | Expected |
|-----------|----------|----------|
| DB failure during upsert | Events fetched, DB write fails | Transaction rolled back, water marks unchanged, job retry |
| Success | Events fetched and stored | Water marks updated in same transaction |

### Test: Failure counting

| Test Case | Scenario | Expected |
|-----------|----------|----------|
| First failure | Job fails | sync_failure_count = 1 |
| Third consecutive failure | sync_failure_count reaches 3 | Calendar excluded from background sync |
| Success after failures | Job succeeds | sync_failure_count reset to 0 |

---

## Edge Case Tests

**File:** `service/internal/handler/calendars_edge_test.go` (new)

### Test: Concurrent navigation

| Test Case | Scenario | Expected |
|-----------|----------|----------|
| Rapid navigation | 3 requests in 100ms: Jan, Feb, Mar | Only final request (Mar) fully processed |
| Request during active fetch | Request A in progress, Request B arrives for different week | Request A context cancelled, Request B proceeds |

### Test: Rate limiting

| Test Case | Scenario | Expected |
|-----------|----------|----------|
| Google returns 429 | First fetch fails with rate limit | Retry with exponential backoff |
| 3 consecutive 429s | All retries fail | Return error to client |

### Test: Calendar removal

| Test Case | Scenario | Expected |
|-----------|----------|----------|
| Calendar deleted from Google | Sync finds calendar gone | Events marked orphaned, connection marked needs_reauth |
| User disconnects calendar | DELETE /api/calendars/:id | Events remain but become orphaned |

---

## Test Data Fixtures

### Standard test calendar

```go
var testCalendar = &store.Calendar{
    ID:            uuid.MustParse("..."),
    ConnectionID:  uuid.MustParse("..."),
    ExternalID:    "primary",
    Name:          "Test Calendar",
    MinSyncedDate: timePtr(time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)),
    MaxSyncedDate: timePtr(time.Date(2025, 1, 26, 0, 0, 0, 0, time.UTC)),
    LastSyncedAt:  timePtr(time.Now().Add(-1 * time.Hour)),
    SyncToken:     stringPtr("test-sync-token"),
}
```

### Standard test events

```go
var testEvents = []*calendar.Event{
    {
        Id:      "event1",
        Summary: "Team Meeting",
        Start:   &calendar.EventDateTime{DateTime: "2025-01-15T09:00:00Z"},
        End:     &calendar.EventDateTime{DateTime: "2025-01-15T10:00:00Z"},
    },
    // ... more events
}
```

---

## Implementation Order

1. **Create CalendarClient interface** - Extract from existing CalendarService
2. **Create MockCalendarClient** - Implement interface with configurable responses
3. **Add unit tests for sync decision logic** - Extend week_test.go
4. **Add unit tests for job coalescing** - New jobs_test.go
5. **Add integration tests for event fetch** - New calendars_test.go
6. **Add integration tests for job worker** - New worker_test.go
7. **Add edge case tests** - New calendars_edge_test.go

## Success Criteria

- All PRD edge cases have corresponding tests
- Code coverage > 80% for sync-related packages
- Tests run in < 30 seconds (no real API calls)
- Tests are deterministic (no time.Now() dependencies without mocking)
