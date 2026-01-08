# PRD: Calendar Sync and Classification UX

**Issue:** GitHub Issue #52
**Priority:** P0-Critical (blocks core workflow)
**Date:** 2026-01-08

---

## Problem Statement

Users are confused about:
1. **When sync and classification run** - The system behavior is opaque
2. **How to manually trigger sync/reclassification** - No clear UI controls
3. **How "skipped" works** - It's treated like classification but should be orthogonal
4. **What happens after sync** - No clear feedback or status indicators

This blocks the core workflow: users don't know if their calendar is up-to-date or how to fix misclassified events.

---

## Current Implementation

### Sync Behavior

**Backend (`calendars.go:209-327`):**
- **Trigger:** Manual API call (`POST /calendar-connections/{id}/sync`)
- **Process:**
  1. Fetches events from Google Calendar (incremental if sync token exists, full sync otherwise)
  2. Upserts events to database
  3. Marks orphaned events (deleted from Google)
  4. **Auto-runs classification** on newly synced events
  5. Returns counts: `events_created`, `events_updated`, `events_orphaned`

**Frontend (`+page.svelte`):**
- **Auto-sync on page load:** Checks for stale connections (>24h since last sync)
- **On-demand sync:** Triggered when viewing dates outside default window (-366 to +32 days)
- **Visual indicator:** Spinning icon with "Syncing calendar events..." text
- **No manual sync button** visible to user on main page

### Classification Behavior

**Types:**
1. **Rule-based:** Evaluates queries against events via `ApplyRules()`
2. **Fingerprint-based:** Matches domains/emails/keywords from project settings
3. **Manual:** User clicks a project button on an event

**When Classification Runs:**
- After sync (automatic)
- On single event classify (manual)
- On bulk classify (manual API, no UI)
- Never as standalone user action - tightly coupled to sync

**Two-Pass System:**
1. **Skip pass:** Evaluates "did not attend" rules, sets `is_skipped=true`
2. **Project pass:** Evaluates project rules, assigns `project_id`
- Both passes always run; a skipped event can still have a project assignment

### Time Entry Creation

- When an event is classified to a project, triggers `RecalculateTimeEntries()` for affected dates
- Uses analyzer to handle overlaps and rounding
- **Silent process** - no user visibility into when/why entries are recalculated

### "Skipped" Event Handling

- `is_skipped` flag set by skip rules OR manual user action
- Skipped events are **still displayed** in UI (with ✕ indicator)
- Skipped events are **excluded from time entry calculations**
- **No "unskip" mechanism** - once skipped, cannot be easily reversed from main view

---

## User Scenarios

### 1. Daily Review (Primary Workflow)
**Persona:** Consultant
**Frequency:** Daily (morning or end of day)
**Goal:** Review yesterday's meetings, classify pending events

**Current Pain:**
- Opens app, sees events but not sure if calendar is synced
- No "last synced" timestamp
- Can't tell if classification rules ran
- Doesn't know how to force re-sync

**Expected:**
- Clear sync status with timestamp
- Manual refresh button
- Confidence that all events are loaded

### 2. Fixing Misclassifications
**Persona:** Employee
**Frequency:** Weekly (before timesheet submission)
**Goal:** Reclassify events that were auto-classified incorrectly

**Current Pain:**
- Changes classification rules, but existing events don't update
- No way to say "re-run rules on this week"
- No way to say "re-run rules on this event"

**Expected:**
- "Reclassify Week" button with preview
- "Reclassify Event" option in event context menu
- Clear indicator showing which events were classified by rules vs. manual

### 3. Handling "Did Not Attend" Events
**Persona:** Consultant
**Frequency:** Several times per week
**Goal:** Mark declined/missed meetings as "did not attend"

**Current Pain:**
- "Skip" button sets `is_skipped`, but can't undo from main view
- Skip feels like a classification but behaves differently

**Expected:**
- "Did Not Attend" toggle (on/off) separate from classification
- Skipped events still visible but clearly marked
- Easy to toggle back if mistake

### 4. Historical Data Import
**Persona:** New user
**Frequency:** Once (onboarding)
**Goal:** Import past months of meetings to generate historical timesheet

**Current Pain:**
- On-demand sync exists but user doesn't know about it
- No progress indicator for large syncs
- No feedback when sync completes

