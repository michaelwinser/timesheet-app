# Known Bugs

Track bugs here. Mark as [FIXED] when resolved.

---

## Open Bugs

### BUG-034: Application is single-user only - multi-user auth breaks
**Reported:** 2025-12-07
**Severity:** High
**Description:**
The application architecture is single-user only. When multiple users log in, the application uses the first user's calendar access tokens for all subsequent users.

**Root cause:**
- `auth_tokens` table has no `user_id` column
- No user session management
- OAuth tokens are stored globally without user association
- All calendar operations use the single set of stored tokens

**Impact:**
- User A logs in and syncs their calendar → works correctly
- User B logs in on a different device/browser → sees User A's calendar data
- Security issue: Users can access other users' calendar data
- Data integrity issue: Classifications from different users get mixed

**Current workaround:**
- Deploy separate instances per user (TrueNAS, Docker, etc.)
- Or, only use the app with one Google account

**Proper fix requires:**
1. Add user authentication system (sessions, cookies, JWT)
2. Add `users` table with user identification
3. Add `user_id` foreign key to `auth_tokens`, `events`, `time_entries`, `projects`, `classification_rules`
4. Update all queries to filter by `user_id`
5. Add user login/logout flows
6. Add user context middleware to track current user per request

**Related to:**
- Future work: Multi-user support (see design.md section 10)
- Google Cloud Run migration would naturally require multi-user support

**Note:**
This is a fundamental architectural limitation, not a bug per se. The PRD explicitly states "single user" but this documents the observed multi-user collision behavior for future reference.

---

### BUG-033: Implement user settings system with auto-roundup option
**Reported:** 2025-12-07
**Severity:** Medium
**Description:**
Add a user settings/preferences system to store configuration options. First setting: auto-roundup hours to nearest 15 minutes when classifying events.

**Settings infrastructure needed:**
- Settings storage (localStorage, database, or both)
- Settings UI page/modal accessible from header
- API endpoints for reading/writing settings
- Default values for new users

**Initial settings to implement:**
1. **Auto-roundup hours**: Automatically round event duration to nearest 15 minutes when classifying
   - Options: Off, Round up, Round nearest, Round down
   - Default: Off (preserve exact duration)

**Future settings candidates:**
- Default project for unclassified events
- Hide weekends by default
- Auto-sync interval
- Theme (light/dark/system)
- Week start day (Sunday/Monday)
- Working hours per day target (for daily totals coloring)
- Notification preferences

**UI options:**
- Dedicated /settings page
- Modal accessible from user menu or gear icon
- Inline settings in relevant contexts

**Current behavior:**
No settings system; all behavior is hardcoded.

---

### BUG-032: Show daily hour totals in week view
**Reported:** 2025-12-07
**Severity:** Low
**Description:**
Each day column should display a total of classified hours at the bottom or in the header. This helps users quickly see how much time is logged per day and identify days that need attention.

**Expected features:**
- Total hours displayed per day column (e.g., "6.5 hrs")
- Visual indicator for days under/over target (e.g., <8 hrs = yellow, >8 hrs = green)
- Optional: show classified vs unclassified count
- Updates dynamically as events are classified

**Display options:**
- In day header next to day name/number
- At bottom of day column as footer
- Both header and footer

**Example:**
```
Mon 2          Tue 3          Wed 4
6.5 hrs        8.0 hrs        4.25 hrs
─────────      ─────────      ─────────
[events]       [events]       [events]
```

**Current behavior:**
No per-day totals shown; only weekly total in sidebar.

---

### BUG-031: Implement global search with event/rule management
**Reported:** 2025-12-07
**Severity:** Medium
**Description:**
Add a search feature that allows users to find events across all weeks, then take actions on the results like editing, classifying, or creating rules.

**Expected features:**
- Search input in header or accessible via keyboard shortcut (Cmd+K or /)
- Search across event titles, descriptions, attendees
- Results show matching events from any time period
- Click result to navigate to that week/event
- Bulk actions on search results:
  - Classify all matching events to a project
  - Create a rule from search criteria
  - Export matching events
- Filter results by: classified/unclassified, date range, project

**Use cases:**
- "Find all standups and classify them"
- "Show me all meetings with alice@example.com"
- "Find unclassified events with 'client' in title"
- "Create a rule for all events matching this search"

**UI ideas:**
- Command palette style (like VS Code, Slack)
- Modal with search input and results list
- Keyboard navigation through results
- Preview panel showing event details

**Current behavior:**
Only filter within current week view; no cross-week search.

