# Enforcement Progression Plan

## Philosophy

Move from **Audit → Warn → Block** for each enforcement mechanism. This:
- Establishes baseline of existing violations
- Allows time to fix patterns without breaking workflow
- Builds confidence in check accuracy before blocking
- Avoids false-positive disruption

---

## Current State (Baseline)

| Check | Current Level | Location |
|-------|--------------|----------|
| State sync pattern | Warn (inconsistent) | `.claude/hooks/check-state-sync.sh` |
| Commit message format | None | - |
| Generated code freshness | None | - |
| Migration location | None | - |
| Principle review | None | - |

---

## Phase 1: Audit (Current → +1 week)

**Goal:** Establish baseline. Run checks, log results, don't interrupt workflow.

### 1.1 Create Audit Script

A single script that runs all checks and produces a report.

**Location:** `scripts/audit-principles.sh`

**Checks:**
- State sync violations in Svelte files
- SQL in handler files
- Migration files outside database.go
- Generated code freshness
- Large components (>500 lines)
- Commit message format (recent commits)

**Output:** Markdown report in `docs/analysis/audit-reports/`

### 1.2 Run Weekly Audits

- Manual execution initially
- Review results to calibrate checks
- Identify false positives to fix
- Track violation trends

### 1.3 Success Criteria for Phase 2

- [ ] Audit script runs without errors
- [ ] False positive rate < 10%
- [ ] Existing violations cataloged
- [ ] Fix plan for critical violations

---

## Phase 2: Warn (+1 week → +3 weeks)

**Goal:** Integrate warnings into workflow. Developers see issues but aren't blocked.

### 2.1 Pre-Commit Hook (Warning Mode)

Install hook that runs checks and warns but exits 0.

**Location:** `.git/hooks/pre-commit`

**Behavior:**
- Run state sync check on staged .svelte files
- Run commit message check
- Print warnings with color
- Always exit 0 (allow commit)

### 2.2 Claude Code Hook Enhancement

Update `.claude/settings.json` to warn on more patterns:
- State sync (already exists, ensure consistent)
- Large file edits (>500 lines)
- Handler SQL patterns

**Behavior:** Log warnings but don't BLOCK.

### 2.3 CI Pipeline (Report Only)

Add GitHub Actions workflow that:
- Runs audit script on PRs
- Posts results as PR comment
- Does NOT block merge

### 2.4 Success Criteria for Phase 3

- [ ] Warning hooks installed and running
- [ ] CI pipeline reporting on PRs
- [ ] No new violations being introduced (or quickly fixed)
- [ ] Existing violation count trending down
- [ ] Team familiar with warning messages

---

## Phase 3: Block (+3 weeks → +5 weeks)

**Goal:** Enforce principles. Violations prevent commit/merge.

### 3.1 Pre-Commit Hook (Blocking Mode)

Update hook to exit 1 on violations:
- State sync violations → block
- Bad commit message format → block
- SQL in handlers → block

**Escape hatch:** `git commit --no-verify` for emergencies (logged).

### 3.2 CI Pipeline (Required Check)

Configure GitHub branch protection:
- Require audit check to pass
- Block merge on violations

### 3.3 Claude Code Hook (Blocking Mode)

Update hooks to respond BLOCK instead of warn for critical patterns.

### 3.4 Success Criteria

- [ ] Zero new violations reaching main branch
- [ ] Escape hatch usage < 5% of commits
- [ ] No false positive blocks
- [ ] Team understands and accepts enforcement

---

## Implementation Details

### Audit Script Structure

```bash
#!/bin/bash
# scripts/audit-principles.sh

REPORT_DIR="docs/analysis/audit-reports"
REPORT_FILE="$REPORT_DIR/$(date +%Y%m%d).md"

mkdir -p "$REPORT_DIR"

echo "# Principle Audit Report - $(date +%Y-%m-%d)" > "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# Check 1: State sync violations
echo "## State Synchronization" >> "$REPORT_FILE"
# ... grep commands ...

# Check 2: Handler layering
echo "## Handler Layering" >> "$REPORT_FILE"
# ... grep commands ...

# Summary
echo "## Summary" >> "$REPORT_FILE"
# ... counts ...
```

### Pre-Commit Hook Modes

```bash
#!/bin/bash
# .git/hooks/pre-commit

MODE="${ENFORCEMENT_MODE:-warn}"  # audit | warn | block

run_checks() {
    # ... check logic ...
    return $violations_found
}

run_checks
result=$?

case $MODE in
    audit)
        # Log only
        exit 0
        ;;
    warn)
        # Print warnings, allow commit
        if [ $result -ne 0 ]; then
            echo "⚠️  Warnings found (not blocking)"
        fi
        exit 0
        ;;
    block)
        # Block on violations
        if [ $result -ne 0 ]; then
            echo "🛑 Violations found - commit blocked"
            echo "Use 'git commit --no-verify' to bypass (discouraged)"
            exit 1
        fi
        exit 0
        ;;
esac
```

### CI Workflow Progression

**Phase 2 (Report Only):**
```yaml
- name: Audit principles
  run: ./scripts/audit-principles.sh
  continue-on-error: true

- name: Comment on PR
  uses: actions/github-script@v6
  with:
    script: |
      // Post audit results as comment
```

**Phase 3 (Required):**
```yaml
- name: Audit principles
  run: ./scripts/audit-principles.sh
  # No continue-on-error - will fail PR
```

---

## Tracking Progress

### Weekly Audit Metrics

Track in `docs/analysis/audit-metrics.md`:

| Date | State Sync | Handler SQL | Large Files | Total |
|------|-----------|-------------|-------------|-------|
| 2026-01-13 | ? | ? | ? | ? |
| 2026-01-20 | | | | |

### Phase Transition Checklist

**Audit → Warn:**
- [ ] Audit script stable
- [ ] False positive rate acceptable
- [ ] Team notified of upcoming warnings

**Warn → Block:**
- [ ] No new violations in 2 weeks
- [ ] Existing violations fixed or documented exceptions
- [ ] Team consent to enforcement
- [ ] Escape hatch documented

---

## Exception Process

For legitimate violations:

1. **Document** in `docs/exceptions.md` with rationale
2. **Annotate** code with `// EXCEPTION: [reason]`
3. **Update** check to skip annotated code
4. **Review** exceptions quarterly

---

## Rollback Plan

If blocking causes problems:

1. Change `ENFORCEMENT_MODE=warn` in environment
2. CI: Set `continue-on-error: true`
3. Investigate and fix check accuracy
4. Re-enable blocking when ready