**Expected:**
- Clear feedback when viewing old dates triggers sync
- Progress indicator
- Toast notification on completion

---

## Proposed Solution

### 1. Sync Visibility and Control

#### A. Sync Status Display
**Location:** Header/toolbar area, near date navigation

**States:**
- **Synced:** "Last synced: 2 hours ago" (gray text)
- **Syncing:** Spinner + "Syncing..." (blue text)
- **Stale:** "Last synced: 25 hours ago" (orange text, warning icon)
- **Never:** "Never synced" (red text, requires action)

#### B. Manual Sync Button
**Location:** Next to sync status

**Behavior:**
- Triggers sync for all connected calendars
- Shows progress if slow: "Syncing... 45 events updated"
- On completion: Toast notification "Sync complete: 12 events created, 3 updated"
- **Always runs classification** after sync (confirmed requirement)

#### C. On-Demand Sync Feedback
When user views dates outside default window and on-demand sync triggers:
- Toast notification: "Loading events for October 2023..."
- On completion: "Loaded 47 events from October 2023"

### 2. Classification Visibility and Control

#### A. Classification Status Indicators
**On each event card:**

**Visual Distinctions:**
- **Pending:** Dashed border, no project color
- **Auto-classified (Rule):** Solid border, project color, small "R" badge
- **Auto-classified (Fingerprint):** Solid border, project color, small "F" badge
- **Manual:** Solid border, project color, small lock icon (won't be overridden)
- **Needs Review:** Orange/amber border with "?" badge

**Tooltip on hover:**
- Rule: "Auto-classified by rule: domain:linuxfoundation.org"
- Fingerprint: "Auto-classified by project fingerprint"
- Manual: "Manually classified (locked)"

#### B. Reclassification Controls

**Per-Event:**
Event popup menu:
- "Reclassify with Rules"

**Per-Week:**
Button in week header:
- "⟳ Reclassify Week"

Opens **preview modal** showing:
- Counts by category: "15 pending, 8 auto-classified, 3 manually classified"
- Checkboxes to include/exclude each category
- Preview of changes: "5 events will change projects"
- Confirmation required

**Reclassification Scope Selection:**
User can choose which events to include:
- [ ] Pending events (X would be classified)
- [ ] Auto-classified events (Y would change)
- [ ] Manually classified events (Z would change)

Default: Pending + Auto-classified checked, Manually classified unchecked.

### 3. "Did Not Attend" / Skip Handling

#### Rename and Clarify
**Current:** "Skip" button
**Proposed:** "Did Not Attend" toggle

#### Separate from Classification
**Key principle:** DNA is orthogonal to project classification

**Flow:**
1. User marks event as "Did Not Attend"
2. Event is **still visible** in UI (not hidden)
3. Event is **still classified** to a project (if rules match)
4. Event is **excluded from time entry calculations**
5. User can **toggle off** DNA if mistake

**Visual indicators:**
- DNA events: Gray overlay, strikethrough title, "DNA" chip
- Still shows project color (so user knows what project it would have counted toward)

#### Make Reversible from Main View
- The ✕ icon on skipped events becomes **clickable**
- Clicking toggles DNA status off (unskips the event)
- Tooltip: "Click to mark as attended"

### 4. Time Entry Cleanup

**Automatic deletion criteria:**
Time entries are automatically deleted when ALL of these are true:
1. All source events are skipped/DNA
2. Hours equals 0 (or would calculate to 0)
3. User has NOT edited the description/notes field

**Preserve entries when:**
- User has manually edited hours (even if now 0)
- User has added notes/description text
- Entry was manually created (not from calendar events)

---

## Edge Cases and Error States

### Sync Errors

| Error | User Message | Recovery |
|-------|--------------|----------|
| OAuth token expired | "Calendar connection expired. Reconnect Google Calendar." | [Reconnect] button |
| API rate limit | "Sync paused due to rate limit. Retrying in 5 minutes..." | Auto-retry with countdown |
| Network error | "Sync failed. Check internet connection." | [Retry] button |
| No calendars selected | "No calendars selected for sync." | Link to Settings |

### Classification Edge Cases

| Scenario | Behavior |
|----------|----------|
| Rule syntax error | Don't block sync; log error; show warning in rules UI |
| No rules match | Event stays "pending", user classifies manually |
| Multiple rules match | Use priority order; mark as "needs review" if close confidence |
| Rule points to archived project | Warn in UI: "Rule classifies to archived project" |

### DNA Event Edge Cases

| Scenario | Behavior |
|----------|----------|
| User DNAs event, then rule is added | Event stays DNA (DNA takes precedence) |
| DNA rule matches, user manually un-DNAs | Manual action overrides rule (locks the event) |
| Event updated in Google after DNA | Re-sync doesn't change DNA status |

---

## Success Criteria

### Must Have (v1)
- [ ] Sync status visible with timestamp
- [ ] Manual "Sync Now" button
- [ ] Toast notifications for sync completion
- [ ] Classification source indicators (Rule/Fingerprint/Manual badges)
- [ ] "Reclassify Week" button with preview modal
- [ ] Scope selection in reclassify (pending/auto/manual checkboxes)
- [ ] "Did Not Attend" toggle (replaces "Skip")
- [ ] Unskip from main view (clickable ✕)
- [ ] Auto-delete empty time entries (per criteria above)

### Should Have (v1.5)
- [ ] "Reclassify Day" button
- [ ] "Explain Classification" tooltip showing rule name
- [ ] On-demand sync toast notifications
- [ ] Keyboard shortcut for sync (r)

### Nice to Have (v2)
- [ ] "Lock Classification" to prevent auto-reclassify
- [ ] Bulk "Mark as DNA" for multiple events
- [ ] Sync date range picker in settings

---

## UI Mockups

### Sync Status Header
```
┌──────────────────────────────────────────────────────────┐
│  [◀ Prev]  [Today]  [Next ▶]   Week of Jan 6-12         │
│  ⟳ Last synced: 2 hours ago  [Sync Now]                 │
└──────────────────────────────────────────────────────────┘
```

### Event Card with DNA
```
┌─────────────────────────────────────────────────────────┐
│ 10:00-11:00  Weekly Standup                         [R] │
│ ┌──────────────────────────────────────┐                │
│ │ ■ Alpha-Omega    [✓] Did Not Attend  │                │
│ └──────────────────────────────────────┘                │
│ alice@example.com, bob@example.com                      │
└─────────────────────────────────────────────────────────┘
```

### Reclassify Week Modal
```
┌─────────────────────────────────────────────────────────┐
│ Reclassify Week of Jan 6-12                         [×] │
├─────────────────────────────────────────────────────────┤
│ Select which events to reclassify:                      │
│                                                         │
│ [✓] Pending events (15 events)                          │
│     → 12 will be classified                             │
│                                                         │
│ [✓] Auto-classified events (23 events)                  │
│     → 5 will change projects                            │
│                                                         │
│ [ ] Manually classified events (8 events)               │
│     → 2 would change projects                           │
│                                                         │
│ Preview of changes:                                     │
│   "Team Sync" → Alpha-Omega → Eclipse                   │
│   "Planning" → Unclassified → Alpha-Omega               │
│   "Review" → Eclipse → Alpha-Omega                      │
│                                                         │
│                    [Cancel]  [Reclassify]               │
└─────────────────────────────────────────────────────────┘
```

---

## Implementation Phases

### Phase 1: Sync Visibility
- Add sync status component to header
- Display "last synced" timestamp
- Add manual "Sync Now" button
- Show sync spinner and completion toast

### Phase 2: DNA Improvements
- Rename "Skip" to "Did Not Attend"
- Change to toggle behavior
- Make ✕ clickable to unskip from main view
- Update visual styling

### Phase 3: Classification Indicators
- Add source badges to event cards (R/F/lock)
- Add tooltips showing classification details
- Differentiate "needs review" styling

### Phase 4: Reclassification Controls
- "Reclassify Week" button
- Preview modal with scope checkboxes
- Show counts and change preview

### Phase 5: Time Entry Cleanup
- Implement auto-delete logic for empty entries
- Preserve user-edited entries
- Add "Refresh from Events" button on entries

---

## Decisions Made

| Question | Decision |
|----------|----------|
| Should sync always run classification? | **Yes** - sync always triggers classification |
| How aggressive should reclassification be? | **User choice** - preview shows counts for pending, auto, manual; user selects which to include |
| Should manual classification be locked? | **User choice** - manual events excluded from reclassify by default, but user can opt-in via checkbox |
| What happens to entries when events are skipped? | **Auto-delete** if: 0 hours AND no user-edited description. Preserve if user edited anything. |
