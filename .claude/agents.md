# Specialized Agents for Timesheet App

This document defines specialized agents that can be used with Claude Code to enforce architectural principles and assist with specific tasks.

---

## 1. Architecture Review Agent

**Purpose:** Review code changes for architectural compliance before merge.

**When to Use:** Before committing significant changes (3+ files, new features, API changes).

**Invocation:**
```
Review the staged changes for architectural compliance. Check:
1. State synchronization: Do any Svelte files store object references in $state?
2. Layering: Do handlers contain business logic or SQL?
3. API-First: Are API changes reflected in the OpenAPI spec?
4. Migrations: Are any SQL migration files added outside database.go?
Report violations with file:line references.
```

**Checks Performed:**
- [ ] No `$state<EntityType>` patterns (should be `$state<string>` for IDs)
- [ ] No SQL in handler files
- [ ] No business logic in store files
- [ ] API spec updated if endpoints changed
- [ ] Migrations only in database.go

---

## 2. PRD Compliance Agent

**Purpose:** Verify implementation matches PRD requirements.

**When to Use:** After implementing a feature, before marking it complete.

**Invocation:**
```
Compare the implementation against the PRD at [docs/prd-xxx.md].
For each requirement in the PRD:
1. Is it implemented?
2. Does it match the specified behavior?
3. Are there edge cases mentioned that aren't handled?
Report gaps and discrepancies.
```

**Output Format:**
```markdown
## PRD Compliance Report

### Implemented
- [x] Requirement 1 - implemented in file.go:123
- [x] Requirement 2 - implemented in Component.svelte:45

### Gaps
- [ ] Requirement 3 - Not found in implementation
- [ ] Edge case: "When X happens" - Not handled

### Discrepancies
- Requirement 4: PRD says X, implementation does Y
```

---

## 3. Integration Test Generator Agent

**Purpose:** Generate CLI integration tests from PRD scenarios.

**When to Use:** When writing new PRD scenarios or implementing features.

**Invocation:**
```
Generate CLI integration tests for the following PRD scenario:

GIVEN [setup conditions]
WHEN [action]
THEN [expected outcome]

Output a bash script that:
1. Sets up the test data via API calls
2. Executes the action
3. Verifies the expected outcome
4. Cleans up
```

**Output Format:**
```bash
#!/bin/bash
# Test: [Scenario Name]
# Source: [PRD reference]

source tests/integration/lib/api.sh
source tests/integration/lib/assert.sh

# Setup
PROJECT_ID=$(api_create_project '{"name": "Test Project"}')

# Execute
api_post "/api/rules/apply"

# Verify
RESULT=$(api_get "/api/projects/$PROJECT_ID")
assert_equals "$(echo $RESULT | jq -r .status)" "active"

# Cleanup
api_delete "/api/projects/$PROJECT_ID"
```

---

## 4. State Pattern Validator Agent

**Purpose:** Specifically check Svelte components for state synchronization violations.

**When to Use:** When modifying or creating Svelte components with popups/modals.

**Invocation:**
```
Scan the following Svelte files for state synchronization pattern violations:
[list of files]

Look for:
1. $state variables that hold entity types (CalendarEvent, Project, TimeEntry, etc.)
2. Direct assignment of objects from arrays to $state variables
3. Missing $derived patterns for entity lookups

Report violations with suggested fixes.
```

**Patterns to Flag:**
```svelte
// VIOLATION: Entity type in $state
let selectedProject = $state<Project | null>(null);

// VIOLATION: Direct object assignment
selectedProject = projects.find(p => p.id === id);

// CORRECT: ID in $state, derived object
let selectedProjectId = $state<string | null>(null);
const selectedProject = $derived(projects.find(p => p.id === selectedProjectId));
```

---

## 5. Complexity Hotspot Agent

**Purpose:** Extra scrutiny for changes in high-complexity areas.

**When to Use:** When modifying calendar sync, classification, time entries, or invoicing.

**Invocation:**
```
This change touches a complexity hotspot: [AREA].

Perform enhanced review:
1. List all files being modified in this area
2. Identify potential edge cases
3. Check for interaction with related systems
4. Verify test coverage exists
5. Flag any changes that could affect data integrity

Areas and their concerns:
- Calendar Sync: OAuth tokens, incremental sync, event deduplication, timezones
- Classification: Rule parsing, confidence scoring, fingerprint matching
- Time Entries: Overlap calculation, user edit preservation, staleness
- Invoicing: Billing periods, rate calculation, entry locking
```

---

## 6. Commit Message Validator Agent

**Purpose:** Validate commit messages follow conventions.

**When to Use:** As a pre-commit check.

**Invocation:**
```
Validate this commit message:
[message]

Check:
1. Starts with approved verb (Add, Fix, Implement, Refactor, Update, Remove, Redesign, Improve)
2. Describes the "why" not just the "what"
3. References issue number if applicable
4. Body uses bullet points for multiple changes
```

---

## 7. API Spec Sync Agent

**Purpose:** Ensure API implementations match the OpenAPI spec.

**When to Use:** When modifying API endpoints.

**Invocation:**
```
Compare the API implementation against docs/v2/api-spec.yaml:

For each endpoint in the spec:
1. Is it implemented in the handler?
2. Do request/response types match?
3. Are all parameters handled?
4. Is error handling consistent?

For each handler endpoint:
1. Is it in the spec?
2. Are there undocumented parameters?
```

---

## 8. Test Gap Analyzer Agent

**Purpose:** Identify missing tests for stakeholder-critical functionality.

**When to Use:** When planning test improvements.

**Invocation:**
```
Analyze the codebase for test gaps:

1. Read PRDs in docs/ for requirements
2. Read existing tests in service/internal/*/
3. Identify requirements without corresponding tests
4. Prioritize by:
   - Complexity hotspot area
   - User-facing functionality
   - Data integrity concerns

Output a prioritized list of tests to write.
```

---

## Usage from Claude Code

These agents can be invoked by asking Claude Code to perform the specific task. Example:

```
Please run the Architecture Review Agent on the files I just modified.
```

Or for automated workflows, these can be scripted as pre-commit hooks or CI checks.

---

## Future: Automated Agent Hooks

These agents could be automated as:

1. **Pre-commit hooks** - Run validators before allowing commits
2. **CI pipeline stages** - Run compliance checks on PRs
3. **Scheduled audits** - Periodic full-codebase scans

See `docs/analysis/phase2-synthesis.md` Section 9 for implementation details.
