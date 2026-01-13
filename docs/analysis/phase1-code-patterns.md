# Phase 1: Code Patterns Analysis

This document analyzes the coding conventions, patterns, and practices used across the timesheet application codebase. The application consists of a Go backend (`service/`) and a SvelteKit frontend (`web/`).

## 1. Executive Summary

The codebase demonstrates a mature, well-organized structure with clear separation of concerns. Key patterns include:

- **Go Backend**: Clean architecture with handler/store separation, table-driven tests, interface-based mocking, and sentinel errors for business logic
- **Svelte Frontend**: Svelte 5 runes (`$state`, `$derived`, `$props`), typed Props interfaces, centralized styling system, and store-ID derivation pattern for popups
- **State Management**: The critical "store IDs, derive objects" pattern is documented and enforced for popup/detail views
- **Error Handling**: Consistent patterns on both frontend (API client with typed errors) and backend (sentinel errors, wrapped errors)
- **Testing**: Table-driven tests in Go, mock implementations for external services

---

## 2. Go Conventions

### 2.1 Package Organization

The backend follows a clean architecture pattern:

```
service/internal/
├── api/           # Generated OpenAPI types (api.gen.go)
├── handler/       # HTTP handlers (per-domain files)
├── store/         # Database access (per-table files)
├── classification/# Domain logic for event classification
├── sync/          # Calendar sync orchestration
├── google/        # External service integration
├── database/      # Connection pool and migrations
├── crypto/        # Encryption utilities
├── timeentry/     # Time entry business logic
├── mcp/           # MCP tool definitions
└── analyzer/      # Static analysis utilities
```

**Pattern**: Each domain has its own file in `handler/` and `store/`. Domain-specific business logic lives in dedicated packages (e.g., `classification/`, `sync/`).

### 2.2 Naming Conventions

**Files**: Lowercase with underscores for multi-word names (`time_entries.go`, `calendar_events.go`).

**Types**: PascalCase, descriptive nouns:
- Stores: `ProjectStore`, `UserStore`, `CalendarEventStore`
- Handlers: `ProjectHandler`, `AuthHandler`, `CalendarHandler`
- Domain types: `Rule`, `Target`, `Item`, `Result`, `Vote`

**Functions**:
- Constructors: `New<Type>` (e.g., `NewProjectStore`, `NewMockCalendarClient`)
- CRUD operations: `Create`, `GetByID`, `List`, `Update`, `Delete`
- Conversions: `<type>ToAPI` (e.g., `projectToAPI`)

**Constants**: PascalCase for exported, often grouped:
```go
const (
    ConfidenceFloor   = 0.4
    ConfidenceCeiling = 0.6
    TargetDNA         = "DNA"
)
```

### 2.3 Error Handling

**Sentinel Errors**: Package-level errors for business logic:
```go
var (
    ErrProjectNotFound    = errors.New("project not found")
    ErrProjectHasEntries  = errors.New("project has time entries")
    ErrDuplicateShortCode = errors.New("short code already in use")
)
```

**Error Wrapping**: Uses `fmt.Errorf` with `%w` for context:
```go
if err != nil {
    return nil, fmt.Errorf("failed to create connection pool: %w", err)
}
```

**Error Checking Pattern**:
```go
if errors.Is(err, pgx.ErrNoRows) {
    return nil, ErrProjectNotFound
}
```

**Handler Error Responses**: Type-safe API response types:
```go
if errors.Is(err, store.ErrProjectNotFound) {
    return api.GetProject404JSONResponse{
        Code:    "not_found",
        Message: "Project not found",
    }, nil
}
```

### 2.4 Struct Organization

**Store Pattern**: Wrap connection pool, single constructor:
```go
type ProjectStore struct {
    pool *pgxpool.Pool
}

func NewProjectStore(pool *pgxpool.Pool) *ProjectStore {
    return &ProjectStore{pool: pool}
}
```

**Handler Pattern**: Embed stores as dependencies:
```go
type ProjectHandler struct {
    projects *store.ProjectStore
}

func NewProjectHandler(projects *store.ProjectStore) *ProjectHandler {
    return &ProjectHandler{projects: projects}
}
```

**Server Composition**: Embed all handlers:
```go
type Server struct {
    *AuthHandler
    *ProjectHandler
    *TimeEntryHandler
    // ...
}

var _ api.StrictServerInterface = (*Server)(nil)
```

### 2.5 Interface Usage