---

### BUG-028: Persist project visibility filter across week navigations and reloads
**Reported:** 2025-12-07
**Severity:** Low
**Description:**
The project visibility toggle in the week view sidebar currently stores preferences per-week in sessionStorage. This means visibility settings are lost when navigating to a different week or reloading the page.

**Current behavior:**
- Visibility stored with week-specific key (`projectVisibility_${weekStart}`)
- Settings lost on browser close (sessionStorage)
- Each week has independent visibility settings

**Expected behavior:**
- Visibility settings should persist across all weeks (global preference)
- Settings should survive browser close (use localStorage instead of sessionStorage)
- When hiding "Junk" project, it should stay hidden on all weeks

**Implementation options:**
1. Use localStorage with a single key (not week-specific)
2. Add a "Remember" checkbox to make persistence optional
3. Store in database as user preference

---

### BUG-029: Sort project lists alphabetically for consistent UX
**Reported:** 2025-12-07
**Severity:** Low
**Description:**
Project dropdowns and lists should be sorted alphabetically for a consistent user experience. Currently, projects may appear in different orders in different contexts (by ID, by hours, etc.), making it harder to find a specific project.

**Affected areas:**
- Project dropdown in event cards (classify dropdown)
- Project dropdown in time entry cards (reclassify dropdown)
- Project dropdown in rule creation modal
- Project summary sidebar (currently sorted by hours)

**Expected behavior:**
- All project dropdowns sorted alphabetically by name
- Sidebar could offer sort options: by name, by hours, by recent use
- Consistent ordering across all UI elements

**Current behavior:**
- Dropdowns sorted by database order (insertion order / ID)
- Sidebar sorted by hours descending

---

### BUG-030: Week view layout too compressed on wide screens
**Reported:** 2025-12-07
**Severity:** Medium
**Description:**
The week view layout feels compressed and doesn't take advantage of available screen width on larger monitors. There's significant unused space on wide screens that could be used to display more event information.

**Current issues:**
- Fixed/constrained max-width limits horizontal expansion
- Day columns are narrow even when space is available
- Event cards truncate text unnecessarily on wide screens
- Layout doesn't scale well from laptop to desktop monitor

**Expected behavior:**
- Week grid should expand to use available screen width
- Day columns should grow proportionally on wider screens
- Event cards should show more content when space allows
- Responsive breakpoints for ultra-wide displays (1920px+, 2560px+)

**Potential improvements:**
- Remove or increase max-width constraints on week container
- Use CSS grid with flexible column sizing (minmax, fr units)
- Allow event cards to show full titles on wider screens
- Consider multi-row event layout within day columns on very wide screens

**Current behavior:**
Layout has constrained width, leaving empty margins on wide displays.

---

### BUG-003: Projects need short names or codes for compact UI labels
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
Projects only have a full name field. For compact UI elements (badges, corner labels, narrow columns), a short name or 2-letter code would be useful.

**Steps to reproduce:**
1. Create a project with a long name like "Alpha-Omega Security Initiative"
2. View it in compact UI contexts

**Expected behavior:**
Projects should have an optional short_name or code field (e.g., "AO" or "A-O") for use in space-constrained UI elements.

**Actual behavior:**
Only the full project name is available, which may be too long for compact displays.

---

### BUG-005: Rule editing form appears at top of page instead of inline
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
When clicking "Edit" on a rule in the rules list, the edit form appears at the top of the page, requiring the user to scroll up. This breaks context and makes it harder to compare the rule being edited with others.

**Steps to reproduce:**
1. Go to the Rules page
2. Scroll down if there are multiple rules
3. Click "Edit" on a rule

**Expected behavior:**
Edit inline within the rule card itself, or use a modal/popover that appears near the rule being edited.

**Actual behavior:**
Form appears at top of page, user loses their scroll position.

---

### BUG-006: No link to view event in Google Calendar
**Reported:** 2025-12-06
**Severity:** Medium
**Description:**
The calendar event card shows a "Join" link for Google Meet, but there's no way to view the actual calendar event in Google Calendar. This is important for seeing full event details, editing the event, or viewing private/confidential event information.

**Steps to reproduce:**
1. View any event card in the week view

**Expected behavior:**
- Add a popup/modal showing full event details (description, all attendees, etc.)
- Include a "View in Google Calendar" link that opens the event in Google Calendar
- The Meet link should be secondary to the calendar link

**Actual behavior:**
Only the Google Meet "Join" link is shown. No way to access the full event or view it in Google Calendar.

