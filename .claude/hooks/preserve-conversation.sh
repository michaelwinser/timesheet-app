#!/bin/bash
# Preserve Claude Code conversation for later analysis
#
# This hook captures conversation data after significant operations.
# Conversations are stored in .claude/conversations/ with timestamps.
#
# To enable as a PostToolUse hook, add to settings.json:
# "hooks": {
#   "PostToolUse": [{
#     "matcher": ".*",
#     "hooks": [{"type": "command", "command": ".claude/hooks/preserve-conversation.sh"}]
#   }]
# }

CONVERSATIONS_DIR=".claude/conversations"
mkdir -p "$CONVERSATIONS_DIR"

# Generate filename with timestamp
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
SESSION_ID="${CLAUDE_SESSION_ID:-unknown}"
CONVERSATION_FILE="$CONVERSATIONS_DIR/$TIMESTAMP-$SESSION_ID.jsonl"

# Capture environment context
cat >> "$CONVERSATION_FILE" << EOF
{"timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)", "type": "context", "data": {
  "working_dir": "$PWD",
  "git_branch": "$(git branch --show-current 2>/dev/null || echo 'unknown')",
  "git_status": "$(git status --porcelain 2>/dev/null | head -20 | tr '\n' '|')"
}}
EOF

# Note: Full conversation capture requires Claude Code API integration
# This script provides infrastructure; actual conversation data would need
# to be passed via environment or stdin from Claude Code hooks

echo "Conversation context saved to $CONVERSATION_FILE"
