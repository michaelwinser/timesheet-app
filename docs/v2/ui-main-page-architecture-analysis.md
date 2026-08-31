# Main Calendar View Architecture Analysis

This document analyzes the state and component structure of the main calendar view (`web/src/routes/+page.svelte`), which at 1899 lines is the largest and most complex component in the application.

## Network Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                           MAIN CALENDAR VIEW (+page.svelte)                              │
│                                    1899 lines                                            │
└─────────────────────────────────────────────────────────────────────────────────────────┘

╔═══════════════════════════════════════════════════════════════════════════════════════════╗
║                                    CORE DATA STATE                                        ║
╠═══════════════════════════════════════════════════════════════════════════════════════════╣
║                                                                                           ║
║   ┌──────────────┐    ┌────────────────┐    ┌─────────────┐    ┌────────────────────┐    ║
║   │  projects[]  │    │ calendarEvents │    │  entries[]  │    │ calendarConnections│    ║
║   │   Project    │    │ CalendarEvent  │    │  TimeEntry  │    │ CalendarConnection │    ║
║   └──────┬───────┘    └───────┬────────┘    └──────┬──────┘    └─────────┬──────────┘    ║
║          │                    │                    │                     │               ║
║          └────────────────────┼────────────────────┼─────────────────────┘               ║
║                               │                    │                                     ║
║                    ┌──────────▼────────────────────▼─────────┐                           ║
║                    │           loadData()                    │                           ║
║                    │   Fetches all data on mount/nav/sync    │                           ║
║                    └─────────────────────────────────────────┘                           ║
╚═══════════════════════════════════════════════════════════════════════════════════════════╝

╔═══════════════════════════════════════════════════════════════════════════════════════════╗
║                                 VIEW/FILTER STATE                                         ║
╠═══════════════════════════════════════════════════════════════════════════════════════════╣
║                                                                                           ║
║   ┌─────────────┐   ┌───────────┐   ┌─────────────┐   ┌──────────────────────────────┐   ║
║   │ currentDate │──▶│ scopeMode │──▶│ displayMode │   │      visibleProjectIds       │   ║
║   │    Date     │   │day|week|  │   │calendar|list│   │        Set<string>           │   ║
║   └──────┬──────┘   │ full-week │   └──────┬──────┘   │  (persisted to sessionStorage)│   ║
║          │          └─────┬─────┘          │          └──────────────┬───────────────┘   ║
║          │                │                │                         │                   ║
║          ▼                ▼                ▼                         ▼                   ║
║   ┌──────────────────────────────────────────────────────────────────────────────────┐   ║
║   │                              DERIVED STATE                                        │   ║
║   ├──────────────────────────────────────────────────────────────────────────────────┤   ║
║   │  weekStart, weekDays, visibleDays, startDate, endDate                            │   ║
║   │       │                                                                          │   ║
║   │       ▼                                                                          │   ║
║   │  filteredEntries ◀─────┬────────▶ filteredCalendarEvents                         │   ║
║   │       │                │                │                                        │   ║
║   │       ▼                │                ▼                                        │   ║
║   │  entriesByDate         │         eventsByDate                                    │   ║
║   │  {date: Entry[]}       │         {date: Event[]}                                 │   ║
║   │       │                │                │                                        │   ║
║   │       ▼                ▼                ▼                                        │   ║
║   │  projectTotals    pendingEvents    weekendEvents                                 │   ║
║   │  hiddenTotals     reviewEvents                                                   │   ║
║   │  archivedTotals   skippedHours                                                   │   ║
║   │  totalHours       unclassifiedHours                                              │   ║
║   └──────────────────────────────────────────────────────────────────────────────────┘   ║
║                                                                                           ║
║   Filter toggles: showHiddenProjects, showArchivedProjects, showSkippedEvents            ║
╚═══════════════════════════════════════════════════════════════════════════════════════════╝

