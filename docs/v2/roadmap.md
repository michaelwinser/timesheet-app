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

### Not Yet Implemented

| Feature | Priority | Complexity | Reference |
|---------|----------|------------|-----------|
| Classification Rules (query DSL) | High | Medium | [prd-rules-v2.md](../prd-rules-v2.md) |
| Project Fingerprints | High | Low | [prd-rules-v2.md](../prd-rules-v2.md) |
| Billing Periods | Medium | Low | [ADR-002](decisions/002-billing-periods.md) |
| Invoicing | Medium | Medium | [prd-invoicing.md](../prd-invoicing.md) |
| MCP Server | Medium | Medium | [prd-mcp-server.md](../prd-mcp-server.md) |
| Google Sheets Export | Low | Low | [prd-project-spreadsheets.md](../prd-project-spreadsheets.md) |
| LLM Classification | Low | High | [llm-classification-design.md](../llm-classification-design.md) |

---

## Phases

### Phase 1: Classification Rules (Current Priority)

Enable automatic classification of calendar events based on user-defined rules.

**Goal**: Reduce manual classification to edge cases only.

**Deliverables**:

1. **Query Parser & Evaluator**
   - Parse Gmail-style query syntax: `domain:example.com title:"weekly sync"`
   - Evaluate queries against calendar events
   - Support: title, description, attendees, domain, email, response, recurring, day-of-week

2. **Rules API**
   - `GET/POST /api/rules` - List and create rules
   - `GET/PUT/DELETE /api/rules/{id}` - Manage individual rules
   - `POST /api/rules/preview` - Preview matching events for a query

3. **Project Fingerprints**
   - Add domains, emails, keywords to projects
   - Auto-generate rules from fingerprints
   - Store on Project entity (new fields: `fingerprint_domains`, `fingerprint_emails`, `fingerprint_keywords`)

4. **Rules Page (Web)**
   - Text input with live preview
   - Rules grouped by target project
   - Drag-to-reorder priority

5. **Apply Rules on Sync**
   - After fetching events, run classification rules
   - Create/update time entries for matches
   - Track `classification_source: rule` vs `manual`

**Reference**: [prd-rules-v2.md](../prd-rules-v2.md)

---

### Phase 2: Time Entry Enhancements

Improve time entry tracking with features from v1 PRD.

**Deliverables**:

1. **Contributing Events Tracking**
   - Store which calendar events fed into each time entry
   - Display event list in time entry detail

2. **Overlapping Event Handling**
   - When multiple events for same project overlap, use time union (not sum)
   - Store `calculation_details` JSON for audit

3. **Description Accumulation**
   - Merge event titles into entry description
   - Respect user edits (`has_user_edits` flag)

4. **Orphaned Event Handling**
   - Mark events as orphaned when deleted from Google
   - Surface orphaned entries for user review
   - Auto-delete if no user edits and not invoiced

**Reference**: [domain-glossary.md](domain-glossary.md), [prd.md](../prd.md) (Overlapping Events section)

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

### Phase 4: MCP Server

Enable AI-assisted workflows via Model Context Protocol.

**Goal**: Avoid complex UI for bulk operations. Natural language commands like:
- "Mark all events from this week as skipped"
- "Create a rule for all meetings with @alice"
- "Show me potential double-billing"

**Deliverables**:

1. **MCP Server Foundation**
   - Python MCP server with stdio transport
   - Connect to PostgreSQL directly
   - Auth via API key or environment variable

2. **Core Tools**
   - `get_time_entries` - Rich data retrieval for LLM reasoning
   - `list_projects`, `list_rules` - Context for classification
   - `search_events` - Find events by text
   - `bulk_classify` - Classify multiple events at once
   - `create_rule` - Create rules from natural language

3. **Resources & Prompts**
   - `timesheet://projects` - Auto-loaded project list
   - `timesheet://rules` - Current rules for reference
   - Built-in prompts for common workflows

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
| **v2 Architecture** | |
| [architecture.md](architecture.md) | Layer definitions, naming conventions |
| [domain-glossary.md](domain-glossary.md) | Entity definitions, operations |
| [components.md](components.md) | Web client component catalog |
| [api-spec.yaml](api-spec.yaml) | OpenAPI specification |
| **Decisions** | |
| [ADR-001](decisions/001-time-entry-per-day.md) | One entry per project per day |
| [ADR-002](decisions/002-billing-periods.md) | Rate management via periods |

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
| 2024-01-02 | Initial roadmap created |
