# Reusable Guidance for Claude Code Projects

This directory contains project-agnostic guidance extracted from the Timesheet application analysis. These patterns and templates can be applied to other projects.

## Contents

1. **[claude-md-template.md](./claude-md-template.md)** - Template for project CLAUDE.md files with enforcement-focused structure
2. **[agent-templates.md](./agent-templates.md)** - Reusable agent definitions for common needs
3. **[integration-test-framework.md](./integration-test-framework.md)** - CLI-based integration test patterns
4. **[principles-checklist.md](./principles-checklist.md)** - Common architectural principles to consider
5. **[anti-patterns-catalog.md](./anti-patterns-catalog.md)** - Technology-agnostic anti-patterns

## Key Insights

### 1. Documentation Alone Is Insufficient

**The Problem:** Claude Code (and developers) ignore documented principles under time pressure or when context is lost.

**The Solution:** Enforcement mechanisms at multiple levels:
- Pre-commit hooks for immediate feedback
- CI checks for PR validation
- Agent-based review for nuanced evaluation
- Integration tests as runtime guardrails

### 2. Unit Tests Don't Catch Stakeholder Concerns

**The Problem:** Unit tests verify code correctness, not stakeholder intent. Features can pass all tests while violating PRD requirements.

**The Solution:**
- Express PRDs as testable scenarios (GIVEN/WHEN/THEN)
- Create CLI-based integration tests against APIs
- Distinguish "code correctness tests" from "stakeholder intent tests"

### 3. Architectural Drift Accumulates Silently

**The Problem:** Small violations compound. Without continuous review, patterns degrade over time.

**The Solution:**
- Devil's Advocate agent that challenges decisions
- Regular principle enforcement scans
- Project map and lexicon to maintain shared understanding

## How to Use These Templates

### For a New Project

1. Copy `claude-md-template.md` to `.claude/CLAUDE.md`
2. Customize the architectural principles section
3. Add project-specific patterns and anti-patterns
4. Set up basic enforcement hooks

### For an Existing Project

1. Run the analysis approach from the Timesheet project:
   - Analyze commits for patterns
   - Analyze issues for decisions
   - Examine architecture
   - Review code patterns
2. Use the synthesis template to combine findings
3. Codify into CLAUDE.md and agents

### Adapting Agent Templates

1. Copy relevant agents from `agent-templates.md`
2. Update domain-specific references
3. Customize evaluation criteria
4. Add project-specific checks
