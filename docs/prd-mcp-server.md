# PRD: Timesheet MCP Server

## Overview

Create an MCP (Model Context Protocol) server that exposes the timesheet application's functionality to AI assistants like Claude. This enables natural language interactions for time tracking, reporting, and AI-powered analysis.

## Target Use Cases

| # | User Says | Capability |
|---|-----------|------------|
| 1 | "Produce a timesheet report for last month" | Reporting |
| 2 | "Look at last week's entries and propose classification rules" | Rule inference |
| 3 | "Use 6 months of history to classify this month's entries" | Few-shot classification |
| 4 | "Find entries that might be classification errors" | Anomaly detection |
| 5 | "Find entries with 'Michael Scovetta', assign to Alpha-Omega" | Search + bulk classify |
| 6 | "Create a rule for VEX in title OR Munawar as attendee" | Natural language rules |
| 7 | "Make sure I'm not double billing" | Overlap detection (LLM-driven) |

## Design Philosophy

- **Data-rich retrieval**: Expose full event/entry details so the LLM can reason about them
- **Bulk operations**: Enable search-and-act patterns for efficiency
- **Minimal smart logic**: The MCP server exposes data; the LLM does the reasoning
- **Historical context**: Easy access to weeks/months of data for pattern matching

## Goals

1. **AI-Powered Analysis**: Enable the LLM to analyze patterns, detect anomalies, and infer rules from historical data
2. **Natural Language Operations**: Translate user intent into searches, classifications, and rule creation
3. **Efficient Bulk Actions**: Find and modify multiple entries based on criteria

## Non-Goals

- Replacing the web UI (complementary tool)
- Implementing complex analysis logic in the MCP server (LLM does reasoning)
- Real-time sync (request-driven only)
- Multi-user support in a single MCP session (one user per connection)

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Claude/AI      │────▶│  MCP Server     │────▶│  Timesheet API  │
│  Assistant      │◀────│  (Python)       │◀────│  (FastAPI)      │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                              │
                              ▼
                        ┌─────────────────┐
                        │  PostgreSQL     │
                        └─────────────────┘
