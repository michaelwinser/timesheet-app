# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for the Timesheet App v2.

## What is an ADR?

An ADR documents a significant architectural decision, including:
- **Context** - Why we needed to make this decision
- **Decision** - What we decided
- **Consequences** - What happens as a result (good and bad)

## Index

| # | Title | Status |
|---|-------|--------|
| [001](001-time-entry-per-day.md) | One TimeEntry Per Project Per Day | Accepted |
| [002](002-billing-periods.md) | Billing Periods for Rate Management | Proposed |
| [003](003-scoring-classification.md) | Scoring-Based Classification | Accepted |

## Status Values

- **Proposed** - Under discussion
- **Accepted** - Approved for implementation
- **Deprecated** - No longer applicable
- **Superseded** - Replaced by another ADR

## Creating a New ADR

1. Copy the template below
2. Number sequentially (e.g., `003-your-decision.md`)
3. Fill in the sections
4. Add to the index above

## Template

```markdown
# ADR NNN: Title

## Status

Proposed

## Context

What is the issue we're seeing that motivates this decision?

## Decision

What is the change we're proposing?

## Consequences

What becomes easier or harder as a result?
```