╔═══════════════════════════════════════════════════════════════════════════════════════════╗
║                              INTERACTION STATE                                            ║
╠═══════════════════════════════════════════════════════════════════════════════════════════╣
║                                                                                           ║
║  ┌─────────────────────┐     ┌────────────────────┐     ┌─────────────────────┐          ║
║  │   Hover Popup       │     │   Entry Popup      │     │   Highlight         │          ║
║  ├─────────────────────┤     ├────────────────────┤     ├─────────────────────┤          ║
║  │ hoveredEventId ─────┼────▶│ selectedEntryId ───┼────▶│ highlightedTarget   │          ║
║  │ hoveredElement      │     │ selectedEntryAnchor│     │ (from summary bar)  │          ║
║  │       │             │     │       │            │     └─────────────────────┘          ║
║  │       ▼             │     │       ▼            │                                      ║
║  │ hoveredEvent        │     │ selectedEntry      │     ┌─────────────────────┐          ║
║  │ ($derived from      │     │ ($derived from     │     │   Loading States    │          ║
║  │  calendarEvents)    │     │  entries)          │     ├─────────────────────┤          ║
║  └─────────────────────┘     └────────────────────┘     │ loading, syncing    │          ║
║                                                         │ classifyingId       │          ║
║                                                         │ scrollTrigger       │          ║
║                                                         └─────────────────────┘          ║
╚═══════════════════════════════════════════════════════════════════════════════════════════╝

╔═══════════════════════════════════════════════════════════════════════════════════════════╗
║                                 MODAL STATE                                               ║
╠═══════════════════════════════════════════════════════════════════════════════════════════╣
║                                                                                           ║
║  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐  ║
║  │  Add Entry Modal │  │  Go To Date      │  │  Reclassify Week │  │  Explain Modal   │  ║
║  ├──────────────────┤  ├──────────────────┤  ├──────────────────┤  ├──────────────────┤  ║
║  │ showAddModal     │  │ showGoToDateModal│  │ showReclassifyMdl│  │ showExplainModal │  ║
║  │ addDate          │  │                  │  │ reclassifyLoading│  │ explainEventId   │  ║
║  │ addProjectId     │  │                  │  │ reclassifyPreview│  │                  │  ║
║  │ addHours         │  │                  │  │ Results          │  │                  │  ║
║  │ addDescription   │  │                  │  │                  │  │                  │  ║
║  │ addSubmitting    │  │                  │  │                  │  │                  │  ║
║  └──────────────────┘  └──────────────────┘  └──────────────────┘  └──────────────────┘  ║
╚═══════════════════════════════════════════════════════════════════════════════════════════╝

