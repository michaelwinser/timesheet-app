# PRD: Automatic Timesheet Creation from Calendar

## 1. Problem Statement

**What problem are we solving?**
Creating accurate timesheets from calendar data is tedious and error-prone. Users who track billable time often duplicate effort: they schedule meetings and work blocks on their calendar, then manually re-enter the same information into a time-tracking system like Harvest.

**Who experiences this problem?**
Consultants, freelancers, and professionals who bill clients based on time spent, and whose work schedule is already represented in their calendar.

**What's the impact of not solving it?**
- Time wasted on duplicate data entry
- Inaccurate timesheets due to forgotten entries or estimation
- Delayed invoicing when timesheet completion is procrastinated

**Why now?**
The user wants to streamline their personal workflow, with a path to making this available to others if successful.

## 2. Goals & Success Criteria

**What does success look like?**
- A user can review a week's calendar events, classify them by project, and export a Harvest-compatible timesheet in minutes rather than the current manual process
- Classification becomes faster over time as the system learns from past decisions

**How will we measure it?**
- Time to complete a weekly timesheet (target: under 5 minutes for a typical week)
- Percentage of events auto-classified correctly (target: >80% after 4 weeks of use)

**Primary outcomes we're optimizing for:**
1. Speed of timesheet completion
2. Accuracy of time entries
3. Low friction UX that fits naturally with calendar mental model

## 3. Non-Goals & Constraints

**What are we explicitly NOT doing?**
- Building a full time-tracking replacement (Harvest remains the system of record)
- Supporting calendars other than Google Calendar in the initial version
- Handling billing, invoicing, or financial calculations
- Multi-user / team features in the initial version

**Constraints:**
- Must run in a Docker container on TrueNAS
- Single-user deployment initially
- OAuth flow required for Google Calendar access