**External Service Interfaces**: Define at point of use:
```go
// CalendarClient defines the interface for calendar service operations
type CalendarClient interface {
    GetAuthURL(state string) string
    ExchangeCode(ctx context.Context, code string) (*store.OAuthCredentials, error)
    RefreshToken(ctx context.Context, creds *store.OAuthCredentials) (*store.OAuthCredentials, error)
    ListCalendars(ctx context.Context, creds *store.OAuthCredentials) ([]*CalendarInfo, error)
    FetchEvents(ctx context.Context, creds *store.OAuthCredentials, calendarID string, minTime, maxTime time.Time) (*SyncResult, error)
    FetchEventsIncremental(ctx context.Context, creds *store.OAuthCredentials, calendarID string, syncToken string) (*SyncResult, error)
}
```

**Interface Compliance Check**:
```go
var _ CalendarClient = (*MockCalendarClient)(nil)
```

### 2.6 Testing Patterns

**Table-Driven Tests** are the standard pattern:
```go
func TestNormalizeToWeekStart(t *testing.T) {
    tests := []struct {
        name     string
        input    time.Time
        expected time.Time
    }{
        {
            name:     "Monday stays Monday",
            input:    time.Date(2025, 1, 6, 10, 30, 0, 0, time.UTC),
            expected: time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC),
        },
        // ...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := NormalizeToWeekStart(tt.input)
            if !result.Equal(tt.expected) {
                t.Errorf("NormalizeToWeekStart(%v) = %v, want %v", tt.input, result, tt.expected)
            }
        })
    }
}
```

**Mock Pattern**: Struct with tracked calls and configurable responses:
```go
type MockCalendarClient struct {
    mu sync.Mutex

    // Return values
    AuthURL             string
    ExchangeCredentials *store.OAuthCredentials
    ExchangeError       error

    // Call tracking
    ExchangeCalls []string
    RefreshCalls  int
    FetchCalls    []FetchCall
}
```

**Helper Functions**: Utility functions for test setup:
```go
func timePtr(t time.Time) *time.Time {
    return &t
}

func ptr(f float64) *float64 {
    return &f
}
```

### 2.7 Database Migrations

Migrations are defined **inline in Go code**, not separate SQL files:
```go
var migrations = []migration{
    {
        version: 1,
        sql: `
            CREATE TABLE users (
                id UUID PRIMARY KEY,
                email TEXT NOT NULL UNIQUE,
                -- ...
            );
        `,
    },
    {
        version: 2,
        sql: `
            ALTER TABLE calendars ADD COLUMN sync_failure_count INT NOT NULL DEFAULT 0;
        `,
    },
}
```

**Key Convention**: Use `IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS` for idempotent migrations.

---

## 3. Svelte/TypeScript Conventions

### 3.1 Component Organization

**Directory Structure**:
```
web/src/lib/components/
├── primitives/     # Reusable UI building blocks (Button, Modal, Input, Toast)
├── widgets/        # Domain-specific components (EventPopup, TimeGrid, ProjectChip)
└── AppShell.svelte # Layout wrapper
```

**Barrel Exports**: Components exported through `index.ts`:
```typescript
// lib/components/widgets/index.ts
export { default as ProjectChip } from './ProjectChip.svelte';
export { default as TimeEntryCard } from './TimeEntryCard.svelte';
```

### 3.2 Props Pattern

**Typed Props Interface** - always define explicitly:
```svelte
<script lang="ts">
    import type { CalendarEvent, Project } from '$lib/api/types';

    interface Props {
        event: CalendarEvent;
        projects: Project[];
        anchorElement: HTMLElement | null;
        onclassify?: (projectId: string) => void;
        onskip?: () => void;
        onunskip?: () => void;
        onexplain?: () => void;
        onmouseenter?: () => void;
        onmouseleave?: () => void;
    }

    let { event, projects, anchorElement, onclassify, onskip, onunskip, onexplain, onmouseenter, onmouseleave }: Props =
        $props();
</script>
```

**Event Handlers**: Lowercase with `on` prefix in props:
- `onclassify`, `onskip`, `onhover`, `onclose`
- Called with `onclassify?.()` pattern for optional callbacks

### 3.3 State Management with Svelte 5 Runes

**$state** for component-local mutable state:
```svelte
let loading = $state(true);
let currentDate = $state(getDateFromUrl());
let hoveredEventId = $state<string | null>(null);
```

**$derived** for computed values:
```svelte
const activeProjects = $derived(
    projects.filter((p) => !p.is_archived && !p.is_hidden_by_default)
);

// Complex derivations use $derived.by
const projectTotals = $derived.by(() => {
    const totals: Record<string, { project: Project; hours: number }> = {};
    for (const entry of entries) {
        // computation...
    }
    return Object.values(totals).sort((a, b) => a.project.name.localeCompare(b.project.name));
});
```

**$props** for component properties (replaces `export let`):
```svelte
let { event, projects, variant = 'card' }: Props = $props();
```

**$effect** used sparingly for side effects:
```svelte
$effect(() => {
    const _trigger = scrollTrigger;
    if (weekScrollContainer && scopeMode !== 'day' && displayMode === 'calendar') {
        requestAnimationFrame(() => scrollToFirstEvent());
    }
});
```

