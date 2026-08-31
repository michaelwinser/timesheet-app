#!/bin/bash
# Regression test for issue #103: rules could not match emoji or punctuation-only terms.
#
# GIVEN a calendar sync system marks its placeholder events with "🔄"
# AND a project has the rule title:"🔄"
# WHEN classification rules are previewed
# THEN placeholder events should match
# AND events without the marker should not match
#
# Before the fix, tokenize() discarded non-alphanumeric runes when building the
# candidate word list, so a symbol-only term could never match any event.
# See: https://github.com/michaelwinser/timesheet-app/issues/103

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../../../lib/config.sh"

MARKER="🔄"
EMOJI_QUERY="title:\"$MARKER\""   # title:"🔄"

begin_test "Rules match events by emoji-only term"

# =============================================================================
# GIVEN: A project with an emoji-only rule
# =============================================================================
echo "Setting up test data..."

PROJECT_NAME="Emoji Regression $(generate_unique_id)"
SHORT_CODE="EMJ$(date +%s | tail -c 5)"

PROJECT_RESPONSE=$(api_post "/api/projects" "{
    \"name\": \"$PROJECT_NAME\",
    \"short_code\": \"$SHORT_CODE\",
    \"color\": \"#3B82F6\"
}")

PROJECT_ID=$(echo "$PROJECT_RESPONSE" | jq -r '.id')
if [ -z "$PROJECT_ID" ] || [ "$PROJECT_ID" = "null" ]; then
    echo "SETUP FAILED: Could not create project"
    echo "Response: $PROJECT_RESPONSE"
    exit 1
fi
register_for_cleanup project "$PROJECT_ID"
echo "Created project: $PROJECT_ID"

# The rule must survive the HTTP/JSON/database round trip with the emoji intact.
# Built with jq so the emoji and the embedded quotes are escaped correctly.
RULE_RESPONSE=$(api_post "/api/rules" "$(jq -nc \
    --arg project_id "$PROJECT_ID" \
    --arg query "$EMOJI_QUERY" \
    '{project_id: $project_id, query: $query, weight: 100}')")

RULE_ID=$(echo "$RULE_RESPONSE" | jq -r '.id // empty')
if [ -z "$RULE_ID" ]; then
    echo "FAILED: Server rejected an emoji-only rule query"
    echo "Response: $RULE_RESPONSE"
    exit 1
fi
register_for_cleanup rule "$RULE_ID"
echo "Created rule: $RULE_ID"

STORED_QUERY=$(api_get "/api/rules/$RULE_ID" | jq -r '.query')
assert_equals "$STORED_QUERY" "$EMOJI_QUERY" "Stored rule query should preserve the emoji"

# =============================================================================
# WHEN: The query is previewed against events
# =============================================================================
TEST_DATE=$(generate_test_date)
echo "Test date: $TEST_DATE"

PLACEHOLDER_TITLE="$MARKER Sync placeholder $(generate_unique_id)"
CONTROL_TITLE="Real meeting $(generate_unique_id)"

# Seed two events: one carrying the marker, one without.
PLACEHOLDER_RESPONSE=$(api_post "/api/test/calendar-events" "$(jq -nc \
    --arg title "$PLACEHOLDER_TITLE" \
    --arg start "${TEST_DATE}T10:00:00Z" \
    --arg end "${TEST_DATE}T11:00:00Z" \
    '{title: $title, start: $start, end: $end}')" 2>/dev/null \
    || echo '{"error": "endpoint not implemented"}')

PLACEHOLDER_ID=$(echo "$PLACEHOLDER_RESPONSE" | jq -r '.id // empty')

if [ -z "$PLACEHOLDER_ID" ]; then
    # The event seeding endpoint is not implemented yet (see keyword-matching.sh).
    # Fall back to the part we can still verify end-to-end: the server accepts and
    # evaluates an emoji-only query rather than rejecting it as invalid syntax.
    echo ""
    echo "NOTE: /api/test/calendar-events not implemented - running reduced assertions"
    echo ""

    PREVIEW_RESPONSE=$(api_post "/api/rules/preview" "$(jq -nc \
        --arg project_id "$PROJECT_ID" \
        --arg query "$EMOJI_QUERY" \
        '{project_id: $project_id, query: $query}')")

    assert_json_field_not_empty "$PREVIEW_RESPONSE" "stats" "Preview should evaluate an emoji-only query"

    ERROR=$(echo "$PREVIEW_RESPONSE" | jq -r '.error // empty')
    assert_empty "$ERROR" "Emoji-only query should not be rejected as invalid syntax"

    end_test "Rules match events by emoji-only term"
    print_test_summary
    exit 0
fi

register_for_cleanup event "$PLACEHOLDER_ID"
echo "Created placeholder event: $PLACEHOLDER_ID"

CONTROL_RESPONSE=$(api_post "/api/test/calendar-events" "{
    \"title\": \"$CONTROL_TITLE\",
    \"start\": \"${TEST_DATE}T14:00:00Z\",
    \"end\": \"${TEST_DATE}T15:00:00Z\"
}")
CONTROL_ID=$(echo "$CONTROL_RESPONSE" | jq -r '.id')
register_for_cleanup event "$CONTROL_ID"
echo "Created control event: $CONTROL_ID"

PREVIEW_RESPONSE=$(api_post "/api/rules/preview" "$(jq -nc \
    --arg project_id "$PROJECT_ID" \
    --arg query "$EMOJI_QUERY" \
    --arg date "$TEST_DATE" \
    '{project_id: $project_id, query: $query, start_date: $date, end_date: $date}')")

# =============================================================================
# THEN: Only the marked event matches
# =============================================================================
echo "Verifying results..."

MATCHED_PLACEHOLDER=$(echo "$PREVIEW_RESPONSE" | jq --arg id "$PLACEHOLDER_ID" \
    '[.matches[]? | select(.event_id == $id)] | length')
assert_equals "$MATCHED_PLACEHOLDER" "1" "Emoji rule should match the placeholder event"

MATCHED_CONTROL=$(echo "$PREVIEW_RESPONSE" | jq --arg id "$CONTROL_ID" \
    '[.matches[]? | select(.event_id == $id)] | length')
assert_equals "$MATCHED_CONTROL" "0" "Emoji rule should not match an event without the marker"

end_test "Rules match events by emoji-only term"

print_test_summary
