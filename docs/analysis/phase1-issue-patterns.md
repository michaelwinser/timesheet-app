# Phase 1: Issue Patterns Analysis

*Analysis Date: 2026-01-13*

## Executive Summary

The timesheet application has tracked 96 issues over approximately 4 weeks (December 18, 2025 - January 13, 2026). Key patterns observed:

- **High velocity development**: 75 issues closed (78% closure rate), 21 remain open
- **Rapid iteration**: Most issues are closed within 1-2 days of creation
- **Priority system**: Uses P0-P3 labels but many issues (especially early ones) have no priority label
- **Domain-specific labels**: `classifier`, `timeentries`, `testing`, `ui` indicate area of concern
- **Deliberate decisions**: 7 issues marked `wontfix` document explicit decisions NOT to implement features
- **Architecture-focused issues**: Several significant issues (#77, #76, #52, #64) represent architectural thinking and design decisions rather than simple bug fixes

## Issue Management Conventions

### Titling Patterns

Issues use several titling conventions:

1. **Declarative/Problem Statement** (most common):
   - "Creating overlapping billing periods causes a server error" (#96)
   - "Time entries don't update when the events change" (#64)
   - "Skip/unskip in hover popup doesn't update view" (#59)

2. **Imperative/Task-Based**:
   - "Implement Invoices" (#1)
   - "Add keyboard shortcuts to navigate the calendar view" (#12)
   - "Allow user to archive projects" (#72)

3. **Question/Discussion Format**:
   - "Should we show 0h time entries?" (#75)
   - "Should invoicing prevent new time entries from being created...?" (#87)
   - "Figure out how timezones interact with calendar view" (#70)

4. **Feature Requests with Context**:
   - "Need a filter to find unclassified attended meetings that need user attention" (#69)
   - "Need a 'default' classification option by calendar" (#68)

### Labeling System

| Label | Count | Usage |
|-------|-------|-------|
| P2-medium | 17 | Standard improvements and features |
| P3-low | 17 | Polish, nice-to-have |
| wontfix | 7 | Deliberate decisions not to implement |
| P1-high | 6 | Significant pain points, blocking issues |
| classifier | 5 | Classification system issues |
| timeentries | 4 | Time entry calculation issues |
| P0-critical | 2 | Core workflow blockers |
| testing | 1 | Test infrastructure |
| ui | 1 | UI-specific issues |
| enhancement | 1 | Feature enhancements |

**Observations:**
- Many issues lack priority labels entirely (especially issues #1-30)
- P0-critical is used sparingly (only #8 and #52)
- Dual-labeling common (e.g., P2-medium + P3-low on same issue)
- Domain labels (classifier, timeentries) help track problem areas

### Prioritization Patterns

- **P0-critical**: Reserved for issues that completely block core workflows
  - #8: "Classified events are no longer showing up in time entries" - fundamental data flow broken
  - #52: "Calendar sync and classification UX is not clear" - core usability issue

- **P1-high**: Issues causing significant pain
  - #63: Project short codes should be unique (data integrity)
  - #59: State management bug in hover popup (UX regression)
  - #57: Regenerate time entries and rerun rules (workflow pain)
  - #42: Create new project ignores client field (data loss)
  - #40: Rounding issue (incorrect calculations)
  - #6: Auto-sync after calendar changes (workflow friction)

- **P2-medium**: Useful improvements that aren't urgent
- **P3-low**: Polish items, minor improvements

## Resolution Patterns

### Time to Resolution

- Most issues are resolved within **1-2 days** of creation
- Example turnarounds:
  - #90 (Modal z-index): Created 20:04, Closed 20:06 (2 minutes)
  - #91 (Dark mode): Created 20:06, Closed 20:10 (4 minutes)
  - #85 (Text matching): Created 13:57, Closed 13:58 (1 minute)

This indicates:
1. Issues are often created as documentation of work in progress
2. Some issues are created as retrospective documentation
3. Active development with immediate fixes

### Resolution Types

1. **Direct Fix**: Most common - issue identified, fixed, closed
2. **Superseded**: Issue replaced by more comprehensive solution
3. **Wontfix**: Deliberate decision not to implement (7 issues)
4. **Design Discussion**: Issue used to document design decisions (then closed)

### Closure Without Labels

Many issues are closed without any labels, particularly:
- Simple bugs fixed quickly
- Small UI tweaks
- Documentation-style issues

## Significant Issues

### Architecture & Design Decision Issues

| # | Title | Significance |
|---|-------|--------------|
| **77** | Composite View API & Smart Updates | Proposes unified API endpoint to solve consistency and performance issues. Includes detailed implementation plan. OPEN. |
| **76** | Ephemeral Invoicing Model | Extends ephemeral model pattern to invoices. Backend-first calculation. OPEN. |
| **52** | Calendar sync and classification UX is not clear | P0-critical - Identified core UX confusion around sync/classification. CLOSED. |
| **64** | Time entries don't update when the events change | Fundamental data flow issue between events and time entries. CLOSED. |
| **61** | Add integration and E2E tests for calendar sync v2 | Testing strategy documentation, including explicit decisions about NOT mocking Google API. OPEN. |
| **55** | Implement a month view of time entries | Contains extensive UX design exploration with HTML/CSS mocks. OPEN. |

### Issues That Spawned Related Issues

- **#52** (Calendar sync UX) led to:
  - #57 (Regenerate time entries)
  - #59 (Skip/unskip popup fix)
  - #58 (Move badges to popup)

- **#64** (Time entries don't update) is referenced by:
  - PRD documentation
  - #87 (0h time entries in invoicing)

### Critical Bugs

| # | Title | Impact |
|---|-------|--------|
| **8** | Classified events not showing in time entries | P0 - Core data flow broken |
| **59** | Skip/unskip state management | P1 - Structural Svelte state issue, led to CLAUDE.md guidelines |
| **42** | Create new project ignores client field | P1 - Silent data loss |
| **40** | Rounding issue | P1 - Incorrect calculations |

## Feature Area Map

### 1. Calendar Synchronization (High Complexity)
*Issues: #52, #6, #61, #93, #92, #26*

Core functionality connecting Google Calendar to the app. Issues include:
- Sync timing and triggers (#6, #93)
- Special event handling (#92 - working location events)
- Testing strategy (#61)
- UX clarity around when sync runs (#52)

**Complexity Notes:** This area requires careful coordination between Google Calendar API, background jobs, and UI state. Multiple issues around "when does sync happen" and "how does user know sync happened."

### 2. Classification System (High Complexity)
*Issues: #52, #57, #64, #80, #82, #84, #85, #71, #68, #67, #46, #44, #43, #14, #13, #11, #10, #9*

The rule-based event classification system. Issues include:
- Rule matching bugs (#44, #84, #85)
- Reclassification workflows (#57)
- Classification explanation (#80)
- Confidence thresholds (#82)
- Default classification by calendar (#68)
- Keyword matching (#71, #44)

**Complexity Notes:** Classification is the "brain" of the app. Many edge cases around text matching, weight calculations, and user overrides. The PRD documents desired behavior but implementation has had many iterations.

### 3. Time Entry Management (Medium-High Complexity)
*Issues: #64, #11, #8, #87, #75, #73, #66, #21, #15*

Calculated time entries from calendar events. Issues include:
- Orphaned entries when events change (#64, #11)
- 0-hour entries (#75, #87)
- Rounding behavior (#21, #40)
- Manual entry merge behavior (#66)
- Display presentation (#73)

**Complexity Notes:** Time entries are "ephemeral" (calculated from events) but can have manual overrides. The tension between calculated and manual state creates complexity.

### 4. Invoicing (Medium Complexity)
*Issues: #1, #76, #87, #95, #96, #89, #62*

Billing period management and invoice generation. Issues include:
- Invoice creation and regeneration (#1, #62)
- Preventing edits to invoiced entries (#89)
- Billing period interactions (#95, #96)
- Preview accuracy (#76)

**Complexity Notes:** Invoicing locks time entries and billing periods. The "ephemeral invoice" pattern (#76) would simplify preview vs. actual invoice consistency.

### 5. UI/UX Polish (Medium Complexity)
*Issues: #90, #91, #86, #83, #94, #54, #53, #45, #41, #39, #37, #35, #34, #32, #30, #29, #7, #4, #3, #2*

Various UI improvements and bug fixes. Categories include:
- Z-index issues (#90, #86)
- Dark mode (#91, #30, #22)
- Event card layout (#54, #53, #45, #4, #3, #2)
- Popup improvements (#83, #37, #39)
- Auto-save UX (#41, #32, #7)

**Complexity Notes:** UI issues tend to be lower complexity individually but numerous. Pattern emerged around Svelte state management (#59, documented in CLAUDE.md).

### 6. Calendar View (Medium Complexity)
*Issues: #55, #73, #78, #74, #69, #60, #38, #16, #12, #5*

The main calendar display. Issues include:
- Month view design (#55)
- View filters (#78, #69)
- Keyboard navigation (#12, #38)
- Hover popup (#16, #83)
- Layout modes (#5, #73)

**Complexity Notes:** View complexity comes from multiple modes (day/week/month) and performance with many events. #55 contains extensive design exploration.

### 7. Project Management (Low-Medium Complexity)
*Issues: #72, #65, #63, #42, #7*

Project CRUD and configuration. Issues include:
- Project archiving (#72)
- Start/end dates (#65)
- Unique short codes (#63)
- Client field bug (#42)

**Complexity Notes:** Relatively straightforward CRUD but has implications for classification and invoicing.

### 8. Authentication & Settings (Low Complexity)
*Issues: #23, #93*

User authentication and configuration. Issues include:
- Google OAuth (#23 - OPEN)
- Calendar settings (#93)

### 9. Import/Export (Low Complexity)
*Issues: #51, #33*

Data import/export features. Issues include:
- Project/rules import/export (#51)
- Spreadsheet export (#33)

## Wontfix Analysis

Seven issues were deliberately closed as `wontfix`, representing important decisions NOT to do something:

### 1. #70 - "Figure out how timezones interact with calendar view"
**Body:** "Do we need to let the user define a 'primary timezone' so that their timesheets make sense most of the time?"

**Decision:** Not implementing primary timezone. Likely accepting that the app uses system/browser timezone.

### 2. #62 - "The Regenerate Invoice has disappeared"
**Body:** "It's possible that I imagined this but I thought we had a feature on Invoices to regenerate an invoice."

**Decision:** Feature doesn't exist (possibly never did). Alternative workflow is delete and recreate.

### 3. #60 - "Add Reclassify Day button for Day view"
**Body:** Detailed proposal for Day view reclassify button complementing Week view.

**Decision:** Not worth the ~30 minute effort. Reclassify Week is sufficient.

### 4. #46 - "Declined meetings should classify to skipped"
**Body:** "This should be a built-in rule (or a default rule created when the user is created)."

**Decision:** Not making this automatic. Users can create rules if they want this behavior.

### 5. #35 - "Project summary hours cause list to jump around"
**Body:** Visual issue with 1-digit vs 3-digit hour numbers.

**Decision:** Not fixing. Possibly accepted as minor visual glitch.

### 6. #28 - "Persist classification information in the event"
**Body:** Proposal to store classification in Google Calendar event custom properties.

**Decision:** Not implementing. Would couple app tightly to Google Calendar internals.

### 7. #25 - "Show which rules matched, especially when confidence is medium"
**Body:** Request to expose rule matching details in UI.

**Decision:** Superseded by #80 (Add explain classification) which was implemented differently.

### Patterns in Wontfix Decisions

1. **Scope control**: Declining features that add complexity without proportional value (#60, #28)
2. **User empowerment over automation**: Letting users create rules rather than building in special cases (#46)
3. **Simplicity preference**: Accepting simple workflows over complex UI (#62, #35)
4. **Superseded by better solution**: Original request addressed differently (#25)
5. **Technical complexity avoidance**: Avoiding timezone complexity (#70)

## Recommendations

### 1. Codify Priority Label Usage
Many issues lack priority labels. Consider:
- Requiring P0-P3 label on all issues
- Adding label bot or template that prompts for priority
- Documenting criteria for each priority level

### 2. Create Issue Templates
Based on observed patterns, templates for:
- **Bug Report**: Problem description, expected vs actual, steps to reproduce
- **Feature Request**: Problem statement, proposed solution, alternatives considered
- **Design Discussion**: Context, options, decision criteria

### 3. Link Related Issues More Consistently
Several issues reference each other (#52 spawning #57, #59, #58) but links are inconsistent. Consider:
- Using GitHub's "related issues" feature
- Adding a "Related to #X" line in issue bodies
- Parent/child issue relationships for complex features

### 4. Document Wontfix Decisions
Wontfix decisions are valuable product decisions. Consider:
- Adding a comment explaining the decision before closing
- Creating a "decisions" document that aggregates wontfix rationale
- Using wontfix label consistently (some decisions may be missing it)

### 5. Continue Using Issues for Design Documentation
Issues #55, #77, #76 contain excellent design documentation. This pattern works well:
- Detailed problem statement
- Proposed solution with code examples
- Implementation plan
- FAQ section

### 6. Consider Milestone-Based Organization
As the issue count grows, consider:
- Grouping issues into milestones (e.g., "Invoicing v2", "Classification improvements")
- Using GitHub Projects for roadmap visualization

### 7. Address State Management Pattern
Issue #59 revealed a structural Svelte state management issue that was documented in CLAUDE.md. Consider:
- Reviewing other components for similar patterns
- Creating a state management guidelines document
- Adding lint rules or tests to prevent the anti-pattern

### 8. Test Infrastructure Investment
Issue #61 is open and documents testing strategy. Given the complexity in Calendar Sync and Classification, investing in integration tests would reduce regression risk.
