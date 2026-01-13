# Principle Exceptions

This document tracks legitimate exceptions to project principles. Exceptions require:
1. Clear rationale
2. Code annotation
3. Entry in this file
4. Periodic review

---

## Exception Process

### Requesting an Exception

1. Create issue explaining why the principle cannot be followed
2. Get stakeholder approval
3. Add code annotation: `// EXCEPTION: [issue-#] - [brief reason]`
4. Add entry to this file
5. Update audit script to skip annotated code (if needed)

### Reviewing Exceptions

- Review quarterly
- Ask: Is this still valid? Has the situation changed?
- Remove stale exceptions

---

## Active Exceptions

### Commit Message Format

**Scope:** Historical commits (before 2026-01-13)
**Rationale:** Rewriting git history is destructive and provides minimal value. Enforce format going forward only.
**Audit behavior:** Only check recent commits (last 20)

### [Future exceptions go here]

---

## Expired Exceptions

*None yet*

---

## Exception Categories

### Technical Debt
Exceptions due to time constraints. Should have remediation plan.

### False Positive
Audit check flags code that doesn't actually violate the principle. Should lead to audit refinement.

### Legitimate Deviation
The principle doesn't apply to this specific case. Document why.

### External Constraint
Third-party code or API requires the pattern. Can't be changed.
