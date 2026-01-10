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

## UI Code Patterns (Critical)

### State Synchronization
When displaying items from arrays in popups/detail views, **never store object copies in $state**. This creates stale data when the source array updates.

**Anti-pattern:**
```svelte
let hoveredEvent = $state<CalendarEvent | null>(null);
hoveredEvent = eventFromArray; // Creates stale copy
```

**Correct pattern:**
```svelte
let hoveredEventId = $state<string | null>(null);
const hoveredEvent = $derived(events.find(e => e.id === hoveredEventId) ?? null);
```

See `docs/ui-coding-guidelines.md` for full guidelines.

## Database Migrations

Migrations are defined **only** in Go code at `service/internal/database/database.go`. The service runs migrations automatically on startup.

**To add a new migration:**
1. Add a new entry to the `migrations` slice in `database.go`
2. Use the next sequential version number
3. Write idempotent SQL (use `IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`, etc.)

**Do NOT create separate SQL files** - there is no `migrations/` directory. All migration SQL lives in the Go code.