---

### BUG-026: Implement change log with per-item undo
**Reported:** 2025-12-06
**Severity:** Medium
**Description:**
Users need visibility into what changes they've made and the ability to undo individual changes without affecting others.

**Change log features:**
- List of recent actions (classify, reclassify, unclassify, edit description, adjust hours)
- Timestamp for each action
- Link to the affected event/entry (navigates to correct week if needed)
- Persist across page navigation (session or longer)

**Per-item undo features:**
- Each change log entry has an "Undo" button
- Restores that specific item to its previous state
- Does not affect other changes made before or after
- Works even after multiple subsequent changes to same item

**UI options:**
- Collapsible panel or drawer showing recent changes
- Keyboard shortcut (Cmd+Z) for most recent undo
- Toast notifications with inline "Undo" link after each action

**Example change log entries:**
- `12:34` - Classified "Standup" → Project A [Undo]
- `12:32` - Changed hours on "Client call" from 0.50 to 1.00 [Undo]
- `12:30` - Unclassified "Lunch break" [Undo]

**Current behavior:**
No change history; no undo capability.

---

### BUG-025: Add navigation to next/previous week with unclassified events
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
When catching up on timesheets, users need to quickly jump to weeks that have unclassified events rather than navigating week-by-week through fully classified weeks.

**Expected behavior:**
- "Jump to next week with unclassified events" button/shortcut
- "Jump to previous week with unclassified events" button/shortcut
- Visual indicator showing how many weeks back have unclassified items
- Maybe: badge on nav showing total unclassified count

**UI options:**
- Additional nav buttons: `<< Prev Unclassified | Prev | Today | Next | Next Unclassified >>`
- Keyboard shortcuts (e.g., `Shift+←` / `Shift+→`)
- Dropdown showing weeks with pending items

**Use cases:**
- Weekly timesheet catch-up after vacation
- Finding missed classifications from weeks ago
- Quick audit of historical data

**Current behavior:**
Must navigate week-by-week to find unclassified events.

---

### BUG-024: Preserve filter text across page navigation
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
When filtering events in the week view, navigating to a different week clears the filter. The filter text should persist across navigation.

**Steps to reproduce:**
1. Enter text in the filter input (e.g., "standup")
2. Navigate to previous or next week
3. Filter is cleared

**Expected behavior:**
- Filter text persists when navigating between weeks
- Filtered view applies to the new week's events
- Clear button or easy way to reset filter

**Implementation options:**
- Store filter in URL query parameter (`?filter=standup`)
- Store in sessionStorage
- Store in app state

**Current behavior:**
Filter is cleared on page navigation, requiring user to re-enter it.

---

### BUG-023: Add option to search email for event contacts
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
When viewing an event, provide an option to search email for conversations with the attendees. This helps with context recall - "What was this meeting about?" or "What did we discuss last time?"

**Expected behavior:**
- Button or link on event card: "Search email for [attendee]"
- Opens Gmail/email client with search query for that contact
- Could search for all attendees or specific ones
- Optionally filter by date range around the meeting

**Search query examples:**
- `from:alice@example.com OR to:alice@example.com`
- `{from:alice@example.com to:alice@example.com} after:2024/01/01`

**Use cases:**
- Recall context before a meeting
- Find related documents/attachments
- Verify what project a meeting relates to
- Find follow-up action items

**Current behavior:**
No way to search email for event participants from the timesheet app.

---

### BUG-022: Prompt to create rule for 1:1 meetings or single-domain meetings
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
When classifying a meeting that has only one other attendee (1:1) or only attendees from a single external domain, prompt the user to create a contact/domain-based rule for future meetings.

**Trigger conditions:**
1. **1:1 meeting**: Only one attendee besides the user
2. **Single external domain**: All external attendees share the same domain (e.g., all @clientcorp.com)

**Expected behavior:**
After classifying such an event, show a prompt like:
- "Create a rule for all 1:1s with alice@example.com?"
- "Create a rule for all meetings with @clientcorp.com?"

With options:
- "Yes, create rule" → Opens pre-filled rule creation
- "Not now" → Dismisses for this event
- "Don't ask again for this contact/domain" → Remembers preference

**Benefits:**
- Teaches users about rules through contextual prompts
- Captures obvious high-confidence classification patterns
- Reduces future manual classification work

**Current behavior:**
No prompts; user must manually think to create rules.

---

### BUG-021: Auto-generate unique colors for projects from a refined palette
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
Projects should be assigned unique, visually distinct colors automatically from a curated palette. This saves users from picking colors and ensures good contrast/readability.

