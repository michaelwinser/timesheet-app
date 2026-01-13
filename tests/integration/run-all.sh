#!/bin/bash
# Run all integration tests

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/config.sh"

echo "========================================"
echo "Timesheet App Integration Tests"
echo "========================================"
echo "API Base: $API_BASE"
echo "========================================"

# Wait for server to be ready
if ! wait_for_server 30; then
    echo "ERROR: Server is not available"
    exit 1
fi

# Track overall results
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Run all test scenarios
run_scenario() {
    local scenario="$1"
    local name=$(basename "$scenario" .sh)

    echo ""
    echo "========================================"
    echo "Running: $name"
    echo "========================================"

    if bash "$scenario"; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
}

# Find and run all scenarios
for category in classification time-entries invoicing calendar-sync; do
    category_dir="$SCRIPT_DIR/scenarios/$category"
    if [ -d "$category_dir" ]; then
        for scenario in "$category_dir"/*.sh; do
            if [ -f "$scenario" ]; then
                run_scenario "$scenario"
            fi
        done
    fi
done

# Print summary
echo ""
echo "========================================"
echo "INTEGRATION TEST SUMMARY"
echo "========================================"
echo "Total Scenarios: $TOTAL_TESTS"
echo "Passed: $PASSED_TESTS"
echo "Failed: $FAILED_TESTS"
echo "========================================"

if [ $FAILED_TESTS -gt 0 ]; then
    echo "RESULT: FAILED"
    exit 1
else
    echo "RESULT: PASSED"
    exit 0
fi
