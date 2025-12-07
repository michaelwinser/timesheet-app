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

### BUG-035: Migration system lacks tracking - migrations not idempotent
**Reported:** 2025-12-07
**Severity:** Medium
**Description:**
The migration system runs all SQL files on application startup but doesn't track which migrations have been applied. Migrations must be idempotent (using `CREATE TABLE IF NOT EXISTS`) or they will fail on restart.

**Current behavior:**
- Migrations run once on application startup (`main.py`)
- All `.sql` files in `/migrations/` directory are executed in sorted order
- No tracking of which migrations have been applied
- Migrations must be idempotent or app won't restart

**Impact:**
- Cannot write non-idempotent migrations (e.g., `ALTER TABLE ADD COLUMN` fails on second run)
- No visibility into which migrations have been applied
- No rollback capability
- Cannot safely add columns or modify schema incrementally

**Example failure:**
```
sqlite3.OperationalError: duplicate column name: my_response_status
```
This occurs when a migration adds a column, the app restarts, and the migration tries to add the same column again.

**Proposed solution:**
1. Create `schema_migrations` table to track applied migrations
2. Only run migrations that haven't been applied yet
3. Record migration name and timestamp when applied
4. Optional: Add rollback support

Example tracking table:
```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

**Workaround:**
Only write idempotent migrations using `IF NOT EXISTS` clauses.

**Related:**
- See `docs/ABANDONED-multi-user-per-db.md` for lessons learned from previous migration approach
- Would be critical for multi-user support where each user has their own database

---

### BUG-036: Add serialized event text property for simpler rule matching
**Reported:** 2025-12-07
**Severity:** Low
**Description:**
Add a computed property on events that serializes all event properties into a single searchable text string. This would make rule writing simpler by allowing a single "contains" condition instead of multiple separate conditions.

**Current behavior:**
To match an event based on multiple properties, users must create separate rule conditions:
- Title contains "standup"
- OR Attendees contains "team@company.com"
- OR Description contains "daily sync"

**Proposed solution:**
Add a computed `event_text` property that concatenates relevant event properties:
```
event_text = f"{title} {description} {attendees_joined} {meeting_link} {calendar_id}"
```

Example serialized text:
```
"Daily Standup Discuss sprint progress alice@company.com,bob@company.com https://meet.google.com/abc-defg-hij user@gmail.com"
```

**Benefits:**
- Simpler rules: Single "text contains" condition instead of multiple OR conditions
- Better for fuzzy matching and LLM-based classification
- Easier to test rules (can see all searchable text at once)
- More intuitive for users ("search for any event mentioning X")

**Implementation:**
1. Add `event_text` computed property to EventProperties class in `classifier.py`
2. Add "text_contains" or "text_matches" condition type to rules
3. Update rule matching logic to check against event_text
4. Consider caching event_text in database for performance (optional)

**Example use cases:**
- Rule: "text contains 'client name'" matches any event related to that client
- Rule: "text contains 'zoom.us'" matches all Zoom meetings
- Rule: "text contains '@contractor.com'" matches all meetings with contractor attendees

**Alternative approach:**
Instead of a single text blob, could provide structured search across all fields simultaneously (e.g., Elasticsearch-style query). But simple text concatenation is easier to implement and understand.

**Priority:**
Low - current multi-condition rules work, this is a convenience improvement

---

### BUG-037: Add regular expression support to rule conditions
**Reported:** 2025-12-07
**Severity:** Low
**Description:**
Allow regular expressions in rule conditions to enable flexible pattern matching and OR-style patterns within a single property. This would simplify rules that need to match multiple variants of similar text.

**Current behavior:**
To match multiple patterns in a single property, users must create separate rule conditions:
- Title contains "standup"
- OR Title contains "sync"
- OR Title contains "daily meeting"
- OR Title contains "scrum"

**Proposed solution:**
Add a new condition type "matches_regex" or "regex" that accepts regular expression patterns:
```
Title matches_regex "standup|sync|daily meeting|scrum"
```

**Benefits:**
- Fewer rule conditions needed for OR-style matching
- More powerful pattern matching (e.g., case-insensitive, word boundaries, wildcards)
- Better for complex patterns like email domains, phone numbers, dates
- Familiar syntax for technical users

**Implementation:**
1. Add `matches_regex` condition type to classification rules
2. Use Python's `re` module for pattern matching
3. Add regex flags support (case-insensitive, multiline, etc.)
4. Validate regex patterns when creating/updating rules (catch syntax errors)
5. Consider performance implications (regex can be slower than simple string contains)
6. Add UI indicator for regex conditions (e.g., "Regex:" label)

**Example use cases:**
- Match any standup variant: `title matches_regex "stand[ -]?up|daily sync|scrum"`
- Match email domains: `attendees matches_regex "@(company|contractor|client)\.com"`
- Case-insensitive matching: `title matches_regex "(?i)client name"` (case-insensitive flag)
- Word boundaries: `title matches_regex "\bproject\b"` (match "project" but not "projects")
- Date patterns: `title matches_regex "\d{4}-\d{2}-\d{2}"` (YYYY-MM-DD dates)

**UI considerations:**
- Provide regex help/examples in rule creation UI
- Show validation errors for invalid regex patterns
- Consider a "test regex" feature to preview matches
- Maybe provide regex builder/assistant for non-technical users

**Security/Safety:**
- Validate regex patterns to prevent ReDoS (Regular Expression Denial of Service)
- Set regex timeout to prevent hanging on pathological patterns
- Limit regex complexity (max length, nesting depth)

**Performance:**
- Regex matching can be slower than simple string operations
- Consider caching compiled regex patterns
- Maybe add performance warning for complex patterns

**Alternative approaches:**
- Use glob patterns instead (simpler, but less powerful): `title matches "standup*"`
- Provide pre-built pattern library (common patterns like email, URL, etc.)

**Priority:**
Low - current multi-condition rules work, but regex would significantly improve power users' experience

**Related:**
- Works well with BUG-036 (event_text property) - could regex search across all properties

---

### BUG-038: Add keywords and contacts fields to Projects for classification
**Reported:** 2025-12-07
**Severity:** Medium
**Description:**
Allow users to specify keywords and contact emails directly on Project definitions. These serve as classification hints that can be used by LLM classifiers or automatically converted into classification rules.

**Current behavior:**
Projects only have name, color, and visibility fields. To classify events to a project, users must:
1. Manually classify individual events, or
2. Create separate classification rules with attendee/title conditions

There's no natural place to specify "what makes an event belong to this project" in simple terms.

**Proposed solution:**
Add two optional fields to the Projects table/UI:
1. **Keywords**: Comma-separated list of keywords/phrases that indicate this project
2. **Contacts**: Comma-separated list of email addresses or domains associated with this project

**Example Project definition:**
```
Project: Alpha-Omega Security Initiative
Keywords: alpha-omega, security, vulnerability, OSS security
Contacts: alice@example.com, bob@contractor.com, @alpha-omega.dev
```

**Use cases:**

1. **LLM Classification Context:**
   - When using LLM to classify events, include project keywords/contacts in the prompt
   - "This project is about: alpha-omega, security, vulnerability..."
   - "Key people: alice@example.com, bob@contractor.com"
   - Gives LLM better context to make accurate classifications

2. **Auto-generate Rules:**
   - Button: "Generate rules from keywords/contacts"
   - Automatically creates rules like:
     - `title contains "alpha-omega"` → Alpha-Omega project
     - `attendees list_contains alice@example.com` → Alpha-Omega project
     - `attendee_domain = alpha-omega.dev` → Alpha-Omega project
   - Saves users from manually creating these obvious rules

3. **Simpler Project Setup:**
   - Natural place to define project characteristics during setup
   - More intuitive than immediately jumping to rule creation
   - Useful for users who don't understand rule syntax

**Benefits:**
- Centralizes project classification information in one place
- Simpler mental model: "define what the project is about"
- Useful for both manual (LLM) and automatic (rules) classification
- Can be gradually enhanced (start with keywords, add rules later)
- Good foundation for future ML/AI classification features

**Implementation:**

Database schema changes:
```sql
ALTER TABLE projects ADD COLUMN keywords TEXT;
ALTER TABLE projects ADD COLUMN contacts TEXT;
```

UI changes:
1. Add Keywords and Contacts fields to project creation/edit form
2. Show keywords/contacts on project detail/list views
3. Add "Generate Rules" button next to these fields
4. Provide inline help/examples for both fields

Backend changes:
1. Store keywords and contacts as comma-separated text (or JSON array)
2. API endpoint to auto-generate rules from project keywords/contacts
3. Include keywords/contacts in LLM classification prompts (future)

**UI mockup:**
```
Project Name: [Alpha-Omega Security Initiative    ]
Color:        [🎨 #6366f1                         ]
Keywords:     [alpha-omega, security, OSS         ] ℹ️ Words/phrases in event titles
Contacts:     [alice@, @alpha-omega.dev           ] ℹ️ Emails or domains of attendees
              [Generate Rules from Keywords/Contacts]

Visibility:   ☑ Show in dropdowns
```

**Keywords field format:**
- Comma-separated list of keywords or phrases
- Case-insensitive matching
- Examples: "standup, daily sync", "client name", "project-code-123"

**Contacts field format:**
- Comma-separated list of:
  - Full email addresses: `alice@example.com`
  - Email domains (prefix with @): `@clientcorp.com`
- Examples: "alice@company.com, bob@contractor.com, @client.com"

**Auto-generate rules logic:**
For each keyword:
- Create rule: `title contains "{keyword}"` → Project

For each full email:
- Create rule: `attendees list_contains "{email}"` → Project

For each domain:
- Create rule: `attendee_domain = "{domain}"` → Project

**Future enhancements:**
- Use keywords/contacts as training data for ML classification
- Suggest keywords based on already-classified events
- Highlight events that match keywords but aren't classified yet
- Scoring: events matching more keywords = higher classification confidence

**Related bugs:**
- BUG-015: Organize rules by project (keywords/contacts are project-level metadata)
- BUG-016: Email domain as rule property (contacts field can specify domains)
- BUG-036: Event text property (keywords would search this text field)
- BUG-010: Confidence levels (matching multiple keywords = higher confidence)

**Priority:**
Medium - this would significantly simplify project setup and classification, and provides foundation for future LLM/ML features

---

### BUG-039: Improve project show/hide feature with persistent visibility and UI controls
**Reported:** 2025-12-07
**Severity:** Medium
**Description:**
The current project show/hide feature in the week view sidebar uses sessionStorage and is not editable from the Projects page. The visibility state should be persisted in the database with the project, and users should be able to edit it from the Projects management page.

**Current behavior:**
- Project visibility is toggled via checkboxes in the week view sidebar
- Visibility state is stored in sessionStorage (see BUG-028 for related issue)
- Projects table has `is_visible` field but it's not used for show/hide filtering
- No way to edit visibility from the Projects page
- Projects list is sorted alphabetically only

**Issues with current implementation:**
1. SessionStorage is volatile - doesn't persist across browser close/reload
2. Visibility settings are per-week, not global
3. No UI to manage visibility outside of week view
4. Unclear relationship between `is_visible` database field and sessionStorage
5. Projects list doesn't group by visibility status

**Proposed solution:**
Change the implementation so that:
1. **Database-backed visibility**: Use the existing `is_visible` field in projects table as the source of truth
2. **Edit on Projects page**: Add UI controls on `/projects` to toggle visibility for each project
3. **Organized list**: Sort projects by visibility status first (shown → hidden), then alphabetically within each group

**Implementation:**

Database changes:
- `is_visible` field already exists, just need to use it properly
- Default to `true` for new projects

Week view changes:
- Replace sessionStorage logic with database-backed visibility
- When toggling visibility in sidebar, update database via API call
- Remove week-specific visibility keys from sessionStorage
- Filter projects in backend query based on `is_visible` field

Projects page changes:
- Add visibility toggle UI for each project (checkbox, toggle switch, or eye icon)
- Show/hide status clearly (e.g., "👁 Shown" vs "🚫 Hidden")
- Group projects: "Shown Projects" section, then "Hidden Projects" section
- Each section sorted alphabetically

API changes:
- PATCH `/api/projects/{id}` to update `is_visible` field
- Backend filtering honors `is_visible` for dropdowns
- Week view can include all projects in sidebar (for toggling) but filter event display

**UI mockup for Projects page:**
```
Projects

Shown Projects (5)
─────────────────────────────────────────────────
[👁] Alpha-Omega         #6366f1  [Edit] [Hide]
[👁] Client Work         #10b981  [Edit] [Hide]
[👁] FreeBSD             #f59e0b  [Edit] [Hide]
[👁] Internal            #f43f5e  [Edit] [Hide]
[👁] Research            #06b6d4  [Edit] [Hide]

Hidden Projects (2)
─────────────────────────────────────────────────
[🚫] Archived Project    #8b5cf6  [Edit] [Show]
[🚫] Junk               #cbd5e1  [Edit] [Show]

[+ Create New Project]
```

**UI mockup for Week view sidebar:**
```
Week of Dec 2-8, 2024
Total: 32.5 hours

☑ Alpha-Omega      8.5h  ████████░
☑ Client Work      12h   ████████████░
☑ FreeBSD          6h    ██████░
☑ Internal         4h    ████░
☑ Research         2h    ██░
☐ Junk            0h    ░

[Toggle: ☑ Show hidden projects]
```

**Benefits:**
- Persistent visibility settings across sessions
- Centralized project management on Projects page
- Clearer organization: shown vs hidden projects
- Consistent with user expectations (database-backed, not browser storage)
- Removes dependency on sessionStorage for important data

**Related to:**
- BUG-028: Persist project visibility filter (this bug addresses root cause)
- BUG-029: Sort project lists alphabetically (extends to group by visibility first)

**Migration considerations:**
- Existing projects with `is_visible=true` continue to work
- Remove sessionStorage visibility keys (or migrate them to database updates)
- May want to preserve user's current sessionStorage preferences on first load

**Priority:**
Medium - affects organization and persistence of project visibility, which is a core feature

---

### BUG-040: Add "Don't track hours" option for pseudo-projects
**Reported:** 2025-12-07
**Severity:** Medium
**Description:**
Add an option on projects to mark them as non-tracked, meaning events classified to these projects don't accumulate hours in summaries, reports, or timesheets. This is useful for pseudo-projects like "Junk" that classify away unwanted calendar events (synchronized busy blocks, declined meetings, personal events) without inflating work hour totals.

**Current behavior:**
- All classified events contribute to hour totals
- "Junk" or "Did Not Attend" projects show up in weekly summaries
- No way to distinguish between billable/trackable work and administrative classifications
- Users must manually subtract non-work hours from totals

**Use cases:**

1. **Junk/Noise project**: Classify away calendar noise without it counting as work
   - Synchronized busy blocks from personal calendar
   - Placeholder events
   - Cancelled meetings that weren't removed
   - Calendar spam

2. **Did Not Attend**: Mark meetings that were skipped (see BUG-014)
   - Still on calendar, but didn't participate
   - Shouldn't count toward work hours

3. **Personal/Administrative**: Non-billable overhead that shouldn't count
   - Lunch blocks
   - Personal appointments
   - Admin tasks that aren't tracked

4. **Out of Office**: Vacation, sick days, holidays
   - Blocks time on calendar
   - Shouldn't count as working hours

**Proposed solution:**
Add a `track_hours` boolean field to projects (default: `true`). When `false`, events classified to this project:
- Are NOT included in hour totals (weekly summary, daily totals, reports)
- Still appear in the week view (optionally dimmed/styled differently)
- Still count as "classified" (not shown as needing classification)
- Are excluded from timesheet exports

**Implementation:**

Database schema changes:
```sql
ALTER TABLE projects ADD COLUMN track_hours BOOLEAN DEFAULT TRUE;
```

Projects page UI:
```
Project: Junk
Color:   #cbd5e1
☐ Track hours in summaries and reports
   (Uncheck for pseudo-projects like "Junk", "Did Not Attend", etc.)

Visibility: ☐ Show in dropdowns
```

Week view changes:
- Filter hour calculations to exclude `track_hours=false` projects
- Optionally dim/gray out non-tracked event cards
- Maybe add subtle indicator (e.g., "⊗" icon) on non-tracked entries

Backend query changes:
```python
# Week view hour totals
project_hours = db.execute("""
    SELECT p.id, p.name, SUM(te.hours) as total_hours
    FROM time_entries te
    JOIN projects p ON te.project_id = p.id
    WHERE p.track_hours = TRUE  -- ← Filter here
    AND date(te.event_start_time) >= ? AND date(te.event_start_time) <= ?
    GROUP BY p.id
""")

# Total hours calculation
total_hours = db.execute("""
    SELECT SUM(te.hours)
    FROM time_entries te
    JOIN projects p ON te.project_id = p.id
    WHERE p.track_hours = TRUE  -- ← Filter here
    AND date(te.event_start_time) >= ? AND date(te.event_start_time) <= ?
""").fetchone()[0]
```

API changes:
- PATCH `/api/projects/{id}` accepts `track_hours` field
- GET `/api/export` filters to `track_hours=true` projects only

**UI mockup for project summary sidebar:**
```
Week of Dec 2-8, 2024
Total: 32.5 hours  (tracked only)

☑ Alpha-Omega      8.5h  ████████░
☑ Client Work      12h   ████████████░
☑ FreeBSD          6h    ██████░
☑ Internal         4h    ████░
☑ Research         2h    ██░

Non-tracked:
☑ Junk            8.5h   (not counted)
☐ Did Not Attend  2h     (not counted)
```

**Alternative UI: Toggle to show/hide non-tracked:**
```
Week of Dec 2-8, 2024
Total: 32.5 hours

[Tracked Projects]
Alpha-Omega      8.5h
Client Work      12h
FreeBSD          6h

[Show non-tracked projects ▼]  ← Collapsible section
```

**Visual styling for non-tracked events:**
- Lighter opacity (0.6)
- Gray border or muted colors
- Small "⊗" icon in corner
- Tooltip: "Hours not tracked"

**Benefits:**
- Cleaner, more accurate hour totals
- Enables proper "Junk" classification workflow
- Supports "Did Not Attend" use case (BUG-014)
- Reports reflect actual work, not calendar noise
- Users can classify everything without inflating totals

**Edge cases:**
- Export: Should non-tracked entries appear? (Probably not, or separate section)
- Daily totals (BUG-032): Show both tracked and total? Or only tracked?
- Search/filter: Should be able to see non-tracked events when needed

**Related to:**
- BUG-014: "Did not attend" option (this enables that use case)
- BUG-032: Daily hour totals (need to decide what to show)
- BUG-039: Project visibility (similar database-backed project metadata)

**Migration:**
- Set `track_hours=true` for all existing projects
- Optionally prompt user to set `track_hours=false` for projects named "Junk", "Did Not Attend", "Personal", etc.

**Priority:**
Medium - improves accuracy of reports and enables important workflows for filtering calendar noise

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