**Expected behavior:**
- Auto-assign color when creating a new project
- Use a pre-defined palette of refined, professional colors
- Ensure colors are distinct from existing projects
- Colors should work well on both light and dark backgrounds
- Allow manual override if desired

**Palette considerations:**
- Avoid harsh/neon colors
- Ensure sufficient contrast for text readability
- Consider colorblind-friendly options
- Maybe 12-16 distinct hues that cycle

**Example palette (refined):**
- Slate blue: #6366f1
- Emerald: #10b981
- Amber: #f59e0b
- Rose: #f43f5e
- Cyan: #06b6d4
- Violet: #8b5cf6
- Orange: #f97316
- Teal: #14b8a6
- Pink: #ec4899
- Indigo: #4f46e5
- Lime: #84cc16
- Sky: #0ea5e9

**Current behavior:**
User must manually pick a color, often resulting in inconsistent or hard-to-read choices.

---

### BUG-020: Add keyboard navigation for weeks and events
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
Power users should be able to navigate the app using keyboard shortcuts for faster workflow.

**Expected keyboard shortcuts:**
- `←` / `→` or `h` / `l`: Navigate to previous/next week
- `t`: Jump to today/current week
- `j` / `k`: Move between events in a day
- `Tab`: Move between days
- `Enter` or `Space`: Select/expand current event
- `1-9`: Quick-assign to project by number
- `Esc`: Close modal/deselect

**Additional ideas:**
- Visual focus indicator on selected event
- Vim-style navigation (`hjkl`)
- Command palette (`Cmd+K` or `/`) for quick actions

**Current behavior:**
All navigation requires mouse clicks.

---

### BUG-019: Auto-generate rule names from conditions
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
Rule names should be auto-generated as a human-readable summary of the conditions. This saves time and ensures names stay accurate as conditions change.

**Examples:**
- Conditions: `title contains "standup"` → Name: "Title contains 'standup'"
- Conditions: `attendees list_contains alice@example.com` → Name: "Meetings with alice@example.com"
- Conditions: `organizer = bob@example.com AND title contains "1:1"` → Name: "1:1s organized by bob@"
- Conditions: `attendee_domain = clientcorp.com` → Name: "Meetings with @clientcorp.com"

**Expected behavior:**
- Auto-suggest name based on conditions when creating rule
- Update suggestion as conditions change
- Allow manual override
- Option to regenerate name from conditions

**Current behavior:**
User must manually type a rule name.

---

### BUG-017: Add option to hide weekends in week view
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
Most users don't have work events on weekends. The week view should have an option to hide Saturday and Sunday columns to give more space to weekday events.

**Expected behavior:**
- Toggle or setting to show/hide weekends
- When hidden, only Mon-Fri columns are displayed
- Columns can be wider with more room for event cards
- Preference should persist

**Actual behavior:**
All 7 days are always shown, including empty weekend columns.

---

### BUG-016: Add email domain as a rule property for easy classification
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
Email domain is a natural classification signal. Meetings with external attendees from a specific domain (e.g., @clientcorp.com) are almost always for that client's project.

**Expected properties:**
- `attendee_domains`: list of domains from attendee emails
- `organizer_domain`: domain of the meeting organizer
- `external_domains`: domains excluding user's own domain

**Example rules:**
- "Any meeting with @alpha-omega.dev attendees → Alpha-Omega project"
- "Any meeting with @freebsd.org attendees → FreeBSD project"
- "Meetings organized by someone @clientcorp.com → ClientCorp project"

**Benefits:**
- Very easy to set up client/organization-based rules
- High confidence - domain usually maps 1:1 to project
- Works well for external consultants/contractors

**Current behavior:**
Must match on full email addresses; no domain-level matching.

---

### BUG-015: Organize rules by project and support contact-to-project mapping
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
Rules could be better organized by associating them with projects. Additionally, a common pattern is mapping a list of contacts/attendees to a project, which could be simplified with "magic" rules.

**Ideas:**
1. **Rules organized by project**: View/manage rules grouped under their target project
2. **Contact-to-project mapping**: Associate a list of email addresses with a project
   - "Meetings with any of [alice@, bob@, carol@] → Project X"
   - Simpler than creating individual attendee rules
3. **Magic rules**: Pre-built rule templates for common patterns:
   - "1:1 with [person] → [project]"
   - "Any meeting organized by [person] → [project]"
   - "Meetings with only internal attendees → [internal project]"
   - "External meetings with [domain] → [client project]"