**Ruled-out approaches:**
- Service account with delegated access (doesn't scale to multi-user future)
- Desktop app (less portable, harder to deploy on TrueNAS)

Note: The PRD's purpose is to define the problem to be solved through engineering, not to justify business needs or build a case for the product.

## 4. Solution Overview

**High-level description:**
A web application that:
1. Connects to a user's Google Calendar via OAuth
2. Fetches calendar events for a selected time period
3. Displays events in a week view with day focus
4. Allows the user to classify events by project, transforming them into time entries
5. Learns from past classifications to suggest/auto-classify future events
6. Exports classified time entries as Harvest-compatible CSV

**Key user flows:**

1. **Login**: User visits dedicated login page, clicks "Login with Google", completes OAuth flow, and is returned to their original page (or current week if coming from login page directly)
2. **Initial setup**: User authenticates with Google, defines projects (or imports via CSV)
3. **Weekly review**: User opens the week view, sees unclassified events, classifies them one by one (or in bulk)
4. **Export**: User exports the week's time entries as CSV for Harvest import

**Core concepts and terminology:**
- **Event**: A calendar entry from Google Calendar (meeting, work block, etc.)
- **Time Entry**: A classified event with project assignment, ready for export
- **Project**: A billable category defined by the user (with settings for billing, visibility, and hour tracking)
- **Classification**: The act of assigning a project to an event, transforming it into a time entry
- **Did Not Attend**: A flag on events indicating the user did not attend the scheduled meeting (excludes from time tracking)
- **Noise Project**: A special project type that classifies events without accumulating tracked hours

**The "flip card" interaction:**
- Unclassified events display as calendar meetings: title, attendees, video links, etc.
- Once classified, the card "flips" to show time entry data: project, hours, description
- User can flip back and forth to see both representations

## 5. Detailed Requirements

### Functional Requirements

**Authentication & Session Management:**
- Dedicated login page at `/login` with "Login with Google" button
- OAuth 2.0 flow for Google Calendar access (initiated from login page)
- Session-based authentication with 24-hour session expiry
- All application pages require authentication:
  - Unauthenticated requests redirect to `/login?next=<original-url>`
  - After successful OAuth login, redirect to `next` parameter URL (or `/` if not provided)
  - This allows users to resume where they left off after session expiry or server restart
- Home page `/` displays the current week (same as "Today" button behavior)
- Logout functionality:
  - Clears OAuth tokens and session
  - Redirects to `/login` (not directly to OAuth flow)
- User email displayed in header when authenticated
- Session persists across browser sessions (using secure HTTP-only cookies)

**Route structure:**
- `/login` - Login page with "Login with Google" button (public)
- `/auth/login` - OAuth redirect endpoint (internal, initiates Google OAuth flow)
- `/auth/callback` - OAuth callback handler (internal, processes OAuth response)
- `/auth/logout` - Logout endpoint (clears session)
- `/` - Home page showing current week calendar (requires auth)
- `/week/<date>` - Specific week view (requires auth)
- `/projects` - Project management (requires auth)
- `/rules` - Classification rules (requires auth)

**Google Calendar Integration:**
- OAuth 2.0 authentication flow
- Fetch events for a specified date range
- Handle recurring events appropriately
- Sync on demand (not real-time)
- Calendar selection: Start with single calendar; evolve to dropdown selector, then combined multi-calendar view

**Week View UI:**
- Display 7 days with time slots
- Visual approximation of Google Calendar's week view
- Highlight/focus on a single day within the week (today highlighted)
- Events displayed as cards with flip interaction
- Text-based filter: search across all event/entry attributes (title, description, project, attendees, etc.)
- Navigation: Prev/Today/Next buttons for week navigation
- Auto-sync: Page automatically syncs calendar events on load; reloads if new events are found
- Refresh button for manual re-sync
- Future: syntactic sugar for field-specific filters (e.g., `project:name`, `@attendee`)

**Sidebar Project Summary:**
The sidebar displays project-grouped hour totals for the current week view, organized into three sections:

1. **Projects** (main section): Active, non-hidden projects with checkboxes for filtering
   - Each project shows name and total hours
   - Checkbox controls visibility of that project's entries in the week view
   - Uses project color for visual identification

2. **Hidden** (collapsed by default): Projects with `is_hidden_by_default = true`
   - Appears as collapsed section "Hidden (N)" with total hours
   - Click to expand and reveal individual hidden projects
   - Expansion state is session-only (resets on page load)
   - Hidden projects' entries are excluded from week view unless expanded and checked

3. **Archived** (conditional): Only appears when time entries exist for archived projects
   - Appears with warning indicator (⚠) to signal attention needed
   - Shows which archived projects have entries in current view
   - Presence in UI serves as prompt to reclassify those entries

**Event Card (unclassified side):**
- Event title (summary)
- Event description (body/notes)
- Start/end time
- Attendees (if any)
- Video conferencing link (if any)
- Link to open event in Google Calendar (useful for private/confidential events)
- Visual indicator that it's unclassified

**Time Entry Card (classified side):**
- Project dropdown (allows reclassification; empty selection unclassifies)
- Hours (calculated from event duration, editable - see note below)
- One-click "+15m" button (rounds up to nearest 15 min, or adds 15 min if already rounded)
- Description (defaults to event title, editable)
- Visually distinct: colored background using project's assigned color
- Flip button to return to event view

Note on hours editing: Calendar duration often doesn't match actual time spent (e.g., Google's "speedy meetings" feature, meetings running over). Users should be able to override the calculated hours freely.

**Classification:**
- Manual: user selects project from dropdown
- Suggested: system proposes project based on past classifications
- Auto: high-confidence matches classified automatically (user can review/override)
- Bulk actions: classify multiple similar events at once

**Classification Rule Targets:**
Rules can have two types of targets:
1. **Project target**: Assigns event to a specific project (traditional classification)
2. **Did Not Attend target**: Sets the did_not_attend flag on matching events (excludes from time tracking)

A rule must have exactly one target type. This allows rules like:
- "If response_status is 'needsAction', mark as Did Not Attend"
- "If title contains 'Optional:', mark as Did Not Attend"

