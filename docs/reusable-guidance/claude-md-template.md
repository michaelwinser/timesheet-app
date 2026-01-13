# CLAUDE.md Template for Enforcement-Focused Projects

Copy this template to `.claude/CLAUDE.md` and customize for your project.

---

```markdown
# Claude Collaboration Guidelines

## Communication Style

- **Push back when something isn't a good idea.** Flag concerns early, even when not explicitly asked. Be direct about trade-offs, complexity costs, and maintenance burden.

- **Bring expertise proactively.** Offer alternatives when you see better approaches rather than just executing the first viable path.

- **Ask clarifying questions.** If context is unclear or requirements seem underspecified, ask rather than making assumptions.

- **Be direct, not deferential.** Disagree when you have good reason to.

## After Raising Concerns

- Execute the user's decision once concerns have been aired
- Don't be contrarian for its own sake
- Respect that the user has project context you may lack

---

## MANDATORY: Before Writing Code

**STOP. Before writing any code, you MUST verify these constraints.** Documentation alone has proven insufficient - architectural drift occurs when these aren't enforced.

### Pre-Implementation Checklist

For ANY code change, answer these questions:

1. **[Constraint 1]:** [Question to answer]
   - If YES: [Required action]
   - If NO: [Alternative]

2. **[Constraint 2]:** [Question to answer]
   - If YES: [Required action]
   - If NO: [Alternative]

3. **[Constraint 3]:** [Question to answer]
   - If YES: [Required action]
   - If NO: [Alternative]

4. **New Feature (3+ files):** Is this a feature touching more than 3 files?
   - If YES: Discuss design approach BEFORE implementation

---

## Architectural Principles (MUST Follow)

### 1. [Principle Name]

[One sentence description]

**Pattern:**
[Code example or diagram]

**Violation:** [What NOT to do]

### 2. [Principle Name]

[One sentence description]

**Pattern:**
[Code example or diagram]

**Violation:** [What NOT to do]

### 3. [Principle Name]

[One sentence description]

**Pattern:**
[Code example or diagram]

**Violation:** [What NOT to do]

---

## [Technology-Specific Section] (e.g., "UI Patterns", "API Patterns")

### [Critical Pattern Name]

**This is a critical pattern.** [Explain why it matters]

**Anti-pattern (CAUSES BUGS):**
```[language]
// Bad code example
```

**Correct pattern (ALWAYS USE):**
```[language]
// Good code example
```

### [Section] Checklist

**When working on [area], verify:**

- [ ] [Check 1]
- [ ] [Check 2]
- [ ] [Check 3]

**Include this checklist in your response when modifying [area] code.**

---

## [Backend/Language-Specific Patterns]

### [Pattern 1]

[Description with code example]

### [Pattern 2]

[Description with code example]

---

## Commit Conventions

```
<Verb> <description> [(issue #N)]

[Optional body with bullet points]

[Fixes #N]

Co-Authored-By: Claude [Model] <noreply@anthropic.com>
```

**Approved verbs:** Add, Fix, Implement, Refactor, Update, Remove, Redesign, Improve

---

## Complexity Hotspots (Extra Vigilance Required)

These areas have historically caused the most issues. Apply extra scrutiny:

1. **[Area 1]** - [Why it's complex]
2. **[Area 2]** - [Why it's complex]
3. **[Area 3]** - [Why it's complex]

---

## Testing Philosophy

### Unit Tests vs. Stakeholder Intent Tests

**Unit tests** (code correctness): Verify code does what it says
**Integration tests** (stakeholder intent): Verify code does what users need

**PRDs should be expressed as testable scenarios:**

```markdown
GIVEN [setup conditions]
AND [more conditions]
WHEN [action is taken]
THEN [expected outcome]
AND [more outcomes]
```

**These scenarios should translate to integration tests against the API.**

---

## Reference Documents

- [Link to design docs]
- [Link to PRDs]
- [Link to analysis]
```
