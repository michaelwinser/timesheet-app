# Phase 2: Synthesis of Analysis Findings

*Synthesized from Phase 1 analyses of commit patterns, issue patterns, architecture, and code patterns*

---

## 1. Executive Summary: The Top 5 Principles

After analyzing 189 commits, 96 issues, the full architecture, and code patterns across the codebase, five principles emerge as the most critical for project success:

### 1. Architectural Decisions Must Be Enforced, Not Just Documented

**The Problem:** Documentation alone fails. The project owner observed that "without very structured guidance, Claude Code would often ignore the architectural principles and just start writing code." Drift accumulated because review wasn't happening on each change.

**The Principle:** Every architectural decision needs an enforcement mechanism, not just documentation. This includes pre-commit checks, review checklists, and structured rules in CLAUDE.md that are harder to ignore.

### 2. Testing Must Validate Stakeholder Intent, Not Just Code Correctness

**The Problem:** Current tests "were not effective at preventing violations of PRD or Design principles or core functionality." Unit tests verify code works as written, but not that it works as intended.

**The Principle:** PRDs should be expressed as scenarios that translate directly into integration tests. Tests should run against entity models via CLI scripts to verify business requirements.

### 3. Store IDs, Derive Objects (The State Synchronization Pattern)

**The Problem:** Issue #59 revealed a structural bug where storing object copies in `$state` caused stale data in popups. This pattern was repeated multiple times before being caught.

**The Principle:** Never store object copies in reactive state. Store only IDs, derive objects from source arrays. This is now documented in CLAUDE.md but needs automated enforcement.

### 4. API-First Development with Generated Types

**The Problem:** Hand-written API types drift from server implementations, causing runtime errors discovered only in production.

**The Principle:** OpenAPI spec is the source of truth. All API types are generated (oapi-codegen for Go, TypeScript types for frontend). No hand-written API contracts.

### 5. In-Code Migrations with Idempotent SQL

**The Problem:** Separate migration files get out of sync with code, don't get reviewed together, and can be applied inconsistently.

**The Principle:** Migrations live in `database/database.go` as versioned SQL strings. Use `IF NOT EXISTS` patterns. No separate migration files.

---

## 2. The Enforcement Problem

### 2.1 Why Documentation Is Not Enough

The project owner's observation is the central insight: **"Without very structured guidance, Claude Code would often ignore the architectural principles and just start writing code."**

Evidence from the analyses supports this:

1. **Pattern Violations Accumulated:** Issue #59 (state synchronization bug) revealed that the anti-pattern had been introduced multiple times before being codified in CLAUDE.md
2. **Documentation Exists But Is Bypassed:** The codebase has excellent documentation (PRDs, design docs, ADRs, CLAUDE.md) yet drift still occurred
3. **Review Gaps:** "Review wasn't happening on each change, so drift accumulated" - intermittent review allows violations to compound

### 2.2 The Documentation-Enforcement Gap

```
Current State:
  Documentation -----> Human Reader -----> (maybe) Compliance
                          ^
                          |
                    Can be skipped
                    Can be forgotten
                    Can be misinterpreted

Desired State:
  Documentation -----> Automated Check -----> Compliance Required
                          |
                          v
                    Fails build/commit if violated
```

### 2.3 Proposed Enforcement Mechanisms

**Tier 1: Pre-Commit Hooks (Immediate Enforcement)**
- Pattern detection for known anti-patterns
- Required files check (e.g., tests accompany new handlers)
- API spec validation before commit

**Tier 2: CLAUDE.md Structured Rules (AI Enforcement)**
- Move from prose guidelines to structured, checkable rules
- Include explicit "BEFORE writing code, verify..." sections
- Add mandatory checklists that must be addressed in responses

**Tier 3: Review Agents (Automated Review)**
- Agent that runs on each PR/commit
- Validates changes against architectural principles
- Reports violations before merge

**Tier 4: Integration Tests as Guardrails (Runtime Enforcement)**
- PRD scenarios become executable tests
- Tests run in CI, blocking merge on failure
- Focus on stakeholder intent, not just code correctness

---

## 3. The Testing Gap

### 3.1 The Problem Statement

The project owner identified that "testing didn't really seem to help with the problems that I, as the stakeholder, cared about. Tests were not effective at preventing violations of PRD or Design principles or core functionality."

This is a fundamental mismatch between what tests verify and what stakeholders value:

| What Tests Verify | What Stakeholders Care About |
|-------------------|------------------------------|
| Function returns expected output | Feature works as described in PRD |
| Error handling works | User workflow isn't broken |
| Edge cases handled | Business rules are enforced |
| Code doesn't crash | Data integrity is maintained |

