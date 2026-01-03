# Analysis: Classification Service Boundaries

This document analyzes architectural options for the classification system.

## Options Considered

| Approach | Description |
|----------|-------------|
| **A. Single spec, single service** | Add rules endpoints to existing api-spec.yaml |
| **B. Separate spec, same service** | Two OpenAPI specs, one Go binary, classification as pure library |
| **C. Separate service entirely** | Classification as its own microservice |

## Data Dependencies

Classification needs to read/write:

```
Classification System
    ├── Reads: calendar_events, projects, rules
    ├── Writes: rules, classification results on events
    └── LLM calls: Claude API (async, seconds latency)
```

If truly separate service, you face a choice:
1. **Shared database** - Both services hit same Postgres (defeats some isolation benefits)
2. **API calls** - Classification calls main service for events/projects (latency, coupling)
3. **Data replication** - Classification has its own copies (sync complexity)

## Component Analysis

| Component | Separate Service? | Rationale |
|-----------|-------------------|-----------|
| **Rules CRUD** | No | Needs same auth, stored with user data |
| **Query Engine** | No | Stateless library, no reason to isolate |
| **Scoring Engine** | No | Pure computation, library is sufficient |
| **LLM Suggester** | Yes | Different latency (seconds), async, cost-sensitive |
| **MCP Server** | Yes | Already planned as separate process |

## Decision: Option B with Pure Library

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Main Service                            │
│  ┌─────────────────────────────────────────────────────┐    │
│  │                   API Handlers                       │    │
│  │  (Load rules, fetch events, store results)          │    │
│  └─────────────────────────────────────────────────────┘    │
│                           │                                  │
│                           ▼                                  │
│  ┌─────────────────────────────────────────────────────┐    │
│  │            Classification Library                    │    │
│  │  (Pure functions, no DB, no I/O)                    │    │
│  │                                                      │    │
│  │  Classify(rules, items) → results                   │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────────┐
│    Postgres     │     │   LLM Worker    │──▶ Claude API
└─────────────────┘     └─────────────────┘
```

### Classification Library Interface

The library is a pure function with no dependencies on:
- Database or any I/O
- Store package types (CalendarEvent, etc.)
- UUIDs (uses strings for IDs)
- Postgres types

**Implemented in `service/internal/classification/classifier.go`:**

```go
// Input: rules and items (generic maps)
type Rule struct {
    ID        string
    Query     string
    TargetID  string  // Project ID or "DNA" for did-not-attend
    Weight    float64
}

type Item struct {
    ID         string
    Properties map[string]any  // title, description, attendees, etc.
}

// Output: classification results
type Result struct {
    ItemID      string
    TargetID    string
    Confidence  float64
    NeedsReview bool
    Votes       []Vote
}

// Pure functions - no I/O, fully testable
func Classify(rules []Rule, items []Item, config Config) []Result
func ClassifyAttendance(rules []Rule, items []Item, config Config) []AttendanceResult
func PreviewRules(query string, items []Item) ([]string, error)
```

**Orchestration layer in `service/internal/classification/service.go`:**
- Loads rules from database
- Converts store types to library types
- Calls pure Classify function
- Writes results back to database

### Responsibilities

| Layer | Responsibility |
|-------|---------------|
| **API Handlers** | Load rules from DB, fetch events, convert to library types, store results |
| **Classification Library** | Parse queries, evaluate matches, compute scores |
| **Store** | CRUD for rules, events, classification results |

### Benefits

1. **Testable** - Library tests need no database mocks (see `classifier_test.go`)
2. **Portable** - Could extract to separate Go module later
3. **Reusable** - MCP server can import same library
4. **Clear boundary** - Service does I/O, library does computation

### Separate API Spec

Two OpenAPI specs:
- `api-spec.yaml` - Core API (auth, projects, entries, sync)
- `api-classification.yaml` - Classification API (rules, preview, apply)

Both implemented by the same Go service, but documented separately.

### LLM Worker

The LLM suggestion job runs separately:
- Could be goroutine, cron job, or separate binary
- Calls Claude API for rule suggestions
- Writes suggested rules to DB for user review
- Different latency/cost characteristics than main service

## Consequences

### Positive
- Strong component boundary without microservice complexity
- Library is highly testable and portable
- Prepares for extraction if ever needed
- MCP server can reuse classification logic

### Negative
- Main service must convert between store types and library types
- Two API specs to maintain
- Some type duplication between specs

## References

- [ADR-003: Scoring-Based Classification](decisions/003-scoring-classification.md)
- [Roadmap Phase 1](roadmap.md)
