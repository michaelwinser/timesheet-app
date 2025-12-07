# LLM-Based Event Classification Design

## Overview

This document summarizes the experimental implementation of LLM-based calendar event classification for automatic timesheet categorization. The goal is to reduce manual classification effort by using Claude to intelligently assign events to projects based on patterns learned from past classifications.

## Architecture

### Current Classification Hierarchy

1. **Rules-based classification** (highest priority) - Explicit user-defined rules with conditions
2. **LLM-based classification** (experimental) - Claude API for pattern matching
3. **Recurrence-based classification** - Match recurring events to previous classifications

### Components

```
┌─────────────────────────────────────────────────────────────────┐
│                        API Layer                                 │
│  POST /api/llm/classify          - Batch classify events        │
│  GET  /api/llm/preview/{id}      - Preview prompt for event     │
│  POST /api/llm/infer-rules       - Infer rules from examples    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                   services/llm_classifier.py                     │
│                                                                  │
│  classify_events_batch()     - Single API call for N events     │
│  get_classified_examples()   - Fetch training examples          │
│  get_classification_rules()  - Fetch user-defined rules         │
│  build_batch_classification_prompt() - Construct prompt         │
│  infer_rules_from_classifications()  - Rule discovery           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                   services/classifier.py                         │
│                                                                  │
│  EventProperties    - Unified access to native/computed props   │
│  PROPERTY_REGISTRY  - Available properties for classification   │
└─────────────────────────────────────────────────────────────────┘
```

## Event Properties

The classifier uses a rich set of event properties for classification:

### Native Properties (stored in database)
| Property | Type | Description |
|----------|------|-------------|
| `title` | string | Event title/summary |
| `description` | string | Event description/notes |
| `attendees` | string_list | List of attendee emails |
| `meeting_link` | string | Video conferencing URL |
| `my_response_status` | string | User's RSVP: accepted, declined, needsAction, tentative |
| `transparency` | string | Free/busy: opaque (busy) or transparent (free) |
| `visibility` | string | Who can see: default, public, private, confidential |

### Computed Properties (derived at runtime)
| Property | Type | Description |
|----------|------|-------------|
| `weekday` | string | Day name (monday, tuesday, etc.) |
| `time_block` | string | Time of day (morning, afternoon, evening, night) |
| `duration_minutes` | integer | Event duration in minutes |
| `attendee_count` | integer | Number of attendees |
| `attendee_domains` | string_list | Unique email domains from attendees |
| `is_recurring` | boolean | Part of a recurring series |
| `is_all_day` | boolean | All-day event flag |
| `has_meeting_link` | boolean | Has video conferencing link |

## Prompt Structure

The batch classification prompt includes:

1. **Classification Rules** (highest priority)
   - User-defined rules with conditions
   - LLM checks these first before pattern matching

2. **Past Classifications** (few-shot examples)
   - Up to 50 classified events grouped by project
   - 3 detailed examples per project with full properties
   - Prioritizes manual classifications over rule-based

3. **Available Projects**
   - List of valid project names for classification

4. **Events to Classify**
   - Full JSON array with all properties

5. **Instructions**
   - Priority ordering (rules first, then patterns)
   - Guidance on interpreting key properties:
     - `my_response_status`: declined/needsAction → likely didn't attend
     - `transparency`: transparent → free time, not work
     - `attendee_domains`: work domains indicate work meetings

## Key Design Decisions

### Single API Call for Batch Classification

**Problem**: Initial implementation made one API call per event (35 calls for 35 events).

**Solution**: Batch all events into a single prompt and parse array response.

**Benefits**:
- Reduced latency (1 call vs N calls)
- Lower API costs
- Better context for relative classification decisions

### Including Rules in LLM Context

**Problem**: LLM was misclassifying events that matched existing rules (e.g., "Michael / Michael" classified as Personal instead of Alpha-Omega).

**Solution**: Include user-defined rules in the prompt with explicit instruction to check rules first.

**Benefits**:
- LLM respects user preferences
- Rules act as "ground truth" for the LLM
- Consistent behavior between rule engine and LLM

### Response Status for Attendance Detection

**Problem**: OpenSSF meetings were misclassified because user didn't attend (declined/no response).

**Solution**: Added `my_response_status` property and instructed LLM to consider declined/needsAction as "Did not attend".

**Benefits**:
- More accurate time tracking
- Separates "invited but didn't attend" from actual work

## Experimental Features

### Rule Inference from Classifications

The `/api/llm/infer-rules` endpoint asks Claude to discover patterns from past classifications and propose new rules.

**Prompt approach**:
- Provide 100 classified examples (without showing existing rules)
- Ask LLM to identify patterns and generate rule definitions
- Output format matches the rule conditions schema

**Results**: Successfully inferred 15 rules including:
- Domain-based rules (openssf.org → OpenSSF)
- Title pattern rules (contains "FreeBSD" → FreeBSD Board)
- Combined property rules