### 3.2 Current Testing State

From the code patterns analysis:
- Go backend has table-driven unit tests
- Mock implementations for external services
- Frontend has minimal observable test coverage
- Issue #61 (integration test strategy) remains open

### 3.3 Proposed Testing Philosophy: PRD-Driven Integration Tests

**The Core Idea:** Express PRDs as scenarios that translate directly into integration tests.

**Implementation Approach:**

1. **PRD Scenarios Format**
   ```markdown
   ## Scenario: Event Classification

   GIVEN a user has connected their Google Calendar
   AND a calendar event exists with title "Meeting with Acme Corp"
   AND a project exists named "Acme Corp" with keyword rule "acme"
   WHEN the classification rules are applied
   THEN the event should be classified to "Acme Corp" project
   AND a time entry should exist for that project on that date
   ```

2. **CLI Test Scripts**
   Tests run as CLI scripts against the server's entity models:
   ```bash
   # test_classification.sh

   # Setup
   PROJECT_ID=$(curl -s POST /api/projects -d '{"name": "Acme Corp"}' | jq -r .id)
   RULE_ID=$(curl -s POST /api/rules -d '{"project_id": "'$PROJECT_ID'", "query": "text:acme"}' | jq -r .id)

   # Create test event (mock calendar sync)
   EVENT_ID=$(curl -s POST /api/test/calendar-events -d '{"title": "Meeting with Acme Corp"}' | jq -r .id)

   # Execute classification
   curl -s POST /api/rules/apply

   # Verify outcome
   CLASSIFIED_PROJECT=$(curl -s GET /api/calendar-events/$EVENT_ID | jq -r .project_id)
   if [ "$CLASSIFIED_PROJECT" != "$PROJECT_ID" ]; then
     echo "FAIL: Event not classified to expected project"
     exit 1
   fi
   ```

3. **Test Categories**

   | Category | Purpose | What It Catches |
   |----------|---------|-----------------|
   | Entity Model Tests | Verify core domain logic | Business rule violations |
   | Workflow Tests | Verify end-to-end flows | Integration failures |
   | Invariant Tests | Verify data constraints | Data integrity issues |
   | Regression Tests | Verify fixed bugs stay fixed | Re-introduced bugs |

### 3.4 Distinguishing Test Types

**Code Correctness Tests (Current)**
- Unit tests of pure functions
- Mock-based isolation tests
- Table-driven edge case tests
- Purpose: Verify code does what it says

**Stakeholder Intent Tests (Proposed)**
- PRD scenario tests
- CLI integration tests against real entities
- Workflow end-to-end tests
- Purpose: Verify code does what stakeholders need

### 3.5 Implementation Priorities

1. **High Priority:** Time entry calculation tests
   - "One entry per project per day" invariant
   - Overlap handling (union not sum)
   - User edit preservation

2. **High Priority:** Classification system tests
   - Rule matching accuracy
   - Confidence scoring
   - Fingerprint matching

3. **Medium Priority:** Invoicing workflow tests
   - Billing period boundaries
   - Invoiced entry protection
   - Rate calculation

4. **Medium Priority:** Calendar sync tests
   - Event deduplication
   - Incremental sync correctness
   - Deleted event handling

---

## 4. Architectural Principles

### 4.1 API-First Development

**Principle:** The OpenAPI spec (`docs/v2/api-spec.yaml`) is the single source of truth for the API contract.

**Evidence:** The architecture analysis shows code generation from OpenAPI to Go handlers and TypeScript types.

**Enforcement Mechanisms:**
- Pre-commit hook: Validate spec file is valid OpenAPI
- CI check: Verify generated code is up-to-date with spec
- CLAUDE.md rule: "Before adding/modifying API endpoints, update the OpenAPI spec first"

### 4.2 Handler/Service/Store Layering

**Principle:** Three-layer architecture with clear responsibilities:
- Handlers: HTTP concerns, validation, response formatting
- Services: Business logic orchestration, cross-cutting concerns
- Stores: Database operations, SQL queries

**Evidence:** The codebase demonstrates this pattern consistently in `handler/*.go`, `classification/service.go`, `store/*.go`.

**Enforcement Mechanisms:**
- CLAUDE.md rule: "Handlers must not contain business logic or SQL"
- Code review checklist item: "Is business logic in service layer?"
- Static analysis: Flag SQL imports in handler packages

### 4.3 Pure Functions for Business Logic

**Principle:** Separate pure functions (no I/O) from orchestration services.

