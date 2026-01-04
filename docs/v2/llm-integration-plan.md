# LLM Integration (Phase 1.7) - Implementation Plan

## Summary

Add LLM-powered rule suggestions that analyze manual classifications and propose rules for user review. Include a feedback mechanism to capture "why" when users override classifications.

## Current State

The codebase is **well-prepared** for LLM integration:
- `llm` classification source already in enum (migration #5)
- `classification_overrides` table exists for training data (migration #8)
- `GetRecentOverrides()` method designed for "LLM training"
- Confidence tracking (0.0-1.0) with thresholds already implemented
- NeedsReview flag identifies low-confidence events

## Design

### API Endpoints

```
POST /api/rules/suggest   - Generate rule suggestions using LLM
```

**Request:**
```json
{
  "since_days": 7    // Look back N days for patterns (default: 7)
}
```

**Response:**
```json
{
  "suggestions": [
    {
      "query": "domain:acme.com",
      "project_id": "uuid",
      "project_name": "Acme Corp",
      "confidence": 0.85,
      "reasoning": "5 events with acme.com domain were manually classified to this project",
      "matched_event_count": 5,
      "sample_titles": ["Weekly Sync", "Project Review", "1:1 with Alice"]
    }
  ],
  "analyzed_count": 23
}
```

### Reclassification Feedback

When classifying an event via `PUT /api/calendar-events/{id}/classify`, add optional reason:

**Request (extended):**
```json
{
  "project_id": "uuid",
  "reason": "This is actually a client meeting, not internal"  // optional
}
```

The handler will:
1. Check if event was previously classified by rule/llm/fingerprint
2. If so, record in `classification_overrides` with from_source and reason
3. Proceed with classification as normal

### UI Flow

**Rules Page - Suggested Rules Section:**
```
┌─────────────────────────────────────────────────────────────┐
│ SUGGESTED RULES (3)                         [Analyze Again] │
├─────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ 💡 domain:acme.com → Acme Corp           [Accept] [X]  │ │
│ │    Based on 5 manual classifications                   │ │
│ └─────────────────────────────────────────────────────────┘ │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ 💡 title:standup → Daily Work            [Accept] [X]  │ │
│ │    Based on 8 manual classifications                   │ │
│ └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

**Calendar View - Reclassify with Reason:**
When clicking a project dot on an already-classified event, show a simple prompt:
```
┌─────────────────────────────────────────┐
│ Why are you reclassifying this event?   │
│ ┌─────────────────────────────────────┐ │
│ │ [Optional explanation...]           │ │
│ └─────────────────────────────────────┘ │
│ This helps improve future suggestions  │
│                                         │
│            [Skip]  [Save]               │
└─────────────────────────────────────────┘
```

## Files to Modify

### Backend

| File | Changes |
|------|---------|
| `docs/v2/api-spec.yaml` | Add `POST /api/rules/suggest` endpoint |
| `service/internal/api/api.gen.go` | Regenerate from spec |
| `service/internal/llm/service.go` | **NEW** - LLM service for Anthropic API |
| `service/internal/llm/prompt.go` | **NEW** - Prompt construction |
| `service/internal/handler/rules.go` | Add SuggestRules handler |
| `service/internal/handler/calendars.go` | Update classifyCalendarEvent to record overrides |
| `service/cmd/server/main.go` | Add LLM service initialization |

### Frontend

| File | Changes |
|------|---------|
| `web/src/lib/api/types.ts` | Add RuleSuggestion types |
| `web/src/lib/api/client.ts` | Add suggestRules() method |
| `web/src/routes/rules/+page.svelte` | Add suggested rules section |
| `web/src/routes/+page.svelte` | Add reclassify reason prompt |

## Implementation Steps

### Step 1: Backend - LLM Service Package

Create `service/internal/llm/`:

**service.go** - Core LLM interface
```go
type Service struct {
    apiKey string
    model  string
}

type RuleSuggestion struct {
    Query            string
    ProjectID        uuid.UUID
    ProjectName      string
    Confidence       float64
    Reasoning        string
    MatchedEventCount int
    SampleTitles     []string
}

func (s *Service) SuggestRules(ctx context.Context,
    events []*store.CalendarEvent,
    projects []*store.Project,
    overrides []*store.ClassificationOverride) ([]RuleSuggestion, error)
```

**prompt.go** - Prompt construction
```go
func BuildSuggestRulesPrompt(events []*CalendarEvent,
    projects []*Project) string
```

Uses Anthropic Messages API with:
- Temperature: 0.2 (low for consistent output)
- Max tokens: 2000
- JSON mode for structured output

### Step 2: Backend - API Endpoint

Add to `api-spec.yaml`:
```yaml
/api/rules/suggest:
  post:
    operationId: suggestRules
    tags: [rules]
    summary: Get LLM-suggested rules based on manual classifications
    requestBody:
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/SuggestRulesRequest'
    responses:
      '200':
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/SuggestRulesResponse'
```

### Step 3: Backend - Override Recording

Update `ClassifyEventRequest` schema:
```yaml
ClassifyEventRequest:
  properties:
    project_id:
      type: string
      format: uuid
    skip:
      type: boolean
    reason:
      type: string
      description: Optional explanation for reclassification
```

Update handler to call `ruleStore.RecordOverride()` when changing classification.

### Step 4: Frontend - Suggested Rules UI

Add to rules page:
- "Suggested Rules" section above existing rules
- Each suggestion shows: query, target project, reasoning
- Accept button: creates rule via existing API
- Dismiss button: removes from list (client-side only)
- "Analyze Again" button: calls suggestRules endpoint

### Step 5: Frontend - Reclassify Reason

Add simple modal/popover when reclassifying:
- Only shows when event has `classification_source` !== 'manual'
- Text input for optional reason
- Skip and Save buttons
- Pass reason to classifyCalendarEvent API

## Environment Variables

```
ANTHROPIC_API_KEY=sk-ant-...     # Required for LLM features
LLM_MODEL=claude-3-5-haiku       # Optional, defaults to haiku for speed/cost
```

## LLM Prompt Strategy

The prompt will include:
1. List of projects with names
2. Recent manually classified events (last 7 days)
3. Recent classification overrides with reasons

Expected output format (JSON):
```json
{
  "suggestions": [
    {
      "query": "domain:acme.com",
      "project_id": "...",
      "reasoning": "...",
      "confidence": 0.85
    }
  ]
}
```

## Out of Scope (Future)

- Automatic LLM classification of pending events (classify-with-llm endpoint)
- Daily scheduled job for suggestions (manual trigger only for now)
- Caching of LLM responses
- Cost tracking / rate limiting
- A/B testing vs rules-only classification