**Benefits:**
- Easier to see "what rules apply to Project X?"
- Faster setup for contact-based classification
- Reduces manual rule creation for common patterns

**Current behavior:**
Rules are flat list, no project grouping. Contact-based rules require manual creation of individual conditions.

---

### BUG-014: Add "did not attend" option to hide events from view
**Reported:** 2025-12-06
**Severity:** Medium
**Description:**
Users need a simple way to mark calendar events they didn't actually attend. These should be hidden from the week view to reduce clutter.

**Possible implementations:**
1. Special "Did Not Attend" pseudo-project that hides classified events
2. Dedicated "skip" or "hide" action on event cards
3. Swipe-to-dismiss gesture
4. Right-click context menu option

**Expected behavior:**
- Quick action to mark event as "did not attend"
- Event disappears from view (or shown in collapsed/dimmed state)
- Optionally: ability to see/restore hidden events
- These events should NOT appear in exports

**Actual behavior:**
No way to dismiss events that were on the calendar but not attended.

---

### BUG-013: Add calendar response status to rule properties
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
Rules should be able to match on calendar event response status (accepted, declined, tentative) and free/busy status. This allows filtering out declined meetings or treating tentative responses differently.

**Expected properties:**
- `response_status`: accepted, declined, tentative, needsAction
- `free_busy_status`: busy, free, tentative, outOfOffice

**Use cases:**
- Skip declined meetings: `response_status != declined`
- Flag tentative meetings for review
- Treat "free" time blocks differently from "busy" meetings

**Actual behavior:**
Response status and free/busy status are not available as rule properties.

---

### BUG-012: Time entry description should be editable
**Reported:** 2025-12-06
**Severity:** Medium
**Description:**
The time entry card displays the description (derived from the calendar event title) but doesn't allow editing it. Users need to customize descriptions for timesheet clarity.

**Steps to reproduce:**
1. Classify an event
2. Try to edit the description on the time entry card

**Expected behavior:**
- Click on description to edit inline, or
- Small edit icon to enable editing
- Save on blur or Enter key

**Actual behavior:**
Description is read-only, cannot be modified.

---

### BUG-011: Time entry card layout needs redesign - title should be prominent
**Reported:** 2025-12-06
**Severity:** Medium
**Description:**
The time entry (classified) card layout makes it hard to see the event description/title. The project dropdown is at the top, pushing the title down. The layout of both card views (event side and entry side) needs more intentional design.

**Current layout (entry side):**
1. Project dropdown (top)
2. Hours + round up button
3. Description (often truncated)
4. Actions

**Expected layout (entry side):**
1. Title/description (prominent, at top)
2. Hours display
3. Project dropdown (can be lower priority)
4. Actions

**More generally:**
- Both card views need consistent, intentional layout
- Key information (what the meeting is) should be immediately visible
- Project selection is a one-time action, shouldn't dominate the view
- Consider visual hierarchy: title > time/hours > project > actions

**Actual behavior:**
Project dropdown dominates the card, title is buried and truncated.

---

### BUG-010: Rules should support confidence levels for ambiguous classifications
**Reported:** 2025-12-06
**Severity:** Medium
**Description:**
Rules should be able to indicate classification confidence, especially for ambiguous cases. For example, "meetings with Deb from FreeBSD" are usually Alpha-Omega work but sometimes FreeBSD board business. These need manual review.

**Use cases:**
1. Single rule with inherent ambiguity (e.g., person works on multiple projects)
2. Multiple conflicting rules match → lower confidence
3. Multiple agreeing rules match → higher confidence
4. Future: LLM-based classification with probability scores
5. Future: automatic rule generation from user behavior

**Expected behavior:**
- Rules can specify a confidence level (high/medium/low or percentage)
- Time entries show confidence indicator (e.g., color coding, icon)
- Low-confidence entries are flagged for manual review
- Priority system could be replaced/augmented by confidence aggregation

**Current behavior:**
Priority only determines which rule wins when multiple match; no concept of classification confidence or need-for-review flagging.

---

### BUG-009: Show indicator for rule-classified entries with link to rule
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
Time entries that were automatically classified by a rule should have a visual indicator (badge/icon) showing they were rule-classified, with a tooltip indicating which rule. Clicking the indicator should link to that rule in the Rules UI.

**Steps to reproduce:**
1. Create a rule that matches an event
2. Sync or trigger rule application
3. View the classified time entry

