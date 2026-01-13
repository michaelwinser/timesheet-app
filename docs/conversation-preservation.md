# Conversation Preservation Strategy

## Purpose

Automatically capture Claude Code conversations and commands for later analysis. This enables:
- Post-mortem analysis of decisions that led to problems
- Pattern recognition in prompts that cause architectural drift
- Training data for improving guidance and agents
- Audit trail for understanding why code was written

---

## Approaches

### Option 1: Claude Code Session Export (Recommended)

Claude Code supports exporting session history. Automate this via:

```bash
# After each session, export the conversation
# Add to your shell profile or post-session script

claude-code export-session --output ".claude/conversations/$(date +%Y%m%d-%H%M%S).json"
```

**Pros:**
- Complete conversation including tool calls and responses
- Structured JSON format
- No hook overhead

**Cons:**
- Requires manual or script invocation
- Post-hoc, not real-time

### Option 2: Git Notes for Commits

Attach conversation context to commits:

```bash
# Post-commit hook that prompts for context
# .git/hooks/post-commit

read -p "Brief description of Claude Code session for this commit: " description
git notes add -m "$description"
```

**Pros:**
- Links conversation context to specific commits
- Integrated with git history

**Cons:**
- Manual input required
- Only captures summary, not full conversation

### Option 3: Hook-Based Logging (Partial)

Use Claude Code hooks to log significant operations:

```json
{
  "hooks": {
    "PostToolUse": [{
      "matcher": "Edit|Write",
      "hooks": [{
        "type": "command",
        "command": ".claude/hooks/log-operation.sh"
      }]
    }]
  }
}
```

**Pros:**
- Automatic capture of file modifications
- Real-time logging

**Cons:**
- Only captures tool use, not full conversation
- Doesn't capture decision reasoning

---

## Recommended Implementation

### 1. Session Export Script

Create `scripts/save-claude-session.sh`:

```bash
#!/bin/bash
# Save current Claude Code session with context

SESSIONS_DIR=".claude/sessions"
mkdir -p "$SESSIONS_DIR"

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
BRANCH=$(git branch --show-current 2>/dev/null || echo "unknown")
SESSION_FILE="$SESSIONS_DIR/${TIMESTAMP}-${BRANCH}.json"

# Export session (if Claude Code supports this)
# claude-code export-session --output "$SESSION_FILE"

# Fallback: prompt user to paste session summary
if [ ! -f "$SESSION_FILE" ]; then
    echo "Claude Code session export not available."
    echo "Please provide a session summary (Ctrl-D to finish):"
    cat > "$SESSIONS_DIR/${TIMESTAMP}-${BRANCH}.md"
fi

# Add git context
git log -5 --oneline > "$SESSIONS_DIR/${TIMESTAMP}-${BRANCH}.git-context"

echo "Session saved to $SESSIONS_DIR/"
```

### 2. Post-Commit Hook

Create `.git/hooks/post-commit`:

```bash
#!/bin/bash
# Prompt for Claude Code session context on commits

# Only prompt if this looks like a Claude-assisted commit
if git log -1 --format=%b | grep -q "Co-Authored-By.*Claude"; then
    echo ""
    echo "This commit was co-authored with Claude Code."
    echo "Add brief notes about the session? (Enter to skip, or type notes):"
    read -r notes
    if [ -n "$notes" ]; then
        git notes add -m "Claude session notes: $notes"
    fi
fi
```

### 3. Conversation Log Structure

Store conversations in `.claude/sessions/` with this structure:

```
.claude/sessions/
├── 20260113-143022-main.json      # Full session export (when available)
├── 20260113-143022-main.md        # Manual session summary
├── 20260113-143022-main.git-context  # Related git history
└── analysis/
    └── 20260113-weekly-review.md  # Periodic analysis of sessions
```

### 4. Analysis Workflow

Weekly review of sessions to identify:

```markdown
## Weekly Session Review

### Sessions Analyzed
- 20260113-143022: Feature X implementation
- 20260112-091500: Bug fix Y

### Patterns Observed

#### Successful Patterns
- [What prompts/approaches worked well]

#### Problematic Patterns
- [What led to issues]
- [Prompts that caused architectural drift]

### Recommendations
- [Updates to CLAUDE.md]
- [New agent definitions]
- [Prompt improvements]
```

---

## Integration with Principles Analysis

Preserved conversations can be analyzed to:

1. **Identify drift patterns:** Find prompts that bypassed architectural principles
2. **Improve CLAUDE.md:** Add explicit guidance for recurring issues
3. **Train agents:** Use problematic sessions as test cases for review agents
4. **Measure effectiveness:** Track whether enforcement mechanisms catch issues

---

## Privacy and Security

- Session files should be in `.gitignore` if they contain sensitive data
- Consider separate storage for sessions with credentials or secrets
- Implement retention policy (e.g., delete sessions older than 90 days)

Add to `.gitignore`:
```
.claude/sessions/
.claude/conversations/
```

---

## Next Steps

1. Create the directory structure
2. Add the save script to `scripts/`
3. Install the post-commit hook
4. Establish weekly review cadence