```

The MCP server will:
- Run as a separate process (stdio transport for Claude Desktop, or SSE for web)
- Connect directly to the PostgreSQL database (same as the main app)
- Reuse existing service modules where possible

## MCP Tools

Tools are organized by use case priority.

### Core Data Retrieval (Use Cases 1-4, 7)

#### `get_time_entries`
Get time entries with full event details for analysis.

**Parameters:**
- `start_date` (string, required): Start date (YYYY-MM-DD)
- `end_date` (string, required): End date (YYYY-MM-DD)
- `project_id` (integer, optional): Filter by project
- `include_unclassified` (boolean, optional): Include unclassified events (default: true)

**Returns:** Array of entries, each containing:
- Time entry: id, hours, description, project_id, project_name
- Event: id, title, description, start_time, end_time, attendees, meeting_link
- Classification: source (manual/rule), rule_id if applicable

**Supports Use Cases:**
- #1 Reporting: Get all entries for a period, LLM formats report
- #3 Few-shot: Get historical entries as examples for classification
- #4 Anomaly detection: LLM analyzes patterns in classification
- #7 Double-billing: LLM checks for overlapping time ranges

---

#### `get_timesheet_summary`
Get aggregated hours for reporting.

**Parameters:**
- `start_date` (string, required): Start date (YYYY-MM-DD)
- `end_date` (string, required): End date (YYYY-MM-DD)
- `group_by` (string, optional): "project" | "day" | "week" (default: "project")

**Returns:**
- Hours by grouping (project/day/week)
- Total classified hours
- Total unclassified hours
- Unclassified event count

**Supports Use Case:** #1 Reporting

---

#### `list_projects`
Get all projects for reference.

**Parameters:**
- `include_archived` (boolean, optional): Include archived projects (default: false)

**Returns:** Array of projects with id, name, color, settings

**Supports Use Cases:** All (LLM needs project list to classify)

---

#### `list_rules`
Get existing rules for reference and rule inference.

**Parameters:**
- `include_disabled` (boolean, optional): Include disabled rules (default: false)

**Returns:** Array of rules with conditions

**Supports Use Case:** #2 Rule inference (LLM sees existing rules as examples)

### Search & Bulk Operations (Use Case 5)

#### `search_events`
Search events by text across all fields.

**Parameters:**
- `query` (string, required): Search text (searches title, description, attendees)
- `start_date` (string, optional): Start date filter
- `end_date` (string, optional): End date filter
- `classified` (boolean, optional): Filter by classification status

**Returns:** Array of matching events with full details

**Example:** "Find all events with 'Michael Scovetta'"

---

#### `bulk_classify`
Classify multiple events at once.

**Parameters:**
- `event_ids` (array, required): Array of event IDs to classify
- `project_id` (integer, required): Project to assign

**Returns:**
- `classified`: Number successfully classified
- `skipped`: Number skipped (already classified or DNA)
- `errors`: Any errors encountered

**Supports Use Case:** #5 Search + bulk classify

### Rule Management (Use Cases 2, 6)

#### `create_rule`
Create a classification rule with complex conditions.

**Parameters:**
- `name` (string, required): Rule name
- `project_id` (integer, required): Target project ID
- `conditions` (array, required): Array of conditions (AND logic)
  - `property_name`: "title", "full_text", "attendees", etc.
  - `condition_type`: "contains", "equals", "starts_with", "matches_regex", "any_contains" (for lists)
  - `condition_value`: Value to match
- `priority` (integer, optional): Higher = evaluated first (default: 50)

**Returns:** Created rule with ID

**Supports Use Cases:**
- #2 Rule inference: LLM analyzes entries, creates rules
- #6 Natural language rules: LLM translates "VEX in title OR Munawar as attendee" to conditions

**Note:** For OR logic, create multiple rules with the same target project.

---

#### `apply_rules`
Apply rules to unclassified events.

**Parameters:**
- `start_date` (string, required): Start date
- `end_date` (string, required): End date
- `dry_run` (boolean, optional): Preview only (default: false)

**Returns:**
- `classified`: Number of events classified
- `matches`: Array of {event_id, rule_id, rule_name} for dry_run

**Supports Use Case:** #2 Rule inference (test proposed rules)

### Single Entry Operations

#### `classify_event`
Classify a single event.

**Parameters:**
- `event_id` (integer, required): Event ID
- `project_id` (integer, required): Project to assign
- `hours` (number, optional): Override hours (default: calculated from event)

**Returns:** Created time entry

---

#### `update_entry`
Update an existing time entry.

**Parameters:**
- `entry_id` (integer, required): Entry ID
- `project_id` (integer, optional): New project
- `hours` (number, optional): New hours

**Returns:** Updated time entry

---

#### `unclassify_entry`
Remove classification from an entry.

**Parameters:**
- `entry_id` (integer, required): Entry ID

**Returns:** Success confirmation

---

#### `set_did_not_attend`
Mark event as did/did not attend.

**Parameters:**
- `event_id` (integer, required): Event ID
- `did_not_attend` (boolean, required): DNA status

**Returns:** Updated event

### Sync

#### `sync_calendar`
Sync events from Google Calendar.

**Parameters:**
- `start_date` (string, required): Start date
- `end_date` (string, required): End date

**Returns:** Sync results (fetched, new, updated, auto-classified)

**Note:** Requires valid OAuth token in session. May need web UI auth first.

## MCP Resources

Resources provide context that Claude can reference without explicit tool calls.

### `timesheet://projects`
List of all active projects. Auto-loaded so LLM knows valid project names/IDs.

### `timesheet://rules`
Current classification rules. Helps LLM understand existing patterns before suggesting new ones.

## MCP Prompts

Pre-built prompts for common workflows (maps to use cases).

### `generate_report`
**Use Case #1**
```
Generate a timesheet report for {start_date} to {end_date}.
Include: hours by project, daily breakdown, any unclassified time.
```

### `suggest_rules`
**Use Case #2**
```
Analyze the classified time entries from {start_date} to {end_date}.
Identify patterns and suggest rules that could automate future classification.
Show me the rules you would create before creating them.
```

### `classify_from_history`
**Use Case #3**
```
Use the time entries from {history_start} to {history_end} as examples.
Classify the unclassified events from {target_start} to {target_end}.
Explain your reasoning for each classification.
```

