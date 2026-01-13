#!/bin/bash
# Audit script for project principles
# Run this to establish baseline and track violations over time
#
# Usage: ./scripts/audit-principles.sh [--json]

set -e

# Colors
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
REPORT_DIR="$PROJECT_ROOT/docs/analysis/audit-reports"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
REPORT_FILE="$REPORT_DIR/$TIMESTAMP.md"
JSON_OUTPUT=false

# Parse arguments
if [ "$1" = "--json" ]; then
    JSON_OUTPUT=true
fi

# Counters
STATE_SYNC_VIOLATIONS=0
HANDLER_SQL_VIOLATIONS=0
MIGRATION_VIOLATIONS=0
LARGE_COMPONENT_VIOLATIONS=0
COMMIT_FORMAT_VIOLATIONS=0

# Output storage
STATE_SYNC_OUTPUT=""
HANDLER_SQL_OUTPUT=""
MIGRATION_OUTPUT=""
LARGE_COMPONENT_OUTPUT=""
COMMIT_FORMAT_OUTPUT=""

mkdir -p "$REPORT_DIR"

# ============================================================================
# Check Functions
# ============================================================================

check_state_sync() {
    echo -e "${BLUE}Checking state synchronization patterns...${NC}"

    # Look for $state with entity types (potential stale data issues)
    # Arrays are OK (they're the source of truth), but single objects are suspicious
    local matches=$(grep -rn '\$state<\(CalendarEvent\|Project\|TimeEntry\|Rule\|Invoice\)\s*|' "$PROJECT_ROOT/web/src" 2>/dev/null | grep -v node_modules | grep -v '\[\]' || true)

    if [ -n "$matches" ]; then
        STATE_SYNC_OUTPUT="$matches"
        STATE_SYNC_VIOLATIONS=$(echo "$matches" | wc -l | tr -d ' ')
    fi
}

check_handler_sql() {
    echo -e "${BLUE}Checking handler layering (SQL in handlers)...${NC}"

    # Look for SQL keywords in handler files
    local matches=$(grep -rln 'SELECT\|INSERT INTO\|UPDATE.*SET\|DELETE FROM' "$PROJECT_ROOT/service/internal/handler" 2>/dev/null || true)

    if [ -n "$matches" ]; then
        HANDLER_SQL_OUTPUT="$matches"
        HANDLER_SQL_VIOLATIONS=$(echo "$matches" | wc -l | tr -d ' ')
    fi
}

check_migration_location() {
    echo -e "${BLUE}Checking migration locations...${NC}"

    # Look for .sql files in service directory (should not exist)
    local sql_files=$(find "$PROJECT_ROOT/service" -name "*.sql" -type f 2>/dev/null || true)

    if [ -n "$sql_files" ]; then
        MIGRATION_OUTPUT="$sql_files"
        MIGRATION_VIOLATIONS=$(echo "$sql_files" | wc -l | tr -d ' ')
    fi

    # Check for migrations directory
    if [ -d "$PROJECT_ROOT/service/migrations" ] || [ -d "$PROJECT_ROOT/migrations" ]; then
        MIGRATION_OUTPUT="${MIGRATION_OUTPUT}
migrations/ directory exists - should be in database.go"
        MIGRATION_VIOLATIONS=$((MIGRATION_VIOLATIONS + 1))
    fi
}

check_large_components() {
    echo -e "${BLUE}Checking component sizes...${NC}"

    local threshold=500

    # Find large Svelte files
    while IFS= read -r file; do
        if [ -f "$file" ]; then
            local lines=$(wc -l < "$file" | tr -d ' ')
            if [ "$lines" -gt "$threshold" ]; then
                LARGE_COMPONENT_OUTPUT="${LARGE_COMPONENT_OUTPUT}
$file: $lines lines (threshold: $threshold)"
                LARGE_COMPONENT_VIOLATIONS=$((LARGE_COMPONENT_VIOLATIONS + 1))
            fi
        fi
    done < <(find "$PROJECT_ROOT/web/src" -name "*.svelte" -type f 2>/dev/null | grep -v node_modules)

    # Trim leading newline
    LARGE_COMPONENT_OUTPUT=$(echo "$LARGE_COMPONENT_OUTPUT" | sed '/^$/d')
}

check_commit_format() {
    echo -e "${BLUE}Checking recent commit message format...${NC}"

    # Approved verbs (imperative mood)
    local allowed_pattern="^(Add|Fix|Implement|Refactor|Update|Remove|Redesign|Improve) "

    # Check last 20 commits
    while IFS='|' read -r hash msg; do
        if [ -n "$hash" ]; then
            if ! echo "$msg" | grep -qE "$allowed_pattern"; then
                COMMIT_FORMAT_OUTPUT="${COMMIT_FORMAT_OUTPUT}
$hash: '$msg'"
                COMMIT_FORMAT_VIOLATIONS=$((COMMIT_FORMAT_VIOLATIONS + 1))
            fi
        fi
    done < <(git -C "$PROJECT_ROOT" log -20 --pretty=format:"%h|%s" 2>/dev/null)

    # Trim leading newline
    COMMIT_FORMAT_OUTPUT=$(echo "$COMMIT_FORMAT_OUTPUT" | sed '/^$/d')
}

# ============================================================================
# Main Execution
# ============================================================================

echo -e "${GREEN}=== Principle Audit Report ===${NC}"
echo "Date: $(date)"
echo "Project: $PROJECT_ROOT"
echo ""

