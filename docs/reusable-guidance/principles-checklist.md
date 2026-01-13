# Architectural Principles Checklist

Use this checklist when establishing or reviewing principles for a project. Not all principles apply to all projects - select what's relevant.

---

## API & Interface Design

### API-First Development
- [ ] Is there a single source of truth for API contracts? (OpenAPI, GraphQL schema, etc.)
- [ ] Are types generated from the contract, not hand-written?
- [ ] Is the contract updated BEFORE implementation?
- [ ] Are breaking changes versioned or handled explicitly?

### Contract Ownership
- [ ] Is it clear who/what owns each interface contract?
- [ ] Are internal vs. external APIs distinguished?
- [ ] Are deprecation policies documented?

---

## Code Organization

### Layering
- [ ] Are there clear layers with distinct responsibilities?
- [ ] Do dependencies flow in one direction (e.g., handlers → services → stores)?
- [ ] Is business logic separated from I/O?
- [ ] Are layer violations detectable (via static analysis or review)?

### Pure Functions
- [ ] Is complex business logic implemented as pure functions where possible?
- [ ] Are side effects isolated to orchestration layers?
- [ ] Are pure functions covered by table-driven tests?

### Module Boundaries
- [ ] Are module responsibilities clear?
- [ ] Is there a documented component map?
- [ ] Are cross-module dependencies explicit?

---

## Data Management

### Database Changes
- [ ] Is there a single migration strategy (files, code, ORM)?
- [ ] Are migrations idempotent?
- [ ] Are migrations versioned?
- [ ] Is migration location documented?

### Data Protection
- [ ] Is soft-delete preferred over hard-delete?
- [ ] Are user edits protected from automatic overwrites?
- [ ] Is sensitive data encrypted at rest?
- [ ] Are there audit trails for important changes?

### Data Consistency
- [ ] Are invariants documented? (e.g., "one X per Y")
- [ ] Are invariants enforced at database level where possible?
- [ ] Is eventual consistency acceptable, and if so, where?

---

## State Management (Frontend)

### Reactive State
- [ ] Is there a clear pattern for mutable vs. derived state?
- [ ] Are stale data issues addressed? (e.g., ID-derivation pattern)
- [ ] Is global state minimized?
- [ ] Are state updates predictable?

### Component Design
- [ ] Is there a clear component hierarchy?
- [ ] Are component interfaces (props) well-defined?
- [ ] Is styling approach consistent?
- [ ] Are components appropriately sized?

---

## External Integrations

### Third-Party Services
- [ ] Are external calls isolated behind interfaces?
- [ ] Are there mock implementations for testing?
- [ ] Is credential management secure?
- [ ] Are failure modes handled?

### Authentication
- [ ] Is auth approach documented?
- [ ] Are tokens handled securely?
- [ ] Is token refresh handled?

---

## Testing Strategy

### Unit Tests
- [ ] Is there a consistent testing pattern? (e.g., table-driven)
- [ ] Are tests co-located with implementation?
- [ ] Is mocking approach standardized?

### Integration Tests
- [ ] Are PRD scenarios translated to tests?
- [ ] Do tests verify stakeholder intent, not just code?
- [ ] Is test data management handled?
- [ ] Can tests run in CI?

### Test Coverage
- [ ] Are complexity hotspots prioritized for testing?
- [ ] Is coverage measured?
- [ ] Are critical paths always tested?

---

## Process & Workflow

### Commit Conventions
- [ ] Is there a commit message format?
- [ ] Are commits linked to issues/PRs?
- [ ] Is co-authorship tracked?

### Code Review
- [ ] Are reviews required before merge?
- [ ] Is there automated validation? (linting, type checking)
- [ ] Are architectural checks part of review?

### Documentation
- [ ] Is there a project map?
- [ ] Is there a lexicon of terms?
- [ ] Are design decisions recorded?
- [ ] Is documentation kept current?

---

## Enforcement

### Automated Checks
- [ ] Are principles enforceable via static analysis?
- [ ] Are there pre-commit hooks?
- [ ] Are there CI pipeline checks?
- [ ] Are violations blocking or warning?

### Manual Review
- [ ] Are there review checklists?
- [ ] Are there specialized review agents?
- [ ] Is there a pushback/challenge mechanism?

### Feedback Loops
- [ ] Are violations tracked?
- [ ] Is guidance updated based on violations?
- [ ] Are enforcement mechanisms evaluated for effectiveness?
