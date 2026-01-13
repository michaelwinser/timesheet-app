#!/bin/bash
# Test configuration

# Server configuration
export API_BASE="${API_BASE:-http://localhost:8080}"
export TEST_API_KEY="${TEST_API_KEY:-test-api-key}"

# Test user configuration
export TEST_USER_EMAIL="${TEST_USER_EMAIL:-test@example.com}"

# Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export INTEGRATION_ROOT="$(dirname "$SCRIPT_DIR")"
export PROJECT_ROOT="$(dirname "$(dirname "$INTEGRATION_ROOT")")"

# Load libraries
source "$SCRIPT_DIR/api.sh"
source "$SCRIPT_DIR/assert.sh"

# Test data helpers
generate_unique_id() {
    echo "test-$(date +%s)-$RANDOM"
}

generate_test_date() {
    # Returns a date in the format YYYY-MM-DD
    # Default to tomorrow to avoid conflicts with real data
    date -v+1d +%Y-%m-%d 2>/dev/null || date -d "+1 day" +%Y-%m-%d
}

# Cleanup tracking
CLEANUP_IDS=()

register_for_cleanup() {
    local type="$1"
    local id="$2"
    CLEANUP_IDS+=("$type:$id")
}

cleanup_test_data() {
    echo "Cleaning up test data..."
    for item in "${CLEANUP_IDS[@]}"; do
        local type="${item%%:*}"
        local id="${item#*:}"
        case "$type" in
            project)
                api_delete "/api/projects/$id" > /dev/null 2>&1
                ;;
            rule)
                api_delete "/api/rules/$id" > /dev/null 2>&1
                ;;
            invoice)
                api_delete "/api/invoices/$id" > /dev/null 2>&1
                ;;
            *)
                echo "Unknown cleanup type: $type"
                ;;
        esac
    done
    CLEANUP_IDS=()
    echo "Cleanup complete"
}

# Trap to ensure cleanup on exit
trap cleanup_test_data EXIT
