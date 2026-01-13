# Claude Collaboration Guidelines

## Communication Style

- **Push back when something isn't a good idea.** Flag concerns early, even when not explicitly asked. Be direct about trade-offs, complexity costs, and maintenance burden.

- **Bring expertise proactively.** Offer alternatives when you see better approaches rather than just executing the first viable path.

- **Ask clarifying questions.** If context is unclear or requirements seem underspecified, ask rather than making assumptions. If you don't understand something, say so - it likely means the explanation needs work.

- **Be direct, not deferential.** Politeness doesn't require compliance. Disagree when you have good reason to.

## After Raising Concerns

- Execute the user's decision once concerns have been aired
- Don't be contrarian for its own sake
- Respect that the user has project context you may lack

---

## MANDATORY: Before Writing Code

**STOP. Before writing any code, you MUST verify these constraints.** Documentation alone has proven insufficient - architectural drift occurs when these aren't enforced.

### Pre-Implementation Checklist

For ANY code change, mentally answer these questions:

1. **API Changes:** Does this touch an API endpoint?
   - If YES: Update `docs/v2/api-spec.yaml` FIRST, then generate code
   - Run: `make generate` after spec changes

2. **Database Changes:** Does this need schema changes?
   - If YES: Add migration to `service/internal/database/database.go`
   - Use idempotent SQL (`IF NOT EXISTS`, etc.)
   - NEVER create separate SQL files

3. **State in UI:** Does this involve displaying data in popups, modals, or detail views?
   - If YES: Follow the ID/Derivation pattern (see below)
   - NEVER store object copies in `$state`

4. **New Feature (3+ files):** Is this a feature touching more than 3 files?
   - If YES: Discuss design approach BEFORE implementation
   - Reference relevant PRD if one exists

---

## Architectural Principles (MUST Follow)

### 1. API-First Development

The OpenAPI spec is the single source of truth. **Never hand-write API types.**

```
Flow: OpenAPI Spec → Generated Code → Implementation
      ↓
      docs/v2/api-spec.yaml
      ↓
      make generate
      ↓
      service/internal/api/api.gen.go
```

**Violation:** Adding API endpoints without updating the spec first.

### 2. Handler/Service/Store Layering

```
┌─────────────┐
│  Handlers   │  HTTP concerns only (validation, response formatting)
├─────────────┤  NO business logic, NO SQL
│  Services   │  Business logic orchestration
├─────────────┤  Cross-cutting concerns
│   Stores    │  Database operations only
└─────────────┘  SQL queries, no business logic
```

**Violations:**
- SQL queries in handlers
- Business logic in stores
- HTTP response formatting in services

### 3. Pure Functions for Business Logic

Separate pure functions (no I/O) from orchestration.

**Example:** `classifier.go` is pure (no DB access), `service.go` handles I/O.

**Why:** Pure functions are testable with table-driven tests without mocks.

### 4. Soft-Delete and Protection Patterns

**Never hard delete user data.** Use:
- `is_archived` for user-hidden items
- `is_suppressed` for system-hidden items
- `has_user_edits` to protect manual changes
- `invoice_id` to lock invoiced entries

**Violation:** `DELETE FROM` without explicit stakeholder approval.

---

## UI Code Patterns (Critical)

### State Synchronization: Store IDs, Derive Objects

**This is the most critical UI pattern.** Issue #59 revealed this bug pattern was introduced multiple times before being caught.

When displaying items from arrays in popups/detail views, **NEVER store object copies in $state**. This creates stale data when the source array updates.

**Anti-pattern (CAUSES BUGS):**
```svelte
let hoveredEvent = $state<CalendarEvent | null>(null);
hoveredEvent = eventFromArray; // Creates stale copy - BUG!
```

**Correct pattern (ALWAYS USE):**
```svelte
let hoveredEventId = $state<string | null>(null);
const hoveredEvent = $derived(events.find(e => e.id === hoveredEventId) ?? null);
```

### Popup/Modal Checklist

**When working on popups, modals, or detail views, verify:**

- [ ] NO `$state` variables hold object references from arrays
- [ ] Pattern used: `let thingId = $state<string | null>(null)`
- [ ] Derived pattern: `const thing = $derived(array.find(x => x.id === thingId))`

**Include this checklist in your response when modifying popup-related code.**

### Component Organization

- **Props:** Always define explicit `Props` interface
- **Styles:** Complex conditional styling goes in `lib/styles/`, not inline
- **Size:** Components over 500 lines should be split

### Error Handling

- User-facing errors: Use toast notifications
- Debug errors: Use `console.error` AND notify user
- Never swallow errors silently

---

## Database Migrations

Migrations are defined **ONLY** in Go code at `service/internal/database/database.go`. The service runs migrations automatically on startup.

**To add a new migration:**
1. Add a new entry to the `migrations` slice in `database.go`
2. Use the next sequential version number
3. Write idempotent SQL (use `IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`, etc.)

**Do NOT create separate SQL files** - there is no `migrations/` directory. All migration SQL lives in the Go code.

---

## Go Backend Patterns

### Error Handling

Use sentinel errors for business conditions:

```go
var ErrProjectNotFound = errors.New("project not found")
var ErrDuplicateShortCode = errors.New("duplicate short code")

// In store code:
if errors.Is(err, pgx.ErrNoRows) {
    return nil, ErrProjectNotFound
}
return nil, fmt.Errorf("failed to fetch project: %w", err)
```

### Testing

All unit tests MUST be table-driven:

```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name     string
        input    InputType
        expected OutputType
        wantErr  bool
    }{
        {"case 1", input1, expected1, false},
        {"case 2", input2, expected2, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
```

---

## Commit Conventions

```
<Verb> <description> [(issue #N)]

[Optional body with bullet points]

[Fixes #N]

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
```

**Approved verbs:** Add, Fix, Implement, Refactor, Update, Remove, Redesign, Improve

**Examples:**
- `Add project archiving feature (issue #72)`
- `Fix classification confidence calculation`
- `Refactor time entry computation to use pure functions`

---

## Complexity Hotspots (Extra Vigilance Required)

These areas have historically caused the most issues. Apply extra scrutiny:

1. **Calendar Synchronization** - External API, multiple sync modes, timezone handling
2. **Classification System** - Query parsing, rule scoring, confidence thresholds
3. **Time Entry Computation** - Ephemeral vs materialized, overlap handling, user edits
4. **Invoicing** - Billing periods, rate changes, entry locking

---

## Testing Philosophy

### Unit Tests vs. Stakeholder Intent Tests

**Unit tests** (what we have): Verify code does what it says
**Integration tests** (what we need): Verify code does what stakeholders need

**PRDs should be expressed as testable scenarios:**

```markdown
GIVEN a user has a project "Acme" with keyword rule "acme"
AND a calendar event exists with title "Acme Meeting"
WHEN classification rules are applied
THEN the event should be classified to "Acme"
AND a time entry should exist for that project
```

**These scenarios should translate to CLI integration tests against the server.**

See `tests/integration/` for the test framework (when implemented).

---

## Reference Documents

- `docs/ui-coding-guidelines.md` - Full UI patterns
- `docs/v2/` - Design documents and API spec
- `docs/analysis/phase2-synthesis.md` - Comprehensive principles analysis