# Run all checks
check_state_sync
check_handler_sql
check_migration_location
check_large_components
check_commit_format

# Calculate totals
TOTAL_VIOLATIONS=$((STATE_SYNC_VIOLATIONS + HANDLER_SQL_VIOLATIONS + MIGRATION_VIOLATIONS + LARGE_COMPONENT_VIOLATIONS + COMMIT_FORMAT_VIOLATIONS))

# ============================================================================
# Output
# ============================================================================

if [ "$JSON_OUTPUT" = true ]; then
    cat << EOF
{
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "total_violations": $TOTAL_VIOLATIONS,
  "checks": {
    "state_sync": {"violations": $STATE_SYNC_VIOLATIONS},
    "handler_sql": {"violations": $HANDLER_SQL_VIOLATIONS},
    "migration_location": {"violations": $MIGRATION_VIOLATIONS},
    "large_components": {"violations": $LARGE_COMPONENT_VIOLATIONS},
    "commit_format": {"violations": $COMMIT_FORMAT_VIOLATIONS}
  }
}
EOF
else
    # Generate Markdown report
    cat > "$REPORT_FILE" << EOF
# Principle Audit Report

**Date:** $(date)
**Generated by:** scripts/audit-principles.sh

## Summary

| Check | Violations | Status |
|-------|------------|--------|
| State Synchronization | $STATE_SYNC_VIOLATIONS | $([ $STATE_SYNC_VIOLATIONS -eq 0 ] && echo "✅" || echo "⚠️") |
| Handler Layering (SQL) | $HANDLER_SQL_VIOLATIONS | $([ $HANDLER_SQL_VIOLATIONS -eq 0 ] && echo "✅" || echo "⚠️") |
| Migration Location | $MIGRATION_VIOLATIONS | $([ $MIGRATION_VIOLATIONS -eq 0 ] && echo "✅" || echo "⚠️") |
| Large Components (>500 lines) | $LARGE_COMPONENT_VIOLATIONS | $([ $LARGE_COMPONENT_VIOLATIONS -eq 0 ] && echo "✅" || echo "⚠️") |
| Commit Format | $COMMIT_FORMAT_VIOLATIONS | $([ $COMMIT_FORMAT_VIOLATIONS -eq 0 ] && echo "✅" || echo "⚠️") |
| **Total** | **$TOTAL_VIOLATIONS** | $([ $TOTAL_VIOLATIONS -eq 0 ] && echo "✅" || echo "⚠️") |

---

## Details

### State Synchronization Violations

$([ -n "$STATE_SYNC_OUTPUT" ] && echo '```' && echo "$STATE_SYNC_OUTPUT" && echo '```' || echo "_No violations found._")

### Handler Layering Violations

$([ -n "$HANDLER_SQL_OUTPUT" ] && echo '```' && echo "$HANDLER_SQL_OUTPUT" && echo '```' || echo "_No violations found._")

### Migration Location Violations

$([ -n "$MIGRATION_OUTPUT" ] && echo '```' && echo "$MIGRATION_OUTPUT" && echo '```' || echo "_No violations found._")

### Large Components (>500 lines)

$([ -n "$LARGE_COMPONENT_OUTPUT" ] && echo '```' && echo "$LARGE_COMPONENT_OUTPUT" && echo '```' || echo "_No violations found._")

### Commit Format Violations (last 20 commits)

$([ -n "$COMMIT_FORMAT_OUTPUT" ] && echo '```' && echo "$COMMIT_FORMAT_OUTPUT" && echo '```' || echo "_No violations found._")

---

## Recommendations

$([ $STATE_SYNC_VIOLATIONS -gt 0 ] && echo "- **State Sync:** Review flagged files and refactor to use ID + \$derived pattern")
$([ $HANDLER_SQL_VIOLATIONS -gt 0 ] && echo "- **Handler SQL:** Move SQL queries to store layer")
$([ $MIGRATION_VIOLATIONS -gt 0 ] && echo "- **Migrations:** Move SQL to database.go")
$([ $LARGE_COMPONENT_VIOLATIONS -gt 0 ] && echo "- **Large Components:** Consider splitting components over 500 lines")
$([ $COMMIT_FORMAT_VIOLATIONS -gt 0 ] && echo "- **Commit Format:** Use imperative verbs: Add, Fix, Implement, Refactor, Update, Remove, Redesign, Improve")
$([ $TOTAL_VIOLATIONS -eq 0 ] && echo "_All checks passed!_")
EOF

    echo ""
    echo -e "${GREEN}=== Summary ===${NC}"
    echo ""
    echo "State Synchronization: $STATE_SYNC_VIOLATIONS violations"
    echo "Handler Layering:      $HANDLER_SQL_VIOLATIONS violations"
    echo "Migration Location:    $MIGRATION_VIOLATIONS violations"
    echo "Large Components:      $LARGE_COMPONENT_VIOLATIONS violations"
    echo "Commit Format:         $COMMIT_FORMAT_VIOLATIONS violations"
    echo "---"
    if [ $TOTAL_VIOLATIONS -gt 0 ]; then
        echo -e "Total:                 ${YELLOW}$TOTAL_VIOLATIONS${NC} violations"
    else
        echo -e "Total:                 ${GREEN}0${NC} violations"
    fi
    echo ""
    echo -e "Report saved to: ${BLUE}$REPORT_FILE${NC}"
fi

# Exit code based on violations (for CI integration)
# In audit mode, always exit 0
# In block mode, exit 1 if violations
exit 0