╔═══════════════════════════════════════════════════════════════════════════════════════════╗
║                              CHILD COMPONENTS                                             ║
╠═══════════════════════════════════════════════════════════════════════════════════════════╣
║                                                                                           ║
║  ┌─────────────────────────────────────────────────────────────────────────────────────┐  ║
║  │                              LAYOUT COMPONENTS                                       │  ║
║  │  ┌─────────────────┐    ┌─────────────────────┐    ┌─────────────────────────────┐  │  ║
║  │  │  DateNavigator  │───▶│  ProjectSummaryBar  │───▶│      Calendar Panel         │  │  ║
║  │  │  - navigation   │    │  - project totals   │    │  (conditional rendering)    │  │  ║
║  │  │  - scope/display│    │  - visibility       │    │                             │  │  ║
║  │  │  - sync button  │    │  - highlight hover  │    │                             │  │  ║
║  │  └─────────────────┘    └─────────────────────┘    └─────────────────────────────┘  │  ║
║  └─────────────────────────────────────────────────────────────────────────────────────┘  ║
║                                                                                           ║
║  ┌─────────────────────────────────────────────────────────────────────────────────────┐  ║
║  │                    CALENDAR VIEW (displayMode='calendar')                           │  ║
║  │                                                                                      │  ║
║  │   scopeMode='day'           scopeMode='week'/'full-week'                            │  ║
║  │   ┌────────────┐           ┌─────────────────────────────────────────────────────┐  │  ║
║  │   │  TimeGrid  │           │  Unified Week Grid (inline ~200 lines)              │  │  ║
║  │   │  (single   │           │  ├─ Day headers with stats                          │  │  ║
║  │   │   day)     │           │  ├─ All-day events row (CompactEventCard)           │  │  ║
║  │   └────────────┘           │  ├─ Hour grid with time legend                      │  │  ║
║  │                            │  └─ Positioned events (inline event cards)          │  │  ║
║  │                            │     └─ calculateEventLayout() for overlaps          │  │  ║
║  │                            └─────────────────────────────────────────────────────┘  │  ║
║  └─────────────────────────────────────────────────────────────────────────────────────┘  ║
║                                                                                           ║
║  ┌─────────────────────────────────────────────────────────────────────────────────────┐  ║
║  │                       LIST VIEW (displayMode='list')                                │  ║
║  │                                                                                      │  ║
║  │   scopeMode='day'           scopeMode='week'/'full-week'                            │  ║
║  │   ┌────────────────────┐   ┌─────────────────────────────────────────────────────┐  │  ║
║  │   │ CompactEventCard   │   │  Day columns grid                                   │  │  ║
║  │   │ CalendarEventCard  │   │  └─ CompactEventCard (variant='compact')            │  │  ║
║  │   │ (hourly groups)    │   │                                                     │  │  ║
║  │   └────────────────────┘   └─────────────────────────────────────────────────────┘  │  ║
║  └─────────────────────────────────────────────────────────────────────────────────────┘  ║
║                                                                                           ║
║  ┌─────────────────────────────────────────────────────────────────────────────────────┐  ║
║  │                            TIME ENTRIES SECTION                                      │  ║
║  │                                                                                      │  ║
║  │   entryDisplayMode='graph'          entryDisplayMode='list'                         │  ║
║  │   ┌────────────────────────┐       ┌────────────────────────┐                       │  ║
║  │   │  TimeEntryBarChart[]   │       │   TimeEntryCard[]      │                       │  ║
║  │   │  (stacked bar per day) │       │   (simple list)        │                       │  ║
║  │   └────────────────────────┘       └────────────────────────┘                       │  ║
║  └─────────────────────────────────────────────────────────────────────────────────────┘  ║
║                                                                                           ║
║  ┌─────────────────────────────────────────────────────────────────────────────────────┐  ║
║  │                               POPUP COMPONENTS                                       │  ║
║  │  ┌─────────────┐   ┌────────────────┐   ┌─────────────────┐   ┌─────────────────┐   │  ║
║  │  │ EventPopup  │   │ TimeEntryPopup │   │ GoToDateModal   │   │ ReclassifyWeek  │   │  ║
║  │  │ (hover)     │   │ (click entry)  │   │ ('g' key)       │   │ Modal           │   │  ║
║  │  └─────────────┘   └────────────────┘   └─────────────────┘   └─────────────────┘   │  ║
║  │                                                                                      │  ║
║  │  ┌─────────────────────────┐   ┌─────────────────────────┐                          │  ║
║  │  │ ExplainClassification   │   │ Add Entry Modal         │                          │  ║
║  │  │ Modal                   │   │ (inline, not extracted) │                          │  ║
║  │  └─────────────────────────┘   └─────────────────────────┘                          │  ║
║  └─────────────────────────────────────────────────────────────────────────────────────┘  ║
╚═══════════════════════════════════════════════════════════════════════════════════════════╝

