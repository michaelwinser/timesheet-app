# PRD Scenario Template

Use this template when writing PRD scenarios. Scenarios written in this format can be directly translated into integration tests.

---

## Feature: [Feature Name]

### Overview

[Brief description of the feature and its purpose]

### User Story

As a [type of user]
I want [some goal]
So that [some reason]

---

## Scenarios

### Scenario 1: [Descriptive Name]

**Priority:** P0/P1/P2/P3

#### Preconditions (GIVEN)

```
GIVEN [initial context]
AND [additional context]
AND [more context if needed]
```

#### Action (WHEN)

```
WHEN [action is performed]
```

#### Expected Outcome (THEN)

```
THEN [expected result]
AND [additional expected result]
AND [more expected results if needed]
```

#### Edge Cases

- **Edge case 1:** [Description] → [Expected behavior]
- **Edge case 2:** [Description] → [Expected behavior]

#### Test Data

```json
{
  "setup": {
    "projects": [
      {"name": "Test Project", "short_code": "TEST"}
    ],
    "events": [
      {"title": "Test Event", "start": "2026-01-15T10:00:00Z"}
    ]
  },
  "expected": {
    "classification": "Test Project",
    "time_entry_hours": 1.0
  }
}
```

---

### Scenario 2: [Another Scenario Name]

[Same format as above]

---

## Invariants

These conditions must ALWAYS hold true:

1. **[Invariant Name]:** [Description]
   - Example: "One time entry per project per day" - A project can never have two separate time entries for the same date

2. **[Another Invariant]:** [Description]

---

## Out of Scope

These behaviors are explicitly NOT supported:

1. [Thing we're not doing] - Reason: [Why]
2. [Another thing] - Reason: [Why]

---

## Integration Test Mapping

| Scenario | Test File | Status |
|----------|-----------|--------|
| Scenario 1 | `tests/integration/scenarios/[area]/scenario1.sh` | Not written |
| Scenario 2 | `tests/integration/scenarios/[area]/scenario2.sh` | Not written |

---

## Example: Classification PRD Scenario

### Scenario: Event classified by keyword in title

**Priority:** P1

#### Preconditions (GIVEN)

```
GIVEN a user has created a project "Acme Corp" with short code "ACME"
AND the project has a keyword rule with query "text:acme"
AND a calendar event exists with title "Weekly Acme Meeting"
AND the event is not already classified
```

#### Action (WHEN)

```
WHEN the user runs classification (via Classify Day or automatic trigger)
```

#### Expected Outcome (THEN)

```
THEN the event should be classified to project "Acme Corp"
AND the classification confidence should be "high" (>= 65%)
AND a time entry should be created for "Acme Corp" on the event's date
AND the time entry hours should equal the event duration
```

#### Edge Cases

- **Multiple keywords match:** Higher weight rule wins
- **Same weight rules:** First created rule wins (deterministic)
- **Keyword in description only:** Still matches (full_text search)

#### Test Data

```json
{
  "setup": {
    "project": {
      "name": "Acme Corp",
      "short_code": "ACME"
    },
    "rule": {
      "query": "text:acme",
      "weight": 100
    },
    "event": {
      "title": "Weekly Acme Meeting",
      "start": "2026-01-15T14:00:00Z",
      "end": "2026-01-15T15:00:00Z"
    }
  },
  "expected": {
    "classification": {
      "project_id": "[ACME project ID]",
      "confidence": "high"
    },
    "time_entry": {
      "project_id": "[ACME project ID]",
      "date": "2026-01-15",
      "hours": 1.0
    }
  }
}
```

---

## Checklist for PRD Authors

Before finalizing a PRD:

- [ ] All scenarios follow GIVEN/WHEN/THEN format
- [ ] Each scenario has clear expected outcomes
- [ ] Edge cases are documented
- [ ] Test data is provided where helpful
- [ ] Invariants are listed
- [ ] Out-of-scope items are documented
- [ ] Scenarios can be translated to CLI integration tests
