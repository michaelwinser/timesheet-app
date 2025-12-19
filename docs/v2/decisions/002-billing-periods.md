# ADR 002: Billing Periods for Rate Management

## Status

Proposed

## Context

In v1, a Project has a single `bill_rate`. This causes problems:

1. **Rate changes** - When rates change, historical entries use the wrong rate
2. **Invoice boundaries** - No clear way to define "uninvoiced" time
3. **Project lifecycle** - No formal start/end dates

## Decision

Introduce **BillingPeriod** as a first-class entity:

```
BillingPeriod:
  - project_id
  - starts_on (date)
  - ends_on (date, nullable for ongoing)
  - hourly_rate (decimal)
```

- Billable projects have one or more BillingPeriods
- Periods must not overlap for the same project
- Time entries are matched to periods by date
- Invoices reference a specific BillingPeriod

## Consequences

### Positive

- **Rate history** - Can change rates without affecting past invoices
- **Clear boundaries** - "Uninvoiced for this period" is unambiguous
- **Audit trail** - Each invoice knows exactly what rate it used
- **Project lifecycle** - Start/end dates emerge naturally from periods

### Negative

- **Complexity** - More entities to manage
- **UI work** - Need interface for managing periods
- **Edge cases** - Entries outside any period need handling

### Neutral

- Could migrate existing `bill_rate` to a single open-ended period
- Non-billable projects simply have no periods

## Implementation Notes

- Validation: periods for a project must not overlap
- Default behavior: if no period covers a date, entry is unbillable (or use most recent?)
- Consider: should `ends_on = null` mean "ongoing" or require explicit end?
