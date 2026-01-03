# ADR 003: Scoring-Based Classification

## Status

Accepted

## Context

Calendar events need to be classified on two dimensions:
1. **Attendance**: Did the user attend? (yes/no)
2. **Project**: Which project should this time be attributed to?

The v1 approach used ordered rule evaluation ("first match wins"), which has problems:
- A high-priority rule can shadow other relevant signals
- No way to express confidence or uncertainty
- Conflicting rules are hidden, not surfaced
- Hard to integrate LLM suggestions alongside rules

Additionally, we want to integrate LLM-based classification (which worked well in v1 experiments) with rule-based classification in a unified system.

## Decision

Use a **scoring/accumulator model** where all matching rules contribute votes to a classification decision, rather than stopping at the first match.

### Core Model

```go
type ClassificationSource string

const (
    SourceManual ClassificationSource = "manual"
    SourceRule   ClassificationSource = "rule"
    SourceLLM    ClassificationSource = "llm"
)

type Rule struct {
    ID        uuid.UUID
    Query     string           // Gmail-style query syntax
    ProjectID *uuid.UUID       // Target project (nil for attendance rules)
    Attended  *bool            // Target attendance (nil for project rules)
    Weight    float64          // Default 1.0, UI shows as "priority" toggle
    IsEnabled bool
}

type ClassificationResult struct {
    ProjectID   *uuid.UUID
    Attended    *bool
    Confidence  float64
    NeedsReview bool
    Source      ClassificationSource
    Votes       []Vote           // For debugging/transparency
}

type Vote struct {
    RuleID    *uuid.UUID       // nil for LLM votes
    ProjectID *uuid.UUID
    Attended  *bool
    Weight    float64
    Source    ClassificationSource
}
```

### Two Independent Evaluations

Attendance and project classification run separately:

```
Event
  │
  ├─► Attendance Evaluation ─► attended=true, confidence=0.9
  │     └─ Rules voting on attended:yes/no
  │
  └─► Project Evaluation ─► project=Acme, confidence=0.7
        └─ Rules voting on project_id
```

Both use the same scoring mechanism.

### Scoring Algorithm

```
For each dimension (attendance, project):
  1. Evaluate all enabled rules against the event
  2. For each matching rule, add its weight to the target's score
  3. Optionally add LLM suggestion as a weighted vote
  4. Winner = target with highest score
  5. Confidence = winner_score / total_score (or normalized)
```

Example:
```
Event: "Acme Weekly Sync" with bob@acme.com

Rule 1: domain:acme.com → Acme (weight 1.0)     ✓ match
Rule 2: title:Weekly → Personal (weight 1.0)    ✓ match
Rule 3: attendees:bob → Acme (weight 1.0)       ✓ match
LLM: suggests Acme (confidence 0.8 → weight 0.8)

Scores:
  Acme: 1.0 + 1.0 + 0.8 = 2.8
  Personal: 1.0

Winner: Acme
Confidence: 2.8 / 3.8 = 0.74
```

### Confidence Thresholds

```go
const (
    ConfidenceFloor   = 0.5  // Below: don't classify, leave pending
    ConfidenceCeiling = 0.8  // Above: auto-classify without flag
)
```

| Confidence | Action |
|------------|--------|
| < 0.5 | Don't classify, leave pending |
| 0.5 - 0.8 | Classify but set `needs_review=true` |
| > 0.8 | Classify, no review flag |

### Rule Weights

- Default weight: 1.0
- UI exposes "priority" toggle that doubles weight (1.0 → 2.0)
- Backend stores actual float for LLM tuning
- LLM can suggest weight adjustments behind the scenes

### Manual Classification Protection

Manual classifications (`source=manual`) are never overwritten by rules or LLM:
- Rules engine skips events with `source=manual`
- LLM suggestions for manual events are ignored
- User must explicitly reclassify to change

### Reclassification Feedback

When a user overrides a rule/LLM classification:

```go
type ClassificationOverride struct {
    EventID       uuid.UUID
    FromProjectID *uuid.UUID
    ToProjectID   *uuid.UUID
    FromSource    ClassificationSource
    Reason        string    // Optional user explanation
    CreatedAt     time.Time
}
```

This becomes training data for LLM rule suggestions.

### LLM Integration

LLM participates in two ways:

1. **As a voter**: LLM confidence becomes a weighted vote alongside rules
2. **As a rule suggester**: Daily job analyzes manual classifications and proposes rules

Rule suggestion trigger:
- 3+ similar manual classifications within a time window
- Pattern detected (common domain, attendee, title keyword)
- Proposed rule shown in "Suggested Rules" section for user review

## Consequences

### Positive

- **Multiple signals combine**: Weak signals accumulate into strong classification
- **Conflicts are visible**: Tie or near-tie scores surface for review
- **Confidence is meaningful**: Derived from rule agreement, not arbitrary priority
- **LLM fits naturally**: Just another weighted vote in the system
- **User control preserved**: Manual classifications are protected
- **Debuggable**: Vote breakdown explains any classification

### Negative

- **More complex than first-match**: Harder to predict outcome from single rule
- **Performance**: Must evaluate all rules, not stop at first match
- **Weight tuning**: May need iteration to find good defaults

### Mitigations

- Cache rule evaluations per event (rules rarely change)
- Start with weight=1.0 for all rules, add priority toggle
- Show vote breakdown in UI for transparency

## Implementation Notes

### Database Changes

```sql
-- Add to calendar_events
ALTER TABLE calendar_events ADD COLUMN classification_confidence FLOAT;
ALTER TABLE calendar_events ADD COLUMN needs_review BOOLEAN DEFAULT false;

-- New table for rules
CREATE TABLE classification_rules (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    query TEXT NOT NULL,
    project_id UUID REFERENCES projects(id),  -- null for attendance rules
    attended BOOLEAN,                          -- null for project rules
    weight FLOAT NOT NULL DEFAULT 1.0,
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Track reclassification feedback
CREATE TABLE classification_overrides (
    id UUID PRIMARY KEY,
    event_id UUID NOT NULL REFERENCES calendar_events(id),
    from_project_id UUID REFERENCES projects(id),
    to_project_id UUID REFERENCES projects(id),
    from_source TEXT,
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Component Boundaries

| Component | Responsibility |
|-----------|---------------|
| `QueryParser` | Parse query string → AST |
| `QueryEvaluator` | AST + Event → match (bool) |
| `RulesEngine` | Evaluate all rules, compute scores |
| `LLMSuggester` | Call Claude API, return weighted suggestions |
| `ClassificationService` | Orchestrate evaluation, apply thresholds |

### API Endpoints

```
GET  /api/rules                    - List rules
POST /api/rules                    - Create rule
GET  /api/rules/{id}               - Get rule
PUT  /api/rules/{id}               - Update rule
DELETE /api/rules/{id}             - Delete rule
POST /api/rules/preview            - Preview matches with conflicts
POST /api/classification/evaluate  - Run classification on events
GET  /api/classification/suggestions - Get pending rule suggestions
```