**Expected behavior:**
- Small badge/icon (e.g., gear or automation symbol) on the entry card
- Tooltip on hover showing "Classified by: [Rule Name]"
- Click to navigate to the rule in the Rules page

**Actual behavior:**
No distinction between manually classified and rule-classified entries.

---

### BUG-008: Rules should auto-classify unclassified events when updated
**Reported:** 2025-12-06
**Severity:** Medium
**Description:**
When rules are created or updated, they should automatically be applied to any unclassified events in the current week. Currently, rules only apply to newly synced events, not existing unclassified ones.

**Steps to reproduce:**
1. Have unclassified events in the current week
2. Create a rule that matches one of those events
3. Go back to the week view

**Expected behavior:**
The matching unclassified events should be automatically classified according to the new/updated rule.

**Actual behavior:**
Existing unclassified events remain unclassified until manually classified or the next sync.

---

### BUG-007: Attendees matching and count behavior is unclear
**Reported:** 2025-12-06
**Severity:** Medium
**Description:**
The attendees property matching is confusing:
1. The current user is almost always included in the attendees list, making rules like "list_contains user@example.com" match nearly every event
2. The attendee count includes the user, so "1 attendee" often means just the user themselves
3. It's unclear whether rules should match "other attendees" vs "all attendees including self"

**Steps to reproduce:**
1. Create a rule with attendees list_contains condition
2. Notice it matches more events than expected because user is always an attendee

**Expected behavior:**
- Consider separating "attendees" from "other_attendees" (excluding self)
- Clarify attendee count display (e.g., "1 other" vs "2 total")
- Document the behavior clearly

**Actual behavior:**
User is included in attendees list, making attendee-based rules overly broad.

---

## Fixed Bugs

### BUG-001: Week view layout clipped with long event descriptions [FIXED]
**Reported:** 2025-12-06
**Fixed:** 2025-12-06
**Severity:** Low
**Description:**
The week view layout gets clipped when the descriptive text of an event is too long. This appears to affect the time entry (classified) view only, not the unclassified event view.

**Fix:**
Changed `.entry-description` CSS from single-line truncation to multi-line with `-webkit-line-clamp: 2` and `word-break: break-word`.

---

### BUG-002: Classified event card doesn't show project association on event side [FIXED]
**Reported:** 2025-12-06
**Fixed:** 2025-12-06
**Severity:** Low
**Description:**
When an event is classified, the time entry side shows the project color as the background. However, when you flip to the event side (calendar view), there's no visual indication that this event has been classified or which project it belongs to.

**Fix:**
Added a colored corner triangle badge (`.project-badge`) to the event side of classified cards. The badge uses the project color and updates dynamically when classifying/unclassifying.

---

### BUG-004: Rule creation uses JavaScript alert for confirmation [FIXED]
**Reported:** 2025-12-06
**Fixed:** 2025-12-06
**Severity:** Low
**Description:**
When creating a rule from the week view modal, a JavaScript `alert()` is used to confirm successful save. This is clunky and breaks the UX flow.

**Fix:**
Implemented a toast notification system with `showToast()` function. Toast notifications appear in the top-right corner with success/error/info styling and auto-dismiss after 3 seconds.

---

### BUG-018: Auto-apply rules when navigating to a different week [FIXED]
**Reported:** 2025-12-06
**Fixed:** 2025-12-07
**Severity:** Low
**Description:**
When navigating to a different week, existing unclassified events were not automatically classified by rules. Rules only applied during sync.

**Fix:**
Added `autoClassifyEvents()` function that calls `/api/rules/apply` on page load for the current week's date range. If any events are classified, the page reloads to show the updated state. Uses sessionStorage to prevent reload loops.

---

### BUG-027: Show project hours summary sidebar in week view [FIXED]
**Reported:** 2025-12-07
**Fixed:** 2025-12-07
**Severity:** Medium
**Description:**
Add a sidebar or panel next to the week view showing total hours per project for the current week. This provides at-a-glance visibility into time allocation and serves as the control for showing/hiding projects in the week view.

**Fix:**
Added a right sidebar (`<aside class="project-summary-sidebar">`) to the week view with:
- Total hours display at the top
- List of projects with checkbox toggles to show/hide
- Color indicator and project name
- Hours per project
- Visual progress bar showing relative allocation
- Responsive design (stacks below grid on narrow screens)

Backend changes in `routes/ui.py` to calculate `project_summary` and `total_hours` from classified events. JavaScript `toggleProjectVisibility()` function added to show/hide event cards by project.
