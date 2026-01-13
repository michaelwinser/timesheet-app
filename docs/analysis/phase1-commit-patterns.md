# Commit Patterns Analysis

*Analysis of 189 commits from 2025-12-06 to 2026-01-12*

## 1. Executive Summary

This codebase demonstrates a disciplined, documentation-first development approach with consistent commit conventions. Key observations:

- **Consistent imperative mood** for commit messages ("Add", "Fix", "Implement")
- **PRD-driven development** with design docs preceding major features
- **Phased implementation** for complex features (e.g., Rules v2 had 4 phases in one day)
- **Architecture rewrite** from Python/FastAPI to Go/SvelteKit occurred mid-project (Dec 19-Jan 2)
- **High bug fix ratio** (~21%) indicating active iteration based on real usage
- **Issue tracking integration** introduced late (Jan 12), with 9 commits referencing issues
- **Single author** (Michael Winser) with Claude Code co-authorship throughout

The development style favors **large, complete commits** over incremental work-in-progress commits. Most feature additions land in a single commit with full implementation.

---

## 2. Commit Message Conventions

### 2.1 Verb Prefixes (Action Words)

| Prefix | Count | Usage |
|--------|-------|-------|
| Add | 73 (39%) | New features, endpoints, documentation |
| Fix | 39 (21%) | Bug fixes |
| Implement | 12 (6%) | Complex features requiring multiple components |
| Remove | 7 (4%) | Cleanup, deprecation |
| Update | 6 (3%) | Documentation, roadmaps |
| Improve | 4 (2%) | Enhancements to existing features |
| Redesign | 3 (2%) | Major UI overhauls |
| Refactor | 2 (1%) | Code reorganization without behavior change |
| Other | 43 (22%) | Various (Extract, Filter, Skip, Create, etc.) |

### 2.2 Message Structure

**Subject Line Pattern:**
```
<Verb> <what> [for/with/to <context>] [(issue #N)]
```

**Examples:**
- `Add go-to-date feature with 'g' keyboard shortcut`
- `Fix invoiced time entries protection (issue #89)`
- `Implement smart sync with water marks and background scheduler`

**Key Observations:**
- **Imperative mood** used consistently (not "Added" or "Adds")
- **No conventional commit prefixes** (no `feat:`, `fix:`, `chore:` patterns)
- **Issue references** appear in parentheses at end when present
- **Subject lines are descriptive** - average ~50 characters, max ~70

### 2.3 Commit Body Usage

Significant commits include detailed bodies with:
- Feature bullet lists
- Technical implementation notes
- "Fixes #N" references for issue tracking
- Co-author attribution: `Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>`

**Example body structure:**
```
Short summary line

- Bullet point 1
- Bullet point 2
- Bullet point 3

Technical details paragraph explaining why or how.

Fixes #41

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
```

### 2.4 Issue Reference Patterns

Issue references appear in two styles:
1. **In subject**: `(issue #33)` - used for primary fixes
2. **In body**: `Fixes #41` - used for GitHub auto-close

Issue tracking was introduced late (first reference: Jan 12, 2026). The 9 issue-linked commits:
- issue #33: Spreadsheet export (2 commits)
- issue #82: Confidence threshold
- issue #89: Invoiced time entries protection
- issue #90: Modal z-index
- issue #91: Dark mode
- issue #92: Working location events
- issue #93: Calendar disconnection (2 commits)

---

## 3. Change Patterns

### 3.1 Commit Size Distribution

| Files Changed | Count | Typical Use Case |
|---------------|-------|------------------|
| 1-5 files | ~140 | Bug fixes, small features |
| 6-10 files | ~30 | Medium features, refactors |
| 11-20 files | ~15 | Major features |
| 20+ files | ~4 | Architecture changes, rewrites |

**Largest commits:**
- 77 files: SvelteKit frontend addition (architectural change)
- 24 files: MCP service removal
- 22 files: Dark mode implementation
- 21 files: Ephemeral time entries

### 3.2 Feature Implementation Patterns

**Single-Commit Features:**
Most features land in a single complete commit. Examples:
- `Add invoicing system with Google Sheets integration` (18 files, ~3,700 lines)
- `Implement classification system core components` (5 files, ~1,357 lines)
- `Add stacked bar chart view for time entries` (12 files)

**Multi-Phase Features:**
Complex features are broken into numbered phases:
- **Rules v2** (Dec 18): 4 commits in sequence
  - Phase 1: Foundation
  - Phase 2: Rules page with query-based UI
  - Phase 3: Project fingerprints
  - Phase 4: Query-based Create-from-Event modal

- **Calendar Sync v2** (Jan 8-9): 15+ commits over 2 days
  - PRD and design doc first
  - Database schema
  - Core implementation
  - Multiple bug fixes and refinements

