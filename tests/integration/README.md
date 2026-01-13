# Integration Test Framework

This directory contains CLI-based integration tests that verify stakeholder intent, not just code correctness.

## Philosophy

**Unit tests** verify that code does what it says.
**Integration tests** verify that code does what stakeholders need.

These tests are derived from PRD scenarios and test against the actual server API, verifying business requirements end-to-end.

## Directory Structure

```
tests/integration/
├── README.md              # This file
├── run-all.sh             # Run all integration tests
├── setup.sh               # Start test server, create test user
├── teardown.sh            # Cleanup test data
├── lib/
│   ├── api.sh             # API helper functions
│   ├── assert.sh          # Assertion helpers
│   └── config.sh          # Test configuration
└── scenarios/
    ├── classification/    # Classification system tests
    ├── time-entries/      # Time entry computation tests
    ├── invoicing/         # Invoicing workflow tests
    └── calendar-sync/     # Calendar synchronization tests
```

## Writing Tests

### 1. Start with a PRD Scenario

```markdown
GIVEN a user has connected their Google Calendar
AND a calendar event exists with title "Meeting with Acme Corp"
AND a project exists named "Acme Corp" with keyword rule "acme"
WHEN the classification rules are applied
THEN the event should be classified to "Acme Corp" project
AND a time entry should exist for that project on that date
```

### 2. Translate to a Test Script

```bash
#!/bin/bash
# Test: Event classified by keyword rule
# PRD: docs/prd-classification.md, Scenario 3

set -e
source "$(dirname "$0")/../lib/api.sh"
source "$(dirname "$0")/../lib/assert.sh"

echo "=== Test: Event classified by keyword rule ==="

# Setup
echo "Setting up test data..."
PROJECT_ID=$(api_create_project '{"name": "Acme Corp", "short_code": "ACME"}')
RULE_ID=$(api_create_rule "{\"project_id\": \"$PROJECT_ID\", \"query\": \"text:acme\", \"weight\": 100}")
EVENT_ID=$(api_create_test_event '{"title": "Meeting with Acme Corp", "start": "2026-01-15T10:00:00Z", "end": "2026-01-15T11:00:00Z"}')

# Execute
echo "Applying classification rules..."
api_post "/api/classification/apply" "{}"

# Verify
echo "Verifying results..."
EVENT=$(api_get "/api/calendar-events/$EVENT_ID")
CLASSIFIED_PROJECT=$(echo "$EVENT" | jq -r '.project_id')
assert_equals "$CLASSIFIED_PROJECT" "$PROJECT_ID" "Event should be classified to Acme Corp"

TIME_ENTRIES=$(api_get "/api/time-entries?date=2026-01-15&project_id=$PROJECT_ID")
ENTRY_COUNT=$(echo "$TIME_ENTRIES" | jq '. | length')
assert_equals "$ENTRY_COUNT" "1" "Should have one time entry for Acme Corp on that date"

# Cleanup
echo "Cleaning up..."
api_delete "/api/projects/$PROJECT_ID"

echo "=== PASSED ==="
```

## API Helper Functions

### lib/api.sh

```bash
#!/bin/bash
# API helper functions for integration tests

API_BASE="${API_BASE:-http://localhost:8080}"
API_KEY="${TEST_API_KEY:-test-api-key}"

api_get() {
    local endpoint="$1"
    curl -s -H "Authorization: Bearer $API_KEY" "$API_BASE$endpoint"
}

api_post() {
    local endpoint="$1"
    local data="$2"
    curl -s -X POST -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" -d "$data" "$API_BASE$endpoint"
}

api_delete() {
    local endpoint="$1"
    curl -s -X DELETE -H "Authorization: Bearer $API_KEY" "$API_BASE$endpoint"
}

api_create_project() {
    local data="$1"
    api_post "/api/projects" "$data" | jq -r '.id'
}

api_create_rule() {
    local data="$1"
    api_post "/api/rules" "$data" | jq -r '.id'
}

api_create_test_event() {
    local data="$1"
    api_post "/api/test/calendar-events" "$data" | jq -r '.id'
}
```

### lib/assert.sh

```bash
#!/bin/bash
# Assertion helpers for integration tests

assert_equals() {
    local actual="$1"
    local expected="$2"
    local message="${3:-Values should be equal}"

    if [ "$actual" != "$expected" ]; then
        echo "ASSERTION FAILED: $message"
        echo "  Expected: $expected"
        echo "  Actual:   $actual"
        exit 1
    fi
}

assert_not_empty() {
    local value="$1"
    local message="${2:-Value should not be empty}"

    if [ -z "$value" ] || [ "$value" = "null" ]; then
        echo "ASSERTION FAILED: $message"
        echo "  Value was empty or null"
        exit 1
    fi
}

assert_json_field() {
    local json="$1"
    local field="$2"
    local expected="$3"
    local message="${4:-JSON field should match}"

    local actual=$(echo "$json" | jq -r ".$field")
    assert_equals "$actual" "$expected" "$message"
}
```

## Running Tests

### Run All Tests

```bash
./tests/integration/run-all.sh
```

### Run Specific Scenario

```bash
./tests/integration/scenarios/classification/keyword-matching.sh
```

### Run with Custom Server

```bash
API_BASE=http://localhost:3000 ./tests/integration/run-all.sh
```

## Test Categories

### 1. Classification Tests (`scenarios/classification/`)

Tests for the rule-based classification system:
- Keyword matching
- Attendee matching
- Calendar name matching
- Confidence scoring
- Multi-rule conflicts

### 2. Time Entry Tests (`scenarios/time-entries/`)

Tests for time entry computation:
- One entry per project per day invariant
- Overlap calculation (union not sum)
- User edit preservation
- Staleness detection
- Ephemeral vs materialized

### 3. Invoicing Tests (`scenarios/invoicing/`)

Tests for invoicing workflows:
- Billing period creation
- Rate calculation
- Invoice locking
- Entry protection after invoicing

### 4. Calendar Sync Tests (`scenarios/calendar-sync/`)

Tests for calendar synchronization:
- Event creation and updates
- Incremental sync
- Deleted event handling
- Calendar selection filtering

## Adding New Tests

1. Identify the PRD scenario to test
2. Create a new `.sh` file in the appropriate `scenarios/` subdirectory
3. Follow the template above
4. Run the test locally to verify
5. Add to CI pipeline

## CI Integration

Add to `.github/workflows/test.yml`:

```yaml
integration-tests:
  runs-on: ubuntu-latest
  services:
    postgres:
      image: postgres:16
      env:
        POSTGRES_PASSWORD: test
      ports:
        - 5432:5432
  steps:
    - uses: actions/checkout@v4
    - name: Start server
      run: |
        cd service && go build -o server ./cmd/server
        ./server &
        sleep 5
    - name: Run integration tests
      run: ./tests/integration/run-all.sh
```

## Relationship to Unit Tests

| Aspect | Unit Tests | Integration Tests |
|--------|-----------|-------------------|
| **Purpose** | Code correctness | Stakeholder intent |
| **Speed** | Fast (ms) | Slower (seconds) |
| **Isolation** | Mocked dependencies | Real server |
| **Scope** | Single function/module | End-to-end workflow |
| **When to Write** | Always for new code | When PRD has scenarios |
| **Failure Meaning** | Code bug | Requirement violation |

Both are valuable. Unit tests catch implementation bugs. Integration tests catch requirement violations.