### 3.4 Template Patterns

**{@const}** for block-scoped computed values:
```svelte
{#each visibleDays as day}
    {@const dateStr = formatDate(day)}
    {@const dayEvents = eventsByDate[dateStr] || []}
    {@const styles = getClassificationStyles({ status, needsReview, isSkipped, projectColor })}
    <!-- use computed values -->
{/each}
```

**Conditional Classes**: Template literal with ternary:
```svelte
<div class="rounded-lg {isToday ? 'bg-zinc-100' : ''} {isBestGuess ? 'ring-1 ring-black/40' : ''}">
```

### 3.5 CSS/Tailwind Approach

**Inline Tailwind** for simple styling:
```svelte
<div class="flex items-center gap-2 p-4 text-sm rounded-lg">
```

**Style System** for complex, state-dependent styling:
```typescript
// lib/styles/classification.ts
export function getClassificationStyles(state: ClassificationState): ClassificationStyles {
    // Pure function computing all styles from state
}
```

**Theme-Aware Colors** via CSS custom properties (defined in `app.css`):
```css
:root {
    --color-surface: 255 255 255;
}
.dark {
    --color-surface: 24 24 27;
}
```

**Dynamic Colors from Data**: Inline styles when color comes from API:
```svelte
<span style="background-color: {project.color}"></span>
```

### 3.6 API Client Pattern

**Singleton Client** with typed methods:
```typescript
class ApiClient {
    private token: string | null = null;

    private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
        // ... fetch implementation
    }

    async listProjects(includeArchived = false): Promise<Project[]> {
        const query = includeArchived ? '?include_archived=true' : '';
        return this.request('GET', `/projects${query}`);
    }

    async createProject(data: ProjectCreate): Promise<Project> {
        return this.request('POST', '/projects', data);
    }
}

export const api = new ApiClient();
```

**Custom Error Class**:
```typescript
export class ApiClientError extends Error {
    constructor(
        public status: number,
        public error: ApiError
    ) {
        super(error.message);
        this.name = 'ApiClientError';
    }
}
```

---

## 4. State Management - The ID/Derivation Pattern

This is the most critical pattern in the UI codebase, documented in both `CLAUDE.md` and `docs/ui-coding-guidelines.md`.

### 4.1 The Problem

When displaying items from arrays in popups/detail views, storing object copies creates stale data when the source array updates:

```svelte
// ANTI-PATTERN: Creates stale copy
let hoveredEvent = $state<CalendarEvent | null>(null);
hoveredEvent = eventFromArray; // This is a snapshot that won't update
```

### 4.2 The Solution

Store only the ID, derive the object from the source array:

```svelte
// CORRECT: ID in state, object derived
let hoveredEventId = $state<string | null>(null);
let hoveredElement = $state<HTMLElement | null>(null);

const hoveredEvent = $derived(
    hoveredEventId ? calendarEvents.find((e) => e.id === hoveredEventId) ?? null : null
);
```

### 4.3 Pattern in Action (from `+page.svelte`)

```svelte
// Event popup - store ID, derive event
let hoveredEventId = $state<string | null>(null);
let hoveredElement = $state<HTMLElement | null>(null);
const hoveredEvent = $derived(
    hoveredEventId ? calendarEvents.find((e) => e.id === hoveredEventId) ?? null : null
);

// Time entry popup - same pattern
let selectedEntryId = $state<string | null>(null);
let selectedEntryAnchor = $state<{ x: number; y: number } | null>(null);
const selectedEntry = $derived(
    selectedEntryId ? entries.find((e) => e.id === selectedEntryId) ?? null : null
);
```

### 4.4 When This Matters

- User actions update the source array (classify event, edit entry)
- Displayed object can change while visible (popups, sidebars)
- Multiple views show the same data

---

## 5. Error Handling

### 5.1 Backend Error Flow

1. **Store Layer**: Return sentinel errors for business conditions
   ```go
   if errors.Is(err, pgx.ErrNoRows) {
       return nil, ErrProjectNotFound
   }
   ```

2. **Handler Layer**: Map to typed API responses
   ```go
   if errors.Is(err, store.ErrProjectNotFound) {
       return api.GetProject404JSONResponse{
           Code:    "not_found",
           Message: "Project not found",
       }, nil
   }
   return nil, err  // Unhandled errors become 500
   ```

### 5.2 Frontend Error Handling

**API Client**: Throws typed errors
```typescript
if (!response.ok) {
    const error: ApiError = await response.json().catch(() => ({
        code: 'unknown',
        message: response.statusText
    }));
    throw new ApiClientError(response.status, error);
}
```