### 3.3 Bug Fix Patterns

Bug fixes typically:
1. **Follow feature additions closely** - often same day or next day
2. **Reference specific symptoms** in subject line
3. **Include technical explanation** in body

**Common bug categories:**
- State synchronization issues (Svelte reactivity)
- PostgreSQL compatibility (types, JSONB)
- Timezone handling
- Dark mode styling
- Z-index layering

### 3.4 Documentation Patterns

Documentation commits follow a pattern:
1. **PRD first** - `Add PRD for X`
2. **Design doc** - `Add design doc for X` (sometimes combined with PRD)
3. **ADR for decisions** - `Add scoring-based classification architecture (ADR-003)`

PRD/Design doc commits precede implementation for:
- MCP server
- Invoicing feature
- Rules System v2
- Calendar Sync v2

---

## 4. Significant Commits

### 4.1 Architectural Milestones

| Hash | Date | Description |
|------|------|-------------|
| `9211352` | Dec 6 | Initial commit - Python/FastAPI/SQLite stack |
| `0651744` | Dec 17 | **PostgreSQL migration** - Multi-user support added |
| `2d8cd14` | Dec 19 | v2 architecture docs - Plans Go/SvelteKit rewrite |
| `48d0fb0` | Jan 2 | **SvelteKit frontend** - Major rewrite, 77 files changed |
| `a97bf41` | Jan 2 | Classification system core - Foundation for rules engine |
| `f6f3ac4` | Jan 8 | Smart sync with water marks - Core sync optimization |

### 4.2 Major Feature Additions

| Hash | Date | Feature | Impact |
|------|------|---------|--------|
| `aa8a8ef` | Dec 18 | Invoicing with Google Sheets | 18 files, +3,695 lines |
| `568cc30` | Dec 18 | MCP server (Phase 1) | Claude Desktop integration |
| `0506e7b` | Dec 18 | Rules v2 Phase 1 | Query-based classification |
| `967f8e7` | Jan 3 | Classification Hub | Bulk actions UI |
| `35adb4e` | Jan 5 | Dark mode | 22 files touched |
| `073b9c0` | Jan 7 | Skip rules | Event exclusion feature |
| `f59ccb9` | Jan 10 | Ephemeral time entries | Calendar sync gap handling |

### 4.3 Complex Bug Fixes

| Hash | Date | Issue | Complexity |
|------|------|-------|------------|
| `e57be4f` | Jan 12 | Invoiced entries protection | 12 files, schema migration |
| `7f5dc59` | Jan 10 | All-day event timezone | Cross-cutting concern |
| `8a915bd` | Jan 6 | Keyword word-boundary matching | Regex logic |
| `a6fe6dd` | Jan 6 | Reclassified event cleanup | State management |

### 4.4 Refactoring Commits

| Hash | Date | Refactor |
|------|------|----------|
| `edc8f44` | Jan 3 | Classifier to domain-agnostic types |
| `99973ee` | Jan 6 | Text color logic centralization |
| `951f861` | Jan 12 | Event layout algorithm extraction |
| `5c80d00` | Jan 8 | Centralized style system |

---

## 5. Work Stream Timeline

### Phase 1: Initial Build (Dec 6-7)
*Python/FastAPI prototype with core functionality*

- `9211352` Initial commit: Vertical Slice 1 complete
- `0f58b0b` Slice 2: Rules-based classification system
- `6fb359c` Add rule management UI
- `61add26` Add experimental LLM classification
- `dd431c4` Implement dedicated login page
- `fd5c490` Add DockerHub publishing support

### Phase 2: Production Readiness (Dec 17-18)
*PostgreSQL migration and major feature push*

- `0651744` **Migrate from SQLite to PostgreSQL**
- `c74ae92` Add project settings, did-not-attend flag
- `c7a02b6` Redesign event cards
- `568cc30` Implement MCP server (Phase 1)
- `aa8a8ef` **Add invoicing system with Google Sheets**
- `0506e7b-c40ff56` Rules v2 (Phases 1-4) - same day

### Phase 3: Go/SvelteKit Rewrite (Dec 19 - Jan 2)
*Complete architecture change*

- `2d8cd14` Add v2 architecture documentation
- `ded53d7` Add Go service scaffold
- `9fcf2bc` Add PostgreSQL persistence for auth
- `b54a325` Add Project and TimeEntry API endpoints
- `48d0fb0` **Add SvelteKit web frontend** (77 files)
- `132b10f` Add calendar data layer
- `1b48746` Add Google Calendar integration
- `a97bf41` **Implement classification system core**

### Phase 4: Classification & Rules (Jan 2-3)
*Building out classification features in new stack*

