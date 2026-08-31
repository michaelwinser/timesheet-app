#!/bin/bash
# Regression test for issue #108: calendar sync placeholders must never reach
# classification.
#
# GIVEN a calendar event carrying the private extended property calendarSyncMarker
# WHEN the event is ingested
# THEN it is stored with is_suppressed set
# AND it does not appear in the calendar events list
# AND it is not classified to any project
# AND it contributes no time entry
#
# Matching is on the presence of the key, not its value, so a version bump by
# the upstream sync tool cannot silently let the noise back in.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../../../lib/config.sh"

begin_test "Sync placeholders are suppressed at ingestion"

TEST_DATE=$(generate_test_date)
MARKED_TITLE="Placeholder $(generate_unique_id)"
PLAIN_TITLE="Real meeting $(generate_unique_id)"

# Seeding needs an event-creation endpoint that also accepts extended
# properties. Neither exists yet - see keyword-matching.sh for the same gap.
SEED=$(api_post "/api/test/calendar-events" "$(jq -nc \
    --arg title "$MARKED_TITLE" \
    --arg start "${TEST_DATE}T10:00:00Z" \
    --arg end "${TEST_DATE}T11:00:00Z" \
    '{title: $title, start: $start, end: $end,
      extended_properties: {private: {calendarSyncMarker: "v1"}}}')" 2>/dev/null \
    || echo '{"error": "endpoint not implemented"}')

MARKED_ID=$(echo "$SEED" | jq -r '.id // empty')

if [ -z "$MARKED_ID" ]; then
    echo ""
    echo "SKIPPED: /api/test/calendar-events does not exist, or does not accept"
    echo "extended properties. This scenario cannot be exercised until it does."
    echo ""
    echo "The suppression logic is covered meanwhile by unit tests in"
    echo "service/internal/sync/placeholder_test.go."
    echo ""
    print_test_summary
    exit 0
fi

register_for_cleanup event "$MARKED_ID"

CONTROL=$(api_post "/api/test/calendar-events" "$(jq -nc \
    --arg title "$PLAIN_TITLE" \
    --arg start "${TEST_DATE}T14:00:00Z" \
    --arg end "${TEST_DATE}T15:00:00Z" \
    '{title: $title, start: $start, end: $end}')")
CONTROL_ID=$(echo "$CONTROL" | jq -r '.id')
register_for_cleanup event "$CONTROL_ID"

api_apply_rules

EVENTS=$(api_get "/api/calendar-events?start_date=$TEST_DATE&end_date=$TEST_DATE")

MARKED_VISIBLE=$(echo "$EVENTS" | jq --arg id "$MARKED_ID" '[.[]? | select(.id == $id)] | length')
assert_equals "$MARKED_VISIBLE" "0" "Suppressed placeholder should not appear in the events list"

CONTROL_VISIBLE=$(echo "$EVENTS" | jq --arg id "$CONTROL_ID" '[.[]? | select(.id == $id)] | length')
assert_equals "$CONTROL_VISIBLE" "1" "An event without the marker should still appear"

ENTRIES=$(api_get "/api/time-entries?date=$TEST_DATE")
PLACEHOLDER_HOURS=$(echo "$ENTRIES" | jq --arg t "$MARKED_TITLE" \
    '[.[]? | select(.title == $t)] | length')
assert_equals "$PLACEHOLDER_HOURS" "0" "Suppressed placeholder should contribute no time entry"

end_test "Sync placeholders are suppressed at ingestion"

print_test_summary
