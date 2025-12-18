# Timesheet MCP Server

An MCP (Model Context Protocol) server that exposes timesheet functionality to AI assistants like Claude Desktop.

## Features

- **list_projects**: List all projects with their settings
- **get_time_entries**: Get time entries with full event details for a date range
- **get_timesheet_summary**: Get a summary of hours by project, day, or week

## Requirements

- Python 3.11+
- PostgreSQL database
- MCP Python SDK (`mcp>=1.0.0`)

## Claude Desktop Configuration

Add the following to your Claude Desktop configuration file:

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "timesheet": {
      "command": "/path/to/timesheet-app/venv/bin/python",
      "args": ["-m", "mcp_server"],
      "cwd": "/path/to/timesheet-app/src",
      "env": {
        "DATABASE_URL": "postgresql://user:password@localhost:5432/timesheet",
        "TIMESHEET_USER_EMAIL": "your-email@example.com"
      }
    }
  }
}
```

### Configuration Notes

1. **command**: Path to the Python interpreter in your virtual environment
2. **cwd**: Must be the `src` directory of the timesheet app
3. **DATABASE_URL**: PostgreSQL connection string
4. **TIMESHEET_USER_EMAIL**: Your email address (must exist in the database)

## Running Manually

For testing, you can run the server directly:

```bash
cd /path/to/timesheet-app/src
DATABASE_URL="postgresql://..." TIMESHEET_USER_EMAIL="..." python -m mcp_server
```

## Example Prompts

Once configured, you can ask Claude:

- "Produce a timesheet report for last week"
- "What projects do I have?"
- "How many hours did I work last month?"
- "Show me a breakdown of my time by project for this week"

## Development

### Running with MCP Inspector

```bash
cd src
DATABASE_URL="..." TIMESHEET_USER_EMAIL="..." mcp dev mcp_server
```

### Testing

```bash
cd src
python -c "
from mcp_server.server import create_server
from mcp_server.tools import ALL_TOOLS
print([t.name for t in ALL_TOOLS])
"
```

## Architecture

```
src/mcp_server/
├── __init__.py       # Package initialization
├── __main__.py       # Entry point
├── auth.py           # Authentication providers
├── server.py         # FastMCP server setup
└── tools/
    ├── __init__.py   # Tool registration
    ├── base.py       # Base tool class
    ├── projects.py   # Project tools
    └── time_entries.py  # Time entry tools
```