**Limitations**:
- JSON parsing can fail on complex condition values
- May propose rules that duplicate existing ones
- Confidence calibration needed

## Test Results

### Batch Classification (35 events, Dec 15-21 2025)

| Classification | Count | Key Signals |
|---------------|-------|-------------|
| Junk | 19 | "Busy", "Out of office" titles, no attendees |
| Alpha-Omega | 8 | Work domains, accepted status, recurring 1:1s |
| Did not attend | 4 | needsAction status + OpenSSF domain |
| Personal | 2 | Private visibility, no work domains |
| FreeBSD Board | 1 | Title match, despite needsAction |
| Eclipse Security | 1 | SLSA meeting, accepted |

### Accuracy Observations

**Correct behaviors**:
- "Michael / Michael (weekly)" → Alpha-Omega (not Personal) due to rule match
- OpenSSF WG meetings with needsAction → Did not attend
- Private calendar blocks → Junk

**Edge cases**:
- Some needsAction events still classified as work (e.g., FreeBSD Board) - LLM weighs domain/title over response status
- May need explicit rule or stronger prompt guidance for specific projects

## Implementation Status

### Completed (Experimental)
- [x] Batch classification API endpoint
- [x] Single API call for N events
- [x] Rules included in prompt context
- [x] Response status property added
- [x] Transparency property added
- [x] Visibility property added
- [x] Rule inference endpoint
- [x] Preview endpoint for debugging

### Not Implemented
- [ ] Automatic classification on sync (currently manual trigger only)
- [ ] UI integration for LLM suggestions
- [ ] Confidence threshold for auto-apply
- [ ] Caching of LLM responses
- [ ] Cost tracking / rate limiting
- [ ] User feedback loop (accept/reject suggestions)
- [ ] A/B testing vs rules-only classification

## Future Considerations

### Integration Options

1. **Suggestion Mode**: Show LLM suggestions in UI, user approves/rejects
2. **Auto-Classify Mode**: Apply high-confidence (>0.9) classifications automatically
3. **Hybrid Mode**: Use LLM only when no rule matches

### Cost Optimization

- Cache classifications for recurring events
- Batch classify at end of week rather than per-event
- Use smaller model (Haiku) for simple classifications
- Only invoke LLM for events that don't match rules

### Accuracy Improvements

- User feedback to fine-tune prompt
- Track classification accuracy over time
- Auto-generate rules from confirmed LLM classifications
- Consider fine-tuning for high-volume users

## Files Modified

```
src/services/llm_classifier.py   - Core LLM classification logic
src/services/classifier.py       - Added new properties to registry
src/services/calendar.py         - Extract response_status, transparency, visibility on sync
src/routes/api.py                - API endpoints for LLM features
```

## Database Changes

```sql
-- Added columns to events table
ALTER TABLE events ADD COLUMN my_response_status TEXT;
ALTER TABLE events ADD COLUMN transparency TEXT;
ALTER TABLE events ADD COLUMN visibility TEXT;

-- Backfill from raw_json
UPDATE events SET my_response_status = (
    SELECT json_extract(value, '$.responseStatus')
    FROM json_each(json_extract(raw_json, '$.attendees'))
    WHERE json_extract(value, '$.self') = 1
    LIMIT 1
) WHERE raw_json IS NOT NULL;

UPDATE events SET transparency = json_extract(raw_json, '$.transparency')
WHERE raw_json IS NOT NULL;

UPDATE events SET visibility = json_extract(raw_json, '$.visibility')
WHERE raw_json IS NOT NULL;
```

## Environment Requirements

```
ANTHROPIC_API_KEY=sk-ant-...  # Required for LLM features
```

## API Reference

### POST /api/llm/classify

Classify multiple events using LLM.

**Request:**
```json
{
  "event_ids": [325, 326, 327]
}
```

**Response:**
```json
{
  "results": [
    {
      "event_id": 325,
      "suggestion": {
        "project_id": 5,
        "project_name": "Alpha-Omega",
        "confidence": 0.9,
        "reasoning": "Rule match: attendee domain contains openssf.org"
      }
    }
  ]
}
```

### GET /api/llm/preview/{event_id}

Preview the classification prompt for debugging.

**Response:**
```json
{
  "event_id": 325,
  "prompt": "You are classifying calendar events...",
  "examples_count": 50,
  "rules_count": 12
}
```

### POST /api/llm/infer-rules

Ask LLM to infer classification rules from past examples.

**Response:**
```json
{
  "examples_used": 100,
  "inferred_rules": [
    {
      "name": "OpenSSF Meetings",
      "project": "OpenSSF",
      "conditions": [
        {"property": "attendee_domains", "operator": "list_contains", "value": "openssf.org"}
      ]
    }
  ]
}
```
