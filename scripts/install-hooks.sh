#!/bin/bash
# Install git hooks for principle enforcement
#
# Usage: ./scripts/install-hooks.sh [audit|warn|block]

set -e

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOOKS_DIR="$PROJECT_ROOT/.git/hooks"
MODE="${1:-warn}"

echo "Installing git hooks in $MODE mode..."

# Create pre-commit hook
cat > "$HOOKS_DIR/pre-commit" << 'HOOK_SCRIPT'
#!/bin/bash
# Pre-commit hook for principle enforcement
# Mode can be: audit, warn, block
# Set via: ENFORCEMENT_MODE=block or edit this file

MODE="${ENFORCEMENT_MODE:-MODE_PLACEHOLDER}"

# Colors
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m'

PROJECT_ROOT="$(git rev-parse --show-toplevel)"
VIOLATIONS=0

# ============================================================================
# Check: State Synchronization in Svelte files
# ============================================================================
check_state_sync() {
    local staged_svelte=$(git diff --cached --name-only --diff-filter=ACM | grep '\.svelte$' || true)

    if [ -z "$staged_svelte" ]; then
        return 0
    fi

    local violations=0

    for file in $staged_svelte; do
        if [ -f "$file" ]; then
            # Check for entity types in $state (potential stale data)
            local matches=$(grep -n '\$state<\(CalendarEvent\|Project\|TimeEntry\|Rule\|Invoice\)' "$file" 2>/dev/null || true)
            if [ -n "$matches" ]; then
                echo -e "${YELLOW}State sync warning in $file:${NC}"
                echo "$matches"
                echo "  → Consider using ID + \$derived pattern instead"
                echo ""
                violations=$((violations + 1))
            fi
        fi
    done

    return $violations
}

# ============================================================================
# Main
# ============================================================================

echo "Running pre-commit checks (mode: $MODE)..."
echo ""

check_state_sync
VIOLATIONS=$((VIOLATIONS + $?))

# Handle based on mode
if [ $VIOLATIONS -gt 0 ]; then
    case $MODE in
        audit)
            echo -e "${YELLOW}Audit: $VIOLATIONS potential issues found (not blocking)${NC}"
            exit 0
            ;;
        warn)
            echo -e "${YELLOW}Warning: $VIOLATIONS potential issues found${NC}"
            echo "Proceeding with commit. Consider fixing these issues."
            exit 0
            ;;
        block)
            echo -e "${RED}Blocked: $VIOLATIONS violations found${NC}"
            echo ""
            echo "Fix the violations above, or use 'git commit --no-verify' to bypass."
            echo "(Bypassing is logged and discouraged)"
            exit 1
            ;;
    esac
else
    echo -e "${GREEN}All checks passed!${NC}"
    exit 0
fi
HOOK_SCRIPT

# Replace placeholder with actual mode
sed -i.bak "s/MODE_PLACEHOLDER/$MODE/" "$HOOKS_DIR/pre-commit"
rm -f "$HOOKS_DIR/pre-commit.bak"

chmod +x "$HOOKS_DIR/pre-commit"

# Create commit-msg hook for format checking
cat > "$HOOKS_DIR/commit-msg" << 'HOOK_SCRIPT'
#!/bin/bash
# Commit message format checker

MODE="${ENFORCEMENT_MODE:-MODE_PLACEHOLDER}"

# Colors
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m'

COMMIT_MSG_FILE="$1"
COMMIT_MSG=$(cat "$COMMIT_MSG_FILE")
FIRST_LINE=$(head -n1 "$COMMIT_MSG_FILE")

ALLOWED_VERBS="Add|Fix|Implement|Refactor|Update|Remove|Redesign|Improve"

# Check format
if ! echo "$FIRST_LINE" | grep -qE "^($ALLOWED_VERBS) "; then
    echo -e "${YELLOW}Commit message format warning:${NC}"
    echo "  Message: $FIRST_LINE"
    echo "  Expected: Start with one of: Add, Fix, Implement, Refactor, Update, Remove, Redesign, Improve"
    echo ""

    case $MODE in
        audit|warn)
            echo "Proceeding anyway (mode: $MODE)"
            exit 0
            ;;
        block)
            echo -e "${RED}Commit blocked. Fix message format or use --no-verify${NC}"
            exit 1
            ;;
    esac
fi

exit 0
HOOK_SCRIPT

sed -i.bak "s/MODE_PLACEHOLDER/$MODE/" "$HOOKS_DIR/commit-msg"
rm -f "$HOOKS_DIR/commit-msg.bak"

chmod +x "$HOOKS_DIR/commit-msg"

echo ""
echo "Hooks installed successfully!"
echo ""
echo "Current mode: $MODE"
echo ""
echo "To change mode:"
echo "  export ENFORCEMENT_MODE=audit|warn|block"
echo "  # or re-run: ./scripts/install-hooks.sh [mode]"
echo ""
echo "Installed hooks:"
echo "  - pre-commit (state sync check)"
echo "  - commit-msg (format check)"