**Component Level**: Try/catch with console.error
```typescript
async function handleClassify(eventId: string, projectId: string) {
    classifyingId = eventId;
    try {
        const result = await api.classifyCalendarEvent(eventId, { project_id: projectId });
        // update state...
    } catch (e) {
        console.error('Failed to classify event:', e);
    } finally {
        classifyingId = null;
    }
}
```

**Toast Notifications**: For user-facing feedback
```typescript
try {
    await api.syncCalendar(conn.id);
    toastContainer?.success(`Sync complete: ${parts.join(', ')} events`);
} catch (e) {
    toastContainer?.error('Sync failed. Please try again.');
}
```

---

## 6. Testing Patterns

### 6.1 Go Testing

**Table-Driven Tests**: Standard approach for all unit tests
- Named test cases with `name` field
- Use `t.Run(tt.name, func(t *testing.T) {...})`
- Structured error messages: `t.Errorf("got %v, want %v", got, want)`

**Mock Objects**:
- Implement interface with `var _ Interface = (*Mock)(nil)`
- Track calls for verification
- Configure return values per-test

**Test Helpers**:
```go
func timePtr(t time.Time) *time.Time { return &t }
func ptr(f float64) *float64 { return &f }
```

### 6.2 Frontend Testing

No formal test files observed in the sample. Testing patterns would be:
- Vitest for unit tests
- Playwright for E2E (common with SvelteKit)

---

## 7. Anti-Patterns Identified

### 7.1 Documented Anti-Patterns (from guidelines)

1. **Storing object copies in $state** - Use ID + $derived instead
2. **Using $effect for derived state** - Use $derived
3. **Duplicate style logic** - Centralize in style system
4. **Complex inline style computation** - Pre-compute in style functions
5. **Mixing API calls with presentation** - Pass handlers from parent

### 7.2 Observed Anti-Patterns

1. **Console.error for all errors** - Some errors might need better user feedback
2. **Inline type assertions** - `as HTMLElement` scattered in templates
3. **Large page components** - `+page.svelte` at 1900 lines could be split

---

## 8. Consistency Analysis

### 8.1 Consistent Patterns (Well-Followed)

- **Go file naming**: Consistent `snake_case.go`
- **Go error handling**: Sentinel errors + wrapping
- **Go test structure**: Table-driven throughout
- **Svelte props**: Always typed Props interface
- **Svelte state**: $state/$derived separation clear
- **API client**: Consistent request/response pattern

### 8.2 Areas Needing Standardization

1. **Component Size**: Some page components are very large (1900+ lines). Consider extracting more into widgets.

2. **Error User Feedback**: Inconsistent between console.error and toast notifications. Need clear guidelines on when to show user-facing errors.

3. **Go Comments**: Some packages have detailed doc comments (classification/classifier.go), others have minimal documentation.

4. **Magic Numbers**: Some timeout/threshold values are inline numbers rather than named constants.

---

## 9. Recommendations

### 9.1 Patterns Worth Codifying

1. **ID/Derivation Pattern**: Already documented, should be enforced via code review checklist

2. **Store Pattern in Go**: Document the `XxxStore` structure as a template for new stores

3. **Handler Response Pattern**: Create template for new handler methods with standard error handling

4. **Table-Driven Test Template**: Provide a snippet for consistent test structure

### 9.2 Improvements to Consider

1. **Split Large Page Components**: The main `+page.svelte` could be refactored into smaller sub-components

2. **Error Handling Guidelines**: Document when to use toast vs console.error vs throw

3. **Constant Extraction**: Move magic numbers to named constants

4. **Go Documentation**: Add package-level documentation to all `internal/` packages

### 9.3 Tool/Process Recommendations

1. **Linting**: Ensure ESLint/Prettier for frontend, `go vet`/`staticcheck` for backend

2. **Code Review Checklist**: Include ID/derivation pattern check for popup-related changes

3. **Component Documentation**: Consider Storybook or similar for widget documentation

---

## Appendix: File Reference

**Go Files Analyzed**:
- `/service/internal/handler/server.go` - Server composition
- `/service/internal/store/projects.go` - Store pattern
- `/service/internal/classification/classifier.go` - Domain logic
- `/service/internal/database/database.go` - Migrations
- `/service/internal/handler/projects.go` - Handler pattern
- `/service/internal/classification/classifier_test.go` - Test patterns
- `/service/internal/sync/week_test.go` - More test patterns
- `/service/internal/google/mock.go` - Mock pattern

**Svelte Files Analyzed**:
- `/web/src/routes/+page.svelte` - Main page component
- `/web/src/lib/components/widgets/EventPopup.svelte` - Widget example
- `/web/src/lib/api/client.ts` - API client
- `/web/src/lib/styles/classification.ts` - Style system

**Documentation Files**:
- `/.claude/CLAUDE.md` - Claude collaboration guidelines
- `/docs/ui-coding-guidelines.md` - UI patterns documentation