### `find_errors`
**Use Case #4**
```
Review the time entries from {start_date} to {end_date}.
Identify any that seem inconsistent with typical patterns:
- Unusual project assignments
- Outlier hours
- Potential misclassifications
```

### `check_double_billing`
**Use Case #7**
```
Check my time entries from {start_date} to {end_date} for potential double-billing.
Look for overlapping time ranges where I might be logging time to multiple projects simultaneously.
```

## Authentication

The MCP server needs to authenticate as a specific user. Options:

1. **Environment Variable**: Set `TIMESHEET_USER_EMAIL` when starting the MCP server
2. **OAuth Flow**: Implement OAuth handshake on first connection (complex)
3. **API Key**: Generate per-user API keys in the web UI (recommended)

**Recommendation**: Start with environment variable for simplicity, add API key support later.

## Implementation Plan

Prioritized by use case value.

### Phase 1: Foundation + Reporting (Use Case #1)
- [ ] Set up MCP server skeleton with stdio transport
- [ ] Implement database connection (reuse existing db module)
- [ ] Implement authentication via environment variable
- [ ] Implement `list_projects` tool
- [ ] Implement `get_time_entries` tool (rich data retrieval)
- [ ] Implement `get_timesheet_summary` tool
- [ ] Test: "Produce a timesheet report for last week"

### Phase 2: Search + Bulk Operations (Use Case #5)
- [ ] Implement `search_events` tool
- [ ] Implement `bulk_classify` tool
- [ ] Test: "Find events with 'Scovetta', assign to Alpha-Omega"

### Phase 3: Rule Management (Use Cases #2, #6)
- [ ] Implement `list_rules` tool
- [ ] Implement `create_rule` tool
- [ ] Implement `apply_rules` tool with dry_run
- [ ] Test: "Propose rules based on last week's classifications"
- [ ] Test: "Create rule for VEX in title OR Munawar as attendee"

### Phase 4: Single Entry Operations
- [ ] Implement `classify_event` tool
- [ ] Implement `update_entry` tool
- [ ] Implement `unclassify_entry` tool
- [ ] Implement `set_did_not_attend` tool

### Phase 5: Resources & Prompts
- [ ] Implement `timesheet://projects` resource
- [ ] Implement `timesheet://rules` resource
- [ ] Implement MCP prompts
- [ ] Add Claude Desktop configuration instructions

### Phase 6: Calendar Sync (Optional)
- [ ] Implement `sync_calendar` tool
- [ ] Handle OAuth token retrieval/storage

## File Structure

```
src/
├── mcp/
│   ├── __init__.py
│   ├── server.py          # MCP server entry point
│   ├── tools/
│   │   ├── __init__.py
│   │   ├── time_entries.py
│   │   ├── projects.py
│   │   ├── rules.py
│   │   └── sync.py
│   ├── resources.py       # MCP resources
│   └── prompts.py         # MCP prompts
```

## Configuration

### Claude Desktop (`claude_desktop_config.json`)

```json
{
  "mcpServers": {
    "timesheet": {
      "command": "python",
      "args": ["-m", "src.mcp.server"],
      "cwd": "/path/to/timesheet-app",
      "env": {
        "DATABASE_URL": "postgresql://user:pass@localhost:5432/timesheet",
        "TIMESHEET_USER_EMAIL": "user@example.com"
      }
    }
  }
}
```

## Success Metrics

1. **Adoption**: Number of users configuring the MCP server
2. **Usage**: Tool invocations per user per week
3. **Accuracy**: Successful classification rate via MCP vs web UI
4. **Time Saved**: Reduction in time spent on manual classification

## Open Questions

1. **OAuth for sync**: Should calendar sync require a fresh web UI login, or can we store/retrieve tokens?
2. **Rate limiting**: Should we limit how much historical data can be retrieved in one call?
3. **Confirmation UX**: For bulk operations, should LLM always preview before executing?

## References

- [MCP Specification](https://modelcontextprotocol.io/)
- [MCP Python SDK](https://github.com/modelcontextprotocol/python-sdk)
- [Claude Desktop MCP Setup](https://modelcontextprotocol.io/quickstart)
