# Known Bugs

Track bugs here. Mark as [FIXED] when resolved.

---

## Open Bugs

### BUG-001: Week view layout clipped with long event descriptions
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
The week view layout gets clipped when the descriptive text of an event is too long. This appears to affect the time entry (classified) view only, not the unclassified event view.

**Steps to reproduce:**
1. Classify an event that has a long title/description
2. View the time entry card (classified side)

**Expected behavior:**
Long text should be truncated or wrapped properly without breaking the card layout.

**Actual behavior:**
Layout is clipped/broken when description text is too long.

---

### BUG-002: Classified event card doesn't show project association on event side
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
When an event is classified, the time entry side shows the project color as the background. However, when you flip to the event side (calendar view), there's no visual indication that this event has been classified or which project it belongs to.

**Steps to reproduce:**
1. Classify an event with a project
2. Click "Flip" to view the calendar event side

**Expected behavior:**
The event side should retain some visual indication of its classification - either the project color as background, a colored corner banner, or similar visual cue.

**Actual behavior:**
The event side looks identical whether classified or not (aside from the Flip button being present).

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

### BUG-004: Rule creation uses JavaScript alert for confirmation
**Reported:** 2025-12-06
**Severity:** Low
**Description:**
When creating a rule from the week view modal, a JavaScript `alert()` is used to confirm successful save. This is clunky and breaks the UX flow.

**Steps to reproduce:**
1. Click the gear icon on an event card
2. Select properties and create a rule
3. Click "Create Rule"

**Expected behavior:**
Use an inline toast notification, success message within the modal, or simply close the modal and show a brief status indicator.

**Actual behavior:**
A browser `alert("Rule created successfully!")` pops up, requiring user to click OK.

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

## Fixed Bugs

(none yet)