**Classification Learning:**
- Store classification decisions in SQLite
- Use past decisions to suggest future classifications
- Initial implementation: rule-based matching on:
  - Title keywords
  - Attendees
  - Calendar source
  - Recurring event series (if last week's instance was classified as Project X, this week's should match)
- Future considerations:
  - Event color as classification signal (user-controlled even on meetings they don't own)
  - Lightweight ML for fuzzy matching

**Project Management:**
- CRUD operations for projects
- Inline editing: click Edit to modify name/client in-place
- Per-project color assignment via color picker (used for time entry card background)
- Import projects from CSV (planned)
- Export projects to CSV (planned)

**Project Settings:**
Each project has the following configurable settings:
- **Does not accumulate hours** (boolean, default: false): Time entries in this project are excluded from hour totals and exports. Useful for "Noise" projects that classify events without tracking time.
- **Billable** (boolean, default: false): Whether time on this project is billable to a client.
- **Bill rate** (decimal, optional): Hourly rate for billing calculations. Only relevant when billable is true.
- **Hidden by default** (boolean, default: false): Time entries in this project are hidden in the UI by default. Users can reveal hidden projects via sidebar toggle. Hidden projects appear in a collapsed "Hidden" group at the bottom of the sidebar.
- **Archived** (boolean, default: false): Archived projects do not appear in the UI at all (dropdowns, sidebar). If rules classify to an archived project, the entry appears in a warning "Archived" section in the sidebar. Existing time entries remain in the database.

**Event Attendance Tracking:**
- Events have a "Did Not Attend" flag (boolean, default: false)
- When set, the event is excluded from time tracking and exports
- Classification rules can set this flag based on conditions (e.g., `my_response_status NOT IN ('accepted', 'tentative')`)
- Users can manually toggle the flag on individual events
- Useful for recurring meetings where attendance varies

**Export:**
- Generate Harvest-compatible CSV
- Columns: Date, Client, Project, Task, Hours, Notes (map to Harvest's import format)
- Export selected date range

**Data Persistence:**
- SQLite database for:
  - Projects
  - Time entries (classified events persist across sessions)
  - Classification rules and history (event patterns → project mappings)
  - Cached calendar events (to reduce API calls)
  - User preferences (including per-project visibility settings)

### Non-Functional Requirements

- **Deployment**: Docker container with docker-compose.yaml for TrueNAS custom app
- **Performance**: Week view should load in under 2 seconds
- **Security**: OAuth tokens stored securely, no plain-text credentials
- **Data**: All data stored locally (SQLite), no external dependencies beyond Google Calendar API

### Edge Cases

- **Multi-day events**: Anchor on the day when the event ends (handles timezone edge cases pragmatically)
- **All-day events**: Treat as classifiable like any other event; user can hide via project visibility if not billable
- **Declined meetings**: Exclude from view by default (configurable)
- **Cancelled events**: Exclude from view
- **Private/confidential events**: Display minimal info with link to open in Google Calendar; classify like any other event
- **Events with no title**: Display as "Untitled" with other available metadata
- **Overlapping events**: See dedicated section below

**Overlapping Events Handling:**

Classify first, merge second. The overlap logic runs after classification:

1. **Containment**: A large event (e.g., conference day) "absorbs" smaller events of the same project/client that fall entirely within it
2. **Perfect overlap**: Merge into a single entry
3. **Partial overlap**: First event (by start time) gets its full duration; subsequent overlapping events get remaining time only

Ethical consideration (billing the same hour twice) is left to the user - they can see overlaps in the UI. Merge functionality is a future enhancement.

## 6. Open Questions

1. **Hours editing bounds**: When a user edits hours, should there be any constraints (e.g., can they enter 2 hours for a 30-minute meeting)? Current assumption: no constraints, trust the user.

2. **Classification confidence thresholds**: What confidence level triggers auto-classification vs. suggestion vs. manual? Needs experimentation.

3. **Undo/history**: Should users be able to undo classifications or see classification history for an event?

4. **Orphaned time entries**: When a calendar event is deleted in Google Calendar, the associated time entry becomes orphaned. These should display with a warning indicator. Need to decide: how can users delete orphaned entries? Should they be auto-deleted after some period?

5. ~~**Week nav link behavior**~~: Resolved. Navigation has "Calendar" link (always goes to current week) plus Prev/Today/Next buttons for date navigation. "Today" button provides quick return to current week from any other week.

6. **Classification timing and control**: When should auto-classification run? Current implementation classifies during sync (tightly coupled). Preference is for more decoupling:
   - Classification should primarily run on the backend as a batch operation
   - Avoid per-event classification calls from the frontend
   - Provide explicit "Reclassify" UI control for user-initiated reclassification of current week or selected events
   - Need to define: What triggers automatic classification? Just sync? Rule changes? Manual refresh?
   - Need to define: Should reclassification overwrite existing classifications or only fill in unclassified events?

## 7. Context for Claude

**Relevant files/directories:**
- `~/claude/prompts/` - reusable prompts that guide Claude's behavior
- `~/claude/projects/timesheet-app/` - this project's home (PRD, design docs, code)

**Architectural constraints:**
- Web app (frontend + backend)
- Docker-based deployment
- SQLite for persistence (single-file, portable)
- Google Calendar API via OAuth 2.0

**Technology considerations (to be decided in design doc):**

Backend candidates (preference for lightweight, no heavy frameworks):
- **Go**: Fast, single binary, minimal runtime. Good for Docker. Less obvious for rapid UI prototyping.
- **Python (FastAPI/Flask)**: Fastest to prototype, excellent Google API libraries. Slightly heavier container.
- **Ruby (Sinatra/Roda)**: Pleasant to write, lightweight. Smaller Google Calendar library ecosystem.
- **Google Apps Script**: Native Calendar integration (no OAuth for calendar), built-in hosting. Limitations: less infrastructure control, JavaScript-only, quotas. Could serve as whole solution for v1 or as companion add-on for sideband classification.

Frontend: TBD (consider lightweight options; HTMX worth exploring for simplicity)

CSS: Consider a calendar/scheduling component library to accelerate week view development

**Implementation notes:**
- Start with core flow: auth → fetch → display → classify → export
- Classification learning can start with simple keyword matching, evolve later
- The flip card interaction is central to the UX - prototype early

**Testing expectations:**
- Unit tests for classification logic
- Integration tests for Google Calendar API interactions
- Manual testing for UI flows initially

**Future directions (not in scope, but inform architecture):**
- Direct Harvest API integration (simple API, natural fast-follow once core flow works)
- Scheduled sync with auto-classification (e.g., 6am daily batch: fetch previous day's events, classify, have ready for review)
- Google Calendar add-on via Apps Script (sideband classification from within Calendar UI)
- Create rule from event: User can create a classification rule directly from an event card. UI shows the event's available properties and current values (title, attendees, weekday, etc.), letting the user select which properties to use as conditions and choose appropriate condition types (contains, equals, list_contains). Pre-populates condition values from the event. Quick path from "I want all events like this one to be classified the same way" to a working rule.
- Week totals summary: Display per-project total hours for the current week view. Shows at a glance how time is distributed across projects, helping users verify their timesheet before export. Could include total classified hours, total unclassified hours, and breakdown by project (using project colors for visual consistency).
- LLM-assisted classification:
  - Batch mode: feed events + rules + history to LLM for classification suggestions and rule generation
  - Interactive mode: natural language commands (e.g., "classify all meetings with user@example.com to ProjectA and make a rule")
  - Retroactive: "reclassify all events from ruleX with new settings"
- Multi-user SaaS offering
- Additional calendar providers (Outlook, CalDAV)
- Docker-based self-hosting distribution
- Google Cloud marketplace app