╔═══════════════════════════════════════════════════════════════════════════════════════════╗
║                             KEY DATA FLOW PATTERNS                                        ║
╠═══════════════════════════════════════════════════════════════════════════════════════════╣
║                                                                                           ║
║   1. CLASSIFICATION FLOW                                                                  ║
║   ┌────────────┐  classify   ┌─────────────┐  update    ┌──────────────┐                 ║
║   │ EventCard  │────────────▶│ handleClass │───────────▶│ calendarEvts │                 ║
║   │ /Popup     │  projectId  │ ify()       │  in-place  │ + entries    │                 ║
║   └────────────┘             └──────┬──────┘            └──────────────┘                 ║
║                                     │                                                    ║
║                                     ▼                                                    ║
║                              api.classifyCalendarEvent()                                 ║
║                              api.listTimeEntries() (reload)                              ║
║                                                                                           ║
║   2. SYNC FLOW                                                                            ║
║   ┌────────────┐  click      ┌─────────────┐  parallel  ┌──────────────┐                 ║
║   │ Sync Btn   │────────────▶│ handleManual│───────────▶│ sync all     │                 ║
║   │ or 'r' key │             │ Sync()      │  connections│ connections │                 ║
║   └────────────┘             └──────┬──────┘            └──────┬───────┘                 ║
║                                     │                          │                         ║
║                                     └──────────┬───────────────┘                         ║
║                                                ▼                                          ║
║                                     reload events + entries + connections                ║
║                                                                                           ║
║   3. VISIBILITY FILTERING                                                                 ║
║   ┌────────────┐  toggle     ┌─────────────┐   derive   ┌──────────────┐                 ║
║   │ Summary    │────────────▶│ visibleProj │───────────▶│ filtered     │                 ║
║   │ Bar        │  projectId  │ ectIds      │            │ Entries/Evts │                 ║
║   └────────────┘             └──────┬──────┘            └──────────────┘                 ║
║                                     │                                                    ║
║                                     ▼                                                    ║
║                              sessionStorage (persist)                                    ║
║                                                                                           ║
║   4. HOVER PATTERN (ID-based, anti-stale-data pattern)                                   ║
║   ┌────────────┐  mouseenter ┌─────────────┐  $derived  ┌──────────────┐                 ║
║   │ Event in   │────────────▶│ hoveredEvt  │───────────▶│ hoveredEvent │                 ║
║   │ Grid       │   event.id  │ Id (string) │  from array│ (always fresh)│                ║
║   └────────────┘             └─────────────┘            └──────────────┘                 ║
╚═══════════════════════════════════════════════════════════════════════════════════════════╝
```

## State Categories

### Core Data State
- `projects: Project[]` - All projects from API
- `calendarEvents: CalendarEvent[]` - Calendar events for current date range
- `entries: TimeEntry[]` - Time entries for current date range
- `calendarConnections: CalendarConnection[]` - Connected calendar accounts

### View/Filter State
- `currentDate: Date` - Currently selected date
- `scopeMode: 'day' | 'week' | 'full-week'` - Date range scope
- `displayMode: 'calendar' | 'list'` - How events are rendered
- `entryDisplayMode: 'graph' | 'list'` - How time entries are rendered
- `visibleProjectIds: Set<string>` - Which projects are visible (persisted to sessionStorage)
- `showHiddenProjects`, `showArchivedProjects`, `showSkippedEvents` - Filter toggles

### Derived State
- `weekStart`, `weekDays`, `visibleDays` - Date calculations
- `startDate`, `endDate` - API query range
- `filteredEntries`, `filteredCalendarEvents` - Filtered by visibility
- `entriesByDate`, `eventsByDate` - Grouped by date string
- `projectTotals`, `hiddenTotals`, `archivedTotals` - Aggregated hours
- `pendingEvents`, `reviewEvents`, `weekendEvents` - Event subsets
- `skippedHours`, `unclassifiedHours`, `totalHours` - Summary metrics

### Interaction State
- `hoveredEventId`, `hoveredElement` - Hover popup (ID-based pattern)
- `selectedEntryId`, `selectedEntryAnchor` - Entry popup
- `highlightedTarget` - Summary bar hover highlight
- `loading`, `syncing`, `classifyingId` - Loading states
- `scrollTrigger` - Triggers scroll-to-first-event

### Modal State
- Add Entry: `showAddModal`, `addDate`, `addProjectId`, `addHours`, `addDescription`, `addSubmitting`
- Go To Date: `showGoToDateModal`
- Reclassify: `showReclassifyModal`, `reclassifyLoading`, `reclassifyPreviewResults`
- Explain: `showExplainModal`, `explainEventId`

## Potential Extraction Candidates

| Component | Est. Lines | Dependencies | Complexity |
|-----------|------------|--------------|------------|
| WeekCalendarGrid | ~350 | eventsByDate, projects, handlers, layout calc | HIGH - inline events, layout calculation |
| TimeEntriesSection | ~80 | entriesByDate, handlers | LOW - mostly template |
| AddEntryModal | ~60 | addX state, handlers | LOW - form + state |
| CalendarPanel | ~400 | Wraps week grid + list views + time entries | HIGH - conditional rendering hub |

## Refactoring Challenges

The component is difficult to refactor because:

1. **Core data arrays** (projects, entries, calendarEvents) are referenced by virtually everything
2. **Classification handlers** need to update both events AND reload entries
3. **Derived state chains** create complex dependencies (visibleProjectIds → filteredEntries → entriesByDate → projectTotals)
4. **Multiple view modes** share the same underlying state but render differently
5. **ID-based hover pattern** (per CLAUDE.md guidelines) requires array access for freshness

### Potential Approaches

1. **Prop drilling** - Pass many callbacks and state down (~15 props for WeekCalendarGrid)
2. **Store/context pattern** - Move shared state to a Svelte store
3. **Accept coupling** - Use bindable state for tight parent-child coupling

The most tractable extraction would be **WeekCalendarGrid** (~350 lines), which would require approximately 15 props/callbacks to maintain parent state connection.