**Evidence:** `classifier.go` is pure (no database access), `service.go` handles I/O orchestration.

**Enforcement Mechanisms:**
- CLAUDE.md rule: "New business logic should be implemented as pure functions where possible"
- Test coverage requirement: Pure functions must have table-driven tests

### 4.4 In-Code Migrations

**Principle:** Database migrations live in `database/database.go`, not separate SQL files.

**Evidence:** This is already documented in CLAUDE.md and consistently followed.

**Enforcement Mechanisms:**
- Pre-commit hook: Fail if `.sql` files are added to migrations directory (which doesn't exist)
- CLAUDE.md rule: Already documented

### 4.5 Soft-Delete and Protection Patterns

**Principle:** Use soft-delete, archive, and suppression rather than hard deletes. Protect user edits.

**Evidence:**
- `is_archived`, `is_suppressed` columns
- `has_user_edits` and `snapshot_computed_hours` for edit protection
- `invoice_id` lock when invoiced

**Enforcement Mechanisms:**
- CLAUDE.md rule: "Never add hard DELETE operations without explicit approval"
- Review checklist: "Does this change handle soft-delete states correctly?"

---

## 5. Coding Principles

### 5.1 State Synchronization (Store IDs, Derive Objects)

**Principle:** Never store object copies in reactive state. Store IDs, derive objects from source arrays.

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

**Enforcement Mechanisms:**
- CLAUDE.md rule: Already documented, but needs reinforcement
- Code review checklist: "Do any `$state` variables hold object references from arrays?"
- ESLint rule (custom): Flag `$state<SomeType | null>` patterns where SomeType is an entity type

### 5.2 Typed Props Interface

**Principle:** All Svelte components must define explicit Props interfaces.

**Evidence:** Consistently followed in observed components.

**Enforcement Mechanisms:**
- ESLint rule: Require Props type on components
- CLAUDE.md rule: "All new components must have typed Props interface"

### 5.3 Go Error Handling

**Principle:** Use sentinel errors for business conditions, wrap errors with context using `%w`.

**Pattern:**
```go
var ErrProjectNotFound = errors.New("project not found")

if errors.Is(err, pgx.ErrNoRows) {
    return nil, ErrProjectNotFound
}

return nil, fmt.Errorf("failed to fetch project: %w", err)
```

**Enforcement Mechanisms:**
- Static analysis: Flag `errors.New()` calls outside of `var` blocks
- CLAUDE.md rule: "Use sentinel errors for business conditions"

### 5.4 Table-Driven Tests

**Principle:** All Go unit tests should use table-driven pattern.

**Evidence:** Consistently followed in `classifier_test.go`, `week_test.go`.

**Enforcement Mechanisms:**
- Code review checklist: "Do new tests use table-driven pattern?"
- CLAUDE.md rule: "New Go tests must be table-driven"

### 5.5 Centralized Style System

**Principle:** Complex style computation belongs in `lib/styles/`, not inline in templates.

**Evidence:** `classification.ts` centralizes classification-related styling.

**Enforcement Mechanisms:**
- CLAUDE.md rule: "Style logic with more than 2 conditions should be extracted to style system"
- Code review checklist: "Is there duplicated style logic?"

---

## 6. Process Principles

### 6.1 Commit Conventions

**Pattern from analysis:**
```
<Verb> <description> [(issue #N)]

Optional detailed body with:
- Bullet points for changes
- Technical rationale

[Fixes #N]

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
```

**Approved verbs:** Add, Fix, Implement, Refactor, Update, Remove, Redesign, Improve

**Enforcement Mechanisms:**
- Pre-commit hook: Validate commit message format
- CLAUDE.md rule: Document the format explicitly

### 6.2 PRD-Before-Code

**Principle:** Major features require PRD and/or design doc before implementation.

**Evidence:** Calendar Sync v2, Rules v2, Invoicing all had PRDs before code.

**Enforcement Mechanisms:**
- CLAUDE.md rule: "Features touching more than 3 files require design discussion first"
- Issue template: Include "Design doc link" field for feature requests

### 6.3 Issue Tracking

**Principle:** Use GitHub issues for tracking work with consistent labeling.

**Evidence:** P0-P3 priority labels, domain labels (classifier, timeentries).

**Recommendations:**
- Require priority label on all issues
- Link commits to issues more consistently
- Document wontfix decisions in comments before closing

### 6.4 Phased Implementation

**Principle:** Complex features should be broken into numbered phases.

**Evidence:** Rules v2 had 4 phases in one day; Calendar Sync v2 had 15+ commits over 2 days.

**Enforcement Mechanisms:**
- CLAUDE.md rule: "Features with >5 planned changes should be phased"
- Issue workflow: Create umbrella issue with phase checklist

---

## 7. Anti-Patterns Catalog

### 7.1 Critical Anti-Patterns (Cause Major Issues)

| Anti-Pattern | Impact | Detection Strategy |
|--------------|--------|-------------------|
| Storing object copies in $state | Stale data in popups, silent failures | ESLint custom rule, code review checklist |
| Business logic in handlers | Untestable code, scattered responsibilities | Static analysis for SQL in handlers |
| Hard deletes without soft-delete | Data loss, broken references | Code review, grep for `DELETE FROM` without `is_` flags |
| Hand-written API types | Runtime type mismatches | CI check for generated file freshness |

### 7.2 Moderate Anti-Patterns (Cause Pain)

| Anti-Pattern | Impact | Detection Strategy |
|--------------|--------|-------------------|
| Large page components (1900+ lines) | Hard to maintain, merge conflicts | LOC threshold warning |
| Console.error without user feedback | Users don't know something failed | Review checklist: "Are errors user-visible?" |
| Magic numbers | Unclear intent, inconsistent values | Static analysis for numeric literals |
| Mixed feature+fix commits | Hard to review, hard to revert | Commit message validation |

### 7.3 Minor Anti-Patterns (Technical Debt)

| Anti-Pattern | Impact | Detection Strategy |
|--------------|--------|-------------------|
| Missing Go package documentation | Hard for new contributors | Doc coverage tool |
| Inline type assertions | Potential runtime failures | TypeScript strict mode |
| Inconsistent error user feedback | Unpredictable UX | Manual review |

---

## 8. Complexity Hotspots

### 8.1 Calendar Synchronization (Highest Complexity)

**Why it's complex:**
- External API dependency (Google Calendar)
- Multiple sync modes (full, incremental, on-demand)
- Background job coordination
- Event deduplication across syncs
- Timezone handling

**Issues related:** #52, #6, #61, #93, #92, #26

**Vigilance required:**
- Test all sync modes independently
- Verify incremental sync handles deleted events
- Monitor sync failure counts
- Handle OAuth token refresh failures

### 8.2 Classification System (High Complexity)

**Why it's complex:**
- Query language parser
- Multi-rule scoring/voting system
- Confidence thresholds
- Fingerprint matching
- User override handling

**Issues related:** 18 issues in the classifier domain

**Vigilance required:**
- Table-driven tests for all query operators
- Edge case testing for text matching
- Confidence calculation verification
- Rule priority handling

### 8.3 Time Entry Computation (Medium-High Complexity)

**Why it's complex:**
- Ephemeral vs materialized state
- Overlap handling (union not sum)
- User edit preservation
- Staleness detection
- Invoice locking

**Issues related:** #64, #11, #8, #87, #75

**Vigilance required:**
- Invariant tests for "one entry per project per day"
- Overlap calculation verification
- User edit preservation across recalculations
- Invoice protection enforcement

### 8.4 Invoicing (Medium Complexity)

**Why it's complex:**
- Billing period management
- Rate changes over time
- Invoice locking behavior
- Google Sheets export integration

**Issues related:** #1, #76, #87, #95, #96, #89

**Vigilance required:**
- Billing period overlap detection
- Rate calculation accuracy
- Invoiced entry protection

---

## 9. Recommended Enforcement Mechanisms

### 9.1 Pre-Commit Hooks

**Implementation:** Add `.pre-commit-config.yaml` or custom Git hooks.

**Checks to implement:**

1. **Commit message format**
   ```bash
   # Verify imperative mood verb
   ALLOWED_VERBS="Add|Fix|Implement|Refactor|Update|Remove|Redesign|Improve"
   if ! echo "$COMMIT_MSG" | grep -qE "^($ALLOWED_VERBS) "; then
     echo "Commit must start with: Add, Fix, Implement, Refactor, Update, Remove, Redesign, or Improve"
     exit 1
   fi
   ```

2. **Generated file freshness**
   ```bash
   # Verify api.gen.go matches spec
   oapi-codegen -config oapi-codegen.yaml docs/v2/api-spec.yaml > /tmp/api.gen.go
   if ! diff -q service/internal/api/api.gen.go /tmp/api.gen.go; then
     echo "api.gen.go is out of date. Run: make generate"
     exit 1
   fi
   ```

3. **No SQL migrations directory**
   ```bash
   if [ -d "migrations" ] || [ -d "service/migrations" ]; then
     echo "SQL migration files found. Migrations must be in database/database.go"
     exit 1
   fi
   ```

### 9.2 Structured CLAUDE.md Rules

Transform CLAUDE.md guidelines into checkable rules:

**Current (prose):**
> When displaying items from arrays in popups/detail views, never store object copies in $state.

**Proposed (structured):**
```markdown
## Mandatory Checklist: Popup/Detail View Changes

Before completing any change that involves popups, modals, or detail views:

- [ ] Verify NO `$state` variables hold object references from arrays
- [ ] Confirm pattern used: `let thingId = $state<string | null>(null)`
- [ ] Confirm derived pattern: `const thing = $derived(array.find(x => x.id === thingId))`
- [ ] If this checklist is not applicable, explain why

**IMPORTANT:** Claude must include this checklist in responses when working on popup-related code.
```

### 9.3 Review Agent

**Concept:** An automated agent that reviews changes before merge.

**Implementation approach:**

1. **Trigger:** On PR creation or update
2. **Scope:** Read changed files, understand context
3. **Checks:**
   - State synchronization pattern compliance
   - Handler/service/store layering
   - API spec consistency
   - Migration format compliance
4. **Output:** PR comment with findings

**Example review agent prompt:**
```
Review this PR for architectural compliance:

1. State Synchronization: Do any Svelte files store object references in $state?
2. Layering: Do handlers contain business logic or SQL?
3. API-First: Are API changes reflected in the OpenAPI spec?
4. Migrations: Are any SQL migration files added outside database.go?

Report violations as PR comment.
```

### 9.4 Integration Test Framework

**Structure:**
```
tests/
├── integration/
│   ├── setup.sh           # Start test server, create test user
│   ├── teardown.sh        # Cleanup
│   ├── scenarios/
│   │   ├── classification.sh
│   │   ├── time_entries.sh
│   │   ├── invoicing.sh
│   │   └── calendar_sync.sh
│   └── lib/
│       ├── api.sh         # Helper functions for API calls
│       └── assert.sh      # Assertion helpers
├── prd-scenarios/
│   ├── classification.md  # PRD scenarios in structured format
│   └── time_entries.md
└── run-integration.sh     # Main test runner
```

**Scenario format:**
```markdown
# Scenario: Event with multiple matching rules

## Setup
- Create project "Acme" with keyword rule "acme"
- Create project "Meetings" with keyword rule "meeting"
- Create event with title "Acme Meeting"

## Execution
- Apply classification rules

## Expected Outcome
- Event classified to higher-scoring project
- Time entry created for winning project only
```

### 9.5 CI Pipeline Enhancements

**Current:** Basic build and test

**Proposed additions:**

1. **Generated code check**
   ```yaml
   - name: Verify generated code
     run: |
       make generate
       git diff --exit-code
   ```

2. **Integration test stage**
   ```yaml
   - name: Run integration tests
     run: tests/run-integration.sh
   ```

3. **Architecture compliance check**
   ```yaml
   - name: Architecture review
     run: ./scripts/architecture-check.sh
   ```

---

## 10. Implementation Roadmap

### Phase 1: Immediate (This Week)

1. **Enhance CLAUDE.md** with structured checklists
2. **Add commit message validation** hook
3. **Create integration test scaffold** with 2-3 initial scenarios

### Phase 2: Short-Term (Next 2 Weeks)

1. **Implement generated code freshness** check in CI
2. **Add 10 PRD scenario tests** covering critical paths
3. **Create review agent** for PR validation

### Phase 3: Medium-Term (Next Month)

1. **Full integration test suite** for all PRD scenarios
2. **Custom ESLint rules** for state synchronization pattern
3. **Architecture compliance dashboard** in CI

### Phase 4: Ongoing

1. **New PRDs include scenarios** in executable format
2. **Review agent** refines based on false positives/negatives
3. **Test coverage** expands with each new feature

---

## Appendix: Cross-Reference to Source Analyses

| Finding | Source Analysis | Section |
|---------|----------------|---------|
| Commit verb conventions | phase1-commit-patterns.md | 2.1 |
| PRD-before-code pattern | phase1-commit-patterns.md | 3.4 |
| State synchronization bug (#59) | phase1-issue-patterns.md | Significant Issues |
| Handler/service/store layering | phase1-architecture.md | 3.2 |
| ID/derivation pattern | phase1-code-patterns.md | 4.0 |
| Table-driven tests | phase1-code-patterns.md | 2.6 |
| Calendar sync complexity | phase1-issue-patterns.md | Feature Area Map |
| In-code migrations | phase1-architecture.md | 3.3 |
