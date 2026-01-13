# Technology-Agnostic Anti-Patterns Catalog

Common anti-patterns that cause problems across projects. Use this as a reference when establishing project principles.

---

## Category: Architectural Drift

### 1. Documentation-Only Principles

**Description:** Principles exist in documentation but have no enforcement mechanism.

**Symptoms:**
- Violations discovered late in development
- "But the docs say..." conversations
- Gradual accumulation of exceptions

**Prevention:**
- Pair every principle with an enforcement mechanism
- Automated checks where possible
- Manual checklists where automation isn't feasible
- Regular audits

**Severity:** High

---

### 2. Layer Violation

**Description:** Code responsibilities leak across architectural layers.

**Examples:**
- Business logic in presentation layer
- Database queries in API handlers
- UI concerns in business logic

**Symptoms:**
- Hard-to-test code
- Scattered changes for single features
- Duplicate logic across layers

**Prevention:**
- Clear layer definitions with examples
- Static analysis to detect cross-layer imports
- Code review checklist item

**Severity:** High

---

### 3. Premature Abstraction

**Description:** Creating abstractions before they're needed.

**Symptoms:**
- Single-use abstractions
- Over-generic interfaces
- Configuration options nobody uses
- "Future-proofing" without clear future

**Prevention:**
- Rule of three: abstract on third use
- Challenge "we might need" justifications
- YAGNI principle enforcement

**Severity:** Medium

---

## Category: State Management

### 4. Stale Data References

**Description:** Holding references to objects that may be updated elsewhere.

**Examples:**
- Storing object copies instead of IDs
- Caching without invalidation
- Derived data not updating with source

**Symptoms:**
- UI showing outdated information
- "But I just updated that!" user complaints
- Inconsistent state between views

**Prevention:**
- Store IDs, derive objects
- Clear cache invalidation strategy
- Immutable data patterns

**Severity:** High

---

### 5. Implicit State Dependencies

**Description:** State changes that implicitly depend on other state.

**Symptoms:**
- Order-dependent initialization
- "Works if you do X first" bugs
- Race conditions

**Prevention:**
- Explicit dependency declaration
- State machines for complex flows
- Initialization order documentation

**Severity:** Medium

---

## Category: Testing

### 6. Test-Code Gap

**Description:** Tests verify implementation details rather than requirements.

**Symptoms:**
- Tests pass but feature is broken
- High coverage, low confidence
- Tests break on refactoring

**Prevention:**
- Test behavior, not implementation
- PRD scenarios as test specifications
- Integration tests for critical paths

**Severity:** High

---

### 7. Mock Overuse

**Description:** Excessive mocking that hides real integration issues.

**Symptoms:**
- Tests pass, production fails
- Complex mock setup
- Mocks that don't match real behavior

**Prevention:**
- Integration tests with real dependencies
- Contract testing for external services
- Mock behavior verification

**Severity:** Medium

---

## Category: API Design

### 8. Contract Drift

**Description:** API implementations diverge from specifications.

**Symptoms:**
- Runtime type errors
- Client/server mismatches
- "The API changed" surprises

**Prevention:**
- Generate types from specifications
- CI checks for spec freshness
- Contract tests

**Severity:** High

---

### 9. Endpoint Proliferation

**Description:** Creating new endpoints for variations that could be parameters.

**Symptoms:**
- Many similar endpoints
- Duplicate handler logic
- Client confusion about which endpoint to use

**Prevention:**
- Query parameters for filtering
- Resource-oriented design
- Endpoint consolidation reviews

**Severity:** Low

---

## Category: Data Management

### 10. Hard Delete Without Audit

**Description:** Permanently deleting data without trail or recovery.

**Symptoms:**
- "Where did my X go?"
- Broken references
- Compliance issues

**Prevention:**
- Soft delete by default
- Audit logging
- Explicit approval for hard deletes

**Severity:** High

---

### 11. Scattered Migrations

**Description:** Database migrations in multiple locations/formats.

**Symptoms:**
- Missing migrations in some environments
- Order conflicts
- "Works on my machine"

**Prevention:**
- Single migration location
- Numbered/versioned migrations
- CI migration verification

**Severity:** Medium

---

## Category: Process

### 12. Large, Mixed Commits

**Description:** Commits combining multiple unrelated changes.

**Symptoms:**
- Hard to review
- Hard to revert specific changes
- Unclear commit history

**Prevention:**
- One concern per commit
- Commit message validation
- PR size limits

**Severity:** Low

---

### 13. Scope Creep via "While I'm Here"

**Description:** Adding unrequested changes during other work.

**Symptoms:**
- PRs larger than expected
- Unexpected behavior changes
- Review burden

**Prevention:**
- Separate PRs for separate concerns
- "Is this in scope?" check
- Change request for additions

**Severity:** Medium

---

### 14. Assumption-Driven Development

**Description:** Implementing based on assumptions rather than requirements.

**Symptoms:**
- "I thought you wanted..."
- Rework after delivery
- Feature misalignment

**Prevention:**
- Clarifying questions before implementation
- PRD scenarios as acceptance criteria
- Incremental delivery with feedback

**Severity:** High

---

## Detection Strategies

### Automated

| Anti-Pattern | Detection Method |
|--------------|------------------|
| Layer Violation | Import analysis, grep for patterns |
| Contract Drift | CI spec freshness check |
| Scattered Migrations | File location check |
| Large Commits | Commit size metrics |

### Manual Review

| Anti-Pattern | Review Question |
|--------------|-----------------|
| Premature Abstraction | "Is this used more than once?" |
| Stale Data | "Where does this data come from? Can it change?" |
| Test-Code Gap | "Does this test verify the requirement?" |
| Scope Creep | "Was this requested?" |

### Conversation

| Anti-Pattern | Question to Ask |
|--------------|-----------------|
| Documentation-Only | "How would we catch this violation?" |
| Assumption-Driven | "What makes you think this is the requirement?" |
| Hard Delete | "What if we need to recover this?" |
