#!/bin/bash
# Assertion helpers for integration tests

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Track test results
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Basic assertions
assert_equals() {
    local actual="$1"
    local expected="$2"
    local message="${3:-Values should be equal}"

    TESTS_RUN=$((TESTS_RUN + 1))

    if [ "$actual" = "$expected" ]; then
        echo -e "${GREEN}PASS${NC}: $message"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}FAIL${NC}: $message"
        echo "  Expected: $expected"
        echo "  Actual:   $actual"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

assert_not_equals() {
    local actual="$1"
    local not_expected="$2"
    local message="${3:-Values should not be equal}"

    TESTS_RUN=$((TESTS_RUN + 1))

    if [ "$actual" != "$not_expected" ]; then
        echo -e "${GREEN}PASS${NC}: $message"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}FAIL${NC}: $message"
        echo "  Should not be: $not_expected"
        echo "  Actual:        $actual"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

assert_not_empty() {
    local value="$1"
    local message="${2:-Value should not be empty}"

    TESTS_RUN=$((TESTS_RUN + 1))

    if [ -n "$value" ] && [ "$value" != "null" ]; then
        echo -e "${GREEN}PASS${NC}: $message"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}FAIL${NC}: $message"
        echo "  Value was empty or null"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

assert_empty() {
    local value="$1"
    local message="${2:-Value should be empty}"

    TESTS_RUN=$((TESTS_RUN + 1))

    if [ -z "$value" ] || [ "$value" = "null" ]; then
        echo -e "${GREEN}PASS${NC}: $message"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}FAIL${NC}: $message"
        echo "  Expected empty but got: $value"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

# JSON assertions
assert_json_field() {
    local json="$1"
    local field="$2"
    local expected="$3"
    local message="${4:-JSON field $field should equal $expected}"

    local actual=$(echo "$json" | jq -r ".$field")
    assert_equals "$actual" "$expected" "$message"
}

assert_json_field_not_empty() {
    local json="$1"
    local field="$2"
    local message="${3:-JSON field $field should not be empty}"

    local actual=$(echo "$json" | jq -r ".$field")
    assert_not_empty "$actual" "$message"
}

assert_json_array_length() {
    local json="$1"
    local expected="$2"
    local message="${3:-JSON array should have $expected items}"

    local actual=$(echo "$json" | jq '. | length')
    assert_equals "$actual" "$expected" "$message"
}

assert_json_contains() {
    local json="$1"
    local query="$2"
    local message="${3:-JSON should match query: $query}"

    TESTS_RUN=$((TESTS_RUN + 1))

    local result=$(echo "$json" | jq "$query")
    if [ "$result" = "true" ]; then
        echo -e "${GREEN}PASS${NC}: $message"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}FAIL${NC}: $message"
        echo "  Query: $query"
        echo "  Result: $result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

# Numeric assertions
assert_greater_than() {
    local actual="$1"
    local threshold="$2"
    local message="${3:-$actual should be greater than $threshold}"

    TESTS_RUN=$((TESTS_RUN + 1))

    if [ "$(echo "$actual > $threshold" | bc -l)" = "1" ]; then
        echo -e "${GREEN}PASS${NC}: $message"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}FAIL${NC}: $message"
        echo "  Actual: $actual"
        echo "  Threshold: $threshold"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

assert_less_than() {
    local actual="$1"
    local threshold="$2"
    local message="${3:-$actual should be less than $threshold}"

    TESTS_RUN=$((TESTS_RUN + 1))

    if [ "$(echo "$actual < $threshold" | bc -l)" = "1" ]; then
        echo -e "${GREEN}PASS${NC}: $message"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}FAIL${NC}: $message"
        echo "  Actual: $actual"
        echo "  Threshold: $threshold"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

# Test summary
print_test_summary() {
    echo ""
    echo "================================"
    echo "Test Summary"
    echo "================================"
    echo "Total:  $TESTS_RUN"
    echo -e "Passed: ${GREEN}$TESTS_PASSED${NC}"
    echo -e "Failed: ${RED}$TESTS_FAILED${NC}"
    echo "================================"

    if [ $TESTS_FAILED -gt 0 ]; then
        return 1
    else
        return 0
    fi
}

# Test lifecycle
begin_test() {
    local test_name="$1"
    echo ""
    echo "----------------------------------------"
    echo "TEST: $test_name"
    echo "----------------------------------------"
}

end_test() {
    local test_name="$1"
    echo "----------------------------------------"
}