- `b016043` Add Rules API
- `d0a754b` Add Rules UI frontend
- `c091fe5` Auto-apply rules after calendar sync
- `967f8e7` Add Classification Hub
- `b56985e` Redesign calendar view
- `0ba3155` Improve event classification UI

### Phase 5: Calendar View Refinement (Jan 3-4)
*UI polish and viewport improvements*

- `2d535ec` Unify week calendar view
- `5df2d81` Fix scroll behavior
- `fa537d7` Expand calendar sync windows
- `4a85382` Make calendar viewport 50% taller

### Phase 6: Dark Mode & MCP (Jan 5)
*Theme system and Claude Code integration*

- `638ceaa` Add calendar: and text: search properties
- `35adb4e` **Implement dark mode**
- `d7c0413` Fix dark mode issues
- `f6f0af1` Add MCP service to docker-compose
- `ddd807a` Fix MCP OAuth compatibility

### Phase 7: Classification Polish (Jan 6)
*Bug fixes and classification improvements*

- `993302a` Add time entry deletion and recovery
- `63141aa` Redesign calendar UI header
- `a83c767` Add go-to-date feature
- `a502fd7` Add explain_classification MCP tool
- `8a915bd` Fix keywords matching substrings

### Phase 8: Calendar Sync v2 (Jan 7-9)
*Major sync architecture overhaul*

- `942e16e` Consolidate database migrations
- `4484079` Add calendar sync v2 PRD and design doc
- `d123300` Add database schema for sync v2
- `f6f3ac4` **Implement smart sync with water marks**
- `74b20a6` Add frontend sync loading overlay
- `06d2261`-`0ede75d` Multiple sync bug fixes
- `a059457` Extract CalendarClient interface
- `42a077b` Add calendar sync job queue
- `51dcf56` Add background job worker

### Phase 9: Time Entry Enhancements (Jan 10-11)
*Ephemeral entries and chart view*

- `f59ccb9` **Implement ephemeral time entries**
- `778eeae` Add is_all_day flag
- `1b3d2a7` Add stacked bar chart view
- `3a7957b` Add project summary bar

### Phase 10: Issue-Driven Fixes (Jan 12)
*Systematic bug fixing with issue tracking*

- 22 commits in one day
- First use of issue references (#33, #82, #89-94)
- Focus on edge cases and polish
- `e57be4f` Fix invoiced time entries protection
- `c7b650c` Spreadsheet export enhancements
- `521968b` Auto-save time entry edits

---

## 6. Recommendations

### 6.1 Patterns Worth Codifying

**Commit Message Format:**
```
<Verb> <description> [(issue #N)]

Optional detailed body with:
- Bullet points for changes
- Technical rationale

[Fixes #N]

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
```

**Approved Verbs:**
- `Add` - New features, files, or capabilities
- `Fix` - Bug fixes
- `Implement` - Complex multi-component features
- `Refactor` - Code reorganization (no behavior change)
- `Update` - Documentation, configuration changes
- `Remove` - Deletions, deprecations
- `Redesign` - Major UI overhauls
- `Improve` - Enhancements to existing features

### 6.2 Suggested Improvements

1. **Earlier issue tracking** - Consider using issues from project start for traceability

2. **Consistent phase naming** - The "Phase N" pattern in Rules v2 worked well; formalize it for multi-commit features

3. **Test commit pattern** - Tests appear in same commit as features; could be separated for reviewability:
   - `Implement X`
   - `Add tests for X`

4. **Changelog generation** - The verb-based prefixes enable automated changelog generation

### 6.3 Anti-Patterns to Avoid

1. **"Fix" without symptom** - Vague fix descriptions like `Fix bug` should include what was broken

2. **Large mixed commits** - Some commits mix features + fixes + refactoring; prefer separation

3. **Migration coupling** - Database migrations sometimes bundled with unrelated changes

### 6.4 Notable Conventions

- **Co-authorship attribution** - Claude Code contributions properly attributed
- **PRD-before-code** - Design documents precede major features
- **Same-day iteration** - Bug fixes often follow features within hours
- **Roadmap updates** - Phase completion explicitly tracked in commits

---

## Appendix: High-Activity Days

| Date | Commits | Theme |
|------|---------|-------|
| Dec 18, 2025 | 28 | Rules v2, Invoicing, MCP server |
| Jan 12, 2026 | 22 | Issue-driven bug fixes |
| Jan 8, 2026 | 19 | Calendar Sync v2 |
| Jan 2, 2026 | 18 | SvelteKit frontend, Classification |
| Jan 6, 2026 | 18 | Classification polish |
| Jan 5, 2026 | 15 | Dark mode, MCP fixes |
| Jan 3, 2026 | 14 | Calendar view redesign |
| Jan 10, 2026 | 13 | Ephemeral entries, chart view |
