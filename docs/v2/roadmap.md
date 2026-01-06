# Roadmap - Timesheet App v2

This document tracks what's been built, what's in progress, and what's planned. It rationalizes requirements from the v1 PRDs with the v2 architecture.

---

## Vision

A personal timesheet app that:
1. **Syncs** calendar events from Google Calendar
2. **Classifies** events to projects (manually, by rules, or via AI)
3. **Tracks** time with one entry per project per day
4. **Invoices** billable time to clients
5. **Exports** to CSV or Google Sheets

The **MCP Server** enables AI-assisted workflows, avoiding complex UI for bulk operations.

---

## Current Status

### Implemented

| Component | Status | Notes |
|-----------|--------|-------|
| **Auth** | Done | Email/password + Google OAuth |
| **Projects** | Done | CRUD, colors, archive, hidden, no-accumulate flags |
| **Time Entries** | Done | CRUD, one per project/day enforced |
| **Calendar Connections** | Done | Google OAuth, token storage |
| **Multi-Calendar Selection** | Done | Select which calendars to sync |
| **Calendar Event Sync** | Done | Incremental sync with tokens |
| **Manual Classification** | Done | One-click project assignment |
| **Day/Week View** | Done | Time grid, URL-based navigation |
| **Classification UI** | Done | Project color circles, reclassify support |

### Completed

| Component | Status | Notes |
|-----------|--------|-------|
| **MCP Server** | Done | Full tool suite for AI-assisted workflows |

### Not Yet Implemented

| Feature | Priority | Complexity | Reference |
|---------|----------|------------|-----------|
| Time Entry Enhancements | High | High | [prd-time-entry-enhancements.md](prd-time-entry-enhancements.md) |
| Billing Periods | Medium | Low | [ADR-002](decisions/002-billing-periods.md) |
| Invoicing | Medium | Medium | [prd-invoicing.md](../prd-invoicing.md) |
| Google Sheets Export | Low | Low | [prd-project-spreadsheets.md](../prd-project-spreadsheets.md) |

---

## Phases

### Phase 1: Classification System (Current Priority)

A hybrid classification system combining rules, LLM suggestions, and manual input.

**Goal**: Reduce manual classification to edge cases only, while preserving user control.

**Architecture**: See [ADR-003](decisions/003-scoring-classification.md)

#### 1.1 Classification Model

**Two Independent Dimensions:**
- **Attendance**: Did I attend? (yes/no) - evaluated separately
- **Project**: Which project? - evaluated separately

**Scoring/Accumulator Approach:**
- All matching rules "vote" for their target (not first-match-wins)
- Multiple rules agreeing increases confidence
- Conflicting rules surface for user review

**Confidence Thresholds:**
- Below floor (0.5): Don't classify, leave pending
- Floor to ceiling (0.5-0.8): Classify but flag for review
- Above ceiling (0.8): Auto-classify, no flag

**Classification Sources:**
- `manual`: User clicked to classify
- `rule`: Rules engine classified
- `llm`: LLM suggestion accepted

#### 1.2 Query Engine

- Parse Gmail-style syntax: `domain:example.com title:"weekly sync"`
- Properties: title, description, attendees, domain, email, response, recurring, day-of-week
- Evaluate queries against calendar events
- Return match scores for accumulator

#### 1.3 Rules Engine

- Rules have query + target (project or "did not attend") + weight
- All rules evaluated; matching rules contribute votes
- UI shows "priority" toggle (doubles weight internally)
- Weights stored as floats for LLM tuning

**API:**
- `GET/POST /api/rules` - List and create rules
- `GET/PUT/DELETE /api/rules/{id}` - Manage individual rules
- `POST /api/rules/preview` - Preview with conflict detection

**Preview Response:**
```json
{
  "matches": [...],
  "conflicts": [
    {"event_id": "...", "current_project": "X", "current_source": "manual", "proposed": "Y"}
  ],
  "stats": {"total": 47, "would_change": 5, "manual_conflicts": 2}
}
```

#### 1.4 Review Indicators

Visual feedback for events that need attention.

| Location | Indicator |
|----------|-----------|
| Week/Day view header | Badge: "N events need review" |
| Event card | Yellow dot if `needs_review=true` |
| Rules page | Section: "Suggested rules (N)" |

#### 1.5 Project Fingerprints

- Add domains, emails, keywords to projects
- Auto-generate rules from fingerprints
- Fingerprint rules contribute to scoring like any other rule

#### 1.6 Search UI (Classification Hub)

Gmail-style search that serves multiple purposes:
- **Search**: Find events matching criteria
- **Preview**: See what a rule would match
- **Classify**: Apply to search results
- **Create Rule**: Save search as rule

#### 1.7 LLM Integration (Deferred)

Deferred in favor of MCP server, which enables experimentation with LLM-assisted workflows without building complex UI.

**Reference**: [prd-rules-v2.md](../prd-rules-v2.md)

---

### Phase 2: Time Entry Enhancements (Ready for Implementation)

**Status**: Requirements complete. Ready for implementation.

**PRD**: [prd-time-entry-enhancements.md](prd-time-entry-enhancements.md)

**Mocks**: [mocks/time-entry-enhancements.html](mocks/time-entry-enhancements.html)

#### Core Change

Time entries become a **computed view** over classified events, updating automatically until protected by the user.

#### Protection Model

| State | How it happens | Behavior |
|-------|----------------|----------|
| **Unlocked** | Default | System updates freely |
| **Pinned** | User edits hours/title/description | Protected from auto-update |
| **Locked** | User clicks Lock Day/Week | Protected from auto-update |
| **Invoiced** | Added to invoice | Immutable |

#### Key Features

