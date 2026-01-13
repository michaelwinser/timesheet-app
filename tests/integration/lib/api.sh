#!/bin/bash
# API helper functions for integration tests

# Configuration
API_BASE="${API_BASE:-http://localhost:8080}"
API_KEY="${TEST_API_KEY:-test-api-key}"

# Core HTTP functions
api_get() {
    local endpoint="$1"
    curl -s -H "Authorization: Bearer $API_KEY" "$API_BASE$endpoint"
}

api_post() {
    local endpoint="$1"
    local data="$2"
    curl -s -X POST \
        -H "Authorization: Bearer $API_KEY" \
        -H "Content-Type: application/json" \
        -d "$data" \
        "$API_BASE$endpoint"
}

api_put() {
    local endpoint="$1"
    local data="$2"
    curl -s -X PUT \
        -H "Authorization: Bearer $API_KEY" \
        -H "Content-Type: application/json" \
        -d "$data" \
        "$API_BASE$endpoint"
}

api_patch() {
    local endpoint="$1"
    local data="$2"
    curl -s -X PATCH \
        -H "Authorization: Bearer $API_KEY" \
        -H "Content-Type: application/json" \
        -d "$data" \
        "$API_BASE$endpoint"
}

api_delete() {
    local endpoint="$1"
    curl -s -X DELETE -H "Authorization: Bearer $API_KEY" "$API_BASE$endpoint"
}

# Entity creation helpers
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
    # Note: This endpoint may need to be implemented for testing
    api_post "/api/test/calendar-events" "$data" | jq -r '.id'
}

api_create_time_entry() {
    local data="$1"
    api_post "/api/time-entries" "$data" | jq -r '.id'
}

api_create_invoice() {
    local data="$1"
    api_post "/api/invoices" "$data" | jq -r '.id'
}

# Classification helpers
api_classify_day() {
    local date="$1"
    api_post "/api/classification/day/$date" "{}"
}

api_apply_rules() {
    api_post "/api/rules/apply" "{}"
}

# Query helpers
api_get_events_for_date() {
    local date="$1"
    api_get "/api/calendar-events?date=$date"
}

api_get_time_entries_for_date() {
    local date="$1"
    api_get "/api/time-entries?date=$date"
}

api_get_project() {
    local id="$1"
    api_get "/api/projects/$id"
}

# Health check
api_health_check() {
    local response=$(curl -s -o /dev/null -w "%{http_code}" "$API_BASE/health")
    if [ "$response" = "200" ]; then
        return 0
    else
        return 1
    fi
}

# Wait for server
wait_for_server() {
    local max_attempts="${1:-30}"
    local attempt=1

    echo "Waiting for server at $API_BASE..."
    while [ $attempt -le $max_attempts ]; do
        if api_health_check; then
            echo "Server is ready!"
            return 0
        fi
        echo "Attempt $attempt/$max_attempts - server not ready, waiting..."
        sleep 1
        attempt=$((attempt + 1))
    done

    echo "Server failed to start after $max_attempts attempts"
    return 1
}
