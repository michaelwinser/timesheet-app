#!/bin/bash
# Check for state synchronization anti-patterns in Svelte files
# This detects when objects from arrays are stored directly in $state instead of using IDs

set -e

# Colors for output
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get files to check - either from arguments or staged files
if [ $# -gt 0 ]; then
    FILES="$@"
else
    FILES=$(git diff --cached --name-only --diff-filter=ACM | grep '\.svelte$' || true)
fi

if [ -z "$FILES" ]; then
    exit 0
fi

ISSUES_FOUND=0

# Pattern 1: $state with complex type that looks like it holds array items
# e.g., let hoveredEvent = $state<CalendarEvent | null>(null);
# followed by assignment from an array item
check_state_pattern() {
    local file="$1"

    # Look for $state declarations with object types (contains spaces or complex generics)
    # that could be holding array items
    local suspicious_patterns=$(grep -n '\$state<[A-Z][a-zA-Z]*\s*|\s*null>' "$file" 2>/dev/null || true)

    if [ -n "$suspicious_patterns" ]; then
        # Check if these are combined with hover/selected/popup-like variable names
        echo "$suspicious_patterns" | while read -r line; do
            if echo "$line" | grep -qiE '(hovered|selected|active|current|popup|modal|detail)'; then
                echo -e "${YELLOW}POTENTIAL ISSUE${NC} in $file:"
                echo "  $line"
                echo "  -> Consider using ID-based derivation instead of storing object copies"
                echo "  -> See docs/ui-coding-guidelines.md 'State Synchronization' section"
                echo ""
                ISSUES_FOUND=1
            fi
        done
    fi
}

echo "Checking Svelte files for state synchronization patterns..."
echo ""

for file in $FILES; do
    if [ -f "$file" ]; then
        check_state_pattern "$file"
    fi
done

if [ $ISSUES_FOUND -eq 1 ]; then
    echo -e "${RED}Review the above patterns - they may cause stale data in popups/detail views${NC}"
    echo ""
    echo "Anti-pattern: let hoveredEvent = \$state<Event | null>(null)"
    echo "Good pattern: let hoveredEventId = \$state<string | null>(null)"
    echo "              const hoveredEvent = \$derived(events.find(e => e.id === hoveredEventId))"
    exit 0  # Warning only, don't block commits
fi

echo "No state synchronization issues detected."