1. **Time Entry Analyzer** - Pure function computing entries from events
2. **Overlap Handling** - Union for same-project, detection for cross-project (>15m)
3. **Rounding** - 15m granularity with transparent calculation details
4. **Contributing Events** - Track and display which events feed each entry
5. **Lock Day/Week** - Bulk protection for sign-off workflow
6. **Stale Indicators** - Orange dot on protected items when computed differs
7. **Orphaned Events** - Handle deleted calendar events gracefully

#### Implementation Phases

- 2.1: Analyzer Foundation (union, rounding, calculation_details)
- 2.2: Contributing Events (junction table, API, basic UI)
- 2.3: Protection Model (pinned, locked, stale fields)
- 2.4: Live Updates (wire into sync/classification, flash feedback)
- 2.5: UI Polish (indicators, lock controls, detail view)
- 2.6: Cross-Project Overlaps (detection, one-click fixes)
- 2.7: Title/Description Generation

---

### Phase 3: Billing & Invoicing

Enable invoicing for billable projects.

**Deliverables**:

1. **Billing Periods**
   - Date ranges with hourly rates per project
   - Support rate changes over time
   - Reference: [ADR-002](decisions/002-billing-periods.md)

2. **Invoice Generation**
   - Create invoice from uninvoiced entries in date range
   - Calculate totals using applicable rates
   - Sequential invoice numbering

3. **Invoice Status Flow**
   - Draft (editable) → Sent (locked) → Paid (archived)
   - Lock time entries when invoice sent

4. **Invoice Export**
   - CSV download
   - Google Sheets export (optional, Phase 4)

**Reference**: [prd-invoicing.md](../prd-invoicing.md)

---

### Phase 4: MCP Server (Complete)

**Status**: Done.

AI-assisted workflows via Model Context Protocol. Enables natural language commands like:
- "Mark all events from this week as skipped"
- "Create a rule for all meetings with @alice"
- "Show me potential double-billing"

**Implemented Tools**:
- `list_projects`, `list_rules` - Context for classification
- `list_pending_events`, `search_events` - Find events
- `classify_event`, `bulk_classify` - Classification operations
- `create_rule`, `preview_rule`, `apply_rules` - Rule management
- `create_time_entry`, `get_time_summary` - Time tracking

**Reference**: [prd-mcp-server.md](../prd-mcp-server.md)

---

### Phase 5: Polish & Integrations

**Deliverables**:

1. **Google Sheets Export**
   - Per-project spreadsheet with invoice worksheets
   - Reference: [prd-project-spreadsheets.md](../prd-project-spreadsheets.md)

2. **Create Rule from Event**
   - Modal showing event properties as checkboxes
   - Live preview of matching events
   - One-click rule creation

3. **Project Statistics**
   - Hours this week/month/all-time per project
   - Displayed on project detail page

4. **Sidebar Project Summary**
   - Hours by project for current view
   - Filter toggle per project
   - Hidden/archived sections

---

### Future Considerations

Not currently planned, but architecture supports:

| Feature | Notes |
|---------|-------|
| **LLM Classification** | Use AI to classify ambiguous events |
| **Multi-user / Teams** | User ID on all entities enables this |
| **Additional Calendars** | Outlook, CalDAV (provider abstraction exists) |
| **Mobile App** | API-first design supports this |
| **Harvest Export** | CSV export is compatible |

---

## Document Map

| Document | Purpose |
|----------|---------|
| **v1 PRDs** | |
| [prd.md](../prd.md) | Original product requirements |
| [prd-rules-v2.md](../prd-rules-v2.md) | Query-based rules system |
| [prd-invoicing.md](../prd-invoicing.md) | Invoice generation |
| [prd-mcp-server.md](../prd-mcp-server.md) | AI assistant integration |
| [prd-project-spreadsheets.md](../prd-project-spreadsheets.md) | Google Sheets export |
| **v2 PRDs** | |
| [prd-time-entry-enhancements.md](prd-time-entry-enhancements.md) | Phase 2: Time entry calculation and protection |
| **v2 Mocks** | |
| [mocks/time-entry-enhancements.html](mocks/time-entry-enhancements.html) | Phase 2: UI wireframes |
| **v2 Architecture** | |
| [architecture.md](architecture.md) | Layer definitions, naming conventions |
| [domain-glossary.md](domain-glossary.md) | Entity definitions, operations |
| [components.md](components.md) | Web client component catalog |
| [api-spec.yaml](api-spec.yaml) | OpenAPI specification |
| **Decisions** | |
| [ADR-001](decisions/001-time-entry-per-day.md) | One entry per project per day |
| [ADR-002](decisions/002-billing-periods.md) | Rate management via periods |
| [ADR-003](decisions/003-scoring-classification.md) | Scoring-based classification |

---

## Key Differences: v1 → v2

| Aspect | v1 | v2 |
|--------|----|----|
| **Backend** | Python/FastAPI | Go |
| **Database** | SQLite | PostgreSQL |
| **Frontend** | Server-rendered templates | SvelteKit SPA |
| **Time Entries** | 1:1 with events | 1 per project/day (accumulation) |
| **Rules** | Structured dropdowns | Query DSL with live preview |
| **Calendar** | Single calendar | Multi-calendar selection |

---

## Changelog

| Date | Change |
|------|--------|
| 2026-01-05 | Phase 2 requirements complete; PRD and mocks ready for implementation |
| 2026-01-05 | Marked Phase 4 (MCP Server) as complete |
| 2026-01-05 | Deferred Phase 1.7 (LLM Integration) in favor of MCP experimentation |
| 2026-01-02 | Reordered Phase 1: Review Indicators (1.4), Fingerprints (1.5), Search UI (1.6), LLM (1.7) |
| 2025-01-02 | Refined Phase 1 with scoring-based classification, LLM integration |
| 2025-01-02 | Initial roadmap created |
