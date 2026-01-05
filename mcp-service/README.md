# Timesheet MCP Service

MCP (Model Context Protocol) server for AI-based analysis and manipulation of timesheet data.

## Quick Start

### 1. Install dependencies

```bash
cd mcp-service
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

### 2. Configure environment

```bash
cp .env.example .env
# Edit .env with your API key
```

You need to generate an API key in your timesheet application and add it to `.env`:

```
TIMESHEET_API_URL=http://localhost:8080
TIMESHEET_API_KEY=your-api-key-here
```

### 3. Run with Claude Code

Set environment variables and start Claude Code:

```bash
export TIMESHEET_API_URL=http://localhost:8080
export TIMESHEET_API_KEY=your-api-key

# From project root
claude
```

The MCP server is automatically configured via `.mcp.json` in the project root.

## Available Tools

| Tool | Description |
|------|-------------|
| `list_projects` | List all projects (needed for classification) |
| `get_time_summary` | Aggregate time by project/date/week |
| `list_pending_events` | Show calendar events needing classification |
| `classify_event` | Assign event to project or skip |
| `bulk_classify_events` | Classify multiple events by query |
| `create_time_entry` | Log time manually |
| `search_events` | Search events with Gmail-style queries |
| `list_rules` | Show classification rules |
| `sync_calendar` | Trigger calendar sync |

## Query Syntax

For `bulk_classify_events` and `search_events`, use Gmail-style queries:

```
title:standup              # Events with "standup" in title
title:"weekly sync"        # Exact phrase match
domain:acme.com            # Attendee email domain
attendee:bob@acme.com      # Specific attendee
calendar:Work              # Events from specific calendar

# Combine terms
title:meeting domain:acme.com
```

## HTTP Mode (Optional)

For running as a standalone HTTP server (useful for remote access):

```bash
export MCP_TRANSPORT=http
export MCP_HOST=127.0.0.1
export MCP_PORT=3001
python server.py
```

Then configure Claude Code to connect via HTTP:

```bash
claude mcp add timesheet-http \
  --transport http \
  --header "Authorization: Bearer your-api-key" \
  http://localhost:3001/mcp
```

## Docker

Build and run:

```bash
docker build -t timesheet-mcp .

# Stdio mode (for local Claude Code)
docker run --rm -i \
  -e TIMESHEET_API_URL=http://host.docker.internal:8080 \
  -e TIMESHEET_API_KEY=your-key \
  timesheet-mcp

# HTTP mode
docker run -d \
  -e TIMESHEET_API_URL=http://host.docker.internal:8080 \
  -e TIMESHEET_API_KEY=your-key \
  -e MCP_TRANSPORT=http \
  -p 3001:3001 \
  timesheet-mcp
```

## Security Notes

- **API Key**: The MCP server uses your API key to authenticate with the timesheet API. Keep this secret.
- **Stdio transport**: Most secure for local use. No network exposure.
- **HTTP transport**: Only use behind a reverse proxy with HTTPS for production.
- **Scopes**: Currently all tools have full access. Consider implementing scoped API keys for production.

## Development

Run tests:

```bash
pytest tests/
```

Format code:

```bash
ruff format .
ruff check --fix .
```
